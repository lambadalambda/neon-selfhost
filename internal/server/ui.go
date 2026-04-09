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
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Manrope:wght@500;600;700&family=JetBrains+Mono:wght@400;500&display=swap" rel="stylesheet">
  <style>
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
      --focus-ring: rgba(59, 130, 246, 0.45);
      --surface-muted: #f7f8fa;
      --surface-hover: #f2f4f7;
      --duration-fast: 120ms;
      --duration-base: 180ms;
      --ease-out: cubic-bezier(0.16, 1, 0.3, 1);
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
      transition: background var(--duration-fast) var(--ease-out), border-color var(--duration-fast) var(--ease-out);
    }

    .nav-list li[data-action]:hover {
      background: #ffffff;
      border-color: var(--line);
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

    .sidebar-select {
      width: 100%;
      border-radius: 8px;
      border: 1px solid var(--line);
      background: #fff;
      color: #243042;
      font-weight: 600;
      padding: 9px 10px;
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
      font-size: 1.14rem;
      letter-spacing: -0.01em;
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
      background: var(--surface-muted);
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
      transition: box-shadow var(--duration-fast) var(--ease-out), transform var(--duration-fast) var(--ease-out), background var(--duration-fast) var(--ease-out), border-color var(--duration-fast) var(--ease-out);
    }

    button:hover {
      transform: translateY(-1px);
      box-shadow: 0 6px 12px rgba(26, 34, 47, 0.12);
    }

    button:disabled {
      cursor: not-allowed;
      opacity: 0.5;
      transform: none;
      box-shadow: none;
      background: #eff1f4;
      color: #8a93a2;
      border-color: #d8dce3;
    }

    .btn-primary:disabled {
      background: #9aa3b1;
      border-color: #9aa3b1;
      color: #f7f9fc;
    }

    button:focus-visible,
    input:focus-visible,
    select:focus-visible,
    textarea:focus-visible,
    .nav-list li[data-action]:focus-visible,
    .sql-history-item:focus-visible {
      outline: 2px solid var(--focus-ring);
      outline-offset: 2px;
    }

    button:focus:not(:focus-visible),
    input:focus:not(:focus-visible),
    select:focus:not(:focus-visible),
    textarea:focus:not(:focus-visible) {
      outline: none;
    }

    .btn-primary {
      background: #1b1f27;
      color: #fff;
      border-color: #1b1f27;
    }

    .btn-ghost {
      background: var(--surface-muted);
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
      min-width: 0;
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
      transition: background var(--duration-fast) var(--ease-out);
    }

    .table-row:hover {
      background: var(--surface-soft);
    }

    .table-row:last-child {
      border-bottom: 0;
    }

    .branches-head,
    .branches-row {
      min-width: 0;
      grid-template-columns: 1.45fr .9fr .85fr 1.15fr .8fr 1.6fr;
    }

    .table-head > div,
    .table-row > div {
      min-width: 0;
    }

    .branches-row > div:nth-child(4) {
      overflow-wrap: anywhere;
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

    .overview-grid {
      display: grid;
      gap: 10px;
      grid-template-columns: 1fr 1fr;
    }

    .overview-card {
      border: 1px solid var(--line);
      border-radius: 10px;
      background: var(--surface-soft);
      padding: 10px;
      display: grid;
      gap: 8px;
    }

    .overview-card h3 {
      margin: 0;
      font-size: 0.96rem;
    }

    .overview-fields {
      display: grid;
      gap: 8px;
      grid-template-columns: 1fr 1fr;
    }

    .overview-field {
      display: grid;
      gap: 2px;
      padding: 8px;
      border: 1px solid var(--line);
      border-radius: 8px;
      background: #fff;
    }

    .overview-field label {
      color: var(--muted);
      font-size: 0.76rem;
      text-transform: uppercase;
      letter-spacing: 0.06em;
      font-weight: 700;
    }

    .overview-field strong {
      font-size: 0.92rem;
      word-break: break-word;
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

    .sql-shell {
      border: 1px solid var(--line);
      border-radius: 12px;
      background: #fff;
      overflow: hidden;
      display: grid;
      grid-template-columns: 270px 1fr;
      min-height: 520px;
    }

    .sql-library {
      border-right: 1px solid var(--line);
      background: #fafbfc;
      padding: 12px;
      display: grid;
      gap: 10px;
      align-content: start;
    }

    .sql-library h3 {
      margin: 0;
      font-size: 1.05rem;
    }

    .sql-library small {
      color: var(--muted);
      font-size: 0.8rem;
    }

    .sql-tabstrip {
      display: inline-flex;
      border: 1px solid var(--line);
      border-radius: 10px;
      overflow: hidden;
      width: fit-content;
      background: #fff;
    }

    .sql-tabstrip button {
      border: 0;
      border-right: 1px solid var(--line);
      border-radius: 0;
      padding: 8px 12px;
      background: #fff;
      font-weight: 600;
      box-shadow: none;
      transform: none;
    }

    .sql-tabstrip button:last-child {
      border-right: 0;
    }

    .sql-tabstrip button.active {
      background: #eff2f6;
    }

    .sql-history {
      list-style: none;
      margin: 0;
      padding: 0;
      display: grid;
      gap: 8px;
      max-height: 420px;
      overflow: auto;
    }

    .sql-history-item {
      border: 1px solid var(--line);
      border-radius: 10px;
      background: #fff;
      padding: 9px;
      display: grid;
      gap: 4px;
      cursor: pointer;
      transition: border-color var(--duration-fast) var(--ease-out), background var(--duration-fast) var(--ease-out);
    }

    .sql-history-item:hover {
      border-color: #c7ccd5;
      background: var(--surface-soft);
    }

    .sql-history-item strong {
      font-size: 0.9rem;
      line-height: 1.35;
    }

    .sql-history-item small {
      color: var(--muted);
      font-size: 0.78rem;
    }

    .sql-workspace {
      display: grid;
      grid-template-rows: auto 1fr auto auto;
      min-height: 520px;
    }

    .sql-toolbar {
      border-bottom: 1px solid var(--line);
      padding: 10px;
      display: grid;
      gap: 8px;
      grid-template-columns: 1.2fr auto auto;
      align-items: center;
    }

    .sql-toolbar .sql-tag {
      border: 1px solid var(--line);
      border-radius: 9px;
      padding: 8px 10px;
      background: #fff;
      color: #2c3440;
      font-weight: 600;
      white-space: nowrap;
    }

    .sql-editor-wrap {
      display: grid;
      grid-template-columns: 46px 1fr;
      min-height: 320px;
      max-height: 520px;
      overflow: hidden;
      border-bottom: 1px solid var(--line);
    }

    .sql-lines {
      margin: 0;
      padding: 10px 8px;
      border-right: 1px solid var(--line);
      background: #f7f9fc;
      color: #68809f;
      font: 500 0.85rem/1.5 "JetBrains Mono", "SF Mono", monospace;
      text-align: right;
      overflow: hidden;
      user-select: none;
      white-space: pre;
    }

    .sql-editor-input {
      border: 0;
      border-radius: 0;
      padding: 10px 12px;
      width: 100%;
      height: 100%;
      resize: none;
      outline: none;
      font: 500 0.98rem/1.5 "JetBrains Mono", "SF Mono", monospace;
      background: #ffffff;
      color: #17304e;
    }

    .sql-status {
      padding: 8px 12px;
      border-bottom: 1px solid var(--line);
      color: var(--muted);
      font-size: 0.9rem;
      display: flex;
      gap: 8px;
      align-items: center;
      justify-content: space-between;
      flex-wrap: wrap;
    }

    .sql-runbar {
      padding: 10px 12px;
      display: flex;
      justify-content: space-between;
      gap: 8px;
      align-items: center;
      flex-wrap: wrap;
      transition: background var(--duration-base) var(--ease-out);
    }

    .sql-write-toggle {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      color: var(--muted);
      font-size: 0.85rem;
      font-weight: 600;
      user-select: none;
      padding: 6px 8px;
      border-radius: 8px;
      transition: background var(--duration-fast) var(--ease-out), color var(--duration-fast) var(--ease-out);
    }

    .sql-write-toggle:hover {
      background: var(--surface-soft);
    }

    .sql-write-toggle input {
      width: 18px;
      height: 18px;
      margin: 0;
      flex: 0 0 auto;
      accent-color: #1b1f27;
    }

    .sql-mode-indicator {
      display: inline-flex;
      align-items: center;
      border: 1px solid var(--line);
      border-radius: 999px;
      padding: 3px 8px;
      font-size: 0.72rem;
      font-weight: 700;
      letter-spacing: 0.07em;
      text-transform: uppercase;
      background: #f7f8fa;
      color: var(--muted);
      white-space: nowrap;
    }

    .sql-mode-indicator.write-enabled {
      border-color: rgba(186, 58, 53, 0.3);
      background: rgba(186, 58, 53, 0.08);
      color: #962d2a;
    }

    .sql-editor-wrap.write-enabled {
      box-shadow: inset 0 0 0 1px rgba(186, 58, 53, 0.22);
      background: #fff8f7;
    }

    .sql-runbar.write-enabled {
      background: rgba(186, 58, 53, 0.06);
    }

    .sql-results {
      padding: 10px 12px;
      border-top: 1px solid var(--line);
      background: #fcfcfd;
      color: #2d3442;
      font-size: 0.86rem;
      min-height: 54px;
    }

    .sql-results-meta {
      display: flex;
      gap: 8px;
      align-items: center;
      flex-wrap: wrap;
      margin-bottom: 8px;
      color: var(--muted);
    }

    .sql-result-table-wrap {
      border: 1px solid var(--line);
      border-radius: 8px;
      overflow: auto;
      max-height: 220px;
      background: #fff;
    }

    .sql-result-table {
      width: 100%;
      border-collapse: collapse;
      font-size: 0.82rem;
    }

    .sql-result-table th,
    .sql-result-table td {
      border-bottom: 1px solid var(--line);
      border-right: 1px solid var(--line);
      padding: 7px 8px;
      text-align: left;
      vertical-align: top;
      white-space: pre-wrap;
      word-break: break-word;
    }

    .sql-result-table th:last-child,
    .sql-result-table td:last-child {
      border-right: 0;
    }

    .sql-result-table tr:last-child td {
      border-bottom: 0;
    }

    .sql-result-table th {
      position: sticky;
      top: 0;
      background: #f5f8fc;
      color: #314154;
      font-weight: 700;
      z-index: 1;
    }

    .sql-result-error {
      color: var(--danger);
      font-weight: 700;
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

    @media (prefers-reduced-motion: reduce) {
      *, *::before, *::after {
        animation-duration: 0.01ms !important;
        animation-iteration-count: 1 !important;
        transition-duration: 0.01ms !important;
      }

      .monitoring-chart::after {
        animation: none;
      }
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
      transition: border-color var(--duration-fast) var(--ease-out), background var(--duration-fast) var(--ease-out);
    }

    .dashboard-branch-item:hover {
      border-color: #c7ccd5;
      background: #ffffff;
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

      .overview-grid,
      .overview-fields {
        grid-template-columns: 1fr;
      }

      .sql-shell {
        grid-template-columns: 1fr;
      }

      .sql-library {
        border-right: 0;
        border-bottom: 1px solid var(--line);
      }

      .sql-toolbar {
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
          <li class="active" data-role="nav-dashboard" data-action="navigate" data-page="dashboard" role="button" tabindex="0" aria-label="Open dashboard">Dashboard</li>
          <li data-role="nav-branches" data-action="navigate" data-page="branches" role="button" tabindex="0" aria-label="Open branches">Branches</li>
        </ul>
      </section>

      <section class="nav-section">
        <h2>Branch</h2>
        <select class="sidebar-select" data-role="sidebar-branch-select"></select>
        <ul class="nav-list">
          <li data-role="nav-branch-overview" data-action="navigate" data-page="branch-overview" role="button" tabindex="0" aria-label="Open branch overview">Overview</li>
          <li data-role="nav-sql-editor" data-action="navigate" data-page="sql-editor" role="button" tabindex="0" aria-label="Open SQL editor">SQL Editor</li>
        </ul>
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

      <section class="page-section is-hidden" data-role="page-branch-overview">
        <article class="panel" data-role="branch-overview-panel">
          <header class="panel-header">
            <h2>Branch overview</h2>
            <p data-role="branch-overview-subtitle">basic branch metadata and connection details</p>
          </header>
          <div class="panel-body">
            <div class="overview-grid">
              <section class="overview-card" data-role="branch-overview-basic">
                <h3>Basic information</h3>
                <div class="overview-fields">
                  <div class="overview-field">
                    <label>Branch</label>
                    <strong data-role="branch-overview-name">main</strong>
                  </div>
                  <div class="overview-field">
                    <label>Parent</label>
                    <strong data-role="branch-overview-parent">-</strong>
                  </div>
                  <div class="overview-field">
                    <label>Created</label>
                    <strong data-role="branch-overview-created">-</strong>
                  </div>
                  <div class="overview-field">
                    <label>Endpoint</label>
                    <strong class="mono" data-role="branch-overview-endpoint">-</strong>
                  </div>
                </div>
              </section>

              <section class="overview-card" data-role="branch-overview-connect">
                <h3>Connect</h3>
                <div class="connect-stack">
                  <div class="cmd-row">
                    <span class="cmd-label mono">DSN</span>
                    <input class="cmd" data-role="branch-overview-dsn" readonly value="Loading branch DSN...">
                    <button class="btn-ghost" data-action="copy-overview-dsn">Copy DSN</button>
                  </div>
                  <div class="cmd-row">
                    <span class="cmd-label mono">psql</span>
                    <input class="cmd" data-role="branch-overview-psql" readonly value="Loading psql command...">
                    <button class="btn-ghost" data-action="copy-overview-psql">Copy psql</button>
                  </div>
                  <div class="cmd-row">
                    <span class="cmd-label mono">Password</span>
                    <input class="cmd" data-role="branch-overview-password" readonly value="Loading password...">
                    <button class="btn-ghost" data-action="copy-overview-password">Copy password</button>
                  </div>
                </div>
              </section>
            </div>
          </div>
        </article>
      </section>

      <section class="page-section is-hidden" data-role="page-sql-editor">
        <div class="sql-shell">
          <aside class="sql-library">
            <h3>SQL Editor</h3>
            <small data-role="sql-editor-branch-label">branch: main</small>
            <div class="sql-tabstrip">
              <button class="active" data-action="sql-tab" data-sql-tab="saved">Saved</button>
              <button data-action="sql-tab" data-sql-tab="history">History</button>
            </div>
            <ul class="sql-history" data-role="sql-history-list"></ul>
          </aside>

          <section class="sql-workspace">
            <div class="sql-toolbar">
              <input data-role="sql-query-title" value="Untitled" aria-label="Query title">
              <button class="btn-ghost" data-action="save-sql">Save</button>
              <span class="sql-tag" data-role="sql-editor-branch-pill">main · endpoint unknown</span>
            </div>

            <div class="sql-editor-wrap">
              <pre class="sql-lines mono" data-role="sql-editor-lines">1</pre>
              <textarea class="sql-editor-input" data-role="sql-editor-input" spellcheck="false">SELECT now();</textarea>
            </div>

            <div class="sql-status">
              <span data-role="sql-editor-status">Ready to connect</span>
              <span class="sql-mode-indicator" data-role="sql-mode-indicator">Read-only</span>
              <button class="btn-ghost" data-action="copy-overview-dsn">Copy branch DSN</button>
            </div>

            <div class="sql-runbar">
              <label class="sql-write-toggle" title="Keep unchecked for safe read-only queries. Enable only when you want to run writes.">
                <input type="checkbox" data-role="sql-allow-writes">
                <span>Enable write queries</span>
              </label>
              <button class="btn-primary" data-action="run-sql">Run</button>
            </div>

            <div class="sql-results" data-role="sql-editor-result">Select a branch, review SQL, then click Run.</div>
          </section>
        </div>
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
      selectedBranch: 'main',
      selectedBranchConnection: null,
      sqlTab: 'saved',
      sqlHistory: [
        {
          id: 'seed-1',
          title: 'show branch tables',
          query: 'SELECT schemaname, tablename FROM pg_tables WHERE schemaname NOT IN (\'pg_catalog\', \'information_schema\') ORDER BY 1,2;',
          saved: true,
          branch: 'main',
          timestamp: 'sample',
        },
      ],
      branchFilter: '',
      currentPage: 'dashboard',
    };

    const refs = {
      pageTitle: document.querySelector('[data-role="page-title"]'),
      pageSubtitle: document.querySelector('[data-role="page-subtitle"]'),
      pageDashboard: document.querySelector('[data-role="page-dashboard"]'),
      pageBranchOverview: document.querySelector('[data-role="page-branch-overview"]'),
      pageSqlEditor: document.querySelector('[data-role="page-sql-editor"]'),
      pageBranches: document.querySelector('[data-role="page-branches"]'),
      navDashboard: document.querySelector('[data-role="nav-dashboard"]'),
      navBranches: document.querySelector('[data-role="nav-branches"]'),
      navBranchOverview: document.querySelector('[data-role="nav-branch-overview"]'),
      navSqlEditor: document.querySelector('[data-role="nav-sql-editor"]'),
      newBranchCTA: document.querySelector('[data-role="new-branch-cta"]'),
      newBranchName: document.querySelector('[data-role="new-branch-name"]'),
      healthPill: document.querySelector('[data-role="health-pill"]'),
      sidebarBranchSelect: document.querySelector('[data-role="sidebar-branch-select"]'),
      publishedCountChip: document.querySelector('[data-role="published-count-chip"]'),
      branchOverviewSubtitle: document.querySelector('[data-role="branch-overview-subtitle"]'),
      branchOverviewName: document.querySelector('[data-role="branch-overview-name"]'),
      branchOverviewParent: document.querySelector('[data-role="branch-overview-parent"]'),
      branchOverviewCreated: document.querySelector('[data-role="branch-overview-created"]'),
      branchOverviewEndpoint: document.querySelector('[data-role="branch-overview-endpoint"]'),
      branchOverviewDSN: document.querySelector('[data-role="branch-overview-dsn"]'),
      branchOverviewPSQL: document.querySelector('[data-role="branch-overview-psql"]'),
      branchOverviewPassword: document.querySelector('[data-role="branch-overview-password"]'),
      sqlEditorBranchLabel: document.querySelector('[data-role="sql-editor-branch-label"]'),
      sqlEditorBranchPill: document.querySelector('[data-role="sql-editor-branch-pill"]'),
      sqlHistoryList: document.querySelector('[data-role="sql-history-list"]'),
      sqlQueryTitle: document.querySelector('[data-role="sql-query-title"]'),
      sqlEditorInput: document.querySelector('[data-role="sql-editor-input"]'),
      sqlEditorLines: document.querySelector('[data-role="sql-editor-lines"]'),
      sqlEditorStatus: document.querySelector('[data-role="sql-editor-status"]'),
      sqlModeIndicator: document.querySelector('[data-role="sql-mode-indicator"]'),
      sqlEditorResult: document.querySelector('[data-role="sql-editor-result"]'),
      sqlAllowWrites: document.querySelector('[data-role="sql-allow-writes"]'),
      sqlRunButton: document.querySelector('[data-action="run-sql"]'),
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
      const nextPage = pageName === 'branches' || pageName === 'branch-overview' || pageName === 'sql-editor' ? pageName : 'dashboard';
      state.currentPage = nextPage;

      const dashboardActive = nextPage === 'dashboard';
      const branchOverviewActive = nextPage === 'branch-overview';
      const sqlEditorActive = nextPage === 'sql-editor';
      const branchesActive = nextPage === 'branches';

      refs.pageDashboard.classList.toggle('is-hidden', !dashboardActive);
      refs.pageBranchOverview.classList.toggle('is-hidden', !branchOverviewActive);
      refs.pageSqlEditor.classList.toggle('is-hidden', !sqlEditorActive);
      refs.pageBranches.classList.toggle('is-hidden', !branchesActive);
      refs.navDashboard.classList.toggle('active', dashboardActive);
      refs.navBranches.classList.toggle('active', branchesActive);
      refs.navBranchOverview.classList.toggle('active', branchOverviewActive);
      refs.navSqlEditor.classList.toggle('active', sqlEditorActive);
      refs.newBranchCTA.classList.toggle('is-hidden', !branchesActive);

      if (dashboardActive) {
        refs.pageTitle.textContent = 'Project dashboard';
        refs.pageSubtitle.textContent = 'View storage and branch health at a glance, then drive branch and endpoint workflows below.';
        return;
      }

      if (branchesActive) {
        refs.pageTitle.textContent = String(state.branches.length) + ' Branches';
        refs.pageSubtitle.textContent = 'Instantly branch your data to deliver faster, safer experimentation and more reliable CI/CD flows.';
        return;
      }

      if (sqlEditorActive) {
        const selectedBranchForSQL = branchByName(state.selectedBranch);
        refs.pageTitle.textContent = 'SQL Editor';
        if (!selectedBranchForSQL) {
          refs.pageSubtitle.textContent = 'Select a branch from the left sidebar to open SQL editor context.';
          return;
        }

        refs.pageSubtitle.textContent = selectedBranchForSQL.name + (selectedBranchForSQL.name === 'main' ? ' (default)' : '') + ' · run queries against this branch endpoint';
        return;
      }

      const selectedBranch = branchByName(state.selectedBranch);
      refs.pageTitle.textContent = 'Branch overview';
      if (!selectedBranch) {
        refs.pageSubtitle.textContent = 'Select a branch from the left sidebar to inspect its details.';
        return;
      }

      refs.pageSubtitle.textContent = selectedBranch.name + (selectedBranch.name === 'main' ? ' (default)' : '') + ' · parent: ' + (selectedBranch.parent || '-');
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

    function branchByName(branchName) {
      for (let i = 0; i < state.branches.length; i += 1) {
        if (state.branches[i].name === branchName) {
          return state.branches[i];
        }
      }
      return null;
    }

    function endpointByBranch(branchName) {
      for (let i = 0; i < state.endpoints.length; i += 1) {
        if (state.endpoints[i].branch === branchName) {
          return state.endpoints[i];
        }
      }
      return null;
    }

    function normalizeSelectedBranch() {
      if (!state.branches.length) {
        state.selectedBranch = '';
        return;
      }

      if (branchByName(state.selectedBranch)) {
        return;
      }

      if (branchByName('main')) {
        state.selectedBranch = 'main';
        return;
      }

      state.selectedBranch = state.branches[0].name;
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
      normalizeSelectedBranch();

      const options = state.branches
        .map((item) => '<option value="' + escapeHTML(item.name) + '">' + escapeHTML(item.name) + '</option>')
        .join('');

      refs.parentSelect.innerHTML = options || '<option value="main">main</option>';

      refs.sidebarBranchSelect.innerHTML = options || '<option value="">no branches</option>';
      refs.sidebarBranchSelect.disabled = state.branches.length === 0;

      if (state.selectedBranch !== '') {
        refs.sidebarBranchSelect.value = state.selectedBranch;
      }

      if (state.branches.some((item) => item.name === 'main')) {
        refs.parentSelect.value = 'main';
      } else if (state.branches.length > 0) {
        refs.parentSelect.value = state.branches[0].name;
      }
    }

    function renderBranchOverview() {
      const selectedBranch = branchByName(state.selectedBranch);
      if (!selectedBranch) {
        refs.branchOverviewSubtitle.textContent = 'select a branch from the left to see details';
        refs.branchOverviewName.textContent = '-';
        refs.branchOverviewParent.textContent = '-';
        refs.branchOverviewCreated.textContent = '-';
        refs.branchOverviewEndpoint.textContent = '-';
        refs.branchOverviewDSN.value = 'No selected branch';
        refs.branchOverviewPSQL.value = 'No selected branch';
        refs.branchOverviewPassword.value = 'No selected branch';
        return;
      }

      refs.branchOverviewSubtitle.textContent = selectedBranch.name + (selectedBranch.name === 'main' ? ' (default)' : '');
      refs.branchOverviewName.textContent = selectedBranch.name;
      refs.branchOverviewParent.textContent = selectedBranch.parent || '-';
      refs.branchOverviewCreated.textContent = formatCreatedAt(selectedBranch.created_at);

      const endpoint = endpointByBranch(selectedBranch.name);
      if (endpoint && endpoint.published && endpoint.port > 0) {
        refs.branchOverviewEndpoint.textContent = (endpoint.host || '127.0.0.1') + ':' + String(endpoint.port) + ' (' + (endpoint.status || 'unknown') + ')';
      } else {
        refs.branchOverviewEndpoint.textContent = 'not published';
      }

      const connection = state.selectedBranchConnection;
      if (!connection || !connection.published || !connection.port) {
        refs.branchOverviewDSN.value = 'Endpoint is not published';
        refs.branchOverviewPSQL.value = 'Endpoint is not published';
        refs.branchOverviewPassword.value = 'Endpoint is not published';
        return;
      }

      refs.branchOverviewDSN.value = connection.dsn || makeDSN(connection);
      refs.branchOverviewPSQL.value = makePSQLCommand(connection);
      refs.branchOverviewPassword.value = getConnectionPassword(connection);
    }

    async function refreshSelectedBranchConnection(silent) {
      if (!state.selectedBranch) {
        state.selectedBranchConnection = null;
        renderBranchOverview();
        renderSQLEditorContext();
        return;
      }

      try {
        const response = await api('GET', '/api/v1/branches/' + encodeURIComponent(state.selectedBranch) + '/connection');
        state.selectedBranchConnection = response.connection || null;
      } catch (err) {
        state.selectedBranchConnection = null;
        if (!silent) {
          showMessage('Failed loading branch overview connection: ' + err.message, 'err');
        }
      }

      renderBranchOverview();
      renderSQLEditorContext();
    }

    function renderSQLEditorLineNumbers() {
      const value = refs.sqlEditorInput.value || '';
      const totalLines = value.split('\n').length;
      const rows = [];
      for (let line = 1; line <= totalLines; line += 1) {
        rows.push(String(line));
      }
      refs.sqlEditorLines.textContent = rows.join('\n');
      refs.sqlEditorLines.scrollTop = refs.sqlEditorInput.scrollTop;
    }

    function formatSQLHistoryTime(value) {
      if (value === 'sample') {
        return 'sample';
      }

      const parsed = new Date(value);
      if (Number.isNaN(parsed.getTime())) {
        return value;
      }

      return parsed.toLocaleString(undefined, {
        month: 'short',
        day: 'numeric',
        hour: 'numeric',
        minute: '2-digit',
      });
    }

    function renderSQLHistory() {
      const selectedBranch = state.selectedBranch || 'main';
      const entries = state.sqlHistory.filter((item) => {
        if (state.sqlTab === 'saved' && !item.saved) {
          return false;
        }
        if (state.sqlTab === 'history' && item.saved) {
          return false;
        }

        if (item.branch === selectedBranch) {
          return true;
        }

        return item.branch === 'main' && selectedBranch === 'main';
      });

      if (!entries.length) {
        refs.sqlHistoryList.innerHTML = '<li class="sql-history-item"><strong>No ' + escapeHTML(state.sqlTab) + ' queries for this branch yet.</strong><small>Run or save a query to populate this list.</small></li>';
        return;
      }

      refs.sqlHistoryList.innerHTML = entries
        .slice(0, 24)
        .map((entry) => {
          const statusSuffix = entry.status ? (' · ' + entry.status) : '';
          return '<li class="sql-history-item" data-action="open-sql-history" data-sql-id="' + escapeHTML(entry.id) + '" role="button" tabindex="0">'
            + '<strong>' + escapeHTML(entry.title) + '</strong>'
            + '<small>' + escapeHTML((entry.branch || selectedBranch) + ' · ' + formatSQLHistoryTime(entry.timestamp) + statusSuffix) + '</small>'
            + '</li>';
        })
        .join('');
    }

    function applySQLModeVisualState() {
      const writeEnabled = Boolean(refs.sqlAllowWrites.checked && !refs.sqlAllowWrites.disabled);
      const editorWrap = document.querySelector('.sql-editor-wrap');
      const runbar = document.querySelector('.sql-runbar');
      if (editorWrap) {
        editorWrap.classList.toggle('write-enabled', writeEnabled);
      }
      if (runbar) {
        runbar.classList.toggle('write-enabled', writeEnabled);
      }

      if (refs.sqlRunButton) {
        refs.sqlRunButton.classList.toggle('btn-danger', writeEnabled);
        refs.sqlRunButton.classList.toggle('btn-primary', !writeEnabled);
      }

      if (!refs.sqlModeIndicator) {
        return;
      }

      refs.sqlModeIndicator.classList.toggle('write-enabled', writeEnabled);
      refs.sqlModeIndicator.textContent = writeEnabled ? 'Write mode' : 'Read-only';
    }

    function setSQLTab(tabName) {
      state.sqlTab = tabName === 'history' ? 'history' : 'saved';
      document.querySelectorAll('[data-action="sql-tab"]').forEach((node) => {
        node.classList.toggle('active', node.getAttribute('data-sql-tab') === state.sqlTab);
      });
      renderSQLHistory();
    }

    function renderSQLEditorContext() {
      const selectedBranch = branchByName(state.selectedBranch);
      if (!selectedBranch) {
        refs.sqlEditorBranchLabel.textContent = 'branch: (none)';
        refs.sqlEditorBranchPill.textContent = 'no branch selected';
        refs.sqlEditorStatus.textContent = 'Select a branch to prepare SQL connection context';
        refs.sqlRunButton.disabled = true;
        refs.sqlAllowWrites.disabled = true;
        refs.sqlAllowWrites.checked = false;
        applySQLModeVisualState();
        return;
      }

      refs.sqlEditorBranchLabel.textContent = 'branch: ' + selectedBranch.name;

      const endpoint = endpointByBranch(selectedBranch.name);
      const endpointStatus = endpoint && endpoint.published ? (endpoint.status || 'published') : 'unpublished';
      refs.sqlEditorBranchPill.textContent = selectedBranch.name + ' · ' + endpointStatus;

      const connection = state.selectedBranchConnection;
      if (!connection || !connection.published || !connection.port) {
        refs.sqlEditorStatus.textContent = 'Endpoint is not published for this branch yet';
        refs.sqlRunButton.disabled = true;
        refs.sqlAllowWrites.disabled = true;
        refs.sqlAllowWrites.checked = false;
        applySQLModeVisualState();
        return;
      }

      refs.sqlEditorStatus.textContent = 'Ready to connect · ' + (connection.host || '127.0.0.1') + ':' + String(connection.port);
      refs.sqlRunButton.disabled = false;
      refs.sqlAllowWrites.disabled = false;
      applySQLModeVisualState();
    }

    function appendSQLHistoryEntry(title, query, branchName, saved, status) {
      state.sqlHistory.unshift({
        id: (saved ? 'saved-' : 'run-') + Date.now() + '-' + Math.floor(Math.random() * 1000),
        title,
        query,
        saved,
        branch: branchName,
        status,
        timestamp: new Date().toISOString(),
      });
      renderSQLHistory();
    }

    function formatSQLResultCell(value) {
      if (value === null || value === undefined) {
        return '<span class="mono">NULL</span>';
      }

      if (typeof value === 'object') {
        return escapeHTML(JSON.stringify(value));
      }

      return escapeHTML(String(value));
    }

    function renderSQLResultSuccess(result) {
      const columns = Array.isArray(result.columns) ? result.columns : [];
      const rows = Array.isArray(result.rows) ? result.rows : [];
      const commandTag = result.command_tag || 'QUERY';
      const durationMS = Number(result.duration_ms || 0);
      const rowCount = Number(result.row_count || rows.length);
      const truncated = Boolean(result.truncated);

      const meta = '<div class="sql-results-meta">'
        + '<span><strong>' + escapeHTML(commandTag) + '</strong></span>'
        + '<span>' + escapeHTML(String(rowCount)) + ' rows</span>'
        + '<span>' + escapeHTML(String(durationMS)) + ' ms</span>'
        + (truncated ? '<span class="badge warn">Truncated</span>' : '')
        + '</div>';

      if (!columns.length) {
        refs.sqlEditorResult.innerHTML = meta + '<div>No row data returned.</div>';
        return;
      }

      const header = columns
        .map((column) => '<th>' + escapeHTML(column.name) + '<br><small>' + escapeHTML(column.type || '') + '</small></th>')
        .join('');

      const bodyRows = rows
        .map((row) => {
          const cells = columns
            .map((_, index) => {
              const value = index < row.length ? row[index] : null;
              return '<td>' + formatSQLResultCell(value) + '</td>';
            })
            .join('');

          return '<tr>' + cells + '</tr>';
        })
        .join('');

      refs.sqlEditorResult.innerHTML = meta
        + '<div class="sql-result-table-wrap">'
        + '<table class="sql-result-table">'
        + '<thead><tr>' + header + '</tr></thead>'
        + '<tbody>' + bodyRows + '</tbody>'
        + '</table>'
        + '</div>';
    }

    function renderSQLResultError(message) {
      refs.sqlEditorResult.innerHTML = '<div class="sql-result-error">' + escapeHTML(message) + '</div>';
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
          const isSelected = branchName === state.selectedBranch;

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
          const selectedBadge = isSelected ? ' <span class="badge ok">Selected</span>' : '';
          const createdAt = formatCreatedAt(item.created_at);
          const branchPrefix = isRootBranch ? '' : '<span class="branch-prefix mono">|-</span>';
          const parentLabel = isRootBranch ? '-' : (item.parent || '-');

          return '<div class="table-row branches-row">'
            + '<div class="cell-strong">' + branchPrefix + escapeHTML(branchName) + defaultBadge + selectedBadge + '</div>'
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

        renderBranchSelectors();
        await refreshSelectedBranchConnection(true);

        setPage(state.currentPage);
        renderStats();
        renderDashboardBranches();
        renderBranches();
        renderEndpoints();
        renderSQLHistory();
        renderSQLEditorLineNumbers();
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

        if (action === 'sql-tab') {
          setSQLTab(actionTarget.getAttribute('data-sql-tab'));
          return;
        }

        if (action === 'open-sql-history') {
          const sqlID = actionTarget.getAttribute('data-sql-id');
          const entry = state.sqlHistory.find((item) => item.id === sqlID);
          if (!entry) {
            return;
          }

          refs.sqlQueryTitle.value = entry.title;
          refs.sqlEditorInput.value = entry.query;
          renderSQLEditorLineNumbers();
          setPage('sql-editor');
          showMessage('Loaded query from ' + state.sqlTab + ' list.', 'ok');
          return;
        }

        if (action === 'save-sql') {
          const title = refs.sqlQueryTitle.value.trim() || 'Untitled query';
          const query = refs.sqlEditorInput.value;
          const branchName = state.selectedBranch || 'main';
          appendSQLHistoryEntry(title, query, branchName, true, 'saved');
          setSQLTab('saved');
          showMessage('Query saved locally for ' + branchName + '.', 'ok');
          return;
        }

        if (action === 'run-sql') {
          const branchName = state.selectedBranch || 'main';
          const connection = state.selectedBranchConnection;
          if (!connection || !connection.published || !connection.port) {
            throw new Error('branch endpoint is not published');
          }

          const query = refs.sqlEditorInput.value.trim();
          if (!query) {
            throw new Error('query is empty');
          }

          const title = refs.sqlQueryTitle.value.trim() || 'Untitled query';
          const allowWrites = Boolean(refs.sqlAllowWrites.checked);
          refs.sqlRunButton.disabled = true;
          refs.sqlAllowWrites.disabled = true;
          refs.sqlEditorStatus.textContent = 'Running query on ' + branchName + '...';
          refs.sqlEditorResult.textContent = 'Running query...';

          try {
            const response = await api('POST', '/api/v1/branches/' + encodeURIComponent(branchName) + '/sql/execute', {
              sql: query,
              allow_writes: allowWrites,
            });
            const result = response.result || {};
            renderSQLResultSuccess(result);
            appendSQLHistoryEntry(title, query, branchName, false, 'ok');
            const modeLabel = result.read_only ? 'read-only' : 'write-enabled';
            refs.sqlEditorStatus.textContent = 'Last run: ' + (result.command_tag || 'QUERY') + ' · ' + String(result.duration_ms || 0) + ' ms · ' + modeLabel;
            showMessage('Query executed on ' + branchName + '.', 'ok');
          } catch (runErr) {
            renderSQLResultError(runErr.message || 'sql execution failed');
            appendSQLHistoryEntry(title, query, branchName, false, 'error');
            refs.sqlEditorStatus.textContent = 'Execution failed';
            showMessage('SQL execution failed: ' + runErr.message, 'err');
          } finally {
            renderSQLHistory();
            renderSQLEditorContext();
          }
          return;
        }

        if (action === 'copy-branch-dsn') {
          if (branch && branch !== state.selectedBranch) {
            state.selectedBranch = branch;
            renderBranchSelectors();
            await refreshSelectedBranchConnection(true);
            renderBranches();
          }

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

        if (action === 'copy-overview-dsn') {
          const value = state.currentPage === 'sql-editor' ? (state.selectedBranchConnection && state.selectedBranchConnection.dsn ? state.selectedBranchConnection.dsn : refs.branchOverviewDSN.value) : refs.branchOverviewDSN.value;
          await copyTextToClipboard(value);
          showMessage('Branch DSN copied.', 'ok');
          return;
        }

        if (action === 'copy-overview-psql') {
          await copyTextToClipboard(refs.branchOverviewPSQL.value);
          showMessage('Branch overview psql command copied.', 'ok');
          return;
        }

        if (action === 'copy-overview-password') {
          await copyTextToClipboard(refs.branchOverviewPassword.value);
          showMessage('Branch overview password copied.', 'ok');
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

    async function onSidebarBranchSelectChange(event) {
      state.selectedBranch = event.target.value.trim();
      renderSQLHistory();
      setPage('branch-overview');
      renderBranches();
      await refreshSelectedBranchConnection(false);
    }

    function onActionKeydown(event) {
      if (event.key !== 'Enter' && event.key !== ' ') {
        return;
      }

      const actionTarget = event.target.closest('[data-action]');
      if (!actionTarget) {
        return;
      }

      event.preventDefault();
      actionTarget.click();
    }

    document.addEventListener('click', onPanelClick);
    document.addEventListener('keydown', onActionKeydown);
    document.querySelector('[data-action="create-branch"]').addEventListener('submit', onCreateBranchSubmit);
    refs.branchFilter.addEventListener('input', onBranchFilterInput);
    refs.sidebarBranchSelect.addEventListener('change', onSidebarBranchSelectChange);
    refs.sqlAllowWrites.addEventListener('change', renderSQLEditorContext);
    refs.sqlEditorInput.addEventListener('input', renderSQLEditorLineNumbers);
    refs.sqlEditorInput.addEventListener('scroll', function onSQLEditorScroll() {
      refs.sqlEditorLines.scrollTop = refs.sqlEditorInput.scrollTop;
    });

    setSQLTab('saved');
    renderSQLEditorLineNumbers();
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
