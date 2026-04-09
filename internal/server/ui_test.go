package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestRootServesConsoleUI(t *testing.T) {
	handler := New(Config{Version: "test-version"})

	res := performRequest(t, handler, http.MethodGet, "/", "")
	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	if got := res.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("expected text/html content type, got %q", got)
	}

	body := res.Body.String()
	if !strings.Contains(body, "Neon Selfhost Console") {
		t.Fatal("expected console title in UI response")
	}

	if !strings.Contains(body, "data-role=\"connection-command\"") {
		t.Fatal("expected connection command placeholder in UI response")
	}

	if !strings.Contains(body, "/api/v1/endpoints/primary/connection") {
		t.Fatal("expected UI script to call primary connection API")
	}
}

func TestRootRequiresAuthWhenBasicAuthEnabled(t *testing.T) {
	handler := New(Config{
		Version:           "test-version",
		BasicAuthUser:     "admin",
		BasicAuthPassword: "secret",
	})

	res := performRequest(t, handler, http.MethodGet, "/", "")
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, res.Code)
	}

	assertAPIErrorCode(t, res, "unauthorized")
}
