package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBasicAuthRequiresCredentials(t *testing.T) {
	handler := New(Config{
		Version:           "test-version",
		BasicAuthUser:     "admin",
		BasicAuthPassword: "secret",
	})

	res := performRequest(t, handler, http.MethodGet, "/api/v1/status", "")

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, res.Code)
	}

	if got := res.Header().Get("WWW-Authenticate"); got != `Basic realm="neon-selfhost"` {
		t.Fatalf("expected WWW-Authenticate header %q, got %q", `Basic realm="neon-selfhost"`, got)
	}

	assertAPIErrorCode(t, res, "unauthorized")
}

func TestBasicAuthRejectsInvalidCredentials(t *testing.T) {
	handler := New(Config{
		Version:           "test-version",
		BasicAuthUser:     "admin",
		BasicAuthPassword: "secret",
	})

	res := performRequestWithBasicAuth(t, handler, http.MethodGet, "/api/v1/status", "", "admin", "wrong")

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, res.Code)
	}

	assertAPIErrorCode(t, res, "unauthorized")
}

func TestBasicAuthAllowsValidCredentials(t *testing.T) {
	handler := New(Config{
		Version:           "test-version",
		BasicAuthUser:     "admin",
		BasicAuthPassword: "secret",
	})

	res := performRequestWithBasicAuth(t, handler, http.MethodGet, "/api/v1/status", "", "admin", "secret")

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}
}

func performRequestWithBasicAuth(t *testing.T, handler http.Handler, method string, path string, body string, user string, password string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.SetBasicAuth(user, password)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}
