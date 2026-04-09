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

	if !strings.Contains(body, "data-role=\"connection-dsn\"") {
		t.Fatal("expected DSN placeholder in UI response")
	}

	if !strings.Contains(body, "data-role=\"connection-env\"") {
		t.Fatal("expected env snippet placeholder in UI response")
	}

	if !strings.Contains(body, "data-role=\"connection-password\"") {
		t.Fatal("expected password placeholder in UI response")
	}

	if !strings.Contains(body, "data-action=\"copy-psql-command\"") {
		t.Fatal("expected psql copy action in UI response")
	}

	if !strings.Contains(body, "data-action=\"copy-dsn\"") {
		t.Fatal("expected dsn copy action in UI response")
	}

	if !strings.Contains(body, "data-action=\"copy-env-snippet\"") {
		t.Fatal("expected env snippet copy action in UI response")
	}

	if !strings.Contains(body, "data-action=\"copy-password\"") {
		t.Fatal("expected password copy action in UI response")
	}

	if !strings.Contains(body, "data-action=\"reset-branch\"") {
		t.Fatal("expected branch reset action in UI response")
	}

	if !strings.Contains(body, "data-role=\"branch-filter\"") {
		t.Fatal("expected branch filter in UI response")
	}

	if !strings.Contains(body, "data-role=\"endpoint-list\"") {
		t.Fatal("expected published endpoint list in UI response")
	}

	if !strings.Contains(body, "data-action=\"publish-branch-endpoint\"") {
		t.Fatal("expected publish endpoint action in UI response")
	}

	if !strings.Contains(body, "data-action=\"unpublish-branch-endpoint\"") {
		t.Fatal("expected unpublish endpoint action in UI response")
	}

	if !strings.Contains(body, "data-action=\"copy-branch-dsn\"") {
		t.Fatal("expected branch dsn copy action in UI response")
	}

	if !strings.Contains(body, "DATABASE_URL=") {
		t.Fatal("expected env snippet label in UI response")
	}

	if !strings.Contains(body, "/api/v1/endpoints/primary/connection") {
		t.Fatal("expected UI script to call primary connection API")
	}

	if !strings.Contains(body, "/api/v1/endpoints") {
		t.Fatal("expected UI script to call branch endpoints list API")
	}

	if !strings.Contains(body, "/connection") {
		t.Fatal("expected UI script to call per-branch connection API")
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
