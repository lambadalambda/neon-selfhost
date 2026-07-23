package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

type sqlExecuteResponse struct {
	Result struct {
		Branch     string `json:"branch"`
		Database   string `json:"database"`
		ReadOnly   bool   `json:"read_only"`
		CommandTag string `json:"command_tag"`
		DurationMS int64  `json:"duration_ms"`
		Truncated  bool   `json:"truncated"`
		Limits     struct {
			MaxRows  int `json:"max_rows"`
			MaxBytes int `json:"max_bytes"`
		} `json:"limits"`
		Columns []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"columns"`
		Rows     [][]any `json:"rows"`
		RowCount int     `json:"row_count"`
	} `json:"result"`
}

type sqlDatabasesResponse struct {
	Databases []string `json:"databases"`
	Default   string   `json:"default"`
}

func TestExecuteSQLReturnsResultPayload(t *testing.T) {
	executor := &fakeSQLQueryExecutor{
		result: sqlExecutionResult{
			Branch:     "main",
			Database:   "postgres",
			CommandTag: "SELECT 1",
			DurationMS: 5,
			Truncated:  false,
			MaxRows:    200,
			MaxBytes:   1024,
			Columns: []sqlExecutionColumn{{
				Name: "answer",
				Type: "int4",
			}},
			Rows:     [][]any{{float64(1)}},
			RowCount: 1,
		},
	}

	handler := New(Config{Version: "test-version", SQLExecutor: executor})
	res := performRequest(t, handler, http.MethodPost, "/api/v1/branches/main/sql/execute", `{"sql":"SELECT 1"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	if len(executor.calls) != 1 {
		t.Fatalf("expected one sql execution call, got %d", len(executor.calls))
	}

	if executor.calls[0].branchName != "main" {
		t.Fatalf("expected branch %q, got %q", "main", executor.calls[0].branchName)
	}

	if executor.calls[0].query != "SELECT 1" {
		t.Fatalf("expected query %q, got %q", "SELECT 1", executor.calls[0].query)
	}
	if executor.calls[0].database != "" {
		t.Fatalf("expected omitted database to preserve the endpoint default, got %q", executor.calls[0].database)
	}

	if !executor.calls[0].readOnly {
		t.Fatal("expected SQL execution to be read-only by default")
	}

	var payload sqlExecuteResponse
	decodeJSON(t, res, &payload)

	if payload.Result.Branch != "main" {
		t.Fatalf("expected payload branch %q, got %q", "main", payload.Result.Branch)
	}
	if payload.Result.Database != "postgres" {
		t.Fatalf("expected payload database %q, got %q", "postgres", payload.Result.Database)
	}

	if payload.Result.RowCount != 1 {
		t.Fatalf("expected row_count %d, got %d", 1, payload.Result.RowCount)
	}

	if payload.Result.CommandTag != "SELECT 1" {
		t.Fatalf("expected command tag %q, got %q", "SELECT 1", payload.Result.CommandTag)
	}

	if !payload.Result.ReadOnly {
		t.Fatal("expected read_only=true in default SQL execution payload")
	}
}

func TestExecuteSQLUsesRequestedDatabase(t *testing.T) {
	executor := &fakeSQLQueryExecutor{result: sqlExecutionResult{Branch: "main", Database: "pleroma", CommandTag: "SELECT 1"}}
	handler := New(Config{Version: "test-version", SQLExecutor: executor})

	res := performRequest(t, handler, http.MethodPost, "/api/v1/branches/main/sql/execute", `{"database":"pleroma","sql":"SELECT 1"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}
	if len(executor.calls) != 1 || executor.calls[0].database != "pleroma" {
		t.Fatalf("expected execution against pleroma, got %#v", executor.calls)
	}
}

func TestExecuteSQLRejectsInvalidDatabase(t *testing.T) {
	handler := New(Config{Version: "test-version", SQLExecutor: &fakeSQLQueryExecutor{}})
	res := performRequest(t, handler, http.MethodPost, "/api/v1/branches/main/sql/execute", `{"database":"bad\u0000name","sql":"SELECT 1"}`)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.Code)
	}
	assertAPIErrorCode(t, res, "validation_error")
}

func TestParseSQLConnectionConfigPreservesDatabaseName(t *testing.T) {
	config, err := parseSQLConnectionConfig(branchEndpointState{
		Host:     "127.0.0.1",
		Port:     5432,
		Database: "postgres",
		User:     "cloud_admin",
		Password: "secret",
	}, "127.0.0.1", "/pleroma")
	if err != nil {
		t.Fatalf("parse connection config: %v", err)
	}
	if config.Database != "/pleroma" {
		t.Fatalf("expected exact database name %q, got %q", "/pleroma", config.Database)
	}
}

func TestClassifySQLConnectError(t *testing.T) {
	databaseErr := classifySQLConnectError(&pgconn.PgError{Code: "3D000", Message: "database does not exist"})
	var sqlErr *sqlExecutionError
	if !errors.As(databaseErr, &sqlErr) {
		t.Fatalf("expected missing database to be a SQL error, got %T", databaseErr)
	}

	authErr := classifySQLConnectError(&pgconn.PgError{Code: "28P01", Message: "password authentication failed"})
	if !errors.Is(authErr, ErrPrimaryEndpointUnavailable) {
		t.Fatalf("expected authentication failure to mark endpoint unavailable, got %v", authErr)
	}

	timeoutErr := classifySQLConnectError(context.DeadlineExceeded)
	if !errors.Is(timeoutErr, context.DeadlineExceeded) || errors.Is(timeoutErr, ErrPrimaryEndpointUnavailable) {
		t.Fatalf("expected timeout classification to remain a timeout, got %v", timeoutErr)
	}
}

func TestListSQLDatabasesReturnsBranchDatabases(t *testing.T) {
	executor := &fakeSQLQueryExecutor{
		databaseList: sqlDatabaseList{Names: []string{"pleroma", "postgres"}, Default: "postgres"},
	}
	handler := New(Config{Version: "test-version", SQLExecutor: executor})

	res := performRequest(t, handler, http.MethodGet, "/api/v1/branches/main/databases", "")
	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}
	if len(executor.databaseCalls) != 1 || executor.databaseCalls[0] != "main" {
		t.Fatalf("expected database listing for main, got %#v", executor.databaseCalls)
	}

	var payload sqlDatabasesResponse
	decodeJSON(t, res, &payload)
	if payload.Default != "postgres" {
		t.Fatalf("expected default database %q, got %q", "postgres", payload.Default)
	}
	if strings.Join(payload.Databases, ",") != "pleroma,postgres" {
		t.Fatalf("unexpected databases: %#v", payload.Databases)
	}
}

func TestListSQLDatabasesReturnsUnavailableWhenExecutorUnavailable(t *testing.T) {
	executor := &fakeSQLQueryExecutor{databaseErr: fmt.Errorf("%w: endpoint unavailable", ErrPrimaryEndpointUnavailable)}
	handler := New(Config{Version: "test-version", SQLExecutor: executor})

	res := performRequest(t, handler, http.MethodGet, "/api/v1/branches/main/databases", "")
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, res.Code)
	}
	assertAPIErrorCode(t, res, "endpoint_unavailable")
}

func TestListSQLDatabasesReturnsNotFoundForUnknownBranch(t *testing.T) {
	handler := New(Config{Version: "test-version", SQLExecutor: &fakeSQLQueryExecutor{}})
	res := performRequest(t, handler, http.MethodGet, "/api/v1/branches/missing/databases", "")
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, res.Code)
	}
	assertAPIErrorCode(t, res, "not_found")
}

func TestExecuteSQLAllowsWritesWhenRequested(t *testing.T) {
	executor := &fakeSQLQueryExecutor{
		result: sqlExecutionResult{
			Branch:     "main",
			CommandTag: "UPDATE 1",
			DurationMS: 7,
			RowCount:   0,
		},
	}

	handler := New(Config{Version: "test-version", SQLExecutor: executor})
	res := performRequest(t, handler, http.MethodPost, "/api/v1/branches/main/sql/execute", `{"sql":"UPDATE app.documents SET title='x' WHERE id=1","allow_writes":true,"confirm_protected_writes":true}`)
	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	if len(executor.calls) != 1 {
		t.Fatalf("expected one sql execution call, got %d", len(executor.calls))
	}

	if executor.calls[0].readOnly {
		t.Fatal("expected SQL execution to run with writes enabled when allow_writes=true")
	}

	var payload sqlExecuteResponse
	decodeJSON(t, res, &payload)
	if payload.Result.ReadOnly {
		t.Fatal("expected read_only=false in SQL execution payload when writes are enabled")
	}
}

func TestExecuteSQLRejectsUnconfirmedWritesToProtectedBranch(t *testing.T) {
	executor := &fakeSQLQueryExecutor{}
	handler := New(Config{Version: "test-version", SQLExecutor: executor})

	res := performRequest(t, handler, http.MethodPost, "/api/v1/branches/main/sql/execute", `{"sql":"UPDATE app.documents SET title='x' WHERE id=1","allow_writes":true}`)
	if res.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, res.Code)
	}
	assertAPIErrorCode(t, res, "protected_confirmation_required")
	if len(executor.calls) != 0 {
		t.Fatalf("expected protected write rejection before execution, got %d calls", len(executor.calls))
	}
}

func TestExecuteSQLRejectsMultiStatement(t *testing.T) {
	handler := New(Config{Version: "test-version", SQLExecutor: &fakeSQLQueryExecutor{}})
	res := performRequest(t, handler, http.MethodPost, "/api/v1/branches/main/sql/execute", `{"sql":"SELECT 1; SELECT 2;"}`)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.Code)
	}

	assertAPIErrorCode(t, res, "validation_error")
}

func TestExecuteSQLRejectsOversizedRequestBody(t *testing.T) {
	handler := New(Config{Version: "test-version", SQLExecutor: &fakeSQLQueryExecutor{}})
	body := `{"sql":"` + strings.Repeat("x", 130*1024) + `"}`
	res := performRequest(t, handler, http.MethodPost, "/api/v1/branches/main/sql/execute", body)
	if res.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status %d, got %d", http.StatusRequestEntityTooLarge, res.Code)
	}

	assertAPIErrorCode(t, res, "request_too_large")
}

func TestExecuteSQLReturnsSQLValidationErrorPayload(t *testing.T) {
	handler := New(Config{Version: "test-version", SQLExecutor: &fakeSQLQueryExecutor{err: &sqlExecutionError{Message: "syntax error", SQLState: "42601", Position: 8}}})
	res := performRequest(t, handler, http.MethodPost, "/api/v1/branches/main/sql/execute", `{"sql":"SELECT FROM"}`)
	if res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status %d, got %d", http.StatusUnprocessableEntity, res.Code)
	}

	assertAPIErrorCode(t, res, "sql_error")
}

func TestExecuteSQLReturnsUnavailableWhenExecutorUnavailable(t *testing.T) {
	handler := New(Config{Version: "test-version", SQLExecutor: &fakeSQLQueryExecutor{err: fmt.Errorf("%w: endpoint unavailable", ErrPrimaryEndpointUnavailable)}})
	res := performRequest(t, handler, http.MethodPost, "/api/v1/branches/main/sql/execute", `{"sql":"SELECT 1"}`)
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, res.Code)
	}

	assertAPIErrorCode(t, res, "endpoint_unavailable")
}

func TestExecuteSQLClassifiesTransactionOutcomeErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "write result truncated", err: ErrSQLWriteResultTruncated, wantStatus: http.StatusUnprocessableEntity, wantCode: "result_limit"},
		{name: "commit rolled back", err: ErrSQLCommitFailed, wantStatus: http.StatusInternalServerError, wantCode: "commit_failed"},
		{name: "commit outcome unknown", err: ErrSQLCommitOutcomeUnknown, wantStatus: http.StatusServiceUnavailable, wantCode: "commit_outcome_unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := New(Config{Version: "test-version", SQLExecutor: &fakeSQLQueryExecutor{err: tt.err}})
			res := performRequest(t, handler, http.MethodPost, "/api/v1/branches/main/sql/execute", `{"sql":"INSERT INTO app.documents(title) VALUES ('x')","allow_writes":true,"confirm_protected_writes":true}`)
			if res.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, res.Code)
			}
			assertAPIErrorCode(t, res, tt.wantCode)
		})
	}
}

func TestExecuteSQLReturnsNotFoundForUnknownBranch(t *testing.T) {
	handler := New(Config{Version: "test-version", SQLExecutor: &fakeSQLQueryExecutor{result: sqlExecutionResult{Branch: "missing"}}})
	res := performRequest(t, handler, http.MethodPost, "/api/v1/branches/missing/sql/execute", `{"sql":"SELECT 1"}`)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, res.Code)
	}

	assertAPIErrorCode(t, res, "not_found")
}

type fakeSQLQueryExecutor struct {
	result        sqlExecutionResult
	err           error
	calls         []sqlExecutionCall
	databaseList  sqlDatabaseList
	databaseErr   error
	databaseCalls []string
}

type sqlExecutionCall struct {
	branchName string
	database   string
	query      string
	readOnly   bool
}

func (f *fakeSQLQueryExecutor) Execute(_ context.Context, branchName string, database string, query string, readOnly bool) (sqlExecutionResult, error) {
	f.calls = append(f.calls, sqlExecutionCall{branchName: branchName, database: database, query: query, readOnly: readOnly})
	if f.err != nil {
		return f.result, f.err
	}

	if strings.TrimSpace(f.result.Branch) == "" {
		f.result.Branch = branchName
	}
	f.result.ReadOnly = readOnly

	return f.result, nil
}

func (f *fakeSQLQueryExecutor) Databases(_ context.Context, branchName string) (sqlDatabaseList, error) {
	f.databaseCalls = append(f.databaseCalls, branchName)
	if f.databaseErr != nil {
		return sqlDatabaseList{}, f.databaseErr
	}
	return f.databaseList, nil
}

func TestCountSQLStatementsHandlesCommentsAndQuotes(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantCount int
		wantErr   bool
	}{
		{name: "single simple", query: "SELECT 1", wantCount: 1},
		{name: "single with trailing semicolon", query: "SELECT 1;", wantCount: 1},
		{name: "two statements", query: "SELECT 1; SELECT 2", wantCount: 2},
		{name: "semicolon in string", query: "SELECT ';'", wantCount: 1},
		{name: "semicolon in comment", query: "SELECT 1 -- ;\n", wantCount: 1},
		{name: "unterminated quote", query: "SELECT 'x", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, err := countSQLStatements(tt.query)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if count != tt.wantCount {
				t.Fatalf("expected count %d, got %d", tt.wantCount, count)
			}
		})
	}
}

func TestValidateSingleStatementQuery(t *testing.T) {
	if err := validateSingleStatementQuery("SELECT 1"); err != nil {
		t.Fatalf("expected valid statement, got %v", err)
	}

	if err := validateSingleStatementQuery("SELECT 1; SELECT 2"); err == nil {
		t.Fatal("expected multi-statement validation error")
	}

	if err := validateSingleStatementQuery(""); err == nil {
		t.Fatal("expected empty query validation error")
	}
}
