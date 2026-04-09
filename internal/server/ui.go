package server

import (
	"html"
	"net/http"
	"strings"
)

const consoleHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Neon Selfhost Console</title>
  <style>
    @import url("https://fonts.googleapis.com/css2?family=Manrope:wght@500;600;700&family=JetBrains+Mono:wght@400;500&display=swap");

    :root {
      --bg: #f4f5f7;
      --sidebar: #eff1f4;
      --surface: #ffffff;
      --surface-soft: #f8f9fb;
      --line: #d8dce3;
      --ink: #1d2128;
      --muted: #626b79;
      --ok: #178f58;
      --warn: #b86a1b;
      --danger: #ba3a35;
      --radius: 12px;
      --radius-sm: 9px;
      --shadow: 0 14px 28px rgba(18, 26, 36, 0.08);
    }

    * {
      box-sizing: border-box;
    }

    body {
      margin: 0;
      background: var(--bg);
      color: var(--ink);
      font-family: "Manrope", "Avenir Next", "Segoe UI", sans-serif;
      line-height: 1.45;
    }

    .app {
      min-height: 100vh;
      display: grid;
      grid-template-columns: 250px 1fr;
    }

    .sidebar {
      background: var(--sidebar);
      border-right: 1px solid var(--line);
      padding: 18px 14px;
      display: grid;
      align-content: start;
      gap: 18px;
    }

    .brand {
      display: grid;
      gap: 4px;
      padding: 2px 6px;
    }

    .brand strong {
      font-size: 1.02rem;
      letter-spacing: 0.01em;
    }

    .brand small {
      color: var(--muted);
      font-size: 0.81rem;
    }

    .nav-section {
      display: grid;
      gap: 8px;
    }

    .nav-section h2 {
      margin: 0;
      color: var(--muted);
      font-size: 0.75rem;
      text-transform: uppercase;
      letter-spacing: 0.08em;
      padding: 0 6px;
    }

    .nav-list {
      list-style: none;
      margin: 0;
      padding: 0;
      display: grid;
      gap: 4px;
    }

    .nav-list li {
      padding: 9px 10px;
      border-radius: 8px;
      border: 1px solid transparent;
      color: #2f3744;
      font-weight: 600;
      font-size: 0.92rem;
    }

    .nav-list li.active {
      background: #ffffff;
      border-color: var(--line);
      box-shadow: 0 1px 0 rgba(0, 0, 0, 0.02);
    }

    .branch-chip {
      padding: 9px 10px;
      border: 1px solid var(--line);
      background: #fff;
      border-radius: 8px;
      font-weight: 600;
      display: flex;
      justify-content: space-between;
      gap: 8px;
      align-items: center;
    }

    .workspace {
      padding: 24px;
      display: grid;
      gap: 16px;
      align-content: start;
    }

    .topbar {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      align-items: flex-start;
      flex-wrap: wrap;
    }

    .title-stack {
      display: grid;
      gap: 4px;
    }

    .title-stack h1 {
      margin: 0;
      font-size: clamp(1.35rem, 2.3vw, 2.1rem);
      letter-spacing: 0.01em;
    }

    .title-stack p {
      margin: 0;
      color: var(--muted);
      font-size: 0.93rem;
    }

    .top-actions {
      display: flex;
      gap: 8px;
      flex-wrap: wrap;
      align-items: center;
    }

    .pill {
      padding: 7px 11px;
      border-radius: 999px;
      border: 1px solid var(--line);
      background: #fff;
      color: var(--muted);
      font-size: 0.83rem;
      font-weight: 700;
    }

    .pill.ok {
      color: var(--ok);
      border-color: rgba(23, 143, 88, 0.28);
      background: rgba(23, 143, 88, 0.08);
    }

    .pill.warn {
      color: var(--warn);
      border-color: rgba(184, 106, 27, 0.28);
      background: rgba(184, 106, 27, 0.08);
    }

    .pill.bad {
      color: var(--danger);
      border-color: rgba(186, 58, 53, 0.28);
      background: rgba(186, 58, 53, 0.08);
    }

    .stats {
      display: grid;
      grid-template-columns: repeat(4, minmax(0, 1fr));
      gap: 10px;
    }

    .stat-card {
      border: 1px solid var(--line);
      border-radius: var(--radius-sm);
      background: var(--surface);
      box-shadow: 0 3px 10px rgba(18, 26, 36, 0.03);
      padding: 11px 12px;
      display: grid;
      gap: 4px;
    }

    .stat-card label {
      color: var(--muted);
      font-size: 0.78rem;
      text-transform: uppercase;
      letter-spacing: 0.06em;
    }

    .stat-card strong {
      font-size: 1.02rem;
    }

    .grid {
      display: grid;
      gap: 12px;
    }

    .grid.two {
      grid-template-columns: 1.5fr 1fr;
    }

    .panel {
      border: 1px solid var(--line);
      border-radius: var(--radius);
      background: var(--surface);
      box-shadow: var(--shadow);
      overflow: hidden;
    }

    .panel-header {
      display: flex;
      justify-content: space-between;
      gap: 10px;
      align-items: baseline;
      padding: 12px 14px;
      border-bottom: 1px solid var(--line);
      background: linear-gradient(180deg, #ffffff, #f9fafb);
    }

    .panel-header h2 {
      margin: 0;
      font-size: 1.02rem;
    }

    .panel-header p {
      margin: 0;
      color: var(--muted);
      font-size: 0.82rem;
    }

    .panel-body {
      padding: 12px 14px 14px;
      display: grid;
      gap: 10px;
    }

    .toolbar,
    form {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      align-items: center;
    }

    input,
    select,
    button {
      border-radius: 9px;
      border: 1px solid var(--line);
      font: inherit;
      color: inherit;
      background: #fff;
      padding: 9px 10px;
    }

    input,
    select {
      min-width: 0;
      flex: 1 1 170px;
    }

    button {
      cursor: pointer;
      font-weight: 700;
      transition: box-shadow 120ms ease, transform 120ms ease;
    }

    button:hover {
      transform: translateY(-1px);
      box-shadow: 0 6px 12px rgba(26, 34, 47, 0.12);
    }

    button:disabled {
      cursor: not-allowed;
      opacity: 0.55;
      transform: none;
      box-shadow: none;
    }

    .btn-primary {
      background: #1b1f27;
      color: #fff;
      border-color: #1b1f27;
    }

    .btn-ghost {
      background: #f7f8fa;
      color: #2f3744;
    }

    .btn-warn {
      background: #fff8ef;
      color: #8d4f16;
      border-color: rgba(184, 106, 27, 0.3);
    }

    .btn-danger {
      background: #fff5f4;
      color: #962d2a;
      border-color: rgba(186, 58, 53, 0.3);
    }

    .table-scroll {
      overflow-x: auto;
      border: 1px solid var(--line);
      border-radius: 10px;
    }

    .table-head,
    .table-row {
      min-width: 840px;
      display: grid;
      grid-template-columns: 1.2fr .8fr .9fr 1.2fr 1.5fr;
      gap: 8px;
      align-items: center;
      padding: 10px 12px;
    }

    .table-head {
      background: #f8f9fb;
      border-bottom: 1px solid var(--line);
      font-size: 0.76rem;
      color: var(--muted);
      text-transform: uppercase;
      letter-spacing: 0.06em;
      font-weight: 700;
    }

    .table-row {
      border-bottom: 1px solid var(--line);
      font-size: 0.88rem;
      background: #fff;
    }

    .table-row:last-child {
      border-bottom: 0;
    }

    .cell-strong {
      font-weight: 700;
    }

    .mono {
      font-family: "JetBrains Mono", "SF Mono", "Menlo", monospace;
    }

    .row-actions {
      display: flex;
      justify-content: flex-end;
      gap: 6px;
      flex-wrap: wrap;
    }

    .badge {
      display: inline-flex;
      align-items: center;
      gap: 5px;
      padding: 4px 8px;
      border-radius: 999px;
      font-size: 0.76rem;
      font-weight: 700;
      border: 1px solid var(--line);
      color: var(--muted);
      background: #f7f8fa;
      width: fit-content;
    }

    .badge.ok {
      color: var(--ok);
      border-color: rgba(23, 143, 88, 0.3);
      background: rgba(23, 143, 88, 0.1);
    }

    .badge.warn {
      color: var(--warn);
      border-color: rgba(184, 106, 27, 0.3);
      background: rgba(184, 106, 27, 0.1);
    }

    .badge.bad {
      color: var(--danger);
      border-color: rgba(186, 58, 53, 0.3);
      background: rgba(186, 58, 53, 0.1);
    }

    .badge.muted {
      color: var(--muted);
    }

    .connect-stack {
      display: grid;
      gap: 8px;
    }

    .cmd-row {
      display: grid;
      grid-template-columns: auto 1fr auto;
      gap: 8px;
      align-items: center;
    }

    .cmd-label {
      padding: 9px 10px;
      border-radius: 8px;
      border: 1px solid var(--line);
      background: #f8f9fb;
      font-size: 0.76rem;
      text-transform: uppercase;
      letter-spacing: 0.07em;
      color: var(--muted);
      font-weight: 700;
      min-width: 76px;
      text-align: center;
    }

    .cmd {
      width: 100%;
      border: 1px solid var(--line);
      border-radius: 8px;
      padding: 10px;
      font-size: 0.83rem;
      background: #f9fafc;
      color: #2d3442;
      font-family: "JetBrains Mono", "SF Mono", "Menlo", monospace;
    }

    .endpoint-list {
      list-style: none;
      margin: 0;
      padding: 0;
      display: grid;
      gap: 8px;
      max-height: 360px;
      overflow: auto;
    }

    .endpoint-item {
      border: 1px solid var(--line);
      border-radius: 10px;
      background: var(--surface-soft);
      padding: 10px;
      display: grid;
      gap: 8px;
    }

    .endpoint-top {
      display: flex;
      justify-content: space-between;
      gap: 8px;
      flex-wrap: wrap;
      align-items: center;
    }

    .endpoint-top strong {
      font-size: 0.95rem;
    }

    .endpoint-meta {
      color: var(--muted);
      font-size: 0.81rem;
    }

    .endpoint-actions {
      display: flex;
      gap: 6px;
      flex-wrap: wrap;
    }

    .operations {
      list-style: none;
      margin: 0;
      padding: 0;
      display: grid;
      gap: 8px;
      max-height: 220px;
      overflow: auto;
    }

    .operations li {
      border: 1px solid var(--line);
      border-radius: 9px;
      background: var(--surface-soft);
      padding: 8px 10px;
      font-size: 0.83rem;
    }

    .message {
      min-height: 22px;
      margin: 0;
      font-size: 0.87rem;
      font-weight: 700;
      color: var(--muted);
    }

    .message.ok {
      color: var(--ok);
    }

    .message.err {
      color: var(--danger);
    }

    .footer {
      margin-top: 2px;
      color: var(--muted);
      font-size: 0.82rem;
      display: flex;
      justify-content: space-between;
      gap: 8px;
      flex-wrap: wrap;
    }

    @media (max-width: 1200px) {
      .stats {
        grid-template-columns: repeat(2, minmax(0, 1fr));
      }
    }

    @media (max-width: 1020px) {
      .app {
        grid-template-columns: 1fr;
      }

      .sidebar {
        border-right: 0;
        border-bottom: 1px solid var(--line);
      }

      .grid.two {
        grid-template-columns: 1fr;
      }

      .cmd-row {
        grid-template-columns: 1fr;
      }

      .cmd-label {
        width: fit-content;
      }
    }
  </style>
</head>
<body>
  <div class="app">
    <aside class="sidebar">
      <div class="brand">
        <strong>Neon Selfhost Console</strong>
        <small>single project operator view</small>
      </div>

      <section class="nav-section">
        <h2>Project</h2>
        <ul class="nav-list">
          <li>Dashboard</li>
          <li class="active">Branches</li>
          <li>Restore</li>
          <li>Operations</li>
        </ul>
      </section>

      <section class="nav-section">
        <h2>Current Branch</h2>
        <div class="branch-chip">
          <span data-role="active-branch">main</span>
          <span class="mono" data-role="runtime-state">unknown</span>
        </div>
      </section>
    </aside>

    <main class="workspace">
      <header class="topbar">
        <div class="title-stack">
          <h1>Branches & Computes</h1>
          <p>Publish branch endpoints, connect directly, and keep primary workflow controls in one place.</p>
        </div>
        <div class="top-actions">
          <span class="pill" data-role="health-pill">Health: checking...</span>
          <button class="btn-ghost" data-action="refresh">Refresh</button>
        </div>
      </header>

      <section class="stats">
        <div class="stat-card">
          <label>Branches</label>
          <strong data-role="stat-branches">0</strong>
        </div>
        <div class="stat-card">
          <label>Published Endpoints</label>
          <strong data-role="stat-endpoints">0</strong>
        </div>
        <div class="stat-card">
          <label>Primary Status</label>
          <strong data-role="stat-primary">unknown</strong>
        </div>
        <div class="stat-card">
          <label>Recent Operations</label>
          <strong data-role="stat-operations">0</strong>
        </div>
      </section>

      <section class="grid two">
        <article class="panel">
          <header class="panel-header">
            <h2>Branch Directory</h2>
            <p>switch, publish, connect, reset, and delete</p>
          </header>
          <div class="panel-body">
            <form data-action="create-branch">
              <input name="name" placeholder="new-branch-name" required>
              <select name="parent" data-role="parent-select"></select>
              <button class="btn-primary" type="submit">New Branch</button>
            </form>

            <div class="toolbar">
              <input data-role="branch-filter" placeholder="Search branches by name or parent">
            </div>

            <div class="table-scroll">
              <div class="table-head">
                <div>Branch</div>
                <div>Parent</div>
                <div>Primary</div>
                <div>Endpoint</div>
                <div style="text-align:right;">Actions</div>
              </div>
              <div data-role="branch-list"></div>
            </div>
          </div>
        </article>

        <article class="panel">
          <header class="panel-header">
            <h2>Primary Endpoint</h2>
            <p data-role="endpoint-note">Loading primary connection...</p>
          </header>
          <div class="panel-body">
            <div class="toolbar">
              <span class="badge muted" data-role="endpoint-status">unknown</span>
              <span class="mono" data-role="endpoint-address">127.0.0.1:55433</span>
            </div>

            <div class="connect-stack">
              <div class="cmd-row">
                <span class="cmd-label mono">psql</span>
                <input class="cmd" data-role="connection-command" readonly value="Loading psql command...">
                <button class="btn-ghost" data-action="copy-psql-command">Copy psql</button>
              </div>

              <div class="cmd-row">
                <span class="cmd-label mono">DSN</span>
                <input class="cmd" data-role="connection-dsn" readonly value="Loading DSN...">
                <button class="btn-ghost" data-action="copy-dsn">Copy DSN</button>
              </div>

              <div class="cmd-row">
                <span class="cmd-label mono">Password</span>
                <input class="cmd" data-role="connection-password" readonly value="Loading password...">
                <button class="btn-ghost" data-action="copy-password">Copy password</button>
              </div>

              <div class="cmd-row">
                <span class="cmd-label mono">.env</span>
                <input class="cmd" data-role="connection-env" readonly value="DATABASE_URL=Loading...">
                <button class="btn-ghost" data-action="copy-env-snippet">Copy .env</button>
              </div>
            </div>

            <div class="toolbar">
              <button class="btn-primary" data-action="endpoint-start">Start</button>
              <button class="btn-warn" data-action="endpoint-stop">Stop</button>
            </div>
          </div>
        </article>
      </section>

      <section class="grid two">
        <article class="panel">
          <header class="panel-header">
            <h2>Published Endpoints</h2>
            <p>direct branch connections without primary switching</p>
          </header>
          <div class="panel-body">
            <ul class="endpoint-list" data-role="endpoint-list"></ul>
          </div>
        </article>

        <article class="panel">
          <header class="panel-header">
            <h2>Restore To Timestamp</h2>
            <p>create branch from source timeline history</p>
          </header>
          <div class="panel-body">
            <form data-action="restore-branch">
              <select name="source" data-role="restore-source"></select>
              <input type="datetime-local" name="timestamp" required>
              <input name="name" placeholder="optional restore branch name">
              <button class="btn-primary" type="submit">Restore Branch</button>
            </form>
          </div>
        </article>
      </section>

      <article class="panel">
        <header class="panel-header">
          <h2>Recent Operations</h2>
          <p>latest controller operation log entries</p>
        </header>
        <div class="panel-body">
          <ul class="operations" data-role="operations"></ul>
        </div>
      </article>

      <p class="message" data-role="message"></p>

      <footer class="footer">
        <span>Controller version <strong data-role="controller-version">{{VERSION}}</strong></span>
        <span>API: <span class="mono">/api/v1/*</span></span>
      </footer>
    </main>
  </div>

  <script>
    const state = {
      branches: [],
      connection: null,
      endpoints: [],
      operations: [],
      branchFilter: '',
    };

    const refs = {
      healthPill: document.querySelector('[data-role="health-pill"]'),
      endpointNote: document.querySelector('[data-role="endpoint-note"]'),
      endpointStatus: document.querySelector('[data-role="endpoint-status"]'),
      activeBranch: document.querySelector('[data-role="active-branch"]'),
      runtimeState: document.querySelector('[data-role="runtime-state"]'),
      endpointAddress: document.querySelector('[data-role="endpoint-address"]'),
      connectionCommand: document.querySelector('[data-role="connection-command"]'),
      connectionDSN: document.querySelector('[data-role="connection-dsn"]'),
      connectionPassword: document.querySelector('[data-role="connection-password"]'),
      connectionEnv: document.querySelector('[data-role="connection-env"]'),
      parentSelect: document.querySelector('[data-role="parent-select"]'),
      restoreSource: document.querySelector('[data-role="restore-source"]'),
      branchFilter: document.querySelector('[data-role="branch-filter"]'),
      branchList: document.querySelector('[data-role="branch-list"]'),
      endpointList: document.querySelector('[data-role="endpoint-list"]'),
      operations: document.querySelector('[data-role="operations"]'),
      statBranches: document.querySelector('[data-role="stat-branches"]'),
      statEndpoints: document.querySelector('[data-role="stat-endpoints"]'),
      statPrimary: document.querySelector('[data-role="stat-primary"]'),
      statOperations: document.querySelector('[data-role="stat-operations"]'),
      message: document.querySelector('[data-role="message"]'),
      controllerVersion: document.querySelector('[data-role="controller-version"]'),
    };

    function escapeHTML(value) {
      return String(value)
        .replaceAll('&', '&amp;')
        .replaceAll('<', '&lt;')
        .replaceAll('>', '&gt;')
        .replaceAll('"', '&quot;')
        .replaceAll("'", '&#39;');
    }

    function showMessage(text, kind) {
      refs.message.textContent = text || '';
      refs.message.classList.remove('ok', 'err');
      if (kind === 'ok') {
        refs.message.classList.add('ok');
      }
      if (kind === 'err') {
        refs.message.classList.add('err');
      }
    }

    function endpointStatusClass(status) {
      const value = String(status || 'unknown').toLowerCase();
      if (value === 'running' || value === 'active') {
        return 'badge ok';
      }
      if (value === 'starting') {
        return 'badge warn';
      }
      if (value === 'error' || value === 'unhealthy') {
        return 'badge bad';
      }
      return 'badge muted';
    }

    async function api(method, path, payload) {
      const options = {
        method,
        headers: {
          'Accept': 'application/json',
        },
      };

      if (payload !== undefined) {
        options.headers['Content-Type'] = 'application/json';
        options.body = JSON.stringify(payload);
      }

      const response = await fetch(path, options);
      const text = await response.text();
      const data = text ? JSON.parse(text) : {};

      if (!response.ok) {
        const message = data && data.error && data.error.message ? data.error.message : response.statusText;
        const err = new Error(message || 'request failed');
        err.code = data && data.error ? data.error.code : 'request_failed';
        throw err;
      }

      return data;
    }

    function getConnectionPassword(connection) {
      if (typeof connection.password === 'string' && connection.password.length > 0) {
        return connection.password;
      }

      if (typeof connection.user === 'string' && connection.user.length > 0) {
        return connection.user;
      }

      return 'cloud_admin';
    }

    function encodeSegment(value) {
      return encodeURIComponent(String(value));
    }

    function makeConnectionURL(connection, includePassword) {
      const host = connection.host || '127.0.0.1';
      const port = connection.port || 55433;
      const user = connection.user || 'cloud_admin';
      const password = getConnectionPassword(connection);
      const database = connection.database || 'postgres';

      let userInfo = encodeSegment(user);
      if (includePassword) {
        userInfo = userInfo + ':' + encodeSegment(password);
      }

      return 'postgresql://' + userInfo + '@' + host + ':' + port + '/' + encodeSegment(database) + '?sslmode=disable';
    }

    function makeDSN(connection) {
      return makeConnectionURL(connection, true);
    }

    function quoteShellSingle(value) {
      return "'" + String(value).replaceAll("'", "'\"'\"'") + "'";
    }

    function makePSQLCommand(connection) {
      const password = getConnectionPassword(connection);
      return 'PGPASSWORD=' + quoteShellSingle(password) + ' psql "' + makeConnectionURL(connection, false) + '"';
    }

    function makeEnvSnippet(connection) {
      return 'DATABASE_URL="' + makeDSN(connection) + '"';
    }

    function endpointByBranch(branchName) {
      for (let i = 0; i < state.endpoints.length; i += 1) {
        if (state.endpoints[i].branch === branchName) {
          return state.endpoints[i];
        }
      }
      return null;
    }

    function renderStats() {
      refs.statBranches.textContent = String(state.branches.length);
      refs.statEndpoints.textContent = String(state.endpoints.length);
      refs.statPrimary.textContent = state.connection && state.connection.status ? state.connection.status : 'unknown';
      refs.statOperations.textContent = String(state.operations.length);
    }

    function renderConnection(connection) {
      state.connection = connection;

      const status = connection.status || 'unknown';
      refs.endpointStatus.className = endpointStatusClass(status);
      refs.endpointStatus.textContent = status;
      refs.activeBranch.textContent = connection.branch || 'main';
      refs.runtimeState.textContent = connection.runtime_state || 'unknown';
      refs.endpointAddress.textContent = (connection.host || '127.0.0.1') + ':' + String(connection.port || 55433);
      refs.connectionCommand.value = makePSQLCommand(connection);
      refs.connectionDSN.value = makeDSN(connection);
      refs.connectionPassword.value = getConnectionPassword(connection);
      refs.connectionEnv.value = makeEnvSnippet(connection);

      const readiness = connection.ready ? 'ready' : 'not ready';
      refs.endpointNote.textContent = 'Primary branch ' + (connection.branch || 'main') + ' is ' + readiness + '.';
    }

    async function copyTextToClipboard(value) {
      if (navigator.clipboard && navigator.clipboard.writeText) {
        await navigator.clipboard.writeText(value);
        return;
      }

      const probe = document.createElement('textarea');
      probe.value = value;
      probe.setAttribute('readonly', '');
      probe.style.position = 'absolute';
      probe.style.left = '-9999px';
      document.body.appendChild(probe);
      probe.select();
      const copied = document.execCommand('copy');
      document.body.removeChild(probe);
      if (!copied) {
        throw new Error('clipboard copy is unavailable in this browser');
      }
    }

    function renderHealth(health) {
      refs.healthPill.classList.remove('ok', 'warn', 'bad');
      if (health.status === 'ok') {
        refs.healthPill.classList.add('ok');
      } else if (health.status === 'degraded') {
        refs.healthPill.classList.add('warn');
      } else {
        refs.healthPill.classList.add('bad');
      }

      refs.healthPill.textContent = 'Health: ' + health.status;
    }

    function renderBranchSelectors() {
      const options = state.branches
        .map((item) => '<option value="' + escapeHTML(item.name) + '">' + escapeHTML(item.name) + '</option>')
        .join('');

      refs.parentSelect.innerHTML = options;
      refs.restoreSource.innerHTML = options;

      if (state.connection && state.connection.branch) {
        refs.parentSelect.value = state.connection.branch;
        refs.restoreSource.value = state.connection.branch;
      } else {
        refs.parentSelect.value = 'main';
        refs.restoreSource.value = 'main';
      }
    }

    function renderBranches() {
      const query = state.branchFilter.toLowerCase();
      const visible = state.branches.filter((item) => {
        if (!query) {
          return true;
        }
        return item.name.toLowerCase().includes(query) || String(item.parent || '').toLowerCase().includes(query);
      });

      if (!visible.length) {
        refs.branchList.innerHTML = '<div class="table-row"><div class="cell-strong">No branches match filter.</div><div>-</div><div>-</div><div>-</div><div class="row-actions"></div></div>';
        return;
      }

      refs.branchList.innerHTML = visible
        .map((item) => {
          const branchName = item.name;
          const endpoint = endpointByBranch(branchName);
          const isActiveBranch = state.connection && state.connection.branch === branchName;
          const isProtected = branchName === 'main';

          const primaryStatus = isActiveBranch
            ? (state.connection && state.connection.status ? state.connection.status : 'unknown')
            : 'idle';

          let endpointText = 'not published';
          if (endpoint && endpoint.published) {
            endpointText = (endpoint.host || '127.0.0.1') + ':' + String(endpoint.port || 0) + ' (' + (endpoint.status || 'unknown') + ')';
          }

          const publishButton = endpoint && endpoint.published
            ? '<button class="btn-warn" data-action="unpublish-branch-endpoint" data-branch="' + escapeHTML(branchName) + '">Unpublish</button>'
            : '<button class="btn-ghost" data-action="publish-branch-endpoint" data-branch="' + escapeHTML(branchName) + '">Publish</button>';
          const connectDisabled = endpoint && endpoint.published && endpoint.port > 0 ? '' : 'disabled';
          const resetDisabled = isProtected ? 'disabled' : '';
          const deleteDisabled = isProtected ? 'disabled' : '';
          const activeSuffix = isActiveBranch ? ' (active)' : '';

          return '<div class="table-row">'
            + '<div class="cell-strong">' + escapeHTML(branchName + activeSuffix) + '</div>'
            + '<div>' + escapeHTML(item.parent || '-') + '</div>'
            + '<div><span class="' + endpointStatusClass(primaryStatus) + '">' + escapeHTML(primaryStatus) + '</span></div>'
            + '<div class="mono">' + escapeHTML(endpointText) + '</div>'
            + '<div class="row-actions">'
            + '<button class="btn-ghost" data-action="switch-branch" data-branch="' + escapeHTML(branchName) + '">Switch</button>'
            + publishButton
            + '<button class="btn-ghost" data-action="copy-branch-dsn" data-branch="' + escapeHTML(branchName) + '" ' + connectDisabled + '>Copy DSN</button>'
            + '<button class="btn-warn" data-action="reset-branch" data-branch="' + escapeHTML(branchName) + '" ' + resetDisabled + '>Reset</button>'
            + '<button class="btn-danger" data-action="delete-branch" data-branch="' + escapeHTML(branchName) + '" ' + deleteDisabled + '>Delete</button>'
            + '</div>'
            + '</div>';
        })
        .join('');
    }

    function renderEndpoints() {
      if (!state.endpoints.length) {
        refs.endpointList.innerHTML = '<li class="endpoint-item"><div class="endpoint-meta">No published endpoints. Publish a branch to get a direct connection.</div></li>';
        return;
      }

      refs.endpointList.innerHTML = state.endpoints
        .map((item) => {
          const status = item.status || 'unknown';
          const activeConnections = Number(item.active_connections || 0);
          const host = item.host || '127.0.0.1';
          const port = item.port || 0;
          const address = host + ':' + String(port);
          const connectionInfo = activeConnections > 0 ? String(activeConnections) + ' active connections' : 'no active clients';
          return '<li class="endpoint-item">'
            + '<div class="endpoint-top">'
            + '<strong>' + escapeHTML(item.branch) + '</strong>'
            + '<span class="' + endpointStatusClass(status) + '">' + escapeHTML(status) + '</span>'
            + '</div>'
            + '<div class="endpoint-meta mono">' + escapeHTML(address) + ' | ' + escapeHTML(connectionInfo) + '</div>'
            + '<div class="endpoint-actions">'
            + '<button class="btn-ghost" data-action="copy-branch-dsn" data-branch="' + escapeHTML(item.branch) + '">Copy DSN</button>'
            + '<button class="btn-warn" data-action="unpublish-branch-endpoint" data-branch="' + escapeHTML(item.branch) + '">Unpublish</button>'
            + '</div>'
            + '</li>';
        })
        .join('');
    }

    function renderOperations(operations) {
      if (!operations.length) {
        refs.operations.innerHTML = '<li>No operations yet.</li>';
        return;
      }

      refs.operations.innerHTML = operations
        .slice(0, 20)
        .map((item) => {
          const message = item.message ? ' - ' + item.message : '';
          return '<li><strong>' + escapeHTML(item.type) + '</strong> '
            + '<span class="mono">' + escapeHTML(item.status) + '</span>'
            + '<br><small>' + escapeHTML(item.started_at + message) + '</small></li>';
        })
        .join('');
    }

    async function loadAll() {
      try {
        showMessage('Refreshing...', '');
        const responses = await Promise.all([
          api('GET', '/api/v1/status'),
          api('GET', '/api/v1/health'),
          api('GET', '/api/v1/endpoints/primary/connection'),
          api('GET', '/api/v1/branches'),
          api('GET', '/api/v1/endpoints'),
          api('GET', '/api/v1/operations'),
        ]);

        const status = responses[0];
        const health = responses[1];
        const connection = responses[2];
        const branches = responses[3];
        const endpoints = responses[4];
        const operations = responses[5];

        refs.controllerVersion.textContent = status.version || refs.controllerVersion.textContent;
        renderHealth(health);
        renderConnection(connection.connection || {});

        state.branches = (branches.branches || []).slice();
        state.endpoints = (endpoints.endpoints || []).slice();
        state.operations = (operations.operations || []).slice();

        renderStats();
        renderBranchSelectors();
        renderBranches();
        renderEndpoints();
        renderOperations(state.operations);
        showMessage('Console is up to date.', 'ok');
      } catch (err) {
        showMessage('Refresh failed: ' + err.message, 'err');
      }
    }

    async function onCreateBranchSubmit(event) {
      event.preventDefault();
      const form = event.currentTarget;
      const name = form.elements.name.value.trim();
      const parent = form.elements.parent.value.trim();
      if (!name) {
        showMessage('Branch name is required.', 'err');
        return;
      }

      try {
        await api('POST', '/api/v1/branches', { name, parent });
        form.reset();
        showMessage('Branch ' + name + ' created.', 'ok');
        await loadAll();
      } catch (err) {
        showMessage('Create failed: ' + err.message, 'err');
      }
    }

    async function onRestoreSubmit(event) {
      event.preventDefault();
      const form = event.currentTarget;
      const source = form.elements.source.value.trim();
      const name = form.elements.name.value.trim();
      const timestampLocal = form.elements.timestamp.value;
      if (!timestampLocal) {
        showMessage('Restore timestamp is required.', 'err');
        return;
      }

      const payload = {
        source_branch: source,
        timestamp: new Date(timestampLocal).toISOString(),
      };
      if (name) {
        payload.name = name;
      }

      try {
        const result = await api('POST', '/api/v1/restore', payload);
        const branchName = result.restore && result.restore.branch ? result.restore.branch.name : '(unknown)';
        showMessage('Restore branch created: ' + branchName, 'ok');
        form.reset();
        await loadAll();
      } catch (err) {
        showMessage('Restore failed: ' + err.message, 'err');
      }
    }

    async function onPanelClick(event) {
      const actionTarget = event.target.closest('[data-action]');
      if (!actionTarget) {
        return;
      }

      const action = actionTarget.getAttribute('data-action');
      const branch = actionTarget.getAttribute('data-branch');

      try {
        if (action === 'switch-branch') {
          await api('POST', '/api/v1/endpoints/primary/switch', { branch });
          showMessage('Switched primary endpoint to ' + branch + '.', 'ok');
          await loadAll();
          return;
        }

        if (action === 'publish-branch-endpoint') {
          await api('POST', '/api/v1/branches/' + encodeURIComponent(branch) + '/publish');
          showMessage('Published endpoint for ' + branch + '.', 'ok');
          await loadAll();
          return;
        }

        if (action === 'unpublish-branch-endpoint') {
          await api('POST', '/api/v1/branches/' + encodeURIComponent(branch) + '/unpublish');
          showMessage('Unpublished endpoint for ' + branch + '.', 'ok');
          await loadAll();
          return;
        }

        if (action === 'copy-branch-dsn') {
          const response = await api('GET', '/api/v1/branches/' + encodeURIComponent(branch) + '/connection');
          const branchConnection = response.connection || {};
          if (!branchConnection.published || !branchConnection.port) {
            throw new Error('branch endpoint is not published');
          }
          const dsn = branchConnection.dsn || makeDSN(branchConnection);
          await copyTextToClipboard(dsn);
          showMessage('Branch DSN copied for ' + branch + '.', 'ok');
          return;
        }

        if (action === 'delete-branch') {
          if (!confirm('Delete branch ' + branch + '?')) {
            return;
          }
          await api('DELETE', '/api/v1/branches/' + encodeURIComponent(branch));
          showMessage('Deleted branch ' + branch + '.', 'ok');
          await loadAll();
          return;
        }

        if (action === 'reset-branch') {
          if (!confirm('Reset branch ' + branch + ' from its parent? This will replace its attachment timeline.')) {
            return;
          }
          await api('POST', '/api/v1/branches/' + encodeURIComponent(branch) + '/reset');
          showMessage('Reset branch ' + branch + ' from parent.', 'ok');
          await loadAll();
          return;
        }

        if (action === 'endpoint-start') {
          await api('POST', '/api/v1/endpoints/primary/start');
          showMessage('Primary endpoint started.', 'ok');
          await loadAll();
          return;
        }

        if (action === 'endpoint-stop') {
          await api('POST', '/api/v1/endpoints/primary/stop');
          showMessage('Primary endpoint stopped.', 'ok');
          await loadAll();
          return;
        }

        if (action === 'copy-psql-command') {
          await copyTextToClipboard(refs.connectionCommand.value);
          showMessage('psql command copied to clipboard.', 'ok');
          return;
        }

        if (action === 'copy-dsn') {
          await copyTextToClipboard(refs.connectionDSN.value);
          showMessage('DSN copied to clipboard.', 'ok');
          return;
        }

        if (action === 'copy-password') {
          await copyTextToClipboard(refs.connectionPassword.value);
          showMessage('Password copied to clipboard.', 'ok');
          return;
        }

        if (action === 'copy-env-snippet') {
          await copyTextToClipboard(refs.connectionEnv.value);
          showMessage('DATABASE_URL snippet copied to clipboard.', 'ok');
          return;
        }

        if (action === 'refresh') {
          await loadAll();
        }
      } catch (err) {
        showMessage('Action failed: ' + err.message, 'err');
      }
    }

    function onBranchFilterInput(event) {
      state.branchFilter = event.target.value.trim();
      renderBranches();
    }

    document.addEventListener('click', onPanelClick);
    document.querySelector('[data-action="create-branch"]').addEventListener('submit', onCreateBranchSubmit);
    document.querySelector('[data-action="restore-branch"]').addEventListener('submit', onRestoreSubmit);
    refs.branchFilter.addEventListener('input', onBranchFilterInput);

    loadAll();
  </script>
</body>
</html>
`

func writeConsoleUI(w http.ResponseWriter, version string) {
	body := strings.Replace(consoleHTML, "{{VERSION}}", html.EscapeString(version), 1)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}
