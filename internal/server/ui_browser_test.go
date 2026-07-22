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
