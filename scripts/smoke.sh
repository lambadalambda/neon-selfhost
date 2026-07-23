#!/usr/bin/env bash
set -euo pipefail

AUTH_USER="${BASIC_AUTH_USER:-admin}"
AUTH_PASSWORD="${BASIC_AUTH_PASSWORD:-change-me}"
BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"

MANAGE_STACK=false
KEEP_STACK=false
VERIFY_WRITER_HANDOFF=false
COMPOSE_PROJECT="${COMPOSE_PROJECT_NAME:-neon-selfhost}"

ORIGINAL_BRANCH=""
CREATED_BRANCH=""
RESTORE_BRANCH=""
PRIMARY_SWITCHED=false
TARGET_BRANCH_CONTAINER_ID=""
PRIMARY_CONTAINER_ID=""

usage() {
  cat <<'EOF'
Usage: ./scripts/smoke.sh [--manage-stack] [--keep-stack] [--verify-writer-handoff]

Options:
  --manage-stack  Start and stop `podman compose --profile neon` for the smoke run.
  --keep-stack    Keep the stack running at the end (only with --manage-stack).
  --verify-writer-handoff
                  Start a lazy target compute and verify its removal during primary switch.
                  Requires --manage-stack; never use against production.
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
  BASIC_AUTH_PASSWORD="${AUTH_PASSWORD}" \
    PRIMARY_ENDPOINT_PASSWORD="${PRIMARY_ENDPOINT_PASSWORD:?set PRIMARY_ENDPOINT_PASSWORD}" \
    COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT}" \
    DOCKER_COMPOSE_PROJECT="${COMPOSE_PROJECT}" \
    podman compose --profile neon --project-name "${COMPOSE_PROJECT}" "$@"
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

branch_compute_ids() {
  local branch_name="$1"
  podman ps -a \
    --filter "label=neon.selfhost.endpoint=branch" \
    --filter "label=neon.selfhost.branch=${branch_name}" \
    --filter "label=com.docker.compose.project=${COMPOSE_PROJECT}" \
    --format '{{.ID}}'
}

running_branch_compute_ids() {
  local branch_name="$1"
  podman ps \
    --filter "label=neon.selfhost.endpoint=branch" \
    --filter "label=neon.selfhost.branch=${branch_name}" \
    --filter "label=com.docker.compose.project=${COMPOSE_PROJECT}" \
    --format '{{.ID}}'
}

wait_for_branch_compute() {
  local branch_name="$1"
  local attempt
  for attempt in $(seq 1 60); do
    local ids
    if ! ids="$(running_branch_compute_ids "${branch_name}")"; then
      log "failed to inspect lazy branch compute for ${branch_name}" >&2
      return 1
    fi
    if [[ -n "${ids}" ]]; then
      TARGET_BRANCH_CONTAINER_ID="${ids}"
      return 0
    fi
    sleep 1
  done

  log "lazy branch compute did not appear for ${branch_name}" >&2
  return 1
}

assert_branch_compute_absent() {
  local branch_name="$1"
  local ids
  if ! ids="$(branch_compute_ids "${branch_name}")"; then
    log "failed to inspect branch computes for ${branch_name}"
    return 1
  fi
  if [[ -n "${ids}" ]]; then
    log "lazy target compute was not removed for ${branch_name}: ${ids}"
    return 1
  fi
}

assert_container_absent() {
  local container_id="$1"
  local status
  set +e
  podman container exists "${container_id}"
  status=$?
  set -e
  case "${status}" in
    1) return 0 ;;
    0)
      log "lazy target compute was not removed: ${container_id}"
      return 1
      ;;
    *)
      log "container engine failed while checking ${container_id} (status ${status})"
      return 1
      ;;
  esac
}

assert_disposable_project_unused() {
  local containers
  if ! containers="$(podman ps -a --filter "label=com.docker.compose.project=${COMPOSE_PROJECT}" --format '{{.ID}}')"; then
    log "failed to inspect disposable Compose project ${COMPOSE_PROJECT}"
    return 1
  fi
  if [[ -n "${containers}" ]]; then
    log "refusing pre-existing Compose project ${COMPOSE_PROJECT}"
    return 1
  fi

  local volume
  for volume in controller_state compute_state compute_cache pageserver_data safekeeper1_data; do
    local status
    set +e
    podman volume exists "${COMPOSE_PROJECT}_${volume}"
    status=$?
    set -e
    case "${status}" in
      1) ;;
      0)
        log "refusing pre-existing volume ${COMPOSE_PROJECT}_${volume}"
        return 1
        ;;
      *)
        log "container engine failed while checking volume ${COMPOSE_PROJECT}_${volume}"
        return 1
        ;;
    esac
  done

  local network_status
  set +e
  podman network exists "${COMPOSE_PROJECT}_neon_internal"
  network_status=$?
  set -e
  case "${network_status}" in
    1) return 0 ;;
    0)
      log "refusing pre-existing network ${COMPOSE_PROJECT}_neon_internal"
      return 1
      ;;
    *)
      log "container engine failed while checking network ${COMPOSE_PROJECT}_neon_internal"
      return 1
      ;;
  esac
}

cleanup() {
  set +e

  if [[ "${PRIMARY_SWITCHED}" == "true" && -n "${ORIGINAL_BRANCH}" ]]; then
    if api_json POST /api/v1/endpoints/primary/switch "{\"branch\":\"${ORIGINAL_BRANCH}\"}" >/dev/null 2>&1; then
      PRIMARY_SWITCHED=false
    else
      log "cleanup could not restore primary to ${ORIGINAL_BRANCH}; preserving ${CREATED_BRANCH}"
    fi
  fi

  if [[ -n "${RESTORE_BRANCH}" ]]; then
    api_json DELETE "/api/v1/branches/${RESTORE_BRANCH}" >/dev/null 2>&1 || true
  fi

  if [[ "${PRIMARY_SWITCHED}" != "true" && -n "${CREATED_BRANCH}" ]]; then
    api_json DELETE "/api/v1/branches/${CREATED_BRANCH}" >/dev/null 2>&1 || true
  fi

  if [[ "${MANAGE_STACK}" == "true" && "${KEEP_STACK}" != "true" ]]; then
    if [[ "${PRIMARY_SWITCHED}" == "true" ]]; then
      log "retaining disposable stack ${COMPOSE_PROJECT} because primary handback failed"
      return
    fi
    log "stopping compose stack"
    if [[ "${VERIFY_WRITER_HANDOFF}" == "true" ]]; then
      compose down --volumes >/dev/null 2>&1 || true
    else
      compose down >/dev/null 2>&1 || true
    fi
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
    --verify-writer-handoff)
      VERIFY_WRITER_HANDOFF=true
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

if [[ "${VERIFY_WRITER_HANDOFF}" == "true" && "${MANAGE_STACK}" != "true" ]]; then
  log "--verify-writer-handoff requires --manage-stack"
  exit 1
fi

if [[ "${VERIFY_WRITER_HANDOFF}" == "true" ]]; then
  HANDOFF_PORT="${CONTROLLER_HOST_PORT:-8080}"
  if [[ ! "${HANDOFF_PORT}" =~ ^[0-9]+$ ]] || ((HANDOFF_PORT < 1 || HANDOFF_PORT > 65535)); then
    log "--verify-writer-handoff requires a valid CONTROLLER_HOST_PORT"
    exit 1
  fi
  if [[ "${BASE_URL}" != "http://127.0.0.1:${HANDOFF_PORT}" && "${BASE_URL}" != "http://localhost:${HANDOFF_PORT}" && "${BASE_URL}" != "http://[::1]:${HANDOFF_PORT}" ]]; then
    log "--verify-writer-handoff requires BASE_URL to match the managed loopback port"
    exit 1
  fi
  COMPOSE_PROJECT="neon-selfhost-handoff-$(date -u +%Y%m%d%H%M%S)-$$-${RANDOM}"
fi

require_command curl
require_command jq

if [[ "${MANAGE_STACK}" == "true" ]]; then
  require_command podman
fi

if [[ "${VERIFY_WRITER_HANDOFF}" == "true" ]]; then
  assert_disposable_project_unused
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
wait_for_ready_branch "${ORIGINAL_BRANCH}"

RUN_ID="$(date -u +%Y%m%d%H%M%S)-${RANDOM}"
CREATED_BRANCH="smoke-${RUN_ID}"
RESTORE_BRANCH="restore-${RUN_ID}"
PROBE_TABLE="writer_handoff_probe_$(date -u +%Y%m%d%H%M%S)_${RANDOM}"
PROBE_TOKEN="handoff-${RUN_ID}-${RANDOM}"

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

if [[ "${VERIFY_WRITER_HANDOFF}" == "true" ]]; then
  log "starting lazy compute for ${CREATED_BRANCH}"
  lazy_sql_json="$(api_json POST "/api/v1/branches/${CREATED_BRANCH}/sql/execute" "{\"database\":\"postgres\",\"sql\":\"CREATE TABLE public.${PROBE_TABLE} AS SELECT '${PROBE_TOKEN}'::text AS token\",\"allow_writes\":true}")"
  assert_jq "${lazy_sql_json}" '.result.read_only == false' "lazy target marker write should commit before handoff"
  wait_for_branch_compute "${CREATED_BRANCH}"
  if [[ "${TARGET_BRANCH_CONTAINER_ID}" == *$'\n'* ]]; then
    log "expected one lazy target compute, found multiple: ${TARGET_BRANCH_CONTAINER_ID}"
    exit 1
  fi
  PRIMARY_CONTAINER_ID="$(compose ps -q compute)"
  if [[ -z "${PRIMARY_CONTAINER_ID}" ]]; then
    log "primary compute container was not found"
    exit 1
  fi
  if ! podman container exists "${PRIMARY_CONTAINER_ID}"; then
    log "compose and Podman are not inspecting the same container engine"
    exit 1
  fi
fi

log "switching primary endpoint to ${CREATED_BRANCH}"
PRIMARY_SWITCHED=true
switch_json="$(api_json POST /api/v1/endpoints/primary/switch "{\"branch\":\"${CREATED_BRANCH}\"}")"
if ! jq -e --arg branch "${CREATED_BRANCH}" '.connection.branch == $branch' >/dev/null <<<"${switch_json}"; then
  log "switch response did not target branch ${CREATED_BRANCH}"
  log "json payload: ${switch_json}"
  exit 1
fi

wait_for_ready_branch "${CREATED_BRANCH}"

if [[ "${VERIFY_WRITER_HANDOFF}" == "true" ]]; then
  assert_branch_compute_absent "${CREATED_BRANCH}"
  assert_container_absent "${TARGET_BRANCH_CONTAINER_ID}"
  current_primary_id="$(compose ps -q compute)"
  if [[ "${current_primary_id}" != "${PRIMARY_CONTAINER_ID}" ]]; then
    log "primary compute container identity changed during handoff"
    exit 1
  fi
  target_sql_json="$(api_json POST "/api/v1/branches/${CREATED_BRANCH}/sql/execute" "{\"database\":\"postgres\",\"sql\":\"SELECT token FROM public.${PROBE_TABLE}\"}")"
  if ! jq -e --arg branch "${CREATED_BRANCH}" --arg token "${PROBE_TOKEN}" '.result.branch == $branch and .result.rows == [[$token]]' >/dev/null <<<"${target_sql_json}"; then
    log "target SQL did not route through primary after handoff"
    log "json payload: ${target_sql_json}"
    exit 1
  fi
  PRIMARY_PROBE_TOKEN="${PROBE_TOKEN}-primary"
  write_json="$(api_json POST "/api/v1/branches/${CREATED_BRANCH}/sql/execute" "{\"database\":\"postgres\",\"sql\":\"UPDATE public.${PROBE_TABLE} SET token = '${PRIMARY_PROBE_TOKEN}'\",\"allow_writes\":true}")"
  assert_jq "${write_json}" '.result.read_only == false' "primary handoff write should commit"
  verify_json="$(api_json POST "/api/v1/branches/${CREATED_BRANCH}/sql/execute" "{\"database\":\"postgres\",\"sql\":\"SELECT token FROM public.${PROBE_TABLE}\"}")"
  if ! jq -e --arg token "${PRIMARY_PROBE_TOKEN}" '.result.rows == [[$token]]' >/dev/null <<<"${verify_json}"; then
    log "primary handoff write did not remain visible"
    log "json payload: ${verify_json}"
    exit 1
  fi
  api_json POST "/api/v1/branches/${CREATED_BRANCH}/sql/execute" "{\"database\":\"postgres\",\"sql\":\"DROP TABLE public.${PROBE_TABLE}\",\"allow_writes\":true}" >/dev/null
  assert_branch_compute_absent "${CREATED_BRANCH}"
fi

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
if api_json POST /api/v1/endpoints/primary/switch "{\"branch\":\"${ORIGINAL_BRANCH}\"}" >/dev/null; then
  PRIMARY_SWITCHED=false
elif [[ "${ORIGINAL_BRANCH}" != "main" ]]; then
  log "switching back to ${ORIGINAL_BRANCH} failed, falling back to main"
  api_json POST /api/v1/endpoints/primary/switch '{"branch":"main"}' >/dev/null
  ORIGINAL_BRANCH="main"
  PRIMARY_SWITCHED=false
else
  log "switching primary endpoint back to ${ORIGINAL_BRANCH} failed"
  exit 1
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
