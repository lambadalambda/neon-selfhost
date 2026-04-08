package server

import (
	"net/http"
	"testing"
)

type primaryConnectionResponse struct {
	Connection struct {
		Status   string `json:"status"`
		Branch   string `json:"branch"`
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Database string `json:"database"`
		User     string `json:"user"`
		DSN      string `json:"dsn,omitempty"`
	} `json:"connection"`
}

func TestPrimaryConnectionDefaultsToStoppedMain(t *testing.T) {
	handler := New(Config{Version: "test-version"})
	res := performRequest(t, handler, http.MethodGet, "/api/v1/endpoints/primary/connection", "")

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	var payload primaryConnectionResponse
	decodeJSON(t, res, &payload)

	if payload.Connection.Status != "stopped" {
		t.Fatalf("expected status %q, got %q", "stopped", payload.Connection.Status)
	}

	if payload.Connection.Branch != "main" {
		t.Fatalf("expected branch %q, got %q", "main", payload.Connection.Branch)
	}
}

func TestPrimaryEndpointStartThenStop(t *testing.T) {
	handler := New(Config{Version: "test-version"})

	startRes := performRequest(t, handler, http.MethodPost, "/api/v1/endpoints/primary/start", "")
	if startRes.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, startRes.Code)
	}

	var started primaryConnectionResponse
	decodeJSON(t, startRes, &started)
	if started.Connection.Status != "running" {
		t.Fatalf("expected status %q, got %q", "running", started.Connection.Status)
	}

	stopRes := performRequest(t, handler, http.MethodPost, "/api/v1/endpoints/primary/stop", "")
	if stopRes.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, stopRes.Code)
	}

	var stopped primaryConnectionResponse
	decodeJSON(t, stopRes, &stopped)
	if stopped.Connection.Status != "stopped" {
		t.Fatalf("expected status %q, got %q", "stopped", stopped.Connection.Status)
	}
}

func TestPrimaryEndpointSwitchChangesBranchAndStarts(t *testing.T) {
	handler := New(Config{Version: "test-version"})

	createRes := performRequest(t, handler, http.MethodPost, "/api/v1/branches", `{"name":"feature-a"}`)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, createRes.Code)
	}

	switchRes := performRequest(t, handler, http.MethodPost, "/api/v1/endpoints/primary/switch", `{"branch":"feature-a"}`)
	if switchRes.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, switchRes.Code)
	}

	var switched primaryConnectionResponse
	decodeJSON(t, switchRes, &switched)

	if switched.Connection.Branch != "feature-a" {
		t.Fatalf("expected branch %q, got %q", "feature-a", switched.Connection.Branch)
	}

	if switched.Connection.Status != "running" {
		t.Fatalf("expected status %q, got %q", "running", switched.Connection.Status)
	}

	connRes := performRequest(t, handler, http.MethodGet, "/api/v1/endpoints/primary/connection", "")
	if connRes.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, connRes.Code)
	}

	var connection primaryConnectionResponse
	decodeJSON(t, connRes, &connection)

	if connection.Connection.Branch != "feature-a" {
		t.Fatalf("expected branch %q, got %q", "feature-a", connection.Connection.Branch)
	}
}

func TestPrimaryEndpointSwitchRejectsUnknownBranch(t *testing.T) {
	handler := New(Config{Version: "test-version"})

	res := performRequest(t, handler, http.MethodPost, "/api/v1/endpoints/primary/switch", `{"branch":"missing"}`)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.Code)
	}

	assertAPIErrorCode(t, res, "validation_error")
}
