package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

type sqlExecuteResponse struct {
	Result struct {
		Branch     string `json:"branch"`
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

func TestExecuteSQLReturnsResultPayload(t *testing.T) {
	executor := &fakeSQLQueryExecutor{
		result: sqlExecutionResult{
			Branch:     "main",
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

	if !executor.calls[0].readOnly {
		t.Fatal("expected SQL execution to be read-only by default")
	}

	var payload sqlExecuteResponse
	decodeJSON(t, res, &payload)

	if payload.Result.Branch != "main" {
		t.Fatalf("expected payload branch %q, got %q", "main", payload.Result.Branch)
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
	res := performRequest(t, handler, http.MethodPost, "/api/v1/branches/main/sql/execute", `{"sql":"UPDATE app.documents SET title='x' WHERE id=1","allow_writes":true}`)
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

func TestExecuteSQLReturnsNotFoundForUnknownBranch(t *testing.T) {
	handler := New(Config{Version: "test-version", SQLExecutor: &fakeSQLQueryExecutor{result: sqlExecutionResult{Branch: "missing"}}})
	res := performRequest(t, handler, http.MethodPost, "/api/v1/branches/missing/sql/execute", `{"sql":"SELECT 1"}`)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, res.Code)
	}

	assertAPIErrorCode(t, res, "not_found")
}

type fakeSQLQueryExecutor struct {
	result sqlExecutionResult
	err    error
	calls  []sqlExecutionCall
}

type sqlExecutionCall struct {
	branchName string
	query      string
	readOnly   bool
}

func (f *fakeSQLQueryExecutor) Execute(_ context.Context, branchName string, query string, readOnly bool) (sqlExecutionResult, error) {
	f.calls = append(f.calls, sqlExecutionCall{branchName: branchName, query: query, readOnly: readOnly})
	if f.err != nil {
		return sqlExecutionResult{}, f.err
	}

	if strings.TrimSpace(f.result.Branch) == "" {
		f.result.Branch = branchName
	}
	f.result.ReadOnly = readOnly

	return f.result, nil
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
