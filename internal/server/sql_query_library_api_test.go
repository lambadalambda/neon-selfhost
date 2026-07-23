package server

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"neon-selfhost/internal/branch"
)

func TestSavedQueryAPICRUDAndProjectFiltering(t *testing.T) {
	store := newMemorySQLQueryLibraryStore(200)
	handler := New(Config{Version: "test", sqlQueryLibraryStore: store})

	missing := performRequest(t, handler, http.MethodPost, "/api/v1/sql/saved-queries", `{"name":"missing","sql":"SELECT 1","branch":"missing","database":"postgres"}`)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("expected unknown branch status %d, got %d", http.StatusNotFound, missing.Code)
	}

	first := performRequest(t, handler, http.MethodPost, "/api/v1/sql/saved-queries", `{"name":"posts","sql":"SELECT * FROM posts","branch":"main","database":"postgres"}`)
	if first.Code != http.StatusCreated {
		t.Fatalf("expected create status %d, got %d: %s", http.StatusCreated, first.Code, first.Body.String())
	}
	var firstPayload savedSQLQueryEnvelope
	decodeJSON(t, first, &firstPayload)

	branchStore := branch.NewStore()
	if _, err := branchStore.Create("feature-a", "main"); err != nil {
		t.Fatalf("create feature branch: %v", err)
	}
	handler = New(Config{Version: "test", BranchStore: branchStore, sqlQueryLibraryStore: store})
	second := performRequest(t, handler, http.MethodPost, "/api/v1/sql/saved-queries", `{"name":"posts","sql":"SELECT id FROM posts","branch":"feature-a","database":"pleroma"}`)
	if second.Code != http.StatusCreated {
		t.Fatalf("expected duplicate-name create status %d, got %d", http.StatusCreated, second.Code)
	}

	project := performRequest(t, handler, http.MethodGet, "/api/v1/sql/saved-queries?limit=1&offset=1", "")
	if project.Code != http.StatusOK {
		t.Fatalf("expected project list status %d, got %d", http.StatusOK, project.Code)
	}
	var projectPayload savedSQLQueriesEnvelope
	decodeJSON(t, project, &projectPayload)
	if len(projectPayload.Queries) != 1 || projectPayload.Queries[0].ID != firstPayload.Query.ID {
		t.Fatalf("unexpected project-wide page: %+v", projectPayload.Queries)
	}

	filtered := performRequest(t, handler, http.MethodGet, "/api/v1/sql/saved-queries?branch=feature-a&database=pleroma", "")
	var filteredPayload savedSQLQueriesEnvelope
	decodeJSON(t, filtered, &filteredPayload)
	if len(filteredPayload.Queries) != 1 || filteredPayload.Queries[0].Branch != "feature-a" {
		t.Fatalf("unexpected filtered queries: %+v", filteredPayload.Queries)
	}

	patch := performRequest(t, handler, http.MethodPatch, "/api/v1/sql/saved-queries/"+formatSQLLibraryID(firstPayload.Query.ID), `{"name":"new posts","sql":"SELECT id FROM posts ORDER BY id DESC"}`)
	if patch.Code != http.StatusOK {
		t.Fatalf("expected patch status %d, got %d: %s", http.StatusOK, patch.Code, patch.Body.String())
	}
	var patchPayload savedSQLQueryEnvelope
	decodeJSON(t, patch, &patchPayload)
	if patchPayload.Query.Name != "new posts" || patchPayload.Query.Branch != "main" || patchPayload.Query.Database != "postgres" {
		t.Fatalf("unexpected patched query: %+v", patchPayload.Query)
	}

	deleted := performRequest(t, handler, http.MethodDelete, "/api/v1/sql/saved-queries/"+formatSQLLibraryID(firstPayload.Query.ID), "")
	if deleted.Code != http.StatusNoContent || deleted.Body.Len() != 0 {
		t.Fatalf("expected empty 204 delete, got %d %q", deleted.Code, deleted.Body.String())
	}
	notFound := performRequest(t, handler, http.MethodDelete, "/api/v1/sql/saved-queries/"+formatSQLLibraryID(firstPayload.Query.ID), "")
	if notFound.Code != http.StatusNotFound {
		t.Fatalf("expected second delete status %d, got %d", http.StatusNotFound, notFound.Code)
	}
	assertAPIErrorCode(t, notFound, "not_found")
	res := performRequest(t, handler, http.MethodGet, "/api/v1/health", "")
	var health healthEndpointResponse
	decodeJSON(t, res, &health)
	if health.Status != "ok" {
		t.Fatalf("expected an ordinary saved-query 404 to preserve healthy status, got %+v", health)
	}
}

func TestSavedQueryAPIValidation(t *testing.T) {
	handler := New(Config{Version: "test", sqlQueryLibraryStore: newMemorySQLQueryLibraryStore(200)})
	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "empty name", method: http.MethodPost, path: "/api/v1/sql/saved-queries", body: `{"name":" ","sql":"SELECT 1","branch":"main","database":"postgres"}`},
		{name: "empty sql", method: http.MethodPost, path: "/api/v1/sql/saved-queries", body: `{"name":"x","sql":" ","branch":"main","database":"postgres"}`},
		{name: "empty branch", method: http.MethodPost, path: "/api/v1/sql/saved-queries", body: `{"name":"x","sql":"SELECT 1","branch":"","database":"postgres"}`},
		{name: "empty database", method: http.MethodPost, path: "/api/v1/sql/saved-queries", body: `{"name":"x","sql":"SELECT 1","branch":"main","database":""}`},
		{name: "invalid database", method: http.MethodPost, path: "/api/v1/sql/saved-queries", body: `{"name":"x","sql":"SELECT 1","branch":"main","database":"bad\u0000db"}`},
		{name: "oversized sql", method: http.MethodPost, path: "/api/v1/sql/saved-queries", body: `{"name":"x","sql":"` + strings.Repeat("x", defaultSQLExecutionMaxQueryBytes+1) + `","branch":"main","database":"postgres"}`},
		{name: "empty patch", method: http.MethodPatch, path: "/api/v1/sql/saved-queries/1", body: `{}`},
		{name: "empty patch value", method: http.MethodPatch, path: "/api/v1/sql/saved-queries/1", body: `{"name":" "}`},
		{name: "invalid id", method: http.MethodPatch, path: "/api/v1/sql/saved-queries/nope", body: `{"name":"x"}`},
		{name: "limit zero", method: http.MethodGet, path: "/api/v1/sql/saved-queries?limit=0"},
		{name: "limit too high", method: http.MethodGet, path: "/api/v1/sql/saved-queries?limit=1001"},
		{name: "negative offset", method: http.MethodGet, path: "/api/v1/sql/history?offset=-1"},
		{name: "invalid filter database", method: http.MethodGet, path: "/api/v1/sql/history?database=bad%00db"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := performRequest(t, handler, tt.method, tt.path, tt.body)
			if res.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, res.Code, res.Body.String())
			}
			assertAPIErrorCode(t, res, "validation_error")
		})
	}
}

func TestSavedQueryAPIPreservesWhitespaceInDatabaseName(t *testing.T) {
	store := newMemorySQLQueryLibraryStore(200)
	handler := New(Config{Version: "test", sqlQueryLibraryStore: store})
	res := performRequest(t, handler, http.MethodPost, "/api/v1/sql/saved-queries", `{"name":"spaced","sql":"SELECT 1","branch":"main","database":" analytics "}`)
	if res.Code != http.StatusCreated {
		t.Fatalf("expected create status %d, got %d: %s", http.StatusCreated, res.Code, res.Body.String())
	}
	var created savedSQLQueryEnvelope
	decodeJSON(t, res, &created)
	if created.Query.Database != " analytics " {
		t.Fatalf("expected exact database name, got %q", created.Query.Database)
	}

	filtered := performRequest(t, handler, http.MethodGet, "/api/v1/sql/saved-queries?database=%20analytics%20", "")
	var listed savedSQLQueriesEnvelope
	decodeJSON(t, filtered, &listed)
	if len(listed.Queries) != 1 || listed.Queries[0].ID != created.Query.ID {
		t.Fatalf("expected exact database filter match, got %+v", listed.Queries)
	}
}

func TestSQLExecutionRecordsSuccessAndFailureHistory(t *testing.T) {
	store := newMemorySQLQueryLibraryStore(200)
	successExecutor := &fakeSQLQueryExecutor{result: sqlExecutionResult{
		Branch: "main", Database: "actual_db", ReadOnly: true, CommandTag: "SELECT 1", DurationMS: 17,
		Columns: []sqlExecutionColumn{{Name: "secret"}}, Rows: [][]any{{"must-not-be-stored"}},
	}}
	handler := New(Config{Version: "test", SQLExecutor: successExecutor, sqlQueryLibraryStore: store})
	res := performRequest(t, handler, http.MethodPost, "/api/v1/branches/main/sql/execute", `{"title":"Daily check","database":"requested_db","sql":"SELECT 1"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("expected successful execution, got %d", res.Code)
	}

	history, err := store.ListExecutionHistory(context.Background(), sqlQueryLibraryFilter{Limit: 100})
	if err != nil {
		t.Fatalf("list success history: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected one success history row, got %d", len(history))
	}
	if history[0].Title != "Daily check" || history[0].Database != "actual_db" || history[0].Status != sqlHistoryStatusSucceeded || history[0].CommandTag != "SELECT 1" || history[0].DurationMS != 17 {
		t.Fatalf("unexpected success history: %+v", history[0])
	}

	failureExecutor := &fakeSQLQueryExecutor{result: sqlExecutionResult{Branch: "main", Database: "default_db", ReadOnly: true}, err: &sqlExecutionError{Message: "syntax error", SQLState: "42601"}}
	handler = New(Config{Version: "test", SQLExecutor: failureExecutor, sqlQueryLibraryStore: store})
	res = performRequest(t, handler, http.MethodPost, "/api/v1/branches/main/sql/execute", `{"title":"Broken","sql":"SELECT FROM"}`)
	if res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected failed execution response, got %d", res.Code)
	}
	history, _ = store.ListExecutionHistory(context.Background(), sqlQueryLibraryFilter{Limit: 100})
	if len(history) != 2 || history[0].Database != "default_db" || history[0].Status != sqlHistoryStatusFailed || history[0].ErrorCode != "sql_error" {
		t.Fatalf("unexpected failed history: %+v", history)
	}

	unknownExecutor := &fakeSQLQueryExecutor{err: ErrSQLCommitOutcomeUnknown}
	handler = New(Config{Version: "test", SQLExecutor: unknownExecutor, sqlQueryLibraryStore: store})
	_ = performRequest(t, handler, http.MethodPost, "/api/v1/branches/main/sql/execute", `{"sql":"UPDATE posts SET title='x'","allow_writes":true,"confirm_protected_writes":true}`)
	history, _ = store.ListExecutionHistory(context.Background(), sqlQueryLibraryFilter{Limit: 100})
	if history[0].Status != sqlHistoryStatusOutcomeUnknown || history[0].ErrorCode != "commit_outcome_unknown" {
		t.Fatalf("unexpected unknown-outcome history: %+v", history[0])
	}

}

func TestSQLExecutionDoesNotRecordPrevalidationRejections(t *testing.T) {
	store := newMemorySQLQueryLibraryStore(200)
	executor := &fakeSQLQueryExecutor{}
	handler := New(Config{Version: "test", SQLExecutor: executor, sqlQueryLibraryStore: store})

	_ = performRequest(t, handler, http.MethodPost, "/api/v1/branches/main/sql/execute", `{"sql":"SELECT 1; SELECT 2"}`)
	_ = performRequest(t, handler, http.MethodPost, "/api/v1/branches/main/sql/execute", `{"sql":"UPDATE posts SET title='x'","allow_writes":true}`)
	history, err := store.ListExecutionHistory(context.Background(), sqlQueryLibraryFilter{Limit: 100})
	if err != nil {
		t.Fatalf("list history: %v", err)
	}
	if len(history) != 0 || len(executor.calls) != 0 {
		t.Fatalf("expected no execution or history for rejected requests, calls=%d history=%+v", len(executor.calls), history)
	}
}

func TestSQLHistoryAPIListsAndFiltersProjectHistory(t *testing.T) {
	store := newMemorySQLQueryLibraryStore(200)
	for _, entry := range []sqlExecutionHistory{
		{Title: "main", SQL: "SELECT 1", Branch: "main", Database: "postgres", ReadOnly: true, Status: sqlHistoryStatusSucceeded},
		{Title: "feature", SQL: "SELECT 2", Branch: "feature-a", Database: "pleroma", ReadOnly: true, Status: sqlHistoryStatusSucceeded},
	} {
		if _, err := store.AddExecutionHistory(context.Background(), entry); err != nil {
			t.Fatalf("seed history: %v", err)
		}
	}
	handler := New(Config{Version: "test", sqlQueryLibraryStore: store})

	project := performRequest(t, handler, http.MethodGet, "/api/v1/sql/history?limit=1&offset=1", "")
	if project.Code != http.StatusOK {
		t.Fatalf("expected project history status %d, got %d", http.StatusOK, project.Code)
	}
	var projectPayload sqlExecutionHistoryEnvelope
	decodeJSON(t, project, &projectPayload)
	if len(projectPayload.History) != 1 || projectPayload.History[0].Title != "main" {
		t.Fatalf("unexpected project history page: %+v", projectPayload.History)
	}

	filtered := performRequest(t, handler, http.MethodGet, "/api/v1/sql/history?branch=feature-a&database=pleroma", "")
	var filteredPayload sqlExecutionHistoryEnvelope
	decodeJSON(t, filtered, &filteredPayload)
	if len(filteredPayload.History) != 1 || filteredPayload.History[0].Title != "feature" {
		t.Fatalf("unexpected filtered history: %+v", filteredPayload.History)
	}
}

func TestSQLHistoryFailureDoesNotChangeExecutionResponse(t *testing.T) {
	store := &faultSQLQueryLibraryStore{err: errors.New("history unavailable")}
	executor := &fakeSQLQueryExecutor{result: sqlExecutionResult{Branch: "main", Database: "postgres", CommandTag: "SELECT 1"}}
	handler := New(Config{Version: "test", SQLExecutor: executor, sqlQueryLibraryStore: store})
	res := performRequest(t, handler, http.MethodPost, "/api/v1/branches/main/sql/execute", `{"sql":"SELECT 1"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("expected history failure not to alter SQL response, got %d: %s", res.Code, res.Body.String())
	}
	assertSQLQueryLibraryDegraded(t, handler)
}

func TestSQLQueryLibraryStoreFailureReturnsUnavailableAndDegradesHealth(t *testing.T) {
	failure := errors.New("library offline")
	store := &faultSQLQueryLibraryStore{err: failure}
	handler := New(Config{Version: "test", sqlQueryLibraryStore: store})
	res := performRequest(t, handler, http.MethodGet, "/api/v1/sql/saved-queries", "")
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unavailable status %d, got %d", http.StatusServiceUnavailable, res.Code)
	}
	assertAPIErrorCode(t, res, "sql_library_unavailable")
	assertSQLQueryLibraryDegraded(t, handler)
}

func TestCanceledSQLLibraryRequestDoesNotDegradeHealth(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	library := newSQLQueryLibrary(&faultSQLQueryLibraryStore{err: context.Canceled}, false)
	if _, err := library.ListSavedQueries(ctx, sqlQueryLibraryFilter{Limit: 100}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled request error, got %v", err)
	}
	if library.degraded() {
		t.Fatal("expected client cancellation not to degrade SQL library health")
	}
}

func TestConfiguredSQLQueryLibraryInitializationFailureHasNoMemoryFallback(t *testing.T) {
	handler := New(Config{Version: "test", SQLQueryLibraryDBPath: t.TempDir()})
	res := performRequest(t, handler, http.MethodPost, "/api/v1/sql/saved-queries", `{"name":"x","sql":"SELECT 1","branch":"main","database":"postgres"}`)
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unavailable store status %d, got %d", http.StatusServiceUnavailable, res.Code)
	}
	assertAPIErrorCode(t, res, "sql_library_unavailable")
	assertSQLQueryLibraryDegraded(t, handler)
}

func TestHandlerCloseAggregatesStoresAndIsIdempotent(t *testing.T) {
	opStore := newFaultOperationStore()
	sqlStore := &faultSQLQueryLibraryStore{closeErr: errors.New("close SQL library")}
	handler := New(Config{Version: "test", operationStore: opStore, sqlQueryLibraryStore: sqlStore})
	closer, ok := handler.(interface{ Close() error })
	if !ok {
		t.Fatal("handler is not closeable")
	}
	firstErr := closer.Close()
	if !errors.Is(firstErr, sqlStore.closeErr) {
		t.Fatalf("expected aggregate close error, got %v", firstErr)
	}
	secondErr := closer.Close()
	if !errors.Is(secondErr, sqlStore.closeErr) {
		t.Fatalf("expected idempotent close to return first error, got %v", secondErr)
	}
	if opStore.closeCount != 1 || sqlStore.closeCount != 1 {
		t.Fatalf("expected each store closed once, operations=%d sql=%d", opStore.closeCount, sqlStore.closeCount)
	}
}

func TestStatusIncludesSQLQueryLibraryPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controller.db")
	handler := New(Config{Version: "test", SQLQueryLibraryDBPath: path})
	defer handler.(interface{ Close() error }).Close()
	res := performRequest(t, handler, http.MethodGet, "/api/v1/status", "")
	var payload struct {
		Persistence struct {
			Mode          string `json:"sql_query_library_mode"`
			Path          string `json:"sql_query_library_db_path"`
			SchemaVersion int    `json:"sql_query_library_schema_version"`
		} `json:"persistence"`
	}
	decodeJSON(t, res, &payload)
	if payload.Persistence.Mode != "sqlite" || payload.Persistence.Path != path || payload.Persistence.SchemaVersion != sqliteSQLQueryLibrarySchemaVersion {
		t.Fatalf("unexpected SQL library persistence status: %+v", payload.Persistence)
	}
}

func assertSQLQueryLibraryDegraded(t *testing.T, handler http.Handler) {
	t.Helper()
	res := performRequest(t, handler, http.MethodGet, "/api/v1/health", "")
	var payload healthEndpointResponse
	decodeJSON(t, res, &payload)
	for _, check := range payload.Checks {
		if check.Name == "sql_query_library" {
			if check.Status != "degraded" || payload.Status != "degraded" {
				t.Fatalf("expected degraded SQL library health, got %+v", payload)
			}
			return
		}
	}
	t.Fatal("sql_query_library health check missing")
}

func formatSQLLibraryID(id int64) string {
	return strconv.FormatInt(id, 10)
}

type faultSQLQueryLibraryStore struct {
	err        error
	closeErr   error
	closeCount int
}

func (s *faultSQLQueryLibraryStore) CreateSavedQuery(context.Context, savedSQLQuery) (savedSQLQuery, error) {
	return savedSQLQuery{}, s.err
}
func (s *faultSQLQueryLibraryStore) UpdateSavedQuery(context.Context, int64, *string, *string) (savedSQLQuery, error) {
	return savedSQLQuery{}, s.err
}
func (s *faultSQLQueryLibraryStore) DeleteSavedQuery(context.Context, int64) error { return s.err }
func (s *faultSQLQueryLibraryStore) ListSavedQueries(context.Context, sqlQueryLibraryFilter) ([]savedSQLQuery, error) {
	return nil, s.err
}
func (s *faultSQLQueryLibraryStore) AddExecutionHistory(context.Context, sqlExecutionHistory) (sqlExecutionHistory, error) {
	return sqlExecutionHistory{}, s.err
}
func (s *faultSQLQueryLibraryStore) ListExecutionHistory(context.Context, sqlQueryLibraryFilter) ([]sqlExecutionHistory, error) {
	return nil, s.err
}
func (s *faultSQLQueryLibraryStore) Close() error {
	s.closeCount++
	return s.closeErr
}
