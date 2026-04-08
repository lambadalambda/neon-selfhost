package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		Status  string `json:"status"`
		Service string `json:"service"`
		Version string `json:"version"`
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

func TestStatusEndpointRejectsPostMethod(t *testing.T) {
	handler := New(Config{Version: "test-version"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/status", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, res.Code)
	}
}
