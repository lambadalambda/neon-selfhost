package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPageserverValidationIsFailClosedAndDoesNotRequireBasicAuth(t *testing.T) {
	const tenantID = "b3574abe9145331a4f254f6fd2168628"
	handler := New(Config{
		BasicAuthUser:              "admin",
		BasicAuthPassword:          "secret",
		PageserverValidGenerations: map[string]uint32{tenantID: 7},
	})
	req := httptest.NewRequest(http.MethodPost, "/validate", strings.NewReader(`{
		"tenants": [
			{"id": "b3574abe9145331a4f254f6fd2168628", "gen": 7},
			{"id": "b3574abe9145331a4f254f6fd2168628", "gen": 6}
		]
	}`))
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, res.Code, res.Body.String())
	}

	var payload pageserverValidateResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(payload.Tenants) != 2 {
		t.Fatalf("expected two responses for the configured tenant, got %#v", payload.Tenants)
	}
	if payload.Tenants[0].ID != tenantID || !payload.Tenants[0].Valid {
		t.Fatalf("expected matching generation to be valid, got %#v", payload.Tenants[0])
	}
	if payload.Tenants[1].ID != tenantID || payload.Tenants[1].Valid {
		t.Fatalf("expected stale generation to be invalid, got %#v", payload.Tenants[1])
	}
}

func TestPageserverValidationRetriesUnknownTenant(t *testing.T) {
	handler := New(Config{
		PageserverValidGenerations: map[string]uint32{
			"b3574abe9145331a4f254f6fd2168628": 1,
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/validate", strings.NewReader(`{
		"tenants": [{"id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "gen": 1}]
	}`))
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, res.Code)
	}
}

func TestPageserverValidationRetriesWhenUnconfigured(t *testing.T) {
	handler := New(Config{})
	req := httptest.NewRequest(http.MethodPost, "/validate", strings.NewReader(`{
		"tenants": [{"id": "b3574abe9145331a4f254f6fd2168628", "gen": 1}]
	}`))
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, res.Code)
	}
}

func TestPageserverValidationOnlyBypassesAuthForPost(t *testing.T) {
	handler := New(Config{BasicAuthUser: "admin", BasicAuthPassword: "secret"})
	req := httptest.NewRequest(http.MethodGet, "/validate", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, res.Code)
	}
}

func TestPageserverValidationRejectsInvalidJSON(t *testing.T) {
	handler := New(Config{})
	req := httptest.NewRequest(http.MethodPost, "/validate", strings.NewReader(`{"tenants":`))
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.Code)
	}
}
