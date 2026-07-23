//go:build browser

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

type sqlResultLayoutMetrics struct {
	ClientWidth     float64 `json:"clientWidth"`
	ScrollWidth     float64 `json:"scrollWidth"`
	ClientHeight    float64 `json:"clientHeight"`
	ScrollHeight    float64 `json:"scrollHeight"`
	ScrollLeft      float64 `json:"scrollLeft"`
	FirstCellWidth  float64 `json:"firstCellWidth"`
	LongCellWidth   float64 `json:"longCellWidth"`
	PageScrollWidth float64 `json:"pageScrollWidth"`
	ViewportWidth   float64 `json:"viewportWidth"`
	Focused         bool    `json:"focused"`
	ActiveLabel     string  `json:"activeLabel"`
}

type restoreBrowserState struct {
	CurrentPage      string  `json:"currentPage"`
	SelectedBranch   string  `json:"selectedBranch"`
	PreviewName      string  `json:"previewName"`
	RFC3339          string  `json:"rfc3339"`
	ResultText       string  `json:"resultText"`
	StatusText       string  `json:"statusText"`
	MessageText      string  `json:"messageText"`
	Branches         string  `json:"branches"`
	OverviewName     string  `json:"overviewName"`
	OverviewDSN      string  `json:"overviewDSN"`
	OverviewPSQL     string  `json:"overviewPSQL"`
	PageScrollWidth  float64 `json:"pageScrollWidth"`
	ViewportWidth    float64 `json:"viewportWidth"`
	FormColumns      int     `json:"formColumns"`
	FieldsStacked    bool    `json:"fieldsStacked"`
	FieldWidth       float64 `json:"fieldWidth"`
	FormWidth        float64 `json:"formWidth"`
	AriaBusy         string  `json:"ariaBusy"`
	ClearDisabled    bool    `json:"clearDisabled"`
	TimestampInvalid string  `json:"timestampInvalid"`
	TimestampFocused bool    `json:"timestampFocused"`
	RestoreInFlight  bool    `json:"restoreInFlight"`
	RestoreRequests  int     `json:"restoreRequests"`
}

type protectedWriteBrowserState struct {
	SelectedBranch       string `json:"selectedBranch"`
	WarningHidden        bool   `json:"warningHidden"`
	WarningText          string `json:"warningText"`
	ConfirmationHidden   bool   `json:"confirmationHidden"`
	ConfirmationText     string `json:"confirmationText"`
	AllowWrites          bool   `json:"allowWrites"`
	Confirmed            bool   `json:"confirmed"`
	RunDisabled          bool   `json:"runDisabled"`
	Mode                 string `json:"mode"`
	RequestCount         int    `json:"requestCount"`
	LastAllowWrites      bool   `json:"lastAllowWrites"`
	LastConfirmation     bool   `json:"lastConfirmation"`
	MessageText          string `json:"messageText"`
	AllowDisabled        bool   `json:"allowDisabled"`
	ConfirmationDisabled bool   `json:"confirmationDisabled"`
	ConnectionPort       int    `json:"connectionPort"`
	RefreshInFlight      bool   `json:"refreshInFlight"`
}

type sqlLibraryBrowserState struct {
	SavedCount   int    `json:"savedCount"`
	HistoryCount int    `json:"historyCount"`
	Title        string `json:"title"`
	Query        string `json:"query"`
	Database     string `json:"database"`
	ActiveID     string `json:"activeID"`
	ListText     string `json:"listText"`
}

func TestConsoleSQLLibraryPersistsAndSupportsCRUDInBrowser(t *testing.T) {
	if _, err := exec.LookPath("agent-browser"); err != nil {
		t.Skip("agent-browser is required for SQL library browser tests")
	}

	executor := &fakeSQLQueryExecutor{
		result:       sqlExecutionResult{Branch: "main", Database: "postgres", CommandTag: "SELECT 1"},
		databaseList: sqlDatabaseList{Default: "postgres", Names: []string{"postgres", "analytics"}},
	}
	server := httptest.NewServer(New(Config{Version: "browser-test", SQLExecutor: executor}))
	defer server.Close()

	session := fmt.Sprintf("sql-library-%d", os.Getpid())
	defer closeAgentBrowser(session)
	runAgentBrowser(t, session, "open", server.URL)
	runAgentBrowser(t, session, "wait", "--fn", "state.refreshInFlight === false")

	setup := `(async () => {
  state.selectedBranch = 'main';
  state.selectedBranchConnection = {branch: 'main', published: true, status: 'running', host: '127.0.0.1', port: 56000, database: 'postgres', user: 'cloud_admin', password: 'test'};
  state.databasesByBranch.main = {names: ['postgres', 'analytics'], default: 'postgres', loading: false, error: '', selectionRequired: false};
  state.selectedDatabaseByBranch.main = 'postgres';
  setPage('sql-editor');
  renderSQLEditorContext();
  await loadSQLLibrary(false);
})()`
	runAgentBrowser(t, session, "eval", setup)
	runAgentBrowser(t, session, "eval", `(() => {
  window.originalSQLLibraryAPI = api;
  window.pendingSaveStarted = false;
  window.pendingSaveFinished = false;
  window.pendingSaveRequests = 0;
  api = async function(method, path, payload) {
    if (method === 'POST' && path === '/api/v1/sql/saved-queries') {
      window.pendingSaveRequests += 1;
      window.pendingSaveStarted = true;
      await new Promise((resolve) => { window.releasePendingSave = resolve; });
      window.pendingSaveFinished = true;
      return {query: {id: 999, ...payload}};
    }
    return window.originalSQLLibraryAPI(method, path, payload);
  };
  refs.sqlQueryTitle.value = 'Stale save';
  refs.sqlEditorInput.value = 'SELECT 0';
  document.querySelector('[data-action="save-sql"]').click();
  document.querySelector('[data-action="save-sql"]').click();
})()`)
	runAgentBrowser(t, session, "wait", "--fn", "window.pendingSaveStarted === true")
	duplicateSaveRequests := runAgentBrowser(t, session, "eval", `String(window.pendingSaveRequests) + ':' + String(document.querySelector('[data-action="save-sql"]').disabled)`)
	if strings.Trim(duplicateSaveRequests, `"`) != "1:true" {
		t.Fatalf("expected one locked save request after a double click, got %q", duplicateSaveRequests)
	}
	runAgentBrowser(t, session, "select", `[data-role="sql-database-select"]`, "analytics")
	runAgentBrowser(t, session, "eval", `window.releasePendingSave()`)
	runAgentBrowser(t, session, "wait", "--fn", "window.pendingSaveFinished === true")
	staleSave := evalSQLLibraryBrowserState(t, session)
	if staleSave.ActiveID != "" {
		t.Fatalf("expected database change to invalidate an in-flight save, got %#v", staleSave)
	}
	runAgentBrowser(t, session, "eval", `api = window.originalSQLLibraryAPI`)

	runAgentBrowser(t, session, "eval", `(() => {
  state.selectedDatabaseByBranch.main = 'postgres';
  renderSQLEditorContext();
  window.originalRefreshSQLDatabases = refreshSelectedBranchDatabases;
  window.firstOpenStarted = false;
  window.firstOpenFinished = false;
  refreshSelectedBranchDatabases = async function(silent) {
    if (!window.firstOpenStarted) {
      window.firstOpenStarted = true;
      await new Promise((resolve) => { window.releaseFirstOpen = resolve; });
      window.firstOpenFinished = true;
    }
    return window.originalRefreshSQLDatabases(silent);
  };
  window.firstOpen = openSQLLibraryEntry({id: 101, name: 'First', sql: 'SELECT 101', branch: 'main', database: 'postgres'}, true);
})()`)
	runAgentBrowser(t, session, "wait", "--fn", "window.firstOpenStarted === true")
	runAgentBrowser(t, session, "eval", `window.secondOpen = openSQLLibraryEntry({id: 102, name: 'Second', sql: 'SELECT 102', branch: 'main', database: 'analytics'}, true)`)
	runAgentBrowser(t, session, "wait", "--fn", "refs.sqlEditorInput.value === 'SELECT 102'")
	runAgentBrowser(t, session, "eval", `window.releaseFirstOpen()`)
	runAgentBrowser(t, session, "wait", "--fn", "window.firstOpenFinished === true")
	staleOpen := evalSQLLibraryBrowserState(t, session)
	if staleOpen.ActiveID != "102" || staleOpen.Database != "analytics" || staleOpen.Query != "SELECT 102" {
		t.Fatalf("expected latest open action to win, got %#v", staleOpen)
	}
	runAgentBrowser(t, session, "eval", `refreshSelectedBranchDatabases = window.originalRefreshSQLDatabases`)

	runAgentBrowser(t, session, "eval", `(() => {
  state.selectedDatabaseByBranch.main = 'postgres';
  state.activeSavedQueryID = null;
  renderSQLEditorContext();
  refs.sqlQueryTitle.value = 'Daily check';
  refs.sqlEditorInput.value = 'SELECT 1';
})()`)
	runAgentBrowser(t, session, "click", `[data-action="save-sql"]`)
	runAgentBrowser(t, session, "wait", "--fn", "state.savedQueries.length === 1")

	created := evalSQLLibraryBrowserState(t, session)
	if created.SavedCount != 1 || created.Title != "Daily check" || created.Query != "SELECT 1" || created.ActiveID == "" {
		t.Fatalf("expected controller-backed saved query, got %#v", created)
	}

	// Loading a fresh page against the same controller must recover the saved query.
	runAgentBrowser(t, session, "open", server.URL)
	runAgentBrowser(t, session, "wait", "--fn", "state.refreshInFlight === false")
	runAgentBrowser(t, session, "eval", setup)
	reloaded := evalSQLLibraryBrowserState(t, session)
	if reloaded.SavedCount != 1 || !strings.Contains(reloaded.ListText, "Daily check") {
		t.Fatalf("expected saved query after browser reload, got %#v", reloaded)
	}

	runAgentBrowser(t, session, "click", `[data-action="open-sql-history"]`)
	runAgentBrowser(t, session, "wait", "--fn", "state.activeSavedQueryID !== null")
	runAgentBrowser(t, session, "eval", `(() => {
  refs.sqlQueryTitle.value = 'Daily check updated';
  refs.sqlEditorInput.value = 'SELECT 2';
  document.querySelector('[data-action="save-sql"]').click();
})()`)
	runAgentBrowser(t, session, "wait", "--fn", "state.savedQueries.length === 1 && state.savedQueries[0].name === 'Daily check updated'")

	runAgentBrowser(t, session, "eval", `(() => {
  window.prompt = () => 'Daily check renamed';
  document.querySelector('[data-action="rename-saved-sql"]').click();
})()`)
	runAgentBrowser(t, session, "wait", "--fn", "state.savedQueries[0].name === 'Daily check renamed'")

	runAgentBrowser(t, session, "eval", `(async () => {
  await api('POST', '/api/v1/sql/saved-queries', {name: 'Analytics check', sql: 'SELECT 3', branch: 'main', database: 'analytics'});
  refs.sqlLibraryScope.value = 'project';
  refs.sqlLibraryScope.dispatchEvent(new Event('change', {bubbles: true}));
})()`)
	runAgentBrowser(t, session, "wait", "--fn", "state.sqlLibraryScope === 'project' && state.savedQueries.length === 2")
	project := evalSQLLibraryBrowserState(t, session)
	if project.SavedCount != 2 || !strings.Contains(project.ListText, "Analytics check") || !strings.Contains(project.ListText, "Daily check renamed") {
		t.Fatalf("expected project-wide saved query lookup, got %#v", project)
	}

	runAgentBrowser(t, session, "click", `[data-action="open-sql-history"][data-sql-id="2"]`)
	runAgentBrowser(t, session, "wait", "--fn", "selectedSQLDatabase('main') === 'analytics' && refs.sqlEditorInput.value === 'SELECT 3'")
	opened := evalSQLLibraryBrowserState(t, session)
	if opened.Database != "analytics" || opened.Title != "Analytics check" || opened.Query != "SELECT 3" {
		t.Fatalf("expected project result to switch into its database context, got %#v", opened)
	}

	runAgentBrowser(t, session, "eval", `(async () => {
  state.selectedDatabaseByBranch.main = 'postgres';
  state.databasesByBranch.main.selectionRequired = false;
  renderSQLEditorContext();
  refs.sqlQueryTitle.value = 'History check';
  refs.sqlEditorInput.value = 'SELECT 1';
  document.querySelector('[data-action="run-sql"]').click();
})()`)
	runAgentBrowser(t, session, "wait", "--fn", "state.sqlHistory.length === 1")
	runAgentBrowser(t, session, "click", `[data-sql-tab="history"]`)
	history := evalSQLLibraryBrowserState(t, session)
	if history.HistoryCount != 1 || !strings.Contains(history.ListText, "History check") {
		t.Fatalf("expected server-recorded execution history, got %#v", history)
	}

	runAgentBrowser(t, session, "eval", `(async () => {
  window.confirm = () => true;
  setSQLTab('saved');
  refs.sqlLibraryScope.value = 'project';
  state.sqlLibraryScope = 'project';
  await loadSQLLibrary(false);
  document.querySelector('[data-action="delete-saved-sql"]').click();
})()`)
	runAgentBrowser(t, session, "wait", "--fn", "state.savedQueries.length === 1")
	runAgentBrowser(t, session, "click", `[data-action="delete-saved-sql"]`)
	runAgentBrowser(t, session, "wait", "--fn", "state.savedQueries.length === 0")
}

func TestConsoleProtectedBranchWritesInBrowser(t *testing.T) {
	if _, err := exec.LookPath("agent-browser"); err != nil {
		t.Skip("agent-browser is required for protected write tests")
	}

	server := httptest.NewServer(New(Config{Version: "browser-test"}))
	defer server.Close()

	session := fmt.Sprintf("protected-writes-%d", os.Getpid())
	defer closeAgentBrowser(session)
	runAgentBrowser(t, session, "open", server.URL)

	evalProtectedWriteState(t, session, `(() => {
  const connection = (branch, port) => ({branch, published: true, status: 'running', host: '127.0.0.1', port, database: 'postgres', user: 'cloud_admin', password: 'test'});
  window.sqlConnections = {
    main: connection('main', 56000),
    preview: connection('preview', 56001),
    guarded: connection('guarded', 56002),
  };
  state.branches = [
    {name: 'main', parent: '', created_at: new Date().toISOString()},
    {name: 'preview', parent: 'main', protected: false, created_at: new Date().toISOString()},
    {name: 'guarded', parent: 'main', protected: true, created_at: new Date().toISOString()},
  ];
  state.endpoints = Object.values(window.sqlConnections).map((item) => ({branch: item.branch, published: true, status: 'running', host: item.host, port: item.port}));
  state.selectedBranch = 'main';
  state.selectedBranchConnection = window.sqlConnections.main;
  state.databasesByBranch = {
    main: {names: ['postgres', 'pleroma'], default: 'postgres'},
    preview: {names: ['postgres'], default: 'postgres'},
    guarded: {names: ['postgres'], default: 'postgres'},
  };
  state.selectedDatabaseByBranch = {main: 'postgres', preview: 'postgres', guarded: 'postgres'};
  window.sqlRequests = [];
  window.originalProtectedWriteAPI = api;
  api = async function(method, path, payload) {
    if (method === 'POST' && path.includes('/sql/execute')) {
      window.sqlRequests.push(payload);
      return {result: {branch: state.selectedBranch, database: payload.database, read_only: !payload.allow_writes, command_tag: 'SELECT 1', duration_ms: 1, truncated: false, limits: {max_rows: 200, max_bytes: 1048576}, columns: [], rows: [], row_count: 0}};
    }
    if (method === 'GET' && path.endsWith('/connection')) {
      const branch = decodeURIComponent(path.split('/')[4]);
      const responseConnection = window.sqlConnections[branch];
      if (window.deferConnectionResponse) {
        window.connectionRequestStarted = true;
        await new Promise((resolve) => { window.releaseConnectionResponse = resolve; });
      }
      return {connection: responseConnection};
    }
    if (method === 'GET' && path === '/api/v1/status') {
      if (window.failFullRefresh) {
        throw new Error('injected refresh failure');
      }
      if (window.deferFullRefresh) {
        window.fullRefreshStarted = true;
        await new Promise((resolve) => { window.releaseFullRefresh = resolve; });
      }
      return {version: 'browser-test'};
    }
    if (method === 'GET' && path === '/api/v1/health') {
      return {status: 'ok'};
    }
    if (method === 'GET' && path === '/api/v1/branches') {
      return {branches: state.branches.slice()};
    }
    if (method === 'GET' && path === '/api/v1/endpoints') {
      return {endpoints: state.endpoints.slice()};
    }
    return window.originalProtectedWriteAPI(method, path, payload);
  };
  window.protectedWriteSnapshot = function() {
    return {
      selectedBranch: state.selectedBranch,
      warningHidden: refs.sqlProtectedWarning.classList.contains('is-hidden'),
      warningText: refs.sqlProtectedWarning.textContent,
      confirmationHidden: refs.sqlProtectedConfirmation.classList.contains('is-hidden'),
      confirmationText: refs.sqlProtectedConfirmation.textContent,
      allowWrites: refs.sqlAllowWrites.checked,
      confirmed: refs.sqlConfirmProtectedWrites.checked,
      runDisabled: refs.sqlRunButton.disabled,
      mode: refs.sqlModeIndicator.textContent,
      requestCount: window.sqlRequests.length,
      lastAllowWrites: window.sqlRequests.length ? Boolean(window.sqlRequests[window.sqlRequests.length - 1].allow_writes) : false,
      lastConfirmation: window.sqlRequests.length ? Boolean(window.sqlRequests[window.sqlRequests.length - 1].confirm_protected_writes) : false,
      messageText: refs.message.textContent,
      allowDisabled: refs.sqlAllowWrites.disabled,
      confirmationDisabled: refs.sqlConfirmProtectedWrites.disabled,
      connectionPort: state.selectedBranchConnection ? Number(state.selectedBranchConnection.port || 0) : 0,
      refreshInFlight: state.refreshInFlight,
    };
  };
  renderBranchSelectors();
  renderBranches();
  setPage('sql-editor');
  renderSQLEditorContext();
  return protectedWriteSnapshot();
})()`)

	initial := evalProtectedWriteState(t, session, `protectedWriteSnapshot()`)
	if initial.SelectedBranch != "main" || initial.WarningHidden || initial.ConfirmationHidden == false || initial.RunDisabled || initial.AllowWrites {
		t.Fatalf("expected low-friction read-only main context with persistent warning, got %#v", initial)
	}

	runAgentBrowser(t, session, "click", `[data-action="run-sql"]`)
	runAgentBrowser(t, session, "wait", "--fn", "window.sqlRequests.length === 1")
	readOnly := evalProtectedWriteState(t, session, `protectedWriteSnapshot()`)
	if readOnly.RequestCount != 1 || readOnly.LastAllowWrites {
		t.Fatalf("expected read-only query without confirmation, got %#v", readOnly)
	}

	runAgentBrowser(t, session, "focus", `[data-role="sql-allow-writes"]`)
	runAgentBrowser(t, session, "press", "Space")
	needsConfirmation := evalProtectedWriteState(t, session, `protectedWriteSnapshot()`)
	if !needsConfirmation.AllowWrites || needsConfirmation.ConfirmationHidden || !needsConfirmation.RunDisabled || needsConfirmation.Mode != "Confirm writes" {
		t.Fatalf("expected protected writes to require a second confirmation, got %#v", needsConfirmation)
	}

	runAgentBrowser(t, session, "focus", `[data-role="sql-confirm-protected-writes"]`)
	runAgentBrowser(t, session, "press", "Space")
	confirmed := evalProtectedWriteState(t, session, `protectedWriteSnapshot()`)
	if !confirmed.Confirmed || confirmed.RunDisabled || confirmed.Mode != "Write mode" {
		t.Fatalf("expected deliberate protected-write confirmation, got %#v", confirmed)
	}
	runAgentBrowser(t, session, "click", `[data-action="run-sql"]`)
	runAgentBrowser(t, session, "wait", "--fn", "window.sqlRequests.length === 2")
	protectedWrite := evalProtectedWriteState(t, session, `protectedWriteSnapshot()`)
	if !protectedWrite.LastAllowWrites || !protectedWrite.LastConfirmation {
		t.Fatalf("expected protected write request to carry both confirmations, got %#v", protectedWrite)
	}

	bypassed := evalProtectedWriteState(t, session, `(() => {
  refs.sqlConfirmProtectedWrites.checked = false;
  refs.sqlRunButton.disabled = false;
  refs.sqlRunButton.click();
  return protectedWriteSnapshot();
})()`)
	if bypassed.RequestCount != 2 || !strings.Contains(bypassed.MessageText, "confirm protected branch writes") {
		t.Fatalf("expected Run boundary to reject a confirmation bypass, got %#v", bypassed)
	}

	runAgentBrowser(t, session, "select", `[data-role="sidebar-branch-select"]`, "preview")
	runAgentBrowser(t, session, "wait", "--fn", "state.selectedBranch === 'preview'")
	preview := evalProtectedWriteState(t, session, `(() => { setPage('sql-editor'); renderSQLEditorContext(); return protectedWriteSnapshot(); })()`)
	if preview.AllowWrites || preview.Confirmed || !preview.WarningHidden {
		t.Fatalf("expected branch change to clear write mode on an unprotected branch, got %#v", preview)
	}

	evalProtectedWriteState(t, session, `(() => {
  state.selectedBranch = 'main';
  state.selectedBranchConnection = window.sqlConnections.main;
  renderBranchSelectors();
  setPage('sql-editor');
  renderSQLEditorContext();
  refs.sqlAllowWrites.checked = true;
  refs.sqlAllowWrites.dispatchEvent(new Event('change', {bubbles: true}));
  refs.sqlConfirmProtectedWrites.checked = true;
  refs.sqlConfirmProtectedWrites.dispatchEvent(new Event('change', {bubbles: true}));
  refs.sqlDatabaseSelect.value = 'pleroma';
  refs.sqlDatabaseSelect.dispatchEvent(new Event('change', {bubbles: true}));
  return protectedWriteSnapshot();
})()`)
	databaseReset := evalProtectedWriteState(t, session, `protectedWriteSnapshot()`)
	if databaseReset.AllowWrites || databaseReset.Confirmed {
		t.Fatalf("expected database change to clear protected write mode, got %#v", databaseReset)
	}

	evalProtectedWriteState(t, session, `(() => {
  refs.sqlAllowWrites.checked = true;
  refs.sqlAllowWrites.dispatchEvent(new Event('change', {bubbles: true}));
  refs.sqlConfirmProtectedWrites.checked = true;
  refs.sqlConfirmProtectedWrites.dispatchEvent(new Event('change', {bubbles: true}));
  window.deferConnectionResponse = true;
  window.connectionRequestStarted = false;
  window.pendingConnectionRefresh = refreshSelectedBranchConnection(true);
  return protectedWriteSnapshot();
})()`)
	runAgentBrowser(t, session, "wait", "--fn", "window.connectionRequestStarted === true")
	pendingConnection := evalProtectedWriteState(t, session, `protectedWriteSnapshot()`)
	if pendingConnection.AllowWrites || pendingConnection.Confirmed || !pendingConnection.AllowDisabled || !pendingConnection.ConfirmationDisabled {
		t.Fatalf("expected pending connection refresh to lock and clear write controls, got %#v", pendingConnection)
	}

	connectionReset := evalProtectedWriteState(t, session, `(async () => {
  window.deferConnectionResponse = false;
  window.sqlConnections.main = {...window.sqlConnections.main, port: 56100, timeline_id: 'replacement'};
  await refreshSelectedBranchConnection(true);
  window.releaseConnectionResponse();
  await window.pendingConnectionRefresh;
  return protectedWriteSnapshot();
})()`)
	if connectionReset.AllowWrites || connectionReset.Confirmed || connectionReset.ConnectionPort != 56100 {
		t.Fatalf("expected connection change to clear protected write mode, got %#v", connectionReset)
	}

	evalProtectedWriteState(t, session, `(() => {
  window.deferConnectionResponse = true;
  window.connectionRequestStarted = false;
  window.pendingConnectionRefresh = refreshSelectedBranchConnection(true);
  return protectedWriteSnapshot();
})()`)
	runAgentBrowser(t, session, "wait", "--fn", "window.connectionRequestStarted === true")
	evalProtectedWriteState(t, session, `(() => {
  window.deferFullRefresh = true;
  window.fullRefreshStarted = false;
  window.pendingFullRefresh = loadAll();
  return protectedWriteSnapshot();
})()`)
	runAgentBrowser(t, session, "wait", "--fn", "window.fullRefreshStarted === true")
	staleDuringRefresh := evalProtectedWriteState(t, session, `(async () => {
  window.releaseConnectionResponse();
  await window.pendingConnectionRefresh;
  renderSQLEditorContext();
  return protectedWriteSnapshot();
})()`)
	if !staleDuringRefresh.RefreshInFlight || !staleDuringRefresh.AllowDisabled || !staleDuringRefresh.RunDisabled {
		t.Fatalf("expected stale connection completion to remain locked behind newer full refresh, got %#v", staleDuringRefresh)
	}
	evalProtectedWriteState(t, session, `(async () => {
  window.deferConnectionResponse = false;
  window.deferFullRefresh = false;
  window.releaseFullRefresh();
  await window.pendingFullRefresh;
  return protectedWriteSnapshot();
})()`)

	failedRefresh := evalProtectedWriteState(t, session, `(async () => {
  window.failFullRefresh = true;
  await loadAll();
  window.failFullRefresh = false;
  return protectedWriteSnapshot();
})()`)
	if failedRefresh.AllowWrites || failedRefresh.RunDisabled || !strings.Contains(failedRefresh.MessageText, "Refresh failed") {
		t.Fatalf("expected failed full refresh to restore safe read-only context, got %#v", failedRefresh)
	}

	guarded := evalProtectedWriteState(t, session, `(() => {
  state.selectedBranch = 'guarded';
  state.selectedBranchConnection = window.sqlConnections.guarded;
  renderBranchSelectors();
  setPage('sql-editor');
  renderSQLEditorContext();
  refs.sqlAllowWrites.checked = true;
  refs.sqlAllowWrites.dispatchEvent(new Event('change', {bubbles: true}));
  return protectedWriteSnapshot();
})()`)
	if guarded.WarningHidden || guarded.ConfirmationHidden || !guarded.RunDisabled || !strings.Contains(guarded.WarningText, "guarded is protected") {
		t.Fatalf("expected future protected metadata to use the same confirmation flow, got %#v", guarded)
	}
}

func TestConsolePointInTimeRestoreInBrowser(t *testing.T) {
	if _, err := exec.LookPath("agent-browser"); err != nil {
		t.Skip("agent-browser is required for browser restore tests")
	}

	server := httptest.NewServer(New(Config{
		Version: "browser-test",
		BranchAttachmentResolver: staticBranchAttachmentResolver{
			restore:    BranchAttachment{TenantID: "tenant-main", TimelineID: "timeline-browser-restore"},
			restoreLSN: "0/16B6F50",
		},
	}))
	defer server.Close()
	time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second)) + 10*time.Millisecond)

	session := fmt.Sprintf("restore-workflow-%d", os.Getpid())
	defer closeAgentBrowser(session)

	runAgentBrowser(t, session, "open", server.URL)
	runAgentBrowser(t, session, "set", "viewport", "1440", "900")
	runAgentBrowser(t, session, "eval", `setPage('restore')`)
	runAgentBrowser(t, session, "find", "role", "button", "click", "--name", "Restore branch")
	invalid := evalRestoreBrowserState(t, session, `(() => ({
  selectedBranch: state.selectedBranch,
  timestampInvalid: refs.restoreTimestamp.getAttribute('aria-invalid') || '',
  timestampFocused: document.activeElement === refs.restoreTimestamp,
}))()`)
	if invalid.SelectedBranch != "main" || invalid.TimestampInvalid != "true" || !invalid.TimestampFocused {
		t.Fatalf("expected invalid restore timestamp to fail closed and receive focus, got %#v", invalid)
	}

	prepared := evalRestoreBrowserState(t, session, `(() => {
  setPage('restore');
  const now = new Date();
  const local = new Date(now.getTime() - now.getTimezoneOffset() * 60000).toISOString().slice(0, 19);
  refs.restoreTimestamp.value = local;
  refs.restoreTimestamp.dispatchEvent(new Event('input', {bubbles: true}));
  return {
    currentPage: state.currentPage,
    selectedBranch: state.selectedBranch,
    previewName: refs.restoreNamePreview.textContent,
    rfc3339: refs.restoreRFC3339.textContent,
    timestampInvalid: refs.restoreTimestamp.getAttribute('aria-invalid') || '',
  };
})()`)
	if prepared.CurrentPage != "restore" || prepared.SelectedBranch != "main" || !strings.HasPrefix(prepared.PreviewName, "restore-") || !strings.Contains(prepared.RFC3339, "RFC3339:") || prepared.TimestampInvalid != "" {
		t.Fatalf("expected prepared restore form without branch mutation, got %#v", prepared)
	}

	runAgentBrowser(t, session, "eval", `(() => {
  window.originalRestoreAPI = api;
  window.restoreRequestStarted = false;
  window.restoreRequestCount = 0;
  window.releaseRestoreRequest = null;
  api = async function(method, path, payload) {
    if (method === 'POST' && path === '/api/v1/restore') {
      window.restoreRequestCount += 1;
      window.restoreRequestStarted = true;
      await new Promise((resolve) => { window.releaseRestoreRequest = resolve; });
    }
    return window.originalRestoreAPI(method, path, payload);
  };
})()`)
	runAgentBrowser(t, session, "find", "role", "button", "click", "--name", "Restore branch")
	runAgentBrowser(t, session, "wait", "--fn", "window.restoreRequestStarted === true")
	inFlight := evalRestoreBrowserState(t, session, `(() => {
  refs.restoreForm.requestSubmit();
  return {
    ariaBusy: refs.restoreForm.getAttribute('aria-busy') || '',
    clearDisabled: refs.restoreClear.disabled,
    restoreInFlight: state.restoreInFlight,
    restoreRequests: window.restoreRequestCount,
  };
})()`)
	if inFlight.AriaBusy != "true" || !inFlight.ClearDisabled || !inFlight.RestoreInFlight || inFlight.RestoreRequests != 1 {
		t.Fatalf("expected one locked in-flight restore request, got %#v", inFlight)
	}
	runAgentBrowser(t, session, "eval", `window.releaseRestoreRequest()`)
	runAgentBrowser(t, session, "wait", "--text", "Branch restored")
	runAgentBrowser(t, session, "eval", `api = window.originalRestoreAPI`)
	succeeded := evalRestoreBrowserState(t, session, `(() => ({
  currentPage: state.currentPage,
  selectedBranch: state.selectedBranch,
  resultText: refs.restoreResult.textContent,
  statusText: refs.restoreStatus.textContent,
}))()`)
	if succeeded.CurrentPage != "restore" || succeeded.SelectedBranch != "main" {
		t.Fatalf("expected successful restore to wait for explicit navigation, got %#v", succeeded)
	}
	if !strings.Contains(succeeded.ResultText, prepared.PreviewName) || !strings.Contains(succeeded.ResultText, "0/16B6F50") || !strings.Contains(succeeded.StatusText, "completed") {
		t.Fatalf("expected restore result details, got %#v", succeeded)
	}

	runAgentBrowser(t, session, "click", `[data-action="open-restored-branch"]`)
	runAgentBrowser(t, session, "wait", "--fn", "state.currentPage === 'branch-overview' && state.selectedBranch === '"+prepared.PreviewName+"'")
	opened := evalRestoreBrowserState(t, session, `(() => ({
  currentPage: state.currentPage,
  selectedBranch: state.selectedBranch,
  messageText: refs.message.textContent,
  branches: state.branches.map((item) => item.name).join(','),
  overviewName: refs.branchOverviewName.textContent,
  overviewDSN: refs.branchOverviewDSN.value,
  overviewPSQL: refs.branchOverviewPSQL.value,
}))()`)
	if opened.CurrentPage != "branch-overview" || opened.SelectedBranch != prepared.PreviewName || opened.OverviewName != prepared.PreviewName {
		t.Fatalf("expected restored branch overview to open, got %#v", opened)
	}
	if opened.OverviewDSN != "Endpoint is not published" || opened.OverviewPSQL != "Endpoint is not published" {
		t.Fatalf("expected restored branch connection helpers to refresh, got %#v", opened)
	}

	runAgentBrowser(t, session, "eval", `(() => {
  setPage('restore');
  refs.restoreSource.value = 'main';
  refs.restoreTimestamp.value = '2000-01-01T00:00:00';
  refs.restoreTimestamp.dispatchEvent(new Event('input', {bubbles: true}));
  refs.restoreName.value = 'unavailable-history';
  refs.restoreName.dispatchEvent(new Event('input', {bubbles: true}));
})()`)
	runAgentBrowser(t, session, "find", "role", "button", "click", "--name", "Restore branch")
	runAgentBrowser(t, session, "wait", "--text", "outside retained history")
	failed := evalRestoreBrowserState(t, session, `(() => ({
  currentPage: state.currentPage,
  selectedBranch: state.selectedBranch,
  statusText: refs.restoreStatus.textContent,
}))()`)
	if failed.CurrentPage != "restore" || failed.SelectedBranch != prepared.PreviewName || !strings.Contains(failed.StatusText, "Pick a later timestamp") {
		t.Fatalf("expected history failure to preserve active branch and guide recovery, got %#v", failed)
	}

	runAgentBrowser(t, session, "set", "viewport", "390", "844")
	mobile := evalRestoreBrowserState(t, session, `(() => ({
  pageScrollWidth: document.documentElement.scrollWidth,
  viewportWidth: innerWidth,
  formColumns: getComputedStyle(refs.restoreForm).gridTemplateColumns.trim().split(/\s+/).length,
  fieldsStacked: refs.restoreTimestamp.closest('.restore-field').getBoundingClientRect().top > refs.restoreSource.closest('.restore-field').getBoundingClientRect().bottom,
  fieldWidth: refs.restoreSource.closest('.restore-field').getBoundingClientRect().width,
  formWidth: refs.restoreForm.getBoundingClientRect().width,
}))()`)
	if mobile.PageScrollWidth != mobile.ViewportWidth || mobile.FormColumns != 1 || !mobile.FieldsStacked || mobile.FormWidth-mobile.FieldWidth > 1 {
		t.Fatalf("expected contained single-column mobile restore form, got %#v", mobile)
	}
}

func TestConsoleSQLResultLayoutInBrowser(t *testing.T) {
	if _, err := exec.LookPath("agent-browser"); err != nil {
		t.Skip("agent-browser is required for browser layout tests")
	}

	server := httptest.NewServer(New(Config{Version: "browser-test"}))
	defer server.Close()

	session := fmt.Sprintf("sql-result-layout-%d", os.Getpid())
	defer closeAgentBrowser(session)

	runAgentBrowser(t, session, "open", server.URL)
	runAgentBrowser(t, session, "set", "viewport", "1440", "900")

	wide := evalSQLResultLayout(t, session, `(() => {
  setPage('sql-editor');
  renderSQLResultSuccess({
    command_tag: 'SELECT 1',
    duration_ms: 12,
    row_count: 50,
    columns: Array.from({length: 24}, (_, i) => ({name: 'column_' + String(i + 1).padStart(2, '0') + '_identifier', type: 'text'})),
    rows: Array.from({length: 50}, (_, row) => Array.from({length: 24}, (_, column) => 'value_' + row + '_' + column)),
  });
  const wrap = document.querySelector('.sql-result-table-wrap');
  const first = document.querySelector('.sql-result-cell');
  wrap.scrollLeft = 500;
  return {
    clientWidth: wrap.clientWidth,
    scrollWidth: wrap.scrollWidth,
    clientHeight: wrap.clientHeight,
    scrollHeight: wrap.scrollHeight,
    scrollLeft: wrap.scrollLeft,
    firstCellWidth: first.getBoundingClientRect().width,
    pageScrollWidth: document.documentElement.scrollWidth,
    viewportWidth: innerWidth,
  };
	})()`)
	assertSQLResultOverflow(t, wide)

	runAgentBrowser(t, session, "eval", `(() => {
  const wrap = document.querySelector('.sql-result-table-wrap');
  wrap.scrollLeft = 0;
  wrap.focus();
	})()`)
	runAgentBrowser(t, session, "press", "ArrowRight")
	runAgentBrowser(t, session, "wait", "--fn", "document.querySelector('.sql-result-table-wrap').scrollLeft > 0")
	keyboard := evalSQLResultLayout(t, session, `(() => {
  const wrap = document.querySelector('.sql-result-table-wrap');
  return {
    scrollLeft: wrap.scrollLeft,
    focused: document.activeElement === wrap,
    activeLabel: document.activeElement.getAttribute('aria-label') || '',
  };
})()`)
	if !keyboard.Focused || keyboard.ActiveLabel != "SQL query results" || keyboard.ScrollLeft <= 0 {
		t.Fatalf("expected keyboard scrolling in the named result region, got %#v", keyboard)
	}

	longValue := evalSQLResultLayout(t, session, `(() => {
  renderSQLResultSuccess({
    command_tag: 'SELECT 1',
    duration_ms: 12,
    row_count: 1,
    columns: [{name: 'payload', type: 'text'}],
    rows: [[('long_value_').repeat(200)]],
  });
  const wrap = document.querySelector('.sql-result-table-wrap');
  const cell = document.querySelector('.sql-result-table td .sql-result-cell');
  return {
    longCellWidth: cell.getBoundingClientRect().width,
    pageScrollWidth: document.documentElement.scrollWidth,
    viewportWidth: innerWidth,
  };
})()`)
	if longValue.LongCellWidth > 512.5 {
		t.Fatalf("expected long result value to stay within 32rem, got %.1fpx", longValue.LongCellWidth)
	}
	if longValue.PageScrollWidth != longValue.ViewportWidth {
		t.Fatalf("expected long value to remain page-contained, page %.1fpx viewport %.1fpx", longValue.PageScrollWidth, longValue.ViewportWidth)
	}

	runAgentBrowser(t, session, "set", "viewport", "390", "844")
	mobile := evalSQLResultLayout(t, session, `(() => {
  renderSQLResultSuccess({
    command_tag: 'SELECT 1',
    duration_ms: 12,
    row_count: 1,
    columns: Array.from({length: 24}, (_, i) => ({name: 'column_' + String(i + 1).padStart(2, '0') + '_identifier', type: 'text'})),
    rows: [Array.from({length: 24}, (_, i) => 'value_' + i)],
  });
  const wrap = document.querySelector('.sql-result-table-wrap');
  wrap.scrollLeft = 200;
  return {
    clientWidth: wrap.clientWidth,
    scrollWidth: wrap.scrollWidth,
    scrollLeft: wrap.scrollLeft,
    firstCellWidth: document.querySelector('.sql-result-cell').getBoundingClientRect().width,
    pageScrollWidth: document.documentElement.scrollWidth,
    viewportWidth: innerWidth,
  };
})()`)
	if mobile.PageScrollWidth != mobile.ViewportWidth {
		t.Fatalf("expected mobile page containment, page %.1fpx viewport %.1fpx", mobile.PageScrollWidth, mobile.ViewportWidth)
	}
	if mobile.ScrollWidth <= mobile.ClientWidth || mobile.ScrollLeft <= 0 {
		t.Fatalf("expected mobile results to scroll horizontally, got %#v", mobile)
	}
}

func assertSQLResultOverflow(t *testing.T, metrics sqlResultLayoutMetrics) {
	t.Helper()
	if metrics.PageScrollWidth != metrics.ViewportWidth {
		t.Fatalf("expected page containment, page %.1fpx viewport %.1fpx", metrics.PageScrollWidth, metrics.ViewportWidth)
	}
	if metrics.ScrollWidth <= metrics.ClientWidth || metrics.ScrollLeft <= 0 {
		t.Fatalf("expected horizontal result scrolling, got %#v", metrics)
	}
	if metrics.ScrollHeight <= metrics.ClientHeight {
		t.Fatalf("expected vertical result scrolling, got %#v", metrics)
	}
	if metrics.FirstCellWidth < 128 {
		t.Fatalf("expected readable result columns of at least 8rem, got %.1fpx", metrics.FirstCellWidth)
	}
}

func evalSQLResultLayout(t *testing.T, session string, script string) sqlResultLayoutMetrics {
	t.Helper()
	output := runAgentBrowser(t, session, "eval", script)
	var metrics sqlResultLayoutMetrics
	if err := json.Unmarshal([]byte(output), &metrics); err != nil {
		t.Fatalf("decode browser metrics %q: %v", output, err)
	}
	return metrics
}

func evalRestoreBrowserState(t *testing.T, session string, script string) restoreBrowserState {
	t.Helper()
	output := runAgentBrowser(t, session, "eval", script)
	var state restoreBrowserState
	if err := json.Unmarshal([]byte(output), &state); err != nil {
		t.Fatalf("decode restore browser state %q: %v", output, err)
	}
	return state
}

func evalProtectedWriteState(t *testing.T, session string, script string) protectedWriteBrowserState {
	t.Helper()
	output := runAgentBrowser(t, session, "eval", script)
	var state protectedWriteBrowserState
	if err := json.Unmarshal([]byte(output), &state); err != nil {
		t.Fatalf("decode protected write browser state %q: %v", output, err)
	}
	return state
}

func evalSQLLibraryBrowserState(t *testing.T, session string) sqlLibraryBrowserState {
	t.Helper()
	output := runAgentBrowser(t, session, "eval", `(() => ({
  savedCount: state.savedQueries.length,
  historyCount: state.sqlHistory.length,
  title: refs.sqlQueryTitle.value,
  query: refs.sqlEditorInput.value,
  database: selectedSQLDatabase(state.selectedBranch),
  activeID: state.activeSavedQueryID === null ? '' : String(state.activeSavedQueryID),
  listText: refs.sqlHistoryList.textContent,
}))()`)
	var state sqlLibraryBrowserState
	if err := json.Unmarshal([]byte(output), &state); err != nil {
		t.Fatalf("decode SQL library browser state %q: %v", output, err)
	}
	return state
}

func runAgentBrowser(t *testing.T, session string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	commandArgs := append([]string{"--session", session}, args...)
	output, err := exec.CommandContext(ctx, "agent-browser", commandArgs...).CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			t.Fatalf("agent-browser %s timed out: %v", strings.Join(args, " "), ctx.Err())
		}
		t.Fatalf("agent-browser %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func closeAgentBrowser(session string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, "agent-browser", "--session", session, "close").Run()
}
