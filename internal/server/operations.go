package server

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

var ErrOperationInProgress = errors.New("another operation is in progress")

const (
	operationStatusRunning   = "running"
	operationStatusSucceeded = "succeeded"
	operationStatusFailed    = "failed"
	operationStatusRejected  = "rejected"
	defaultOperationLogLimit = 200
	operationInterruptedMsg  = "operation interrupted by controller restart"
)

type operationEntry struct {
	ID         uint64     `json:"id"`
	Type       string     `json:"type"`
	Status     string     `json:"status"`
	Message    string     `json:"message,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type operationStore interface {
	Load(now func() time.Time, maxEntries int) ([]operationEntry, uint64, error)
	Upsert(operationEntry) error
	Close() error
}

type noopOperationStore struct{}

func (noopOperationStore) Load(_ func() time.Time, _ int) ([]operationEntry, uint64, error) {
	return nil, 0, nil
}

func (noopOperationStore) Upsert(_ operationEntry) error {
	return nil
}

func (noopOperationStore) Close() error {
	return nil
}

type sqliteOperationStore struct {
	db            *sql.DB
	legacyLogPath string
	logger        *slog.Logger
}

type operationManager struct {
	mu         sync.Mutex
	now        func() time.Time
	entries    []operationEntry
	maxEntries int
	nextID     uint64
	running    bool
	logger     *slog.Logger
	store      operationStore
}

func newOperationManager(now func() time.Time, maxEntries int, logger *slog.Logger, store operationStore) *operationManager {
	if now == nil {
		now = func() time.Time {
			return time.Now().UTC()
		}
	}

	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	if maxEntries < 1 {
		maxEntries = defaultOperationLogLimit
	}

	if store == nil {
		store = noopOperationStore{}
	}

	loadedEntries, nextID, err := store.Load(now, maxEntries)
	if err != nil {
		logger.Warn("load operation entries failed", "error", err)
		_ = store.Close()
		store = noopOperationStore{}
		loadedEntries = nil
		nextID = 0
	}

	if len(loadedEntries) > maxEntries {
		loadedEntries = loadedEntries[len(loadedEntries)-maxEntries:]
	}

	return &operationManager{
		now:        now,
		entries:    loadedEntries,
		maxEntries: maxEntries,
		nextID:     nextID,
		logger:     logger,
		store:      store,
	}
}

func (m *operationManager) Run(operationType string, fn func() error) error {
	operationID, err := m.start(operationType)
	if err != nil {
		m.reject(operationType, err.Error())
		m.logger.Warn("operation rejected", "type", operationType, "error", err)
		return err
	}

	m.logger.Info("operation started", "id", operationID, "type", operationType)

	err = fn()
	if err != nil {
		m.finish(operationID, operationStatusFailed, err.Error())
		m.logger.Error("operation failed", "id", operationID, "type", operationType, "error", err)
		return err
	}

	m.finish(operationID, operationStatusSucceeded, "")
	m.logger.Info("operation succeeded", "id", operationID, "type", operationType)
	return nil
}

func (m *operationManager) List(limit int) []operationEntry {
	m.mu.Lock()
	defer m.mu.Unlock()

	if limit < 1 || limit > len(m.entries) {
		limit = len(m.entries)
	}

	start := len(m.entries) - limit
	cloned := make([]operationEntry, 0, limit)
	for _, entry := range m.entries[start:] {
		copyEntry := entry
		if entry.FinishedAt != nil {
			finishedAt := *entry.FinishedAt
			copyEntry.FinishedAt = &finishedAt
		}
		cloned = append(cloned, copyEntry)
	}

	return cloned
}

func (m *operationManager) start(operationType string) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return 0, ErrOperationInProgress
	}

	m.nextID++
	now := m.now().UTC()
	m.entries = append(m.entries, operationEntry{
		ID:        m.nextID,
		Type:      operationType,
		Status:    operationStatusRunning,
		StartedAt: now,
	})
	m.persistEntryLocked(m.entries[len(m.entries)-1])
	m.trimEntriesLocked()
	m.running = true

	return m.nextID, nil
}

func (m *operationManager) reject(operationType string, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextID++
	now := m.now().UTC()
	m.entries = append(m.entries, operationEntry{
		ID:         m.nextID,
		Type:       operationType,
		Status:     operationStatusRejected,
		Message:    message,
		StartedAt:  now,
		FinishedAt: &now,
	})
	m.persistEntryLocked(m.entries[len(m.entries)-1])
	m.trimEntriesLocked()
}

func (m *operationManager) finish(operationID uint64, status string, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now().UTC()
	for i := range m.entries {
		if m.entries[i].ID != operationID {
			continue
		}

		m.entries[i].Status = status
		m.entries[i].Message = message
		m.entries[i].FinishedAt = &now
		m.persistEntryLocked(m.entries[i])
		break
	}

	m.running = false
}

func (m *operationManager) trimEntriesLocked() {
	if len(m.entries) <= m.maxEntries {
		return
	}

	start := len(m.entries) - m.maxEntries
	m.entries = append([]operationEntry(nil), m.entries[start:]...)
}

func (m *operationManager) persistEntryLocked(entry operationEntry) {
	if err := m.store.Upsert(entry); err != nil {
		m.logger.Warn("persist operation entry failed", "error", err)
	}
}

func newSQLiteOperationStore(path string, legacyLogPath string, logger *slog.Logger) (operationStore, error) {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return noopOperationStore{}, nil
	}

	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	if err := os.MkdirAll(filepath.Dir(trimmedPath), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", trimmedPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	store := &sqliteOperationStore{db: db, legacyLogPath: strings.TrimSpace(legacyLogPath), logger: logger}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *sqliteOperationStore) init() error {
	if _, err := s.db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		return err
	}

	if _, err := s.db.Exec(`PRAGMA synchronous=NORMAL`); err != nil {
		return err
	}

	if _, err := s.db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		return err
	}

	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS operations (
			id INTEGER PRIMARY KEY,
			type TEXT NOT NULL,
			status TEXT NOT NULL,
			message TEXT NOT NULL DEFAULT '',
			started_at TEXT NOT NULL,
			finished_at TEXT
		)
	`); err != nil {
		return err
	}

	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)
	`); err != nil {
		return err
	}

	if _, err := s.db.Exec(`INSERT OR IGNORE INTO schema_meta (key, value) VALUES ('version', '1')`); err != nil {
		return err
	}

	if s.legacyLogPath != "" {
		if err := s.importLegacyIfEmpty(); err != nil {
			return err
		}
	}

	return nil
}

func (s *sqliteOperationStore) Load(now func() time.Time, maxEntries int) ([]operationEntry, uint64, error) {
	finishedAt := now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.Exec(`UPDATE operations SET status = ?, message = ?, finished_at = ? WHERE status = ?`, operationStatusFailed, operationInterruptedMsg, finishedAt, operationStatusRunning); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(`SELECT id, type, status, message, started_at, finished_at FROM operations ORDER BY id ASC`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entries := make([]operationEntry, 0)
	maxID := uint64(0)

	for rows.Next() {
		var entry operationEntry
		var startedAtRaw string
		var finishedAtRaw sql.NullString
		if err := rows.Scan(&entry.ID, &entry.Type, &entry.Status, &entry.Message, &startedAtRaw, &finishedAtRaw); err != nil {
			return nil, 0, err
		}
		if entry.ID > maxID {
			maxID = entry.ID
		}

		startedAt, err := time.Parse(time.RFC3339Nano, startedAtRaw)
		if err != nil {
			s.logger.Warn("skip operation with invalid started_at", "id", entry.ID, "error", err)
			continue
		}
		entry.StartedAt = startedAt.UTC()

		if finishedAtRaw.Valid {
			finishedAt, err := time.Parse(time.RFC3339Nano, finishedAtRaw.String)
			if err == nil {
				finished := finishedAt.UTC()
				entry.FinishedAt = &finished
			}
		}

		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	if maxEntries > 0 && len(entries) > maxEntries {
		entries = entries[len(entries)-maxEntries:]
	}

	return entries, maxID, nil
}

func (s *sqliteOperationStore) Upsert(entry operationEntry) error {
	return upsertOperation(s.db, entry)
}

type operationExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func upsertOperation(execer operationExecer, entry operationEntry) error {
	startedAt := entry.StartedAt.UTC().Format(time.RFC3339Nano)
	var finishedAt any
	if entry.FinishedAt != nil {
		finishedAt = entry.FinishedAt.UTC().Format(time.RFC3339Nano)
	}

	_, err := execer.Exec(`
		INSERT INTO operations (id, type, status, message, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			type = excluded.type,
			status = excluded.status,
			message = excluded.message,
			started_at = excluded.started_at,
			finished_at = excluded.finished_at
	`, entry.ID, entry.Type, entry.Status, entry.Message, startedAt, finishedAt)

	return err
}

func (s *sqliteOperationStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}

	return s.db.Close()
}

func (s *sqliteOperationStore) importLegacyIfEmpty() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM operations`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		if err := tx.Commit(); err != nil {
			return err
		}
		return nil
	}

	legacyEntries, maxID := loadOperationEntriesFromJSONL(s.legacyLogPath, s.logger)
	if len(legacyEntries) == 0 {
		if err := tx.Commit(); err != nil {
			return err
		}
		return nil
	}

	for _, entry := range legacyEntries {
		if err := upsertOperation(tx, entry); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	s.logger.Info("imported legacy operation log", "path", s.legacyLogPath, "entries", len(legacyEntries), "max_id", maxID)
	return nil
}

func loadOperationEntriesFromJSONL(path string, logger *slog.Logger) ([]operationEntry, uint64) {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return nil, 0
	}

	file, err := os.Open(trimmedPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0
	}
	if err != nil {
		logger.Warn("load operation entries failed", "path", trimmedPath, "error", err)
		return nil, 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	byID := map[uint64]operationEntry{}
	maxID := uint64(0)

	for line := 1; scanner.Scan(); line++ {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}

		var entry operationEntry
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			logger.Warn("skip invalid operation log line", "path", trimmedPath, "line", line, "error", err)
			continue
		}

		if entry.ID == 0 || strings.TrimSpace(entry.Type) == "" {
			logger.Warn("skip malformed operation entry", "path", trimmedPath, "line", line)
			continue
		}

		entry.StartedAt = entry.StartedAt.UTC()
		if entry.FinishedAt != nil {
			finished := entry.FinishedAt.UTC()
			entry.FinishedAt = &finished
		}

		byID[entry.ID] = entry
		if entry.ID > maxID {
			maxID = entry.ID
		}
	}

	if err := scanner.Err(); err != nil {
		logger.Warn("scan operation log failed", "path", trimmedPath, "error", err)
	}

	ids := make([]uint64, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i int, j int) bool {
		return ids[i] < ids[j]
	})

	loaded := make([]operationEntry, 0, len(ids))
	for _, id := range ids {
		loaded = append(loaded, byID[id])
	}

	return loaded, maxID
}
