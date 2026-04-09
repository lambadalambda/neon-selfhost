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

	if !strings.Contains(body, "data-action=\"reset-branch\"") {
		t.Fatal("expected branch reset action in UI response")
	}

	if !strings.Contains(body, "data-role=\"branch-filter\"") {
		t.Fatal("expected branch filter in UI response")
	}

	if !strings.Contains(body, "data-role=\"endpoint-list\"") {
		t.Fatal("expected published endpoint list in UI response")
	}

	if !strings.Contains(body, "data-action=\"copy-branch-dsn\"") {
		t.Fatal("expected branch dsn copy action in UI response")
	}

	if !strings.Contains(body, "Project dashboard") {
		t.Fatal("expected dashboard heading in UI response")
	}

	if !strings.Contains(body, "data-role=\"dashboard-storage\"") {
		t.Fatal("expected dashboard storage metric in UI response")
	}

	if !strings.Contains(body, "data-role=\"dashboard-branches\"") {
		t.Fatal("expected dashboard branches metric in UI response")
	}

	if !strings.Contains(body, "data-role=\"dashboard-branch-list\"") {
		t.Fatal("expected dashboard branch list in UI response")
	}

	if !strings.Contains(body, "data-role=\"page-branches\"") {
		t.Fatal("expected branches page container in UI response")
	}

	if !strings.Contains(body, "data-role=\"nav-branches\"") {
		t.Fatal("expected branches nav item in UI response")
	}

	if strings.Contains(body, "Integrations") {
		t.Fatal("did not expect integrations nav item in UI response")
	}

	if strings.Contains(body, "Settings") {
		t.Fatal("did not expect settings nav item in UI response")
	}

	if !strings.Contains(body, "data-role=\"monitoring-placeholder\"") {
		t.Fatal("expected monitoring placeholder in UI response")
	}

	if !strings.Contains(body, "data-role=\"published-count-chip\"") {
		t.Fatal("expected published endpoint count chip in UI response")
	}

	if !strings.Contains(body, "data-role=\"sidebar-branch-select\"") {
		t.Fatal("expected sidebar branch selector in UI response")
	}

	if !strings.Contains(body, "Branch overview") {
		t.Fatal("expected branch overview heading in UI response")
	}

	if !strings.Contains(body, "data-role=\"branch-overview-basic\"") {
		t.Fatal("expected branch basic info panel in UI response")
	}

	if !strings.Contains(body, "data-role=\"branch-overview-connect\"") {
		t.Fatal("expected branch connect info panel in UI response")
	}

	if !strings.Contains(body, "data-role=\"branch-overview-dsn\"") {
		t.Fatal("expected branch overview DSN field in UI response")
	}

	if !strings.Contains(body, "data-action=\"copy-overview-dsn\"") {
		t.Fatal("expected branch overview DSN copy action in UI response")
	}

	if !strings.Contains(body, "data-role=\"nav-branch-overview\"") {
		t.Fatal("expected branch overview nav item in UI response")
	}

	if !strings.Contains(body, "role=\"button\" tabindex=\"0\"") {
		t.Fatal("expected keyboard-accessible interactive nav and list actions in UI response")
	}

	if !strings.Contains(body, "data-role=\"nav-sql-editor\"") {
		t.Fatal("expected sql editor nav item in UI response")
	}

	if !strings.Contains(body, "data-role=\"page-sql-editor\"") {
		t.Fatal("expected sql editor page container in UI response")
	}

	if !strings.Contains(body, "data-role=\"sql-editor-input\"") {
		t.Fatal("expected sql editor input in UI response")
	}

	if !strings.Contains(body, "data-action=\"run-sql\"") {
		t.Fatal("expected run sql action in UI response")
	}

	if !strings.Contains(body, "data-role=\"sql-allow-writes\"") {
		t.Fatal("expected sql allow writes toggle in UI response")
	}

	if !strings.Contains(body, "data-role=\"sql-mode-indicator\"") {
		t.Fatal("expected sql mode indicator in UI response")
	}

	if !strings.Contains(body, "data-role=\"sql-history-list\"") {
		t.Fatal("expected sql history list in UI response")
	}

	if strings.Contains(body, "Restore To Timestamp") {
		t.Fatal("did not expect restore panel in UI response")
	}

	if strings.Contains(body, "Recent Operations") {
		t.Fatal("did not expect operations panel in UI response")
	}

	if strings.Contains(body, "Primary Endpoint") {
		t.Fatal("did not expect primary endpoint panel in UI response")
	}

	if strings.Contains(body, "copy-psql-command") {
		t.Fatal("did not expect primary psql copy action in UI response")
	}

	if !strings.Contains(body, "/api/v1/endpoints") {
		t.Fatal("expected UI script to call branch endpoints list API")
	}

	if !strings.Contains(body, "/connection") {
		t.Fatal("expected UI script to call per-branch connection API")
	}

	if !strings.Contains(body, "document.addEventListener('keydown', onActionKeydown)") {
		t.Fatal("expected keyboard action handler wiring in UI response")
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
