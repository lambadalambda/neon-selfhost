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
