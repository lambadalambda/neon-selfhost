#!/usr/bin/env bash
set -euo pipefail

AUTH_USER="${BASIC_AUTH_USER:-admin}"
AUTH_PASSWORD="${BASIC_AUTH_PASSWORD:-change-me}"
BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
DB_PASSWORD="${DB_PASSWORD:-cloud_admin}"
SEED_DATABASE="${SEED_DATABASE:-branch_lab}"
ALLOW_REMOTE_RESET="${ALLOW_REMOTE_RESET:-0}"
CURL_CONNECT_TIMEOUT="${CURL_CONNECT_TIMEOUT:-5}"
CURL_MAX_TIME="${CURL_MAX_TIME:-30}"

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"

MANAGE_STACK=false
KEEP_STACK=false
VERIFY_BRANCHING=true
KEEP_VERIFY_BRANCH=false
FORCE_REMOTE=false

VERIFY_BRANCH_NAME=""
CURRENT_BRANCH="main"

usage() {
  cat <<'EOF'
Usage: ./scripts/reset_seed_data.sh [--seed-only] [--manage-stack] [--keep-stack] [--keep-verify-branch] [--force]

Options:
  --seed-only           Reset and seed only (skip branch isolation verification).
  --manage-stack        Start and stop `docker compose --profile neon` for this run.
  --keep-stack          Keep compose stack running at end (requires --manage-stack).
  --keep-verify-branch  Keep verification branch (requires verify mode).
  --force               Allow destructive reset against non-local BASE_URL.
  --help                Show this help.

Environment:
  BASIC_AUTH_USER       Controller basic auth username (default: admin)
  BASIC_AUTH_PASSWORD   Controller basic auth password (default: change-me)
  BASE_URL              Controller base URL (default: http://127.0.0.1:8080)
  ALLOW_REMOTE_RESET    Set to 1 to allow destructive reset on non-local BASE_URL.
  DB_PASSWORD           SQL password for endpoint user (default: cloud_admin)
  SEED_DATABASE         Database to drop/create/seed (default: branch_lab)
  CURL_CONNECT_TIMEOUT  Curl connect timeout in seconds (default: 5)
  CURL_MAX_TIME         Curl request timeout in seconds (default: 30)
EOF
}

log() {
  printf '[db-reset] %s\n' "$*"
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    log "missing required command: $1"
    exit 1
  fi
}

compose() {
  BASIC_AUTH_PASSWORD="${AUTH_PASSWORD}" docker compose --project-directory "${REPO_ROOT}" --profile neon "$@"
}

extract_base_host() {
  local raw_url="$1"
  local no_scheme authority

  no_scheme="${raw_url#*://}"
  authority="${no_scheme%%/*}"
  authority="${authority%%\?*}"
  authority="${authority%%\#*}"
  authority="${authority##*@}"

  if [[ -z "${authority}" ]]; then
    return 1
  fi

  if [[ "${authority}" == \[* ]]; then
    authority="${authority#\[}"
    printf '%s\n' "${authority%%]*}"
    return 0
  fi

  printf '%s\n' "${authority%%:*}"
}

is_local_base_url() {
  local host
  if ! host="$(extract_base_host "${BASE_URL}")"; then
    return 1
  fi

  case "${host}" in
    localhost|127.0.0.1|::1)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

api_json() {
  local method="$1"
  local path="$2"
  local payload="${3-}"

  local body_file
  body_file="$(mktemp)"

  local status curl_rc
  if [[ -n "${payload}" ]]; then
    set +e
    status="$(curl -sS -o "${body_file}" -w '%{http_code}' \
      --connect-timeout "${CURL_CONNECT_TIMEOUT}" \
      --max-time "${CURL_MAX_TIME}" \
      -u "${AUTH_USER}:${AUTH_PASSWORD}" \
      -H 'Accept: application/json' \
      -H 'Content-Type: application/json' \
      -X "${method}" \
      "${BASE_URL}${path}" \
      -d "${payload}")"
    curl_rc=$?
    set -e
  else
    set +e
    status="$(curl -sS -o "${body_file}" -w '%{http_code}' \
      --connect-timeout "${CURL_CONNECT_TIMEOUT}" \
      --max-time "${CURL_MAX_TIME}" \
      -u "${AUTH_USER}:${AUTH_PASSWORD}" \
      -H 'Accept: application/json' \
      -X "${method}" \
      "${BASE_URL}${path}")"
    curl_rc=$?
    set -e
  fi

  if [[ ${curl_rc} -ne 0 ]]; then
    log "request transport failed: ${method} ${path} (curl exit ${curl_rc})" >&2
    rm -f "${body_file}"
    return 1
  fi

  if [[ "${status}" != 2* ]]; then
    log "request failed: ${method} ${path} (HTTP ${status})" >&2
    cat "${body_file}" >&2
    rm -f "${body_file}"
    return 1
  fi

  cat "${body_file}"
  rm -f "${body_file}"
}

wait_for_controller() {
  local attempt
  for attempt in $(seq 1 90); do
    if curl -fsS \
      --connect-timeout "${CURL_CONNECT_TIMEOUT}" \
      --max-time "${CURL_MAX_TIME}" \
      -u "${AUTH_USER}:${AUTH_PASSWORD}" \
      "${BASE_URL}/api/v1/status" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  log "controller did not become ready at ${BASE_URL}"
  return 1
}

wait_for_ready_branch() {
  local expected_branch="$1"
  local attempt
  for attempt in $(seq 1 150); do
    local connection_json
    if ! connection_json="$(api_json GET /api/v1/endpoints/primary/connection)"; then
      sleep 1
      continue
    fi

    if jq -e --arg branch "${expected_branch}" '.connection.branch == $branch and .connection.ready == true' >/dev/null <<<"${connection_json}"; then
      CURRENT_BRANCH="${expected_branch}"
      return 0
    fi
    sleep 1
  done

  log "primary endpoint did not become ready on branch ${expected_branch}"
  return 1
}

wait_for_sql_ready() {
  local database_name="$1"
  local uri attempt
  uri="$(db_uri_for "${database_name}")"

  for attempt in $(seq 1 90); do
    if PGPASSWORD="${DB_PASSWORD}" psql "${uri}" -v ON_ERROR_STOP=1 -qAt -c 'SELECT 1;' >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  log "sql endpoint did not become ready for ${database_name}"
  return 1
}

switch_branch() {
  local branch_name="$1"
  local attempt
  for attempt in $(seq 1 5); do
    if api_json POST /api/v1/endpoints/primary/switch "{\"branch\":\"${branch_name}\"}" >/dev/null; then
      if wait_for_ready_branch "${branch_name}"; then
        return 0
      fi
    fi

    log "switch to ${branch_name} attempt ${attempt}/5 failed, retrying"
    sleep 2
  done

  log "failed to switch primary endpoint to ${branch_name} after retries"
  return 1
}

db_uri_for() {
  local database_name="$1"
  local connection_json
  connection_json="$(api_json GET /api/v1/endpoints/primary/connection)"

  local host port user
  host="$(jq -r '.connection.host // "127.0.0.1"' <<<"${connection_json}")"
  port="$(jq -r '.connection.port // 55433' <<<"${connection_json}")"
  user="$(jq -r '.connection.user // "cloud_admin"' <<<"${connection_json}")"

  printf 'postgresql://%s@%s:%s/%s?sslmode=disable' "${user}" "${host}" "${port}" "${database_name}"
}

psql_exec() {
  local database_name="$1"
  local sql_text="$2"
  local uri attempt
  uri="$(db_uri_for "${database_name}")"

  for attempt in $(seq 1 5); do
    if PGPASSWORD="${DB_PASSWORD}" psql "${uri}" -v ON_ERROR_STOP=1 -qAt -c "${sql_text}"; then
      return 0
    fi

    log "psql command attempt ${attempt}/5 failed against ${database_name}, retrying"
    sleep 2
  done

  log "psql command failed after retries against ${database_name}"
  return 1
}

psql_exec_file() {
  local database_name="$1"
  local sql_text="$2"
  local uri attempt sql_file
  uri="$(db_uri_for "${database_name}")"

  sql_file="$(mktemp)"
  printf '%s\n' "${sql_text}" >"${sql_file}"

  for attempt in $(seq 1 5); do
    if PGPASSWORD="${DB_PASSWORD}" psql "${uri}" -v ON_ERROR_STOP=1 -f "${sql_file}"; then
      rm -f "${sql_file}"
      return 0
    fi

    log "psql file execution attempt ${attempt}/5 failed against ${database_name}, retrying"
    sleep 2
  done

  rm -f "${sql_file}"
  log "psql file execution failed after retries against ${database_name}"
  return 1
}

cleanup() {
  set +e

  if [[ -n "${VERIFY_BRANCH_NAME}" && "${KEEP_VERIFY_BRANCH}" != "true" ]]; then
    if [[ "${CURRENT_BRANCH}" == "${VERIFY_BRANCH_NAME}" ]]; then
      api_json POST /api/v1/endpoints/primary/switch '{"branch":"main"}' >/dev/null 2>&1 || true
      CURRENT_BRANCH="main"
    fi
    api_json DELETE "/api/v1/branches/${VERIFY_BRANCH_NAME}" >/dev/null 2>&1 || true
  fi

  if [[ "${MANAGE_STACK}" == "true" && "${KEEP_STACK}" != "true" ]]; then
    log "stopping compose stack"
    compose down >/dev/null 2>&1 || true
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --seed-only)
      VERIFY_BRANCHING=false
      ;;
    --manage-stack)
      MANAGE_STACK=true
      ;;
    --keep-stack)
      KEEP_STACK=true
      ;;
    --keep-verify-branch)
      KEEP_VERIFY_BRANCH=true
      ;;
    --force)
      FORCE_REMOTE=true
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      log "unknown argument: $1"
      usage
      exit 1
      ;;
  esac
  shift
done

if [[ "${KEEP_STACK}" == "true" && "${MANAGE_STACK}" != "true" ]]; then
  log "--keep-stack requires --manage-stack"
  exit 1
fi

if [[ "${KEEP_VERIFY_BRANCH}" == "true" && "${VERIFY_BRANCHING}" != "true" ]]; then
  log "--keep-verify-branch requires verify mode (omit --seed-only)"
  exit 1
fi

if [[ ! "${SEED_DATABASE}" =~ ^[a-zA-Z_][a-zA-Z0-9_]*$ ]]; then
  log "SEED_DATABASE must be a simple SQL identifier, got: ${SEED_DATABASE}"
  exit 1
fi

if [[ ! "${CURL_CONNECT_TIMEOUT}" =~ ^[0-9]+$ || ! "${CURL_MAX_TIME}" =~ ^[0-9]+$ || "${CURL_CONNECT_TIMEOUT}" -lt 1 || "${CURL_MAX_TIME}" -lt 1 ]]; then
  log "CURL_CONNECT_TIMEOUT and CURL_MAX_TIME must be positive integers"
  exit 1
fi

if ! is_local_base_url; then
  if [[ "${ALLOW_REMOTE_RESET}" != "1" && "${FORCE_REMOTE}" != "true" ]]; then
    log "refusing destructive reset on non-local BASE_URL: ${BASE_URL}"
    log "set ALLOW_REMOTE_RESET=1 or pass --force to override"
    exit 1
  fi

  log "WARNING: running destructive reset against non-local BASE_URL: ${BASE_URL}"
fi

require_command curl
require_command jq
require_command psql

if [[ "${MANAGE_STACK}" == "true" ]]; then
  require_command docker
fi

trap cleanup EXIT

if [[ "${MANAGE_STACK}" == "true" ]]; then
  log "starting compose stack"
  compose up -d --build >/dev/null
fi

log "waiting for controller"
wait_for_controller

log "ensuring primary endpoint is running on main"
switch_branch "main"
wait_for_sql_ready postgres

target_connection_json="$(api_json GET /api/v1/endpoints/primary/connection)"
target_branch="$(jq -r '.connection.branch // "unknown"' <<<"${target_connection_json}")"
target_host="$(jq -r '.connection.host // "127.0.0.1"' <<<"${target_connection_json}")"
target_port="$(jq -r '.connection.port // 55433' <<<"${target_connection_json}")"
target_user="$(jq -r '.connection.user // "cloud_admin"' <<<"${target_connection_json}")"

log "reset target: base_url=${BASE_URL} branch=${target_branch} endpoint=${target_host}:${target_port} user=${target_user} database=${SEED_DATABASE}"

log "resetting database ${SEED_DATABASE} on main"
psql_exec postgres "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '${SEED_DATABASE}' AND pid <> pg_backend_pid();"
psql_exec postgres "DROP DATABASE IF EXISTS ${SEED_DATABASE};"
psql_exec postgres "CREATE DATABASE ${SEED_DATABASE};"

seed_sql="$(cat <<'SQL'
CREATE SCHEMA app;

CREATE TABLE app.accounts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  tier TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE app.documents (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  account_id BIGINT NOT NULL REFERENCES app.accounts(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  body TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO app.accounts (slug, tier)
VALUES
  ('acme', 'pro'),
  ('globex', 'starter'),
  ('initech', 'enterprise');

INSERT INTO app.documents (account_id, title, body)
VALUES
  (1, 'Runbook', 'Production runbook baseline'),
  (1, 'Incident Notes', 'Postmortem notes and action items'),
  (2, 'Roadmap', 'Quarterly roadmap snapshot'),
  (3, 'Architecture', 'System architecture overview');
SQL
)"

psql_exec_file "${SEED_DATABASE}" "${seed_sql}"
psql_exec postgres "ALTER DATABASE ${SEED_DATABASE} SET search_path = app, public;"

baseline_docs="$(psql_exec "${SEED_DATABASE}" 'SELECT count(*) FROM app.documents;')"
baseline_accounts="$(psql_exec "${SEED_DATABASE}" 'SELECT count(*) FROM app.accounts;')"

log "seed complete: ${baseline_accounts} accounts, ${baseline_docs} documents"

if [[ "${VERIFY_BRANCHING}" != "true" ]]; then
  log "seed-only mode complete (branch isolation verification skipped)"
  log "next step: create a branch in UI, switch to it, mutate app.documents, then switch back to main"
  exit 0
fi

VERIFY_BRANCH_NAME="verify-$(date -u +%Y%m%d%H%M%S)-${RANDOM}"
log "creating verification branch ${VERIFY_BRANCH_NAME}"
api_json POST /api/v1/branches "{\"name\":\"${VERIFY_BRANCH_NAME}\",\"parent\":\"main\"}" >/dev/null

log "switching to verification branch ${VERIFY_BRANCH_NAME}"
switch_branch "${VERIFY_BRANCH_NAME}"
wait_for_sql_ready "${SEED_DATABASE}"

log "mutating data on ${VERIFY_BRANCH_NAME}"
psql_exec "${SEED_DATABASE}" "DELETE FROM app.documents WHERE account_id = 1;"
psql_exec "${SEED_DATABASE}" "UPDATE app.accounts SET tier = 'suspended' WHERE slug = 'globex';"

verify_docs="$(psql_exec "${SEED_DATABASE}" 'SELECT count(*) FROM app.documents;')"
verify_tier="$(psql_exec "${SEED_DATABASE}" "SELECT tier FROM app.accounts WHERE slug = 'globex';")"

if [[ "${verify_docs}" == "${baseline_docs}" ]]; then
  log "verification branch did not diverge: document count stayed ${verify_docs}"
  exit 1
fi

if [[ "${verify_tier}" != "suspended" ]]; then
  log "verification branch mutation did not apply as expected"
  exit 1
fi

log "switching back to main and validating isolation"
switch_branch "main"
wait_for_sql_ready "${SEED_DATABASE}"

main_docs_after="$(psql_exec "${SEED_DATABASE}" 'SELECT count(*) FROM app.documents;')"
main_tier_after="$(psql_exec "${SEED_DATABASE}" "SELECT tier FROM app.accounts WHERE slug = 'globex';")"

if [[ "${main_docs_after}" != "${baseline_docs}" ]]; then
  log "isolation check failed: main documents count is ${main_docs_after}, expected ${baseline_docs}"
  exit 1
fi

if [[ "${main_tier_after}" != "starter" ]]; then
  log "isolation check failed: main account tier is ${main_tier_after}, expected starter"
  exit 1
fi

if [[ "${KEEP_VERIFY_BRANCH}" == "true" ]]; then
  log "branch isolation verified; kept branch ${VERIFY_BRANCH_NAME} for manual inspection"
else
  log "branch isolation verified; verification branch will be deleted during cleanup"
fi
