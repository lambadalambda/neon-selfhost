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

    .nav-list li[data-action] {
      cursor: pointer;
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

    .page-section {
      display: grid;
      gap: 12px;
    }

    .is-hidden {
      display: none !important;
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

    .branches-head,
    .branches-row {
      min-width: 1020px;
      grid-template-columns: 1.6fr 1fr .95fr 1.25fr .9fr 1.9fr;
    }

    .cell-strong {
      font-weight: 700;
    }

    .branch-prefix {
      color: var(--muted);
      margin-right: 4px;
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

    .monitoring-placeholder {
      border: 1px dashed rgba(98, 107, 121, 0.42);
      border-radius: 10px;
      background: linear-gradient(180deg, #fbfcfe, #f4f7fb);
      padding: 12px;
      display: grid;
      gap: 10px;
    }

    .monitoring-chart {
      height: 220px;
      border-radius: 8px;
      border: 1px solid var(--line);
      background:
        linear-gradient(180deg, rgba(103, 114, 131, 0.06), rgba(103, 114, 131, 0.01)),
        repeating-linear-gradient(to right, rgba(56, 67, 85, 0.08), rgba(56, 67, 85, 0.08) 1px, transparent 1px, transparent 82px),
        repeating-linear-gradient(to top, rgba(56, 67, 85, 0.08), rgba(56, 67, 85, 0.08) 1px, transparent 1px, transparent 56px);
      position: relative;
      overflow: hidden;
    }

    .monitoring-chart::after {
      content: "";
      position: absolute;
      left: 0;
      right: 0;
      bottom: 26px;
      height: 2px;
      background: linear-gradient(90deg, transparent 0%, rgba(23, 143, 88, 0.65) 18%, rgba(23, 143, 88, 0.9) 50%, rgba(23, 143, 88, 0.65) 82%, transparent 100%);
      transform: translateY(0);
      animation: monitor-wave 5s ease-in-out infinite;
    }

    @keyframes monitor-wave {
      0% { transform: translateY(0); }
      25% { transform: translateY(-12px); }
      50% { transform: translateY(-5px); }
      75% { transform: translateY(-16px); }
      100% { transform: translateY(0); }
    }

    .placeholder-note {
      color: var(--muted);
      font-size: 0.83rem;
      margin: 0;
    }

    .dashboard-branch-list {
      list-style: none;
      margin: 0;
      padding: 0;
      display: grid;
      gap: 8px;
      max-height: 278px;
      overflow: auto;
    }

    .dashboard-branch-item {
      border: 1px solid var(--line);
      border-radius: 10px;
      background: var(--surface-soft);
      padding: 9px 10px;
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 8px;
      flex-wrap: wrap;
    }

    .dashboard-branch-meta {
      display: grid;
      gap: 2px;
    }

    .dashboard-branch-meta strong {
      font-size: 0.92rem;
    }

    .dashboard-branch-meta small {
      color: var(--muted);
      font-size: 0.79rem;
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
          <li class="active" data-role="nav-dashboard" data-action="navigate" data-page="dashboard">Dashboard</li>
          <li data-role="nav-branches" data-action="navigate" data-page="branches">Branches</li>
        </ul>
      </section>

      <section class="nav-section">
        <h2>Endpoint Mode</h2>
        <div class="branch-chip">
          <span>Per-branch endpoints</span>
          <span class="mono" data-role="published-count-chip">0 live</span>
        </div>
      </section>
    </aside>

    <main class="workspace">
      <header class="topbar">
        <div class="title-stack">
          <h1 data-role="page-title">Project dashboard</h1>
          <p data-role="page-subtitle">View storage and branch health at a glance, then drive branch and endpoint workflows below.</p>
        </div>
        <div class="top-actions">
          <span class="pill" data-role="health-pill">Health: checking...</span>
          <button class="btn-ghost" data-action="refresh">Refresh</button>
          <button class="btn-primary is-hidden" data-role="new-branch-cta" data-action="focus-new-branch">New Branch</button>
        </div>
      </header>

      <section class="page-section" data-role="page-dashboard">
        <section class="stats">
          <div class="stat-card">
            <label>Compute</label>
            <strong data-role="dashboard-compute">unknown</strong>
          </div>
          <div class="stat-card">
            <label>Storage</label>
            <strong data-role="dashboard-storage">0 B metadata</strong>
          </div>
          <div class="stat-card">
            <label>Branches</label>
            <strong data-role="dashboard-branches">0</strong>
          </div>
          <div class="stat-card">
            <label>Published Endpoints</label>
            <strong data-role="dashboard-endpoints">0</strong>
          </div>
        </section>

        <section class="grid two">
          <article class="panel">
            <header class="panel-header">
              <h2>Monitoring</h2>
              <p>placeholder until metrics pipeline lands</p>
            </header>
            <div class="panel-body">
              <div class="monitoring-placeholder" data-role="monitoring-placeholder">
                <div class="monitoring-chart"></div>
                <p class="placeholder-note">Metrics are not wired yet. This area will show branch and compute charts in a future slice.</p>
              </div>
            </div>
          </article>

          <article class="panel">
            <header class="panel-header">
              <h2>Branches</h2>
              <p>active branch summary</p>
            </header>
            <div class="panel-body">
              <ul class="dashboard-branch-list" data-role="dashboard-branch-list"></ul>
            </div>
          </article>
        </section>

        <section class="grid two">
          <article class="panel">
            <header class="panel-header">
              <h2>Published Endpoints</h2>
              <p>branch-scoped database endpoints with direct DSN copy</p>
            </header>
            <div class="panel-body">
              <ul class="endpoint-list" data-role="endpoint-list"></ul>
            </div>
          </article>

          <article class="panel">
            <header class="panel-header">
              <h2>Connection Workflow</h2>
              <p>copy a branch DSN from Branches or Published Endpoints</p>
            </header>
            <div class="panel-body">
              <div class="monitoring-placeholder">
                <p class="placeholder-note">Each branch gets its own endpoint. Use <strong>Copy DSN</strong> from the branches list to connect your app or psql directly.</p>
              </div>
            </div>
          </article>
        </section>
      </section>

      <section class="page-section is-hidden" data-role="page-branches">
        <article class="panel">
          <header class="panel-header">
            <h2>Branches</h2>
            <p>branch list with parent lineage and endpoint controls</p>
          </header>
          <div class="panel-body">
            <form data-action="create-branch">
              <input name="name" data-role="new-branch-name" placeholder="new-branch-name" required>
              <select name="parent" data-role="parent-select"></select>
              <button class="btn-primary" type="submit">Create Branch</button>
            </form>

            <div class="toolbar">
              <input data-role="branch-filter" placeholder="Search branches by name or parent">
            </div>

            <div class="table-scroll">
              <div class="table-head branches-head">
                <div>Branch</div>
                <div>Parent</div>
                <div>Compute</div>
                <div>Endpoint</div>
                <div>Created</div>
                <div style="text-align:right;">Actions</div>
              </div>
              <div data-role="branch-list"></div>
            </div>
          </div>
        </article>
      </section>

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
      endpoints: [],
      branchFilter: '',
      currentPage: 'dashboard',
    };

    const refs = {
      pageTitle: document.querySelector('[data-role="page-title"]'),
      pageSubtitle: document.querySelector('[data-role="page-subtitle"]'),
      pageDashboard: document.querySelector('[data-role="page-dashboard"]'),
      pageBranches: document.querySelector('[data-role="page-branches"]'),
      navDashboard: document.querySelector('[data-role="nav-dashboard"]'),
      navBranches: document.querySelector('[data-role="nav-branches"]'),
      newBranchCTA: document.querySelector('[data-role="new-branch-cta"]'),
      newBranchName: document.querySelector('[data-role="new-branch-name"]'),
      healthPill: document.querySelector('[data-role="health-pill"]'),
      publishedCountChip: document.querySelector('[data-role="published-count-chip"]'),
      parentSelect: document.querySelector('[data-role="parent-select"]'),
      branchFilter: document.querySelector('[data-role="branch-filter"]'),
      branchList: document.querySelector('[data-role="branch-list"]'),
      endpointList: document.querySelector('[data-role="endpoint-list"]'),
      dashboardBranchList: document.querySelector('[data-role="dashboard-branch-list"]'),
      dashboardCompute: document.querySelector('[data-role="dashboard-compute"]'),
      dashboardStorage: document.querySelector('[data-role="dashboard-storage"]'),
      dashboardBranches: document.querySelector('[data-role="dashboard-branches"]'),
      dashboardEndpoints: document.querySelector('[data-role="dashboard-endpoints"]'),
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

    function setPage(pageName) {
      const nextPage = pageName === 'branches' ? 'branches' : 'dashboard';
      state.currentPage = nextPage;

      const dashboardActive = nextPage === 'dashboard';
      refs.pageDashboard.classList.toggle('is-hidden', !dashboardActive);
      refs.pageBranches.classList.toggle('is-hidden', dashboardActive);
      refs.navDashboard.classList.toggle('active', dashboardActive);
      refs.navBranches.classList.toggle('active', !dashboardActive);
      refs.newBranchCTA.classList.toggle('is-hidden', dashboardActive);

      if (dashboardActive) {
        refs.pageTitle.textContent = 'Project dashboard';
        refs.pageSubtitle.textContent = 'View storage and branch health at a glance, then drive branch and endpoint workflows below.';
        return;
      }

      refs.pageTitle.textContent = String(state.branches.length) + ' Branches';
      refs.pageSubtitle.textContent = 'Instantly branch your data to deliver faster, safer experimentation and more reliable CI/CD flows.';
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

    function endpointByBranch(branchName) {
      for (let i = 0; i < state.endpoints.length; i += 1) {
        if (state.endpoints[i].branch === branchName) {
          return state.endpoints[i];
        }
      }
      return null;
    }

    function formatBytes(bytes) {
      const safeBytes = Number(bytes);
      if (!Number.isFinite(safeBytes) || safeBytes <= 0) {
        return '0 B';
      }

      const units = ['B', 'KB', 'MB', 'GB'];
      let value = safeBytes;
      let unitIndex = 0;
      while (value >= 1024 && unitIndex < units.length - 1) {
        value /= 1024;
        unitIndex += 1;
      }

      const precision = value >= 10 || unitIndex === 0 ? 0 : 1;
      return value.toFixed(precision) + ' ' + units[unitIndex];
    }

    function estimateMetadataBytes() {
      const payload = JSON.stringify({
        branches: state.branches,
        endpoints: state.endpoints,
      });

      if (typeof TextEncoder !== 'undefined') {
        return new TextEncoder().encode(payload).length;
      }

      return payload.length;
    }

    function renderStats() {
      const runningEndpoints = state.endpoints.filter((item) => {
        const status = String(item.status || '').toLowerCase();
        return status === 'running' || status === 'active';
      }).length;

      refs.dashboardCompute.textContent = String(runningEndpoints) + ' active';
      refs.dashboardStorage.textContent = formatBytes(estimateMetadataBytes()) + ' metadata';
      refs.dashboardBranches.textContent = String(state.branches.length);
      refs.dashboardEndpoints.textContent = String(state.endpoints.length);
      refs.publishedCountChip.textContent = String(state.endpoints.length) + ' live';
    }

    function renderDashboardBranches() {
      if (!state.branches.length) {
        refs.dashboardBranchList.innerHTML = '<li class="dashboard-branch-item"><div class="dashboard-branch-meta"><strong>No branches yet</strong><small>Create a branch to see dashboard activity.</small></div></li>';
        return;
      }

      refs.dashboardBranchList.innerHTML = state.branches
        .slice(0, 8)
        .map((item) => {
          const endpoint = endpointByBranch(item.name);

          let status = 'idle';
          if (endpoint && endpoint.published) {
            status = endpoint.status || 'published';
          }

          let endpointSummary = 'no published endpoint';
          if (endpoint && endpoint.published && endpoint.port > 0) {
            endpointSummary = (endpoint.host || '127.0.0.1') + ':' + String(endpoint.port);
          }

          const parent = item.parent || '-';
          const activeSuffix = item.name === 'main' ? ' (default)' : '';
          return '<li class="dashboard-branch-item">'
            + '<div class="dashboard-branch-meta">'
            + '<strong>' + escapeHTML(item.name + activeSuffix) + '</strong>'
            + '<small>parent: ' + escapeHTML(parent) + ' | ' + escapeHTML(endpointSummary) + '</small>'
            + '</div>'
            + '<span class="' + endpointStatusClass(status) + '">' + escapeHTML(status) + '</span>'
            + '</li>';
        })
        .join('');
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

      refs.parentSelect.innerHTML = options || '<option value="main">main</option>';

	  const hasMain = state.branches.some((item) => item.name === 'main');
	  if (hasMain) {
	    refs.parentSelect.value = 'main';
	    return;
	  }

	  if (state.branches.length > 0) {
	    refs.parentSelect.value = state.branches[0].name;
	  }
    }

    function formatCreatedAt(value) {
      if (typeof value !== 'string' || value.trim() === '') {
        return '-';
      }

      const parsed = new Date(value);
      if (Number.isNaN(parsed.getTime())) {
        return value;
      }

      return parsed.toLocaleDateString(undefined, {
        month: 'short',
        day: 'numeric',
        year: 'numeric',
      });
    }

    function renderBranches() {
      const query = state.branchFilter.toLowerCase();
      const visible = state.branches.filter((item) => {
        if (!query) {
          return true;
        }
        return item.name.toLowerCase().includes(query) || String(item.parent || '').toLowerCase().includes(query);
      });

      const sorted = visible.slice().sort((left, right) => {
        if (left.name === 'main') {
          return -1;
        }
        if (right.name === 'main') {
          return 1;
        }

        const leftParent = String(left.parent || '');
        const rightParent = String(right.parent || '');
        if (leftParent === rightParent) {
          return left.name.localeCompare(right.name);
        }

        if (leftParent === 'main') {
          return -1;
        }
        if (rightParent === 'main') {
          return 1;
        }

        return leftParent.localeCompare(rightParent);
      });

      if (!sorted.length) {
        refs.branchList.innerHTML = '<div class="table-row branches-row"><div class="cell-strong">No branches match filter.</div><div>-</div><div>-</div><div>-</div><div>-</div><div class="row-actions"></div></div>';
        return;
      }

      refs.branchList.innerHTML = sorted
        .map((item) => {
          const branchName = item.name;
          const endpoint = endpointByBranch(branchName);
          const isProtected = branchName === 'main';
          const isRootBranch = branchName === 'main';

          const computeStatus = endpoint && endpoint.published
            ? (endpoint.status || 'published')
            : 'idle';

          let endpointText = 'not published';
          if (endpoint && endpoint.published) {
            endpointText = (endpoint.host || '127.0.0.1') + ':' + String(endpoint.port || 0) + ' (' + (endpoint.status || 'unknown') + ')';
          }

          const connectDisabled = endpoint && endpoint.published && endpoint.port > 0 ? '' : 'disabled';
          const resetDisabled = isProtected ? 'disabled' : '';
          const deleteDisabled = isProtected ? 'disabled' : '';
          const defaultBadge = branchName === 'main' ? ' <span class="badge muted">Default</span>' : '';
          const createdAt = formatCreatedAt(item.created_at);
          const branchPrefix = isRootBranch ? '' : '<span class="branch-prefix mono">|-</span>';
          const parentLabel = isRootBranch ? '-' : (item.parent || '-');

          return '<div class="table-row branches-row">'
            + '<div class="cell-strong">' + branchPrefix + escapeHTML(branchName) + defaultBadge + '</div>'
            + '<div>' + escapeHTML(parentLabel) + '</div>'
            + '<div><span class="' + endpointStatusClass(computeStatus) + '">' + escapeHTML(computeStatus) + '</span></div>'
            + '<div class="mono">' + escapeHTML(endpointText) + '</div>'
            + '<div>' + escapeHTML(createdAt) + '</div>'
            + '<div class="row-actions">'
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
        refs.endpointList.innerHTML = '<li class="endpoint-item"><div class="endpoint-meta">No branch endpoints are live yet. Create a branch to provision one automatically.</div></li>';
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
            + '</div>'
            + '</li>';
        })
        .join('');
    }

    async function loadAll() {
      try {
        showMessage('Refreshing...', '');
        const responses = await Promise.all([
          api('GET', '/api/v1/status'),
          api('GET', '/api/v1/health'),
          api('GET', '/api/v1/branches'),
          api('GET', '/api/v1/endpoints'),
        ]);

        const status = responses[0];
        const health = responses[1];
        const branches = responses[2];
        const endpoints = responses[3];

        refs.controllerVersion.textContent = status.version || refs.controllerVersion.textContent;
        renderHealth(health);

        state.branches = (branches.branches || []).slice();
        state.endpoints = (endpoints.endpoints || []).slice();

        setPage(state.currentPage);
        renderStats();
        renderDashboardBranches();
        renderBranchSelectors();
        renderBranches();
        renderEndpoints();
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

    async function onPanelClick(event) {
      const actionTarget = event.target.closest('[data-action]');
      if (!actionTarget) {
        return;
      }

      const action = actionTarget.getAttribute('data-action');
      const branch = actionTarget.getAttribute('data-branch');

      try {
        if (action === 'navigate') {
          setPage(actionTarget.getAttribute('data-page'));
          return;
        }

        if (action === 'focus-new-branch') {
          setPage('branches');
          if (refs.newBranchName) {
            refs.newBranchName.focus();
          }
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
    refs.branchFilter.addEventListener('input', onBranchFilterInput);

    setPage('dashboard');
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
