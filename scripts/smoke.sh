#!/usr/bin/env bash
set -euo pipefail

AUTH_USER="${BASIC_AUTH_USER:-admin}"
AUTH_PASSWORD="${BASIC_AUTH_PASSWORD:-change-me}"
BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"

MANAGE_STACK=false
KEEP_STACK=false

ORIGINAL_BRANCH=""
CREATED_BRANCH=""
RESTORE_BRANCH=""

usage() {
  cat <<'EOF'
Usage: ./scripts/smoke.sh [--manage-stack] [--keep-stack]

Options:
  --manage-stack  Start and stop `docker compose --profile neon` for the smoke run.
  --keep-stack    Keep the stack running at the end (only with --manage-stack).
  --help          Show this help.

Environment:
  BASIC_AUTH_USER      Controller basic auth username (default: admin)
  BASIC_AUTH_PASSWORD  Controller basic auth password (default: change-me)
  BASE_URL             Controller base URL (default: http://127.0.0.1:8080)
EOF
}

log() {
  printf '[smoke] %s\n' "$*"
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    log "missing required command: $1"
    exit 1
  fi
}

compose() {
  BASIC_AUTH_PASSWORD="${AUTH_PASSWORD}" docker compose --profile neon "$@"
}

api_json() {
  local method="$1"
  local path="$2"
  local payload="${3-}"

  local body_file
  body_file="$(mktemp)"

  local status
  if [[ -n "${payload}" ]]; then
    status="$(curl -sS -o "${body_file}" -w '%{http_code}' \
      -u "${AUTH_USER}:${AUTH_PASSWORD}" \
      -H 'Accept: application/json' \
      -H 'Content-Type: application/json' \
      -X "${method}" \
      "${BASE_URL}${path}" \
      -d "${payload}")"
  else
    status="$(curl -sS -o "${body_file}" -w '%{http_code}' \
      -u "${AUTH_USER}:${AUTH_PASSWORD}" \
      -H 'Accept: application/json' \
      -X "${method}" \
      "${BASE_URL}${path}")"
  fi

  if [[ "${status}" != 2* ]]; then
    log "request failed: ${method} ${path} (HTTP ${status})"
    cat "${body_file}"
    rm -f "${body_file}"
    return 1
  fi

  cat "${body_file}"
  rm -f "${body_file}"
}

assert_jq() {
  local json="$1"
  local filter="$2"
  local message="$3"

  if ! jq -e "${filter}" >/dev/null <<<"${json}"; then
    log "assertion failed: ${message}"
    log "json payload: ${json}"
    exit 1
  fi
}

wait_for_controller() {
  local attempt
  for attempt in $(seq 1 90); do
    if curl -fsS -u "${AUTH_USER}:${AUTH_PASSWORD}" "${BASE_URL}/api/v1/status" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  log "controller did not become ready at ${BASE_URL}"
  return 1
}

wait_for_ready_branch() {
  local branch_name="$1"
  local attempt
  for attempt in $(seq 1 120); do
    local connection_json
    connection_json="$(api_json GET /api/v1/endpoints/primary/connection)"
    if jq -e --arg branch "${branch_name}" '.connection.branch == $branch and .connection.ready == true' >/dev/null <<<"${connection_json}"; then
      return 0
    fi
    sleep 1
  done

  log "endpoint did not become ready on branch ${branch_name}"
  return 1
}

cleanup() {
  set +e

  if [[ -n "${ORIGINAL_BRANCH}" ]]; then
    api_json POST /api/v1/endpoints/primary/switch "{\"branch\":\"${ORIGINAL_BRANCH}\"}" >/dev/null 2>&1 || true
  fi

  if [[ -n "${RESTORE_BRANCH}" ]]; then
    api_json DELETE "/api/v1/branches/${RESTORE_BRANCH}" >/dev/null 2>&1 || true
  fi

  if [[ -n "${CREATED_BRANCH}" ]]; then
    api_json DELETE "/api/v1/branches/${CREATED_BRANCH}" >/dev/null 2>&1 || true
  fi

  if [[ "${MANAGE_STACK}" == "true" && "${KEEP_STACK}" != "true" ]]; then
    log "stopping compose stack"
    compose down >/dev/null 2>&1 || true
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --manage-stack)
      MANAGE_STACK=true
      ;;
    --keep-stack)
      KEEP_STACK=true
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

require_command curl
require_command jq

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

log "checking status endpoint"
status_json="$(api_json GET /api/v1/status)"
assert_jq "${status_json}" '.status == "ok"' "status endpoint should report ok"

health_json="$(api_json GET /api/v1/health)"
assert_jq "${health_json}" '.checks | length >= 3' "health endpoint should include component checks"

connection_json="$(api_json GET /api/v1/endpoints/primary/connection)"
ORIGINAL_BRANCH="$(jq -r '.connection.branch // "main"' <<<"${connection_json}")"
if [[ -z "${ORIGINAL_BRANCH}" || "${ORIGINAL_BRANCH}" == "null" ]]; then
  ORIGINAL_BRANCH="main"
fi

initial_branches_json="$(api_json GET /api/v1/branches)"
if ! jq -e --arg branch "${ORIGINAL_BRANCH}" '.branches | any(.name == $branch)' >/dev/null <<<"${initial_branches_json}"; then
  ORIGINAL_BRANCH="main"
fi

RUN_ID="$(date -u +%Y%m%d%H%M%S)-${RANDOM}"
CREATED_BRANCH="smoke-${RUN_ID}"
RESTORE_BRANCH="restore-${RUN_ID}"

log "creating branch ${CREATED_BRANCH}"
create_json="$(api_json POST /api/v1/branches "{\"name\":\"${CREATED_BRANCH}\",\"parent\":\"main\"}")"
if ! jq -e --arg branch "${CREATED_BRANCH}" '.branch.name == $branch' >/dev/null <<<"${create_json}"; then
  log "create branch response did not include ${CREATED_BRANCH}"
  log "json payload: ${create_json}"
  exit 1
fi

branches_json="$(api_json GET /api/v1/branches)"
if ! jq -e --arg branch "${CREATED_BRANCH}" '.branches | any(.name == $branch)' >/dev/null <<<"${branches_json}"; then
  log "created branch ${CREATED_BRANCH} not found in branch list"
  log "json payload: ${branches_json}"
  exit 1
fi

log "switching primary endpoint to ${CREATED_BRANCH}"
switch_json="$(api_json POST /api/v1/endpoints/primary/switch "{\"branch\":\"${CREATED_BRANCH}\"}")"
if ! jq -e --arg branch "${CREATED_BRANCH}" '.connection.branch == $branch' >/dev/null <<<"${switch_json}"; then
  log "switch response did not target branch ${CREATED_BRANCH}"
  log "json payload: ${switch_json}"
  exit 1
fi

wait_for_ready_branch "${CREATED_BRANCH}"

RESTORE_TIMESTAMP="$(jq -nr 'now - 5 | todateiso8601')"
log "restoring main at ${RESTORE_TIMESTAMP} into ${RESTORE_BRANCH}"
restore_json="$(api_json POST /api/v1/restore "{\"name\":\"${RESTORE_BRANCH}\",\"source_branch\":\"main\",\"timestamp\":\"${RESTORE_TIMESTAMP}\"}")"
if ! jq -e --arg branch "${RESTORE_BRANCH}" '.restore.branch.name == $branch' >/dev/null <<<"${restore_json}"; then
  log "restore response did not include branch ${RESTORE_BRANCH}"
  log "json payload: ${restore_json}"
  exit 1
fi

assert_jq "${restore_json}" '.restore.resolved_lsn != ""' "restore response should include resolved_lsn"

branches_json="$(api_json GET /api/v1/branches)"
if ! jq -e --arg branch "${RESTORE_BRANCH}" '.branches | any(.name == $branch)' >/dev/null <<<"${branches_json}"; then
  log "restore branch ${RESTORE_BRANCH} not found in branch list"
  log "json payload: ${branches_json}"
  exit 1
fi

operations_json="$(api_json GET /api/v1/operations)"
assert_jq "${operations_json}" '.operations | length > 0' "operation log should contain entries"

log "switching primary endpoint back to ${ORIGINAL_BRANCH}"
if ! api_json POST /api/v1/endpoints/primary/switch "{\"branch\":\"${ORIGINAL_BRANCH}\"}" >/dev/null; then
  if [[ "${ORIGINAL_BRANCH}" != "main" ]]; then
    log "switching back to ${ORIGINAL_BRANCH} failed, falling back to main"
    api_json POST /api/v1/endpoints/primary/switch "{\"branch\":\"main\"}" >/dev/null
    ORIGINAL_BRANCH="main"
  fi
fi

log "deleting restore branch ${RESTORE_BRANCH}"
api_json DELETE "/api/v1/branches/${RESTORE_BRANCH}" >/dev/null
RESTORE_BRANCH=""

log "deleting created branch ${CREATED_BRANCH}"
api_json DELETE "/api/v1/branches/${CREATED_BRANCH}" >/dev/null
CREATED_BRANCH=""

final_branches_json="$(api_json GET /api/v1/branches)"
if ! jq -e --arg branch "smoke-${RUN_ID}" '.branches | all(.name != $branch)' >/dev/null <<<"${final_branches_json}"; then
  log "created smoke branch still present after cleanup"
  log "json payload: ${final_branches_json}"
  exit 1
fi

if ! jq -e --arg branch "restore-${RUN_ID}" '.branches | all(.name != $branch)' >/dev/null <<<"${final_branches_json}"; then
  log "restore smoke branch still present after cleanup"
  log "json payload: ${final_branches_json}"
  exit 1
fi

log "smoke test passed"
