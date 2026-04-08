package server

import (
	"fmt"
	"net/http"
	"testing"
)

type healthEndpointResponse struct {
	Status string `json:"status"`
	Checks []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"checks"`
}

func TestHealthEndpointIncludesComponentChecks(t *testing.T) {
	handler := New(Config{Version: "test-version"})
	res := performRequest(t, handler, http.MethodGet, "/api/v1/health", "")

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	var payload healthEndpointResponse
	decodeJSON(t, res, &payload)

	if payload.Status != "ok" {
		t.Fatalf("expected health status %q, got %q", "ok", payload.Status)
	}

	if len(payload.Checks) < 3 {
		t.Fatalf("expected at least 3 health checks, got %d", len(payload.Checks))
	}

	found := map[string]bool{}
	for _, check := range payload.Checks {
		if check.Status != "ok" {
			t.Fatalf("expected check %q to be ok, got %q", check.Name, check.Status)
		}
		found[check.Name] = true
	}

	for _, expected := range []string{"branch_store", "operation_manager", "primary_endpoint"} {
		if !found[expected] {
			t.Fatalf("missing health check %q", expected)
		}
	}
}

func TestHealthEndpointRejectsPostMethod(t *testing.T) {
	handler := New(Config{Version: "test-version"})
	res := performRequest(t, handler, http.MethodPost, "/api/v1/health", "")

	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, res.Code)
	}
}

func TestHealthEndpointReportsDegradedWhenPrimaryEndpointUnavailable(t *testing.T) {
	handler := New(Config{
		Version: "test-version",
		PrimaryEndpoint: failingPrimaryEndpointController{
			connectionErr: fmt.Errorf("%w: docker socket unavailable", ErrPrimaryEndpointUnavailable),
		},
	})

	res := performRequest(t, handler, http.MethodGet, "/api/v1/health", "")

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	var payload healthEndpointResponse
	decodeJSON(t, res, &payload)

	if payload.Status != "degraded" {
		t.Fatalf("expected health status %q, got %q", "degraded", payload.Status)
	}
}

func TestHealthEndpointReportsDegradedWhenPrimaryEndpointStarting(t *testing.T) {
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

	res := performRequest(t, handler, http.MethodGet, "/api/v1/health", "")

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	var payload healthEndpointResponse
	decodeJSON(t, res, &payload)

	if payload.Status != "degraded" {
		t.Fatalf("expected health status %q, got %q", "degraded", payload.Status)
	}
}
