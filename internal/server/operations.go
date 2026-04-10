package server

import (
	"bufio"
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

type operationManager struct {
	mu         sync.Mutex
	now        func() time.Time
	entries    []operationEntry
	maxEntries int
	nextID     uint64
	running    bool
	logger     *slog.Logger
	logPath    string
}

func newOperationManager(now func() time.Time, maxEntries int, logger *slog.Logger, logPath string) *operationManager {
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

	loadedEntries, nextID := loadOperationEntries(logPath, now, logger)
	if len(loadedEntries) > maxEntries {
		loadedEntries = loadedEntries[len(loadedEntries)-maxEntries:]
	}

	return &operationManager{
		now:        now,
		entries:    loadedEntries,
		maxEntries: maxEntries,
		nextID:     nextID,
		logger:     logger,
		logPath:    strings.TrimSpace(logPath),
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
	if strings.TrimSpace(m.logPath) == "" {
		return
	}

	if err := appendOperationEntry(m.logPath, entry); err != nil {
		m.logger.Warn("persist operation entry failed", "path", m.logPath, "error", err)
	}
}

func loadOperationEntries(logPath string, now func() time.Time, logger *slog.Logger) ([]operationEntry, uint64) {
	trimmedPath := strings.TrimSpace(logPath)
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
		entry := byID[id]
		if entry.Status == operationStatusRunning {
			finished := now().UTC()
			entry.Status = operationStatusFailed
			entry.Message = operationInterruptedMsg
			entry.FinishedAt = &finished
			if err := appendOperationEntry(trimmedPath, entry); err != nil {
				logger.Warn("persist interrupted operation marker failed", "path", trimmedPath, "operation_id", id, "error", err)
			}
		}

		loaded = append(loaded, entry)
	}

	return loaded, maxID
}

func appendOperationEntry(path string, entry operationEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()

	encoded, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return err
	}

	return file.Sync()
}
