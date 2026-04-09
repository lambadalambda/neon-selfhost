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
    @import url("https://fonts.googleapis.com/css2?family=Space+Grotesk:wght@400;500;700&family=IBM+Plex+Mono:wght@400;500&display=swap");

    :root {
      --bg: #f5f3ec;
      --bg-strong: #e8f2ee;
      --surface: #ffffff;
      --surface-soft: #f8f7f2;
      --ink: #1f2f34;
      --muted: #5a6a70;
      --line: #d4ded8;
      --accent: #0f8e86;
      --accent-ink: #ffffff;
      --warn: #d26b27;
      --danger: #b43f2d;
      --ok: #1d8a4f;
      --shadow: 0 16px 44px rgba(16, 41, 37, 0.12);
      --radius-lg: 18px;
      --radius-md: 12px;
      --radius-sm: 8px;
    }

    * {
      box-sizing: border-box;
    }

    body {
      margin: 0;
      min-height: 100vh;
      color: var(--ink);
      background:
        radial-gradient(circle at 10% 10%, rgba(15, 142, 134, 0.15), transparent 42%),
        radial-gradient(circle at 90% 0%, rgba(210, 107, 39, 0.14), transparent 36%),
        linear-gradient(150deg, var(--bg), var(--bg-strong));
      font-family: "Space Grotesk", "Avenir Next", "Segoe UI", sans-serif;
      line-height: 1.45;
    }

    .shell {
      max-width: 1160px;
      margin: 0 auto;
      padding: 28px 20px 42px;
    }

    .hero {
      display: flex;
      justify-content: space-between;
      gap: 16px;
      align-items: flex-start;
      margin-bottom: 20px;
    }

    .hero-title {
      margin: 0;
      font-size: clamp(1.4rem, 2.6vw, 2.3rem);
      letter-spacing: 0.01em;
    }

    .hero-subtitle {
      margin: 6px 0 0;
      color: var(--muted);
      font-size: 0.98rem;
    }

    .hero-actions {
      display: flex;
      gap: 10px;
      align-items: center;
      flex-wrap: wrap;
      justify-content: flex-end;
    }

    .pill {
      padding: 7px 10px;
      border-radius: 999px;
      border: 1px solid var(--line);
      background: rgba(255, 255, 255, 0.82);
      color: var(--muted);
      font-size: 0.82rem;
      font-weight: 600;
    }

    .pill.ok {
      color: var(--ok);
      border-color: rgba(29, 138, 79, 0.3);
      background: rgba(29, 138, 79, 0.08);
    }

    .pill.warn {
      color: var(--warn);
      border-color: rgba(210, 107, 39, 0.3);
      background: rgba(210, 107, 39, 0.08);
    }

    .pill.bad {
      color: var(--danger);
      border-color: rgba(180, 63, 45, 0.3);
      background: rgba(180, 63, 45, 0.08);
    }

    .layout {
      display: grid;
      grid-template-columns: 1.2fr 1fr;
      gap: 16px;
    }

    .panel {
      background: var(--surface);
      border: 1px solid var(--line);
      border-radius: var(--radius-lg);
      box-shadow: var(--shadow);
      overflow: hidden;
    }

    .panel-header {
      display: flex;
      justify-content: space-between;
      gap: 8px;
      align-items: baseline;
      padding: 14px 16px;
      border-bottom: 1px solid var(--line);
      background: linear-gradient(180deg, rgba(232, 242, 238, 0.65), rgba(255, 255, 255, 0.7));
    }

    .panel-title {
      margin: 0;
      font-size: 1.04rem;
      letter-spacing: 0.02em;
    }

    .panel-note {
      margin: 0;
      font-size: 0.83rem;
      color: var(--muted);
    }

    .panel-body {
      padding: 14px 16px 16px;
      display: grid;
      gap: 14px;
    }

    .kv-grid {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 10px;
    }

    .kv {
      padding: 10px;
      border: 1px solid var(--line);
      background: var(--surface-soft);
      border-radius: var(--radius-sm);
    }

    .kv label {
      display: block;
      font-size: 0.74rem;
      color: var(--muted);
      text-transform: uppercase;
      letter-spacing: 0.08em;
      margin-bottom: 4px;
    }

    .kv strong {
      display: block;
      font-size: 0.94rem;
      word-break: break-word;
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
      border: 1px solid var(--line);
      background: #ecf2ef;
      color: #324449;
      border-radius: var(--radius-sm);
      padding: 10px 11px;
      font-size: 0.79rem;
      min-width: 68px;
      text-align: center;
      letter-spacing: 0.04em;
      text-transform: uppercase;
    }

    .cmd {
      width: 100%;
      border-radius: var(--radius-sm);
      border: 1px solid var(--line);
      background: #f4f8f7;
      color: #253539;
      padding: 11px;
      font-size: 0.85rem;
      font-family: "IBM Plex Mono", "SF Mono", "Menlo", monospace;
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
      border-radius: var(--radius-sm);
      border: 1px solid var(--line);
      padding: 9px 10px;
      font: inherit;
      color: inherit;
      background: #fff;
    }

    input,
    select {
      min-width: 0;
      flex: 1 1 160px;
    }

    button {
      cursor: pointer;
      transition: transform 120ms ease, box-shadow 120ms ease;
      font-weight: 600;
    }

    button:hover {
      transform: translateY(-1px);
      box-shadow: 0 5px 14px rgba(16, 41, 37, 0.14);
    }

    .primary {
      background: var(--accent);
      color: var(--accent-ink);
      border-color: rgba(0, 0, 0, 0);
    }

    .warn {
      border-color: rgba(210, 107, 39, 0.34);
      color: #9b4f1d;
      background: #fff8f2;
    }

    .danger {
      border-color: rgba(180, 63, 45, 0.35);
      color: #8f2f22;
      background: #fff5f3;
    }

    .quiet {
      background: #f4f8f7;
      color: #324449;
    }

    .stack {
      display: grid;
      gap: 10px;
    }

    .branches {
      display: grid;
      gap: 8px;
      max-height: 320px;
      overflow: auto;
      padding-right: 2px;
    }

    .branch-row {
      display: grid;
      grid-template-columns: 1fr auto;
      gap: 8px;
      align-items: center;
      border: 1px solid var(--line);
      background: var(--surface-soft);
      border-radius: var(--radius-sm);
      padding: 10px;
    }

    .branch-meta strong {
      display: block;
      font-size: 0.94rem;
    }

    .branch-meta small {
      color: var(--muted);
      font-size: 0.78rem;
    }

    .row-actions {
      display: flex;
      gap: 6px;
      flex-wrap: wrap;
      justify-content: flex-end;
    }

    .operations {
      list-style: none;
      margin: 0;
      padding: 0;
      display: grid;
      gap: 8px;
      max-height: 240px;
      overflow: auto;
    }

    .operations li {
      border: 1px solid var(--line);
      border-radius: var(--radius-sm);
      padding: 8px 10px;
      background: var(--surface-soft);
      font-size: 0.84rem;
    }

    .mono {
      font-family: "IBM Plex Mono", "SF Mono", "Menlo", monospace;
    }

    .message {
      min-height: 24px;
      font-size: 0.87rem;
      font-weight: 600;
      color: var(--muted);
    }

    .message.ok {
      color: var(--ok);
    }

    .message.err {
      color: var(--danger);
    }

    .footer {
      margin-top: 18px;
      color: var(--muted);
      font-size: 0.83rem;
      display: flex;
      justify-content: space-between;
      gap: 8px;
      flex-wrap: wrap;
    }

    @media (max-width: 980px) {
      .layout {
        grid-template-columns: 1fr;
      }

      .hero {
        flex-direction: column;
        align-items: stretch;
      }

      .hero-actions {
        justify-content: flex-start;
      }

      .kv-grid {
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
  <main class="shell">
    <section class="hero">
      <div>
        <h1 class="hero-title">Neon Selfhost Console</h1>
        <p class="hero-subtitle">Operate branch, restore, and primary endpoint workflows from one place.</p>
      </div>
      <div class="hero-actions">
        <span class="pill" data-role="health-pill">Health: checking...</span>
        <button class="quiet" data-action="refresh">Refresh</button>
      </div>
    </section>

    <section class="layout">
      <article class="panel">
        <header class="panel-header">
          <h2 class="panel-title">Primary Endpoint</h2>
          <p class="panel-note" data-role="endpoint-note">Loading...</p>
        </header>
        <div class="panel-body">
          <div class="kv-grid">
            <div class="kv">
              <label>Status</label>
              <strong data-role="endpoint-status">unknown</strong>
            </div>
            <div class="kv">
              <label>Active Branch</label>
              <strong data-role="active-branch">main</strong>
            </div>
            <div class="kv">
              <label>Runtime</label>
              <strong data-role="runtime-state">unknown</strong>
            </div>
            <div class="kv">
              <label>Endpoint</label>
              <strong class="mono" data-role="endpoint-address">127.0.0.1:55433</strong>
            </div>
          </div>

          <div class="connect-stack">
            <div class="cmd-row">
              <span class="cmd-label mono">psql</span>
              <input class="cmd" data-role="connection-command" readonly value="Loading psql command...">
              <button class="quiet" data-action="copy-psql-command">Copy psql</button>
            </div>

            <div class="cmd-row">
              <span class="cmd-label mono">DSN</span>
              <input class="cmd" data-role="connection-dsn" readonly value="Loading DSN...">
              <button class="quiet" data-action="copy-dsn">Copy DSN</button>
            </div>

            <div class="cmd-row">
              <span class="cmd-label mono">Password</span>
              <input class="cmd" data-role="connection-password" readonly value="Loading password...">
              <button class="quiet" data-action="copy-password">Copy password</button>
            </div>

            <div class="cmd-row">
              <span class="cmd-label mono">.env</span>
              <input class="cmd" data-role="connection-env" readonly value="DATABASE_URL=Loading...">
              <button class="quiet" data-action="copy-env-snippet">Copy .env</button>
            </div>
          </div>

          <div class="toolbar">
            <button class="primary" data-action="endpoint-start">Start</button>
            <button class="warn" data-action="endpoint-stop">Stop</button>
          </div>
        </div>
      </article>

      <article class="panel">
        <header class="panel-header">
          <h2 class="panel-title">Branches</h2>
          <p class="panel-note">Create, switch, and delete branches</p>
        </header>
        <div class="panel-body stack">
          <form data-action="create-branch">
            <input name="name" placeholder="new-branch-name" required>
            <select name="parent" data-role="parent-select"></select>
            <button class="primary" type="submit">Create</button>
          </form>
          <div class="branches" data-role="branch-list"></div>
        </div>
      </article>

      <article class="panel">
        <header class="panel-header">
          <h2 class="panel-title">Restore To Timestamp</h2>
          <p class="panel-note">Creates a new branch from source timeline history</p>
        </header>
        <div class="panel-body">
          <form data-action="restore-branch">
            <select name="source" data-role="restore-source"></select>
            <input type="datetime-local" name="timestamp" required>
            <input name="name" placeholder="optional restore branch name">
            <button class="primary" type="submit">Restore</button>
          </form>
        </div>
      </article>

      <article class="panel">
        <header class="panel-header">
          <h2 class="panel-title">Recent Operations</h2>
          <p class="panel-note">Latest controller operation log entries</p>
        </header>
        <div class="panel-body">
          <ul class="operations" data-role="operations"></ul>
        </div>
      </article>
    </section>

    <p class="message" data-role="message"></p>

    <footer class="footer">
      <span>Controller version <strong data-role="controller-version">{{VERSION}}</strong></span>
      <span>API: <span class="mono">/api/v1/*</span></span>
    </footer>
  </main>

  <script>
    const state = {
      branches: [],
      connection: null,
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
      branchList: document.querySelector('[data-role="branch-list"]'),
      operations: document.querySelector('[data-role="operations"]'),
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

    function renderConnection(connection) {
      state.connection = connection;
      refs.endpointStatus.textContent = connection.status || 'unknown';
      refs.activeBranch.textContent = connection.branch || 'main';
      refs.runtimeState.textContent = connection.runtime_state || 'unknown';
      refs.endpointAddress.textContent = (connection.host || '127.0.0.1') + ':' + String(connection.port || 55433);
      refs.connectionCommand.value = makePSQLCommand(connection);
      refs.connectionDSN.value = makeDSN(connection);
      refs.connectionPassword.value = getConnectionPassword(connection);
      refs.connectionEnv.value = makeEnvSnippet(connection);

      const readiness = connection.ready ? 'ready' : 'not ready';
      refs.endpointNote.textContent = 'Branch ' + (connection.branch || 'main') + ' is ' + readiness + '. Connect commands always target the current primary branch.';
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
        .map((branch) => '<option value="' + escapeHTML(branch.name) + '">' + escapeHTML(branch.name) + '</option>')
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
      if (!state.branches.length) {
        refs.branchList.innerHTML = '<div class="branch-row"><div class="branch-meta"><strong>No branches</strong><small>Create your first branch.</small></div></div>';
        return;
      }

      refs.branchList.innerHTML = state.branches
        .map((branch) => {
          const isActive = state.connection && branch.name === state.connection.branch;
          const activeTag = isActive ? ' (active)' : '';
          const deleteDisabled = branch.name === 'main' ? 'disabled' : '';
          const resetDisabled = branch.name === 'main' ? 'disabled' : '';
          return '<div class="branch-row">'
            + '<div class="branch-meta">'
            + '<strong>' + escapeHTML(branch.name + activeTag) + '</strong>'
            + '<small>parent: ' + escapeHTML(branch.parent || '-') + ' | created: ' + escapeHTML(branch.created_at) + '</small>'
            + '</div>'
            + '<div class="row-actions">'
            + '<button class="quiet" data-action="switch-branch" data-branch="' + escapeHTML(branch.name) + '">Switch</button>'
            + '<button class="warn" data-action="reset-branch" data-branch="' + escapeHTML(branch.name) + '" ' + resetDisabled + '>Reset</button>'
            + '<button class="danger" data-action="delete-branch" data-branch="' + escapeHTML(branch.name) + '" ' + deleteDisabled + '>Delete</button>'
            + '</div>'
            + '</div>';
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
        .map((op) => {
          const message = op.message ? ' - ' + op.message : '';
          return '<li><strong>' + escapeHTML(op.type) + '</strong> '
            + '<span class="mono">' + escapeHTML(op.status) + '</span>'
            + '<br><small>' + escapeHTML(op.started_at + message) + '</small></li>';
        })
        .join('');
    }

    async function loadAll() {
      try {
        showMessage('Refreshing...', '');
        const [status, health, connection, branches, operations] = await Promise.all([
          api('GET', '/api/v1/status'),
          api('GET', '/api/v1/health'),
          api('GET', '/api/v1/endpoints/primary/connection'),
          api('GET', '/api/v1/branches'),
          api('GET', '/api/v1/operations'),
        ]);

        refs.controllerVersion.textContent = status.version || refs.controllerVersion.textContent;
        renderHealth(health);
        renderConnection(connection.connection || {});
        state.branches = (branches.branches || []).slice();
        renderBranchSelectors();
        renderBranches();
        renderOperations(operations.operations || []);
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

      const timestamp = new Date(timestampLocal).toISOString();
      const payload = {
        source_branch: source,
        timestamp,
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

    document.addEventListener('click', onPanelClick);
    document.querySelector('[data-action="create-branch"]').addEventListener('submit', onCreateBranchSubmit);
    document.querySelector('[data-action="restore-branch"]').addEventListener('submit', onRestoreSubmit);

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
