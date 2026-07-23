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
	if !strings.Contains(body, "data-role=\"sql-database-select\"") {
		t.Fatal("expected sql database selector in UI response")
	}
	if !strings.Contains(body, "/databases") {
		t.Fatal("expected UI script to call per-branch databases API")
	}
	if !strings.Contains(body, "database: selectedDatabase") {
		t.Fatal("expected SQL execution to include the selected database")
	}
	if !strings.Contains(body, "data-role=\"sql-editor-status\" aria-live=\"polite\"") {
		t.Fatal("expected SQL status updates to be announced accessibly")
	}

	if !strings.Contains(body, "Backup &amp; Restore") {
		t.Fatal("expected backup and restore navigation in UI response")
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

func TestConsolePointInTimeRestoreWorkflowFailsClosed(t *testing.T) {
	handler := New(Config{Version: "test-version"})
	res := performRequest(t, handler, http.MethodGet, "/", "")
	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	body := res.Body.String()
	for _, expected := range []string{
		`data-role="nav-restore"`,
		`data-role="page-restore"`,
		`data-action="create-restore"`,
		`data-role="restore-source"`,
		`data-role="restore-timestamp"`,
		`data-role="restore-name"`,
		`data-role="restore-name-preview"`,
		`data-role="restore-rfc3339"`,
		`data-role="restore-status" role="status" aria-live="polite"`,
		`data-role="restore-result" role="status" aria-live="polite"`,
		`aria-busy`,
		`Restore creates a new branch`,
		`/api/v1/restore`,
		`history_unavailable`,
		`outside retained history`,
		`function defaultRestoreName`,
		`data-action="open-restored-branch"`,
		`setPage('branch-overview')`,
		`await refreshSelectedBranchConnection(false)`,
		`state.restoreInFlight`,
		`aria-current`,
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("expected restore workflow marker %q", expected)
		}
	}

	start := strings.Index(body, "async function onRestoreSubmit")
	end := strings.Index(body, "async function onPanelClick")
	if start == -1 || end <= start {
		t.Fatal("expected restore submit and panel click handlers")
	}
	if strings.Contains(body[start:end], "state.selectedBranch =") {
		t.Fatal("restore submission must not change the selected branch")
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

func TestConsoleSQLDatabaseSelectionFailsClosed(t *testing.T) {
	handler := New(Config{Version: "test-version"})
	res := performRequest(t, handler, http.MethodGet, "/", "")
	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	body := res.Body.String()
	checks := []struct {
		name string
		code string
	}{
		{name: "connection request captures branch", code: "const branchName = state.selectedBranch;"},
		{name: "stale response is discarded", code: "state.connectionRequestEpoch !== requestEpoch || state.selectedBranch !== branchName"},
		{name: "connection branch is validated", code: "connection.branch !== branchName"},
		{name: "mismatched connection is cleared before loading", code: "state.selectedBranchConnection && state.selectedBranchConnection.branch !== branchName"},
		{name: "unavailable selection requires confirmation", code: "selectionRequired: true"},
		{name: "history selection cannot silently fall back", code: "entry.selectionRequired || (remembered && !names.includes(remembered))"},
		{name: "write mode reset helper", code: "function clearSQLWriteMode()"},
	}
	for _, check := range checks {
		if !strings.Contains(body, check.code) {
			t.Errorf("expected %s safety check in UI response", check.name)
		}
	}
	if calls := strings.Count(body, "clearSQLWriteMode();"); calls < 3 {
		t.Errorf("expected database transitions to clear write mode, got %d reset calls", calls)
	}
}

func TestConsoleProtectedBranchWritesRequireConfirmation(t *testing.T) {
	handler := New(Config{Version: "test-version"})
	res := performRequest(t, handler, http.MethodGet, "/", "")
	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	body := res.Body.String()
	for _, expected := range []string{
		`data-role="sql-protected-warning" role="note"`,
		`data-role="sql-confirm-protected-writes"`,
		`data-role="sql-protected-confirmation"`,
		`function isProtectedBranch(branch)`,
		`branch.name === 'main'`,
		`branch.protected === true`,
		`refs.sqlConfirmProtectedWrites.checked = false;`,
		`allowWrites && isProtectedBranch(selectedBranch) && !refs.sqlConfirmProtectedWrites.checked`,
		`confirm_protected_writes: confirmProtectedWrites`,
		`const selectedBranch = branchByName(state.selectedBranch);`,
		`const requestEpoch = state.connectionRequestEpoch + 1;`,
		`state.connectionRequestEpoch !== requestEpoch`,
		`state.refreshEpoch !== refreshEpoch`,
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("expected protected-write safety marker %q", expected)
		}
	}

	runStart := strings.Index(body, "if (action === 'run-sql')")
	if runStart == -1 {
		t.Fatal("expected SQL run action handler")
	}
	runEnd := strings.Index(body[runStart:], "if (action === 'copy-branch-dsn')")
	if runEnd == -1 {
		t.Fatal("expected SQL run action handler")
	}
	runHandler := body[runStart : runStart+runEnd]
	if strings.Contains(runHandler, "state.selectedBranch || 'main'") {
		t.Fatal("protected SQL execution must not silently fall back to main")
	}
}

func TestConsoleSQLResultTableKeepsWideResultsReadable(t *testing.T) {
	handler := New(Config{Version: "test-version"})
	res := performRequest(t, handler, http.MethodGet, "/", "")
	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	body := res.Body.String()
	for _, rule := range []string{
		"grid-template-columns: 270px minmax(0, 1fr);",
		"grid-template-columns: minmax(0, 1fr);",
		"width: max-content;",
		".sql-result-cell {",
		"min-width: 8rem;",
		"max-width: 32rem;",
		"word-break: normal;",
		`role="region" aria-label="SQL query results" tabindex="0"`,
		`<th scope="col"><span class="sql-result-cell">`,
		`<td><span class="sql-result-cell">`,
	} {
		if !strings.Contains(body, rule) {
			t.Errorf("expected SQL result layout rule %q", rule)
		}
	}
	if strings.Contains(body, ".sql-result-table {\n      width: max-content;\n      min-width: 100%;") {
		t.Error("did not expect result tables to force narrow result sets to fill the pane")
	}
}
