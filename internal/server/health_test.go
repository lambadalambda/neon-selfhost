package server

import (
	"errors"
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

	if len(payload.Checks) < 4 {
		t.Fatalf("expected at least 4 health checks, got %d", len(payload.Checks))
	}

	found := map[string]bool{}
	for _, check := range payload.Checks {
		if check.Status != "ok" {
			t.Fatalf("expected check %q to be ok, got %q", check.Name, check.Status)
		}
		found[check.Name] = true
	}

	for _, expected := range []string{"branch_store", "operation_manager", "operation_store", "primary_endpoint"} {
		if !found[expected] {
			t.Fatalf("missing health check %q", expected)
		}
	}
}

func TestHealthEndpointReportsDegradedWhenOperationStoreUnavailable(t *testing.T) {
	handler := New(Config{
		Version:         "test-version",
		OperationDBPath: t.TempDir(),
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

	foundStore := false
	for _, check := range payload.Checks {
		if check.Name == "operation_store" {
			foundStore = true
			if check.Status != "degraded" {
				t.Fatalf("expected operation_store to be %q, got %q", "degraded", check.Status)
			}
		}
	}

	if !foundStore {
		t.Fatal("expected operation_store health check")
	}
}

func TestHealthEndpointReportsOperationStoreLoadFailure(t *testing.T) {
	store := newFaultOperationStore()
	store.loadErr = errors.New("load failed")
	handler := New(Config{Version: "test-version", operationStore: store})
	res := performRequest(t, handler, http.MethodPost, "/api/v1/branches", `{"name":"feature-a"}`)
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected mutation refusal after load failure, got %d", res.Code)
	}
	assertOperationStoreDegraded(t, handler)
	if closer, ok := handler.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			t.Fatalf("close handler: %v", err)
		}
	}
	if store.closeCount != 1 {
		t.Fatalf("expected failed-load store to close exactly once, got %d", store.closeCount)
	}
}

func TestHealthEndpointReportsOperationStoreUpsertFailure(t *testing.T) {
	store := newFaultOperationStore()
	store.upsertErr = errors.New("upsert failed")
	handler := New(Config{Version: "test-version", operationStore: store})
	res := performRequest(t, handler, http.MethodPost, "/api/v1/branches", `{"name":"feature-a"}`)
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected mutation refusal without audit persistence, got %d", res.Code)
	}
	assertAPIErrorCode(t, res, "operation_store_unavailable")
	assertOperationStoreDegraded(t, handler)
}

func TestHealthEndpointReportsOperationStoreQueryFailure(t *testing.T) {
	store := newFaultOperationStore()
	store.queryErr = errors.New("query failed")
	handler := New(Config{Version: "test-version", operationStore: store})
	res := performRequest(t, handler, http.MethodGet, "/api/v1/operations", "")
	if res.Code != http.StatusOK {
		t.Fatalf("expected in-memory operation fallback, got %d", res.Code)
	}
	assertOperationStoreDegraded(t, handler)
}

func assertOperationStoreDegraded(t *testing.T, handler http.Handler) {
	t.Helper()
	res := performRequest(t, handler, http.MethodGet, "/api/v1/health", "")
	var payload healthEndpointResponse
	decodeJSON(t, res, &payload)
	for _, check := range payload.Checks {
		if check.Name == "operation_store" {
			if check.Status != "degraded" || payload.Status != "degraded" {
				t.Fatalf("expected degraded operation store and overall health, got %+v", payload)
			}
			return
		}
	}
	t.Fatal("operation_store health check missing")
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
