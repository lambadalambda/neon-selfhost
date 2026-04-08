package server

import (
	"net/http"
	"testing"
	"time"

	"neon-selfhost/internal/branch"
)

type restoreEndpointResponse struct {
	Restore struct {
		Branch struct {
			Name    string `json:"name"`
			Parent  string `json:"parent"`
			Deleted bool   `json:"deleted"`
		} `json:"branch"`
		RequestedAt string `json:"requested_at"`
		ResolvedLSN string `json:"resolved_lsn"`
	} `json:"restore"`
}

func TestRestoreCreatesBranchFromMain(t *testing.T) {
	fixed := time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)
	store := branch.NewStoreWithClock(func() time.Time { return fixed })
	handler := New(Config{Version: "test-version", BranchStore: store})

	body := `{"name":"restore-a","source_branch":"main","timestamp":"2010-01-02T00:00:00Z"}`
	res := performRequest(t, handler, http.MethodPost, "/api/v1/restore", body)

	if res.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, res.Code)
	}

	var payload restoreEndpointResponse
	decodeJSON(t, res, &payload)

	if payload.Restore.Branch.Name != "restore-a" {
		t.Fatalf("expected restored branch name %q, got %q", "restore-a", payload.Restore.Branch.Name)
	}

	if payload.Restore.Branch.Parent != "main" {
		t.Fatalf("expected restored branch parent %q, got %q", "main", payload.Restore.Branch.Parent)
	}

	if payload.Restore.Branch.Deleted {
		t.Fatal("expected restored branch to be active")
	}

	if payload.Restore.RequestedAt != "2010-01-02T00:00:00Z" {
		t.Fatalf("expected requested_at %q, got %q", "2010-01-02T00:00:00Z", payload.Restore.RequestedAt)
	}

	if payload.Restore.ResolvedLSN == "" {
		t.Fatal("expected resolved_lsn in restore response")
	}
}

func TestRestoreRejectsInvalidTimestamp(t *testing.T) {
	handler := New(Config{Version: "test-version"})

	res := performRequest(t, handler, http.MethodPost, "/api/v1/restore", `{"name":"restore-a","timestamp":"not-a-time"}`)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.Code)
	}

	assertAPIErrorCode(t, res, "validation_error")
}

func TestRestoreRejectsFutureTimestamp(t *testing.T) {
	handler := New(Config{Version: "test-version"})

	future := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	body := `{"name":"restore-a","timestamp":"` + future + `"}`
	res := performRequest(t, handler, http.MethodPost, "/api/v1/restore", body)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.Code)
	}

	assertAPIErrorCode(t, res, "validation_error")
}

func TestRestoreRejectsTimestampOutsideSourceHistory(t *testing.T) {
	fixed := time.Date(2010, 1, 2, 0, 0, 0, 0, time.UTC)
	store := branch.NewStoreWithClock(func() time.Time { return fixed })
	handler := New(Config{Version: "test-version", BranchStore: store})

	body := `{"name":"restore-a","source_branch":"main","timestamp":"2010-01-01T00:00:00Z"}`
	res := performRequest(t, handler, http.MethodPost, "/api/v1/restore", body)

	if res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status %d, got %d", http.StatusUnprocessableEntity, res.Code)
	}

	assertAPIErrorCode(t, res, "history_unavailable")
}

func TestRestoreRejectsUnknownSourceBranch(t *testing.T) {
	handler := New(Config{Version: "test-version"})

	body := `{"name":"restore-a","source_branch":"missing","timestamp":"2010-01-01T00:00:00Z"}`
	res := performRequest(t, handler, http.MethodPost, "/api/v1/restore", body)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.Code)
	}

	assertAPIErrorCode(t, res, "validation_error")
}
