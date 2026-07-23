package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"neon-selfhost/internal/branch"
	"neon-selfhost/internal/sqliteutil"

	_ "modernc.org/sqlite"
)

const (
	defaultSQLHistoryRetentionLimit    = 200
	sqliteSQLQueryLibrarySchemaVersion = 1
	sqlHistoryStatusSucceeded          = "succeeded"
	sqlHistoryStatusFailed             = "failed"
	sqlHistoryStatusOutcomeUnknown     = "outcome_unknown"
)

var errSQLSavedQueryNotFound = errors.New("saved SQL query not found")

type savedSQLQuery struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	SQL       string    `json:"sql"`
	Branch    string    `json:"branch"`
	Database  string    `json:"database"`
	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"-"`
}

type sqlExecutionHistory struct {
	ID         int64     `json:"id"`
	Title      string    `json:"title"`
	SQL        string    `json:"sql"`
	Branch     string    `json:"branch"`
	Database   string    `json:"database"`
	ReadOnly   bool      `json:"read_only"`
	Status     string    `json:"status"`
	CommandTag string    `json:"command_tag"`
	DurationMS int64     `json:"duration_ms"`
	ErrorCode  string    `json:"error_code"`
	ExecutedAt time.Time `json:"-"`
}

type savedSQLQueryPayload struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	SQL       string `json:"sql"`
	Branch    string `json:"branch"`
	Database  string `json:"database"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type sqlExecutionHistoryPayload struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	SQL        string `json:"sql"`
	Branch     string `json:"branch"`
	Database   string `json:"database"`
	ReadOnly   bool   `json:"read_only"`
	Status     string `json:"status"`
	CommandTag string `json:"command_tag"`
	DurationMS int64  `json:"duration_ms"`
	ErrorCode  string `json:"error_code"`
	ExecutedAt string `json:"executed_at"`
}

type savedSQLQueryEnvelope struct {
	Query savedSQLQueryPayload `json:"query"`
}

type savedSQLQueriesEnvelope struct {
	Queries []savedSQLQueryPayload `json:"queries"`
}

type sqlExecutionHistoryEnvelope struct {
	History []sqlExecutionHistoryPayload `json:"history"`
}

type createSavedSQLQueryRequest struct {
	Name     string `json:"name"`
	SQL      string `json:"sql"`
	Branch   string `json:"branch"`
	Database string `json:"database"`
}

type updateSavedSQLQueryRequest struct {
	Name *string `json:"name"`
	SQL  *string `json:"sql"`
}

type sqlQueryLibraryFilter struct {
	Branch   string
	Database string
	Limit    int
	Offset   int
}

type sqlQueryLibraryStore interface {
	CreateSavedQuery(context.Context, savedSQLQuery) (savedSQLQuery, error)
	UpdateSavedQuery(context.Context, int64, *string, *string) (savedSQLQuery, error)
	DeleteSavedQuery(context.Context, int64) error
	ListSavedQueries(context.Context, sqlQueryLibraryFilter) ([]savedSQLQuery, error)
	AddExecutionHistory(context.Context, sqlExecutionHistory) (sqlExecutionHistory, error)
	ListExecutionHistory(context.Context, sqlQueryLibraryFilter) ([]sqlExecutionHistory, error)
	Close() error
}

type sqlQueryLibrary struct {
	store sqlQueryLibraryStore
	mu    sync.Mutex
	bad   bool
}

func newSQLQueryLibrary(store sqlQueryLibraryStore, degraded bool) *sqlQueryLibrary {
	return &sqlQueryLibrary{store: store, bad: degraded}
}

func (l *sqlQueryLibrary) markResult(err error) error {
	if err != nil && !errors.Is(err, errSQLSavedQueryNotFound) {
		l.mu.Lock()
		l.bad = true
		l.mu.Unlock()
	}
	return err
}

func (l *sqlQueryLibrary) markRequestResult(ctx context.Context, err error) error {
	if err != nil && ctx.Err() == nil {
		return l.markResult(err)
	}
	return err
}

func (l *sqlQueryLibrary) degraded() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.bad
}

func (l *sqlQueryLibrary) CreateSavedQuery(ctx context.Context, query savedSQLQuery) (savedSQLQuery, error) {
	created, err := l.store.CreateSavedQuery(ctx, query)
	return created, l.markRequestResult(ctx, err)
}

func (l *sqlQueryLibrary) UpdateSavedQuery(ctx context.Context, id int64, name *string, query *string) (savedSQLQuery, error) {
	updated, err := l.store.UpdateSavedQuery(ctx, id, name, query)
	return updated, l.markRequestResult(ctx, err)
}

func (l *sqlQueryLibrary) DeleteSavedQuery(ctx context.Context, id int64) error {
	return l.markRequestResult(ctx, l.store.DeleteSavedQuery(ctx, id))
}

func (l *sqlQueryLibrary) ListSavedQueries(ctx context.Context, filter sqlQueryLibraryFilter) ([]savedSQLQuery, error) {
	queries, err := l.store.ListSavedQueries(ctx, filter)
	return queries, l.markRequestResult(ctx, err)
}

func (l *sqlQueryLibrary) AddExecutionHistory(ctx context.Context, entry sqlExecutionHistory) (sqlExecutionHistory, error) {
	created, err := l.store.AddExecutionHistory(ctx, entry)
	return created, l.markResult(err)
}

func (l *sqlQueryLibrary) ListExecutionHistory(ctx context.Context, filter sqlQueryLibraryFilter) ([]sqlExecutionHistory, error) {
	history, err := l.store.ListExecutionHistory(ctx, filter)
	return history, l.markRequestResult(ctx, err)
}

func (l *sqlQueryLibrary) Close() error {
	return l.store.Close()
}

type unavailableSQLQueryLibraryStore struct {
	err error
}

func (s unavailableSQLQueryLibraryStore) CreateSavedQuery(context.Context, savedSQLQuery) (savedSQLQuery, error) {
	return savedSQLQuery{}, s.err
}
func (s unavailableSQLQueryLibraryStore) UpdateSavedQuery(context.Context, int64, *string, *string) (savedSQLQuery, error) {
	return savedSQLQuery{}, s.err
}
func (s unavailableSQLQueryLibraryStore) DeleteSavedQuery(context.Context, int64) error { return s.err }
func (s unavailableSQLQueryLibraryStore) ListSavedQueries(context.Context, sqlQueryLibraryFilter) ([]savedSQLQuery, error) {
	return nil, s.err
}
func (s unavailableSQLQueryLibraryStore) AddExecutionHistory(context.Context, sqlExecutionHistory) (sqlExecutionHistory, error) {
	return sqlExecutionHistory{}, s.err
}
func (s unavailableSQLQueryLibraryStore) ListExecutionHistory(context.Context, sqlQueryLibraryFilter) ([]sqlExecutionHistory, error) {
	return nil, s.err
}
func (unavailableSQLQueryLibraryStore) Close() error { return nil }

type memorySQLQueryLibraryStore struct {
	mu             sync.Mutex
	now            func() time.Time
	retentionLimit int
	nextSavedID    int64
	nextHistoryID  int64
	saved          []savedSQLQuery
	history        []sqlExecutionHistory
}

func newMemorySQLQueryLibraryStore(retentionLimit int) *memorySQLQueryLibraryStore {
	if retentionLimit < 1 {
		retentionLimit = defaultSQLHistoryRetentionLimit
	}
	return &memorySQLQueryLibraryStore{
		now:            func() time.Time { return time.Now().UTC() },
		retentionLimit: retentionLimit,
		saved:          []savedSQLQuery{},
		history:        []sqlExecutionHistory{},
	}
}

func (s *memorySQLQueryLibraryStore) CreateSavedQuery(_ context.Context, query savedSQLQuery) (savedSQLQuery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextSavedID++
	now := s.now().UTC()
	query.ID = s.nextSavedID
	query.CreatedAt = now
	query.UpdatedAt = now
	s.saved = append(s.saved, query)
	return query, nil
}

func (s *memorySQLQueryLibraryStore) UpdateSavedQuery(_ context.Context, id int64, name *string, query *string) (savedSQLQuery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.saved {
		if s.saved[i].ID != id {
			continue
		}
		if name != nil {
			s.saved[i].Name = *name
		}
		if query != nil {
			s.saved[i].SQL = *query
		}
		s.saved[i].UpdatedAt = s.now().UTC()
		return s.saved[i], nil
	}
	return savedSQLQuery{}, errSQLSavedQueryNotFound
}

func (s *memorySQLQueryLibraryStore) DeleteSavedQuery(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.saved {
		if s.saved[i].ID == id {
			s.saved = append(s.saved[:i], s.saved[i+1:]...)
			return nil
		}
	}
	return errSQLSavedQueryNotFound
}

func (s *memorySQLQueryLibraryStore) ListSavedQueries(_ context.Context, filter sqlQueryLibraryFilter) ([]savedSQLQuery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := make([]savedSQLQuery, 0, len(s.saved))
	for _, query := range s.saved {
		if filter.Branch != "" && query.Branch != filter.Branch {
			continue
		}
		if filter.Database != "" && query.Database != filter.Database {
			continue
		}
		filtered = append(filtered, query)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].ID > filtered[j].ID })
	return pageSavedQueries(filtered, filter), nil
}

func (s *memorySQLQueryLibraryStore) AddExecutionHistory(_ context.Context, entry sqlExecutionHistory) (sqlExecutionHistory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextHistoryID++
	entry.ID = s.nextHistoryID
	if entry.ExecutedAt.IsZero() {
		entry.ExecutedAt = s.now().UTC()
	} else {
		entry.ExecutedAt = entry.ExecutedAt.UTC()
	}
	s.history = append(s.history, entry)
	if len(s.history) > s.retentionLimit {
		s.history = append([]sqlExecutionHistory(nil), s.history[len(s.history)-s.retentionLimit:]...)
	}
	return entry, nil
}

func (s *memorySQLQueryLibraryStore) ListExecutionHistory(_ context.Context, filter sqlQueryLibraryFilter) ([]sqlExecutionHistory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := make([]sqlExecutionHistory, 0, len(s.history))
	for _, entry := range s.history {
		if filter.Branch != "" && entry.Branch != filter.Branch {
			continue
		}
		if filter.Database != "" && entry.Database != filter.Database {
			continue
		}
		filtered = append(filtered, entry)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].ID > filtered[j].ID })
	return pageExecutionHistory(filtered, filter), nil
}

func (*memorySQLQueryLibraryStore) Close() error { return nil }

func pageSavedQueries(entries []savedSQLQuery, filter sqlQueryLibraryFilter) []savedSQLQuery {
	start, end := sqlLibraryPageBounds(len(entries), filter)
	return append([]savedSQLQuery(nil), entries[start:end]...)
}

func pageExecutionHistory(entries []sqlExecutionHistory, filter sqlQueryLibraryFilter) []sqlExecutionHistory {
	start, end := sqlLibraryPageBounds(len(entries), filter)
	return append([]sqlExecutionHistory(nil), entries[start:end]...)
}

func sqlLibraryPageBounds(length int, filter sqlQueryLibraryFilter) (int, int) {
	limit := filter.Limit
	if limit < 1 {
		limit = 100
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > length {
		offset = length
	}
	end := offset + limit
	if end > length {
		end = length
	}
	return offset, end
}

type sqliteSQLQueryLibraryStore struct {
	db             *sql.DB
	retentionLimit int
	schemaVersion  int
	now            func() time.Time
}

func newSQLiteSQLQueryLibraryStore(path string, retentionLimit int) (sqlQueryLibraryStore, error) {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return newMemorySQLQueryLibraryStore(retentionLimit), nil
	}
	if retentionLimit < 1 {
		retentionLimit = defaultSQLHistoryRetentionLimit
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
	store := &sqliteSQLQueryLibraryStore{
		db:             db,
		retentionLimit: retentionLimit,
		now:            func() time.Time { return time.Now().UTC() },
	}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *sqliteSQLQueryLibraryStore) init() error {
	for _, pragma := range []string{`PRAGMA journal_mode=WAL`, `PRAGMA synchronous=NORMAL`, `PRAGMA busy_timeout=5000`} {
		if _, err := s.db.Exec(pragma); err != nil {
			return err
		}
	}
	version, err := sqliteutil.ApplyMigrations(s.db, "sql_query_library", []sqliteutil.Migration{{
		Version: sqliteSQLQueryLibrarySchemaVersion,
		SQL: `
			CREATE TABLE IF NOT EXISTS sql_saved_queries (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT NOT NULL,
				sql_text TEXT NOT NULL,
				branch TEXT NOT NULL,
				database_name TEXT NOT NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			);
			CREATE INDEX IF NOT EXISTS sql_saved_queries_scope_idx ON sql_saved_queries(branch, database_name, id DESC);
			CREATE TABLE IF NOT EXISTS sql_execution_history (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				title TEXT NOT NULL,
				sql_text TEXT NOT NULL,
				branch TEXT NOT NULL,
				database_name TEXT NOT NULL,
				read_only INTEGER NOT NULL,
				status TEXT NOT NULL,
				command_tag TEXT NOT NULL,
				duration_ms INTEGER NOT NULL,
				error_code TEXT NOT NULL,
				executed_at TEXT NOT NULL
			);
			CREATE INDEX IF NOT EXISTS sql_execution_history_scope_idx ON sql_execution_history(branch, database_name, id DESC);
		`,
	}})
	if err != nil {
		return err
	}
	if version > sqliteSQLQueryLibrarySchemaVersion {
		return fmt.Errorf("SQL query library schema version %d is newer than supported version %d", version, sqliteSQLQueryLibrarySchemaVersion)
	}
	s.schemaVersion = version
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := pruneSQLExecutionHistory(tx, s.retentionLimit); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *sqliteSQLQueryLibraryStore) CreateSavedQuery(ctx context.Context, query savedSQLQuery) (savedSQLQuery, error) {
	now := s.now().UTC()
	result, err := s.db.ExecContext(ctx, `INSERT INTO sql_saved_queries (name, sql_text, branch, database_name, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		query.Name, query.SQL, query.Branch, query.Database, formatSQLLibraryTime(now), formatSQLLibraryTime(now))
	if err != nil {
		return savedSQLQuery{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return savedSQLQuery{}, err
	}
	query.ID = id
	query.CreatedAt = now
	query.UpdatedAt = now
	return query, nil
}

func (s *sqliteSQLQueryLibraryStore) UpdateSavedQuery(ctx context.Context, id int64, name *string, query *string) (savedSQLQuery, error) {
	sets := []string{"updated_at = ?"}
	args := []any{formatSQLLibraryTime(s.now().UTC())}
	if name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *name)
	}
	if query != nil {
		sets = append(sets, "sql_text = ?")
		args = append(args, *query)
	}
	args = append(args, id)
	result, err := s.db.ExecContext(ctx, `UPDATE sql_saved_queries SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	if err != nil {
		return savedSQLQuery{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return savedSQLQuery{}, err
	}
	if count == 0 {
		return savedSQLQuery{}, errSQLSavedQueryNotFound
	}
	return s.getSavedQuery(ctx, id)
}

func (s *sqliteSQLQueryLibraryStore) getSavedQuery(ctx context.Context, id int64) (savedSQLQuery, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, sql_text, branch, database_name, created_at, updated_at FROM sql_saved_queries WHERE id = ?`, id)
	query, err := scanSavedSQLQuery(row)
	if errors.Is(err, sql.ErrNoRows) {
		return savedSQLQuery{}, errSQLSavedQueryNotFound
	}
	return query, err
}

func (s *sqliteSQLQueryLibraryStore) DeleteSavedQuery(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM sql_saved_queries WHERE id = ?`, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return errSQLSavedQueryNotFound
	}
	return nil
}

func (s *sqliteSQLQueryLibraryStore) ListSavedQueries(ctx context.Context, filter sqlQueryLibraryFilter) ([]savedSQLQuery, error) {
	filter = normalizeSQLQueryLibraryFilter(filter)
	where, args := sqlQueryLibraryWhere(filter)
	query := `SELECT id, name, sql_text, branch, database_name, created_at, updated_at FROM sql_saved_queries` + where + ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, filter.Limit, filter.Offset)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]savedSQLQuery, 0)
	for rows.Next() {
		entry, err := scanSavedSQLQuery(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *sqliteSQLQueryLibraryStore) AddExecutionHistory(ctx context.Context, entry sqlExecutionHistory) (sqlExecutionHistory, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return sqlExecutionHistory{}, err
	}
	defer tx.Rollback()
	if entry.ExecutedAt.IsZero() {
		entry.ExecutedAt = s.now().UTC()
	} else {
		entry.ExecutedAt = entry.ExecutedAt.UTC()
	}
	readOnly := 0
	if entry.ReadOnly {
		readOnly = 1
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO sql_execution_history (title, sql_text, branch, database_name, read_only, status, command_tag, duration_ms, error_code, executed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.Title, entry.SQL, entry.Branch, entry.Database, readOnly, entry.Status, entry.CommandTag, entry.DurationMS, entry.ErrorCode, formatSQLLibraryTime(entry.ExecutedAt))
	if err != nil {
		return sqlExecutionHistory{}, err
	}
	entry.ID, err = result.LastInsertId()
	if err != nil {
		return sqlExecutionHistory{}, err
	}
	if err := pruneSQLExecutionHistory(tx, s.retentionLimit); err != nil {
		return sqlExecutionHistory{}, err
	}
	if err := tx.Commit(); err != nil {
		return sqlExecutionHistory{}, err
	}
	return entry, nil
}

func pruneSQLExecutionHistory(execer interface {
	Exec(string, ...any) (sql.Result, error)
}, retentionLimit int) error {
	_, err := execer.Exec(`DELETE FROM sql_execution_history WHERE id NOT IN (SELECT id FROM sql_execution_history ORDER BY id DESC LIMIT ?)`, retentionLimit)
	return err
}

func (s *sqliteSQLQueryLibraryStore) ListExecutionHistory(ctx context.Context, filter sqlQueryLibraryFilter) ([]sqlExecutionHistory, error) {
	filter = normalizeSQLQueryLibraryFilter(filter)
	where, args := sqlQueryLibraryWhere(filter)
	query := `SELECT id, title, sql_text, branch, database_name, read_only, status, command_tag, duration_ms, error_code, executed_at FROM sql_execution_history` + where + ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, filter.Limit, filter.Offset)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]sqlExecutionHistory, 0)
	for rows.Next() {
		var entry sqlExecutionHistory
		var readOnly int
		var executedAt string
		if err := rows.Scan(&entry.ID, &entry.Title, &entry.SQL, &entry.Branch, &entry.Database, &readOnly, &entry.Status, &entry.CommandTag, &entry.DurationMS, &entry.ErrorCode, &executedAt); err != nil {
			return nil, err
		}
		entry.ReadOnly = readOnly == 1
		entry.ExecutedAt, err = parseSQLLibraryTime(executedAt)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func sqlQueryLibraryWhere(filter sqlQueryLibraryFilter) (string, []any) {
	where := make([]string, 0, 2)
	args := make([]any, 0, 4)
	if filter.Branch != "" {
		where = append(where, "branch = ?")
		args = append(args, filter.Branch)
	}
	if filter.Database != "" {
		where = append(where, "database_name = ?")
		args = append(args, filter.Database)
	}
	if len(where) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(where, " AND "), args
}

func normalizeSQLQueryLibraryFilter(filter sqlQueryLibraryFilter) sqlQueryLibraryFilter {
	if filter.Limit < 1 {
		filter.Limit = 100
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return filter
}

type sqlLibraryScanner interface {
	Scan(...any) error
}

func scanSavedSQLQuery(scanner sqlLibraryScanner) (savedSQLQuery, error) {
	var query savedSQLQuery
	var createdAt, updatedAt string
	if err := scanner.Scan(&query.ID, &query.Name, &query.SQL, &query.Branch, &query.Database, &createdAt, &updatedAt); err != nil {
		return savedSQLQuery{}, err
	}
	var err error
	query.CreatedAt, err = parseSQLLibraryTime(createdAt)
	if err != nil {
		return savedSQLQuery{}, err
	}
	query.UpdatedAt, err = parseSQLLibraryTime(updatedAt)
	return query, err
}

func formatSQLLibraryTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseSQLLibraryTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse SQL library timestamp: %w", err)
	}
	return parsed.UTC(), nil
}

func (s *sqliteSQLQueryLibraryStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func registerSQLQueryLibraryRoutes(mux *http.ServeMux, library *sqlQueryLibrary, branches *branch.Store, logger *slog.Logger) {
	mux.HandleFunc("GET /api/v1/sql/saved-queries", func(w http.ResponseWriter, r *http.Request) {
		filter, err := parseSQLQueryLibraryFilter(r)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}
		queries, err := library.ListSavedQueries(r.Context(), filter)
		if err != nil {
			writeSQLQueryLibraryError(w, err)
			return
		}
		payload := make([]savedSQLQueryPayload, 0, len(queries))
		for _, query := range queries {
			payload = append(payload, makeSavedSQLQueryPayload(query))
		}
		writeJSON(w, http.StatusOK, savedSQLQueriesEnvelope{Queries: payload})
	})

	mux.HandleFunc("POST /api/v1/sql/saved-queries", func(w http.ResponseWriter, r *http.Request) {
		var request createSavedSQLQueryRequest
		if err := decodeJSONRequest(r, &request, sqlJSONRequestBodyMaxBytes); err != nil {
			writeJSONDecodeError(w, err, sqlJSONRequestBodyMaxBytes)
			return
		}
		query, err := normalizeSavedSQLQuery(request)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}
		if _, err := branches.GetActive(query.Branch); err != nil {
			if errors.Is(err, branch.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "not_found", err.Error())
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		created, err := library.CreateSavedQuery(r.Context(), query)
		if err != nil {
			logger.Warn("create saved SQL query failed", "branch", query.Branch, "database", query.Database, "error", err)
			writeSQLQueryLibraryError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, savedSQLQueryEnvelope{Query: makeSavedSQLQueryPayload(created)})
	})

	mux.HandleFunc("PATCH /api/v1/sql/saved-queries/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := parseSQLQueryLibraryID(r.PathValue("id"))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}
		var request updateSavedSQLQueryRequest
		if err := decodeJSONRequest(r, &request, sqlJSONRequestBodyMaxBytes); err != nil {
			writeJSONDecodeError(w, err, sqlJSONRequestBodyMaxBytes)
			return
		}
		if request.Name == nil && request.SQL == nil {
			writeJSONError(w, http.StatusBadRequest, "validation_error", "at least one of name or sql is required")
			return
		}
		if err := normalizeSavedSQLQueryUpdate(&request); err != nil {
			writeJSONError(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}
		updated, err := library.UpdateSavedQuery(r.Context(), id, request.Name, request.SQL)
		if err != nil {
			writeSQLQueryLibraryError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, savedSQLQueryEnvelope{Query: makeSavedSQLQueryPayload(updated)})
	})

	mux.HandleFunc("DELETE /api/v1/sql/saved-queries/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := parseSQLQueryLibraryID(r.PathValue("id"))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}
		if err := library.DeleteSavedQuery(r.Context(), id); err != nil {
			writeSQLQueryLibraryError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/v1/sql/history", func(w http.ResponseWriter, r *http.Request) {
		filter, err := parseSQLQueryLibraryFilter(r)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}
		history, err := library.ListExecutionHistory(r.Context(), filter)
		if err != nil {
			writeSQLQueryLibraryError(w, err)
			return
		}
		payload := make([]sqlExecutionHistoryPayload, 0, len(history))
		for _, entry := range history {
			payload = append(payload, makeSQLExecutionHistoryPayload(entry))
		}
		writeJSON(w, http.StatusOK, sqlExecutionHistoryEnvelope{History: payload})
	})
}

func normalizeSavedSQLQuery(request createSavedSQLQueryRequest) (savedSQLQuery, error) {
	name := strings.TrimSpace(request.Name)
	if name == "" {
		return savedSQLQuery{}, errors.New("name is required")
	}
	if strings.TrimSpace(request.SQL) == "" {
		return savedSQLQuery{}, errors.New("sql is required")
	}
	if len(request.SQL) > defaultSQLExecutionMaxQueryBytes {
		return savedSQLQuery{}, fmt.Errorf("sql exceeds %d bytes", defaultSQLExecutionMaxQueryBytes)
	}
	branchName := strings.TrimSpace(request.Branch)
	if branchName == "" {
		return savedSQLQuery{}, errors.New("branch is required")
	}
	database := request.Database
	if database == "" {
		return savedSQLQuery{}, errors.New("database is required")
	}
	if err := validateSQLDatabaseName(database); err != nil {
		return savedSQLQuery{}, err
	}
	return savedSQLQuery{Name: name, SQL: request.SQL, Branch: branchName, Database: database}, nil
}

func normalizeSavedSQLQueryUpdate(request *updateSavedSQLQueryRequest) error {
	if request.Name != nil {
		name := strings.TrimSpace(*request.Name)
		if name == "" {
			return errors.New("name is required")
		}
		request.Name = &name
	}
	if request.SQL != nil {
		if strings.TrimSpace(*request.SQL) == "" {
			return errors.New("sql is required")
		}
		if len(*request.SQL) > defaultSQLExecutionMaxQueryBytes {
			return fmt.Errorf("sql exceeds %d bytes", defaultSQLExecutionMaxQueryBytes)
		}
	}
	return nil
}

func parseSQLQueryLibraryFilter(r *http.Request) (sqlQueryLibraryFilter, error) {
	filter := sqlQueryLibraryFilter{
		Branch:   strings.TrimSpace(r.URL.Query().Get("branch")),
		Database: r.URL.Query().Get("database"),
		Limit:    100,
	}
	if filter.Database != "" {
		if err := validateSQLDatabaseName(filter.Database); err != nil {
			return sqlQueryLibraryFilter{}, err
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 1000 {
			return sqlQueryLibraryFilter{}, errors.New("limit must be between 1 and 1000")
		}
		filter.Limit = limit
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		offset, err := strconv.Atoi(raw)
		if err != nil || offset < 0 {
			return sqlQueryLibraryFilter{}, errors.New("offset must be zero or greater")
		}
		filter.Offset = offset
	}
	return filter, nil
}

func parseSQLQueryLibraryID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("id must be a positive integer")
	}
	return id, nil
}

func writeSQLQueryLibraryError(w http.ResponseWriter, err error) {
	if errors.Is(err, errSQLSavedQueryNotFound) {
		writeJSONError(w, http.StatusNotFound, "not_found", "saved SQL query not found")
		return
	}
	writeJSONError(w, http.StatusServiceUnavailable, "sql_library_unavailable", "SQL query library is unavailable")
}

func makeSavedSQLQueryPayload(query savedSQLQuery) savedSQLQueryPayload {
	return savedSQLQueryPayload{
		ID: query.ID, Name: query.Name, SQL: query.SQL, Branch: query.Branch, Database: query.Database,
		CreatedAt: formatSQLLibraryTime(query.CreatedAt), UpdatedAt: formatSQLLibraryTime(query.UpdatedAt),
	}
}

func makeSQLExecutionHistoryPayload(entry sqlExecutionHistory) sqlExecutionHistoryPayload {
	return sqlExecutionHistoryPayload{
		ID: entry.ID, Title: entry.Title, SQL: entry.SQL, Branch: entry.Branch, Database: entry.Database,
		ReadOnly: entry.ReadOnly, Status: entry.Status, CommandTag: entry.CommandTag, DurationMS: entry.DurationMS,
		ErrorCode: entry.ErrorCode, ExecutedAt: formatSQLLibraryTime(entry.ExecutedAt),
	}
}

func recordSQLExecutionHistory(library *sqlQueryLibrary, logger *slog.Logger, entry sqlExecutionHistory) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	if _, err := library.AddExecutionHistory(ctx, entry); err != nil {
		logger.Warn("record SQL execution history failed", "branch", entry.Branch, "database", entry.Database, "status", entry.Status, "error", err)
	}
}

func sqlExecutionHistoryStatus(err error) string {
	if errors.Is(err, ErrSQLCommitOutcomeUnknown) {
		return sqlHistoryStatusOutcomeUnknown
	}
	return sqlHistoryStatusFailed
}

func sqlExecutionHistoryErrorCode(err error) string {
	switch {
	case errors.Is(err, branch.ErrNotFound):
		return "not_found"
	case errors.Is(err, ErrSQLWriteResultTruncated):
		return "result_limit"
	case errors.Is(err, ErrSQLCommitFailed):
		return "commit_failed"
	case errors.Is(err, ErrSQLCommitOutcomeUnknown):
		return "commit_outcome_unknown"
	case isPrimaryEndpointUnavailable(err):
		return "endpoint_unavailable"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		var sqlErr *sqlExecutionError
		if errors.As(err, &sqlErr) {
			return "sql_error"
		}
		return "internal_error"
	}
}
