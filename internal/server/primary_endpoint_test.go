package server

import (
	"net/http"
	"testing"
)

type primaryConnectionResponse struct {
	Connection struct {
		Status         string `json:"status"`
		Ready          bool   `json:"ready"`
		RuntimeState   string `json:"runtime_state,omitempty"`
		RuntimeMessage string `json:"runtime_message,omitempty"`
		Branch         string `json:"branch"`
		Host           string `json:"host"`
		Port           int    `json:"port"`
		Database       string `json:"database"`
		User           string `json:"user"`
		Password       string `json:"password,omitempty"`
		DSN            string `json:"dsn,omitempty"`
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

	if payload.Connection.Ready {
		t.Fatal("expected stopped endpoint to report ready=false")
	}

	if payload.Connection.Branch != "main" {
		t.Fatalf("expected branch %q, got %q", "main", payload.Connection.Branch)
	}

	if payload.Connection.Password != "postgres" {
		t.Fatalf("expected password %q, got %q", "postgres", payload.Connection.Password)
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

	if !started.Connection.Ready {
		t.Fatal("expected started endpoint to report ready=true")
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

	if stopped.Connection.Ready {
		t.Fatal("expected stopped endpoint to report ready=false")
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

	if !switched.Connection.Ready {
		t.Fatal("expected switched endpoint to report ready=true")
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

func TestPrimaryConnectionReportsStartingWhenRuntimeNotReady(t *testing.T) {
	runtime := &fakePrimaryEndpointRuntime{
		running:        true,
		ready:          false,
		readySet:       true,
		runtimeState:   "running",
		runtimeMessage: "container health check is starting",
	}

	handler := New(Config{
		Version: "test-version",
		PrimaryEndpoint: newPrimaryEndpointManagerWithRuntime(runtime, primaryEndpointConnectionInfo{
			Host:     "127.0.0.1",
			Port:     5432,
			Database: "postgres",
			User:     "postgres",
		}, ""),
	})

	res := performRequest(t, handler, http.MethodGet, "/api/v1/endpoints/primary/connection", "")
	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	var payload primaryConnectionResponse
	decodeJSON(t, res, &payload)

	if payload.Connection.Status != "starting" {
		t.Fatalf("expected status %q, got %q", "starting", payload.Connection.Status)
	}

	if payload.Connection.Ready {
		t.Fatal("expected ready=false while runtime is starting")
	}

	if payload.Connection.RuntimeState != "running" {
		t.Fatalf("expected runtime_state %q, got %q", "running", payload.Connection.RuntimeState)
	}

	if payload.Connection.RuntimeMessage != "container health check is starting" {
		t.Fatalf("expected runtime_message %q, got %q", "container health check is starting", payload.Connection.RuntimeMessage)
	}

	if payload.Connection.DSN != "" {
		t.Fatalf("expected no DSN while endpoint is not ready, got %q", payload.Connection.DSN)
	}
}

func TestPrimaryConnectionReportsUnhealthyWhenRuntimeIsUnhealthy(t *testing.T) {
	runtime := &fakePrimaryEndpointRuntime{
		running:        true,
		ready:          false,
		readySet:       true,
		runtimeState:   "unhealthy",
		runtimeMessage: "container health check is unhealthy",
	}

	handler := New(Config{
		Version: "test-version",
		PrimaryEndpoint: newPrimaryEndpointManagerWithRuntime(runtime, primaryEndpointConnectionInfo{
			Host:     "127.0.0.1",
			Port:     5432,
			Database: "postgres",
			User:     "postgres",
		}, ""),
	})

	res := performRequest(t, handler, http.MethodGet, "/api/v1/endpoints/primary/connection", "")
	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	var payload primaryConnectionResponse
	decodeJSON(t, res, &payload)

	if payload.Connection.Status != "unhealthy" {
		t.Fatalf("expected status %q, got %q", "unhealthy", payload.Connection.Status)
	}

	if payload.Connection.Ready {
		t.Fatal("expected ready=false while endpoint is unhealthy")
	}

	if payload.Connection.DSN != "" {
		t.Fatalf("expected no DSN while endpoint is unhealthy, got %q", payload.Connection.DSN)
	}
}

func TestPrimaryConnectionClampsReadyToFalseWhenRuntimeIsStopped(t *testing.T) {
	runtime := &fakePrimaryEndpointRuntime{
		running:  false,
		ready:    true,
		readySet: true,
	}

	handler := New(Config{
		Version: "test-version",
		PrimaryEndpoint: newPrimaryEndpointManagerWithRuntime(runtime, primaryEndpointConnectionInfo{
			Host:     "127.0.0.1",
			Port:     5432,
			Database: "postgres",
			User:     "postgres",
		}, ""),
	})

	res := performRequest(t, handler, http.MethodGet, "/api/v1/endpoints/primary/connection", "")
	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	var payload primaryConnectionResponse
	decodeJSON(t, res, &payload)

	if payload.Connection.Status != "stopped" {
		t.Fatalf("expected status %q, got %q", "stopped", payload.Connection.Status)
	}

	if payload.Connection.Ready {
		t.Fatal("expected ready=false while endpoint is stopped")
	}
}

func TestPrimaryConnectionIncludesConfiguredPassword(t *testing.T) {
	runtime := &fakePrimaryEndpointRuntime{running: true}

	handler := New(Config{
		Version: "test-version",
		PrimaryEndpoint: newPrimaryEndpointManagerWithRuntime(runtime, primaryEndpointConnectionInfo{
			Host:     "127.0.0.1",
			Port:     55433,
			Database: "postgres",
			User:     "cloud_admin",
			Password: "super-secret",
		}, ""),
	})

	res := performRequest(t, handler, http.MethodGet, "/api/v1/endpoints/primary/connection", "")
	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	var payload primaryConnectionResponse
	decodeJSON(t, res, &payload)

	if payload.Connection.Password != "super-secret" {
		t.Fatalf("expected configured password %q, got %q", "super-secret", payload.Connection.Password)
	}
}
