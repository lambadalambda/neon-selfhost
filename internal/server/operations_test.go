package server

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

type operationsListResponse struct {
	Operations []struct {
		Type       string  `json:"type"`
		Status     string  `json:"status"`
		Message    string  `json:"message"`
		StartedAt  string  `json:"started_at"`
		FinishedAt *string `json:"finished_at,omitempty"`
	} `json:"operations"`
}

func TestOperationsEndpointStartsEmpty(t *testing.T) {
	handler := New(Config{Version: "test-version"})
	res := performRequest(t, handler, http.MethodGet, "/api/v1/operations", "")

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	var payload operationsListResponse
	decodeJSON(t, res, &payload)

	if len(payload.Operations) != 0 {
		t.Fatalf("expected 0 operations, got %d", len(payload.Operations))
	}
}

func TestOperationsEndpointIncludesMutationResults(t *testing.T) {
	handler := New(Config{Version: "test-version"})

	first := performRequest(t, handler, http.MethodPost, "/api/v1/branches", `{"name":"feature-a"}`)
	if first.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, first.Code)
	}

	second := performRequest(t, handler, http.MethodPost, "/api/v1/branches", `{"name":"feature-a"}`)
	if second.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, second.Code)
	}

	res := performRequest(t, handler, http.MethodGet, "/api/v1/operations", "")
	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	var payload operationsListResponse
	decodeJSON(t, res, &payload)

	if len(payload.Operations) != 2 {
		t.Fatalf("expected 2 operations, got %d", len(payload.Operations))
	}

	statuses := map[string]int{}
	for _, op := range payload.Operations {
		statuses[op.Status]++
		if op.Type == "" {
			t.Fatal("expected operation type")
		}
		if op.StartedAt == "" {
			t.Fatal("expected operation started_at")
		}
	}

	if statuses["succeeded"] != 1 {
		t.Fatalf("expected 1 succeeded operation, got %d", statuses["succeeded"])
	}

	if statuses["failed"] != 1 {
		t.Fatalf("expected 1 failed operation, got %d", statuses["failed"])
	}
}

func TestOperationManagerRejectsConcurrentRuns(t *testing.T) {
	manager := newOperationManager(nil, 50, nil, nil)

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		done <- manager.Run("create_branch", func() error {
			close(started)
			<-release
			return nil
		})
	}()

	<-started
	err := manager.Run("delete_branch", func() error {
		t.Fatal("expected concurrent operation to be rejected")
		return nil
	})
	if !errors.Is(err, ErrOperationInProgress) {
		t.Fatalf("expected ErrOperationInProgress, got %v", err)
	}

	close(release)
	if runErr := <-done; runErr != nil {
		t.Fatalf("first operation failed: %v", runErr)
	}

	operations := manager.List(10)
	if len(operations) != 2 {
		t.Fatalf("expected 2 operation logs, got %d", len(operations))
	}

	statuses := map[string]int{}
	for _, op := range operations {
		statuses[op.Status]++
	}

	if statuses["succeeded"] != 1 {
		t.Fatalf("expected 1 succeeded operation, got %d", statuses["succeeded"])
	}

	if statuses["rejected"] != 1 {
		t.Fatalf("expected 1 rejected operation, got %d", statuses["rejected"])
	}
}

func TestOperationManagerRecordsFailure(t *testing.T) {
	manager := newOperationManager(nil, 50, nil, nil)

	expected := errors.New("boom")
	err := manager.Run("create_branch", func() error {
		return expected
	})
	if !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}

	operations := manager.List(10)
	if len(operations) != 1 {
		t.Fatalf("expected 1 operation log, got %d", len(operations))
	}

	if operations[0].Status != "failed" {
		t.Fatalf("expected failed status, got %q", operations[0].Status)
	}

	if operations[0].Message != "boom" {
		t.Fatalf("expected failure message %q, got %q", "boom", operations[0].Message)
	}

	if operations[0].FinishedAt == nil {
		t.Fatal("expected failed operation to include finished_at")
	}
}

func TestOperationManagerPersistsAndReloadsEntries(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "controller.db")
	store, err := newSQLiteOperationStore(dbPath, "", nil)
	if err != nil {
		t.Fatalf("new sqlite operation store: %v", err)
	}
	defer store.Close()

	manager := newOperationManager(nil, 50, nil, store)

	if err := manager.Run("create_branch", func() error { return nil }); err != nil {
		t.Fatalf("run succeeded operation: %v", err)
	}

	failure := errors.New("boom")
	err = manager.Run("delete_branch", func() error { return failure })
	if !errors.Is(err, failure) {
		t.Fatalf("expected failure %v, got %v", failure, err)
	}

	reloadedStore, err := newSQLiteOperationStore(dbPath, "", nil)
	if err != nil {
		t.Fatalf("new sqlite operation store reload: %v", err)
	}
	defer reloadedStore.Close()

	reloaded := newOperationManager(nil, 50, nil, reloadedStore)
	operations := reloaded.List(10)
	if len(operations) != 2 {
		t.Fatalf("expected 2 operations after reload, got %d", len(operations))
	}

	if operations[0].Status != operationStatusSucceeded {
		t.Fatalf("expected first operation status %q, got %q", operationStatusSucceeded, operations[0].Status)
	}

	if operations[1].Status != operationStatusFailed {
		t.Fatalf("expected second operation status %q, got %q", operationStatusFailed, operations[1].Status)
	}
}

func TestOperationManagerMarksRunningOperationAsFailedAfterReload(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "controller.db")
	store, err := newSQLiteOperationStore(dbPath, "", nil)
	if err != nil {
		t.Fatalf("new sqlite operation store: %v", err)
	}
	defer store.Close()

	manager := newOperationManager(nil, 50, nil, store)

	if _, err := manager.start("create_branch"); err != nil {
		t.Fatalf("start operation: %v", err)
	}

	reloadedStore, err := newSQLiteOperationStore(dbPath, "", nil)
	if err != nil {
		t.Fatalf("new sqlite operation store reload: %v", err)
	}
	defer reloadedStore.Close()

	reloaded := newOperationManager(nil, 50, nil, reloadedStore)
	operations := reloaded.List(10)
	if len(operations) != 1 {
		t.Fatalf("expected 1 operation after reload, got %d", len(operations))
	}

	if operations[0].Status != operationStatusFailed {
		t.Fatalf("expected interrupted running operation to become %q, got %q", operationStatusFailed, operations[0].Status)
	}

	if operations[0].FinishedAt == nil {
		t.Fatal("expected interrupted running operation to have finished_at after reload")
	}
}

func TestOperationManagerSkipsCorruptLogLines(t *testing.T) {
	baseDir := t.TempDir()
	logPath := filepath.Join(baseDir, "operations.jsonl")
	dbPath := filepath.Join(baseDir, "controller.db")
	content := "{\"id\":1,\"type\":\"create_branch\",\"status\":\"succeeded\",\"started_at\":\"2026-01-01T00:00:00Z\",\"finished_at\":\"2026-01-01T00:00:01Z\"}\nnot-json\n"
	if err := os.WriteFile(logPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write operation log fixture: %v", err)
	}

	store, err := newSQLiteOperationStore(dbPath, logPath, nil)
	if err != nil {
		t.Fatalf("new sqlite operation store with legacy import: %v", err)
	}
	defer store.Close()

	manager := newOperationManager(nil, 50, nil, store)
	operations := manager.List(10)
	if len(operations) != 1 {
		t.Fatalf("expected 1 valid operation entry, got %d", len(operations))
	}

	if operations[0].ID != 1 {
		t.Fatalf("expected operation id 1, got %d", operations[0].ID)
	}
}

func TestSQLiteOperationStoreInitializesSchemaMeta(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "controller.db")
	store, err := newSQLiteOperationStore(dbPath, "", nil)
	if err != nil {
		t.Fatalf("new sqlite operation store: %v", err)
	}
	defer store.Close()

	sqlStore, ok := store.(*sqliteOperationStore)
	if !ok {
		t.Fatalf("expected sqlite operation store type, got %T", store)
	}

	var version sql.NullString
	if err := sqlStore.db.QueryRow(`SELECT value FROM schema_meta WHERE key = 'version'`).Scan(&version); err != nil {
		t.Fatalf("query schema meta version: %v", err)
	}

	if !version.Valid || version.String != "1" {
		t.Fatalf("expected schema version %q, got %+v", "1", version)
	}
}
