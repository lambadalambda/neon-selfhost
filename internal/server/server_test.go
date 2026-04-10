package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatusEndpoint(t *testing.T) {
	handler := New(Config{Version: "test-version"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	contentType := res.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("expected application/json content type, got %q", contentType)
	}

	var payload struct {
		Status      string `json:"status"`
		Service     string `json:"service"`
		Version     string `json:"version"`
		Persistence struct {
			BranchStoreMode        string `json:"branch_store_mode"`
			OperationStoreMode     string `json:"operation_store_mode"`
			DBPath                 string `json:"db_path"`
			BranchSchemaVersion    int    `json:"branch_schema_version"`
			OperationSchemaVersion int    `json:"operation_schema_version"`
		} `json:"persistence"`
	}

	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if payload.Status != "ok" {
		t.Fatalf("expected status field %q, got %q", "ok", payload.Status)
	}

	if payload.Service != "controller" {
		t.Fatalf("expected service field %q, got %q", "controller", payload.Service)
	}

	if payload.Version != "test-version" {
		t.Fatalf("expected version field %q, got %q", "test-version", payload.Version)
	}

	if payload.Persistence.BranchStoreMode != "memory" {
		t.Fatalf("expected branch_store_mode %q, got %q", "memory", payload.Persistence.BranchStoreMode)
	}

	if payload.Persistence.OperationStoreMode != "in_memory" {
		t.Fatalf("expected operation_store_mode %q, got %q", "in_memory", payload.Persistence.OperationStoreMode)
	}
}

func TestStatusEndpointUsesDefaultVersion(t *testing.T) {
	handler := New(Config{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	var payload struct {
		Version string `json:"version"`
	}

	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if payload.Version != "dev" {
		t.Fatalf("expected default version %q, got %q", "dev", payload.Version)
	}
}

func TestStatusEndpointIncludesPersistenceDetails(t *testing.T) {
	handler := New(Config{
		Version:                "test-version",
		BranchStoreMode:        "sqlite",
		BranchSchemaVersion:    1,
		OperationDBPath:        filepath.Join(t.TempDir(), "controller.db"),
		LegacyOperationLogPath: filepath.Join(t.TempDir(), "operations.jsonl"),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	var payload struct {
		Persistence struct {
			BranchStoreMode        string `json:"branch_store_mode"`
			OperationStoreMode     string `json:"operation_store_mode"`
			DBPath                 string `json:"db_path"`
			BranchSchemaVersion    int    `json:"branch_schema_version"`
			OperationSchemaVersion int    `json:"operation_schema_version"`
		} `json:"persistence"`
	}

	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if payload.Persistence.BranchStoreMode != "sqlite" {
		t.Fatalf("expected branch_store_mode %q, got %q", "sqlite", payload.Persistence.BranchStoreMode)
	}

	if payload.Persistence.OperationStoreMode != "sqlite" {
		t.Fatalf("expected operation_store_mode %q, got %q", "sqlite", payload.Persistence.OperationStoreMode)
	}

	if payload.Persistence.DBPath == "" {
		t.Fatal("expected non-empty db_path")
	}

	if payload.Persistence.BranchSchemaVersion != 1 {
		t.Fatalf("expected branch schema version %d, got %d", 1, payload.Persistence.BranchSchemaVersion)
	}

	if payload.Persistence.OperationSchemaVersion != sqliteOperationSchemaVersion {
		t.Fatalf("expected operation schema version %d, got %d", sqliteOperationSchemaVersion, payload.Persistence.OperationSchemaVersion)
	}
}

func TestStatusEndpointRejectsPostMethod(t *testing.T) {
	handler := New(Config{Version: "test-version"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/status", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, res.Code)
	}
}
