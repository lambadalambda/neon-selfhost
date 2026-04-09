#!/usr/bin/env bash
set -euo pipefail

generate_id() {
  local -n resvar=${1}
  printf -v resvar '%08x%08x%08x%08x' ${SRANDOM} ${SRANDOM} ${SRANDOM} ${SRANDOM}
}

PG_VERSION=${PG_VERSION:-16}
ENDPOINT_SELECTION_FILE=${ENDPOINT_SELECTION_FILE:-/var/lib/neon/compute/endpoint-selection.json}

readonly CONFIG_FILE_ORG=/var/db/postgres/configs/config.json
readonly CONFIG_FILE=/tmp/config.json

echo "Waiting for pageserver to be ready"
until nc -z pageserver 6400; do
  sleep 1
done
echo "Pageserver is ready"

cp "${CONFIG_FILE_ORG}" "${CONFIG_FILE}"

role_name="$(jq -r '.spec.cluster.roles[0].name // "cloud_admin"' "${CONFIG_FILE}")"

md5_role_password() {
  local role_password="$1"
  local role_user="$2"
  local digest=""

  if command -v md5sum >/dev/null 2>&1; then
    digest="$(printf '%s%s' "${role_password}" "${role_user}" | md5sum | awk '{print $1}')"
  elif command -v md5 >/dev/null 2>&1; then
    digest="$(printf '%s%s' "${role_password}" "${role_user}" | md5 -q)"
  else
    echo "No md5 tool available for password hashing" >&2
    return 1
  fi

  printf '%s' "${digest}"
}

if [[ -f "${ENDPOINT_SELECTION_FILE}" ]]; then
  selected_tenant_id="$(jq -r '.tenant_id // empty' "${ENDPOINT_SELECTION_FILE}" || true)"
  selected_timeline_id="$(jq -r '.timeline_id // empty' "${ENDPOINT_SELECTION_FILE}" || true)"
  selected_password="$(jq -r '.password // empty' "${ENDPOINT_SELECTION_FILE}" || true)"

  if [[ -n "${selected_tenant_id}" && -n "${selected_timeline_id}" ]]; then
    TENANT_ID=${selected_tenant_id}
    TIMELINE_ID=${selected_timeline_id}
    export TENANT_ID TIMELINE_ID
  fi

  if [[ -n "${selected_password}" ]]; then
    encrypted_password="$(md5_role_password "${selected_password}" "${role_name}")"

    updated_config="$(mktemp)"
    jq --arg role_name "${role_name}" --arg encrypted_password "${encrypted_password}" '
      (.spec.cluster.roles[] | select(.name == $role_name).encrypted_password) = $encrypted_password
    ' "${CONFIG_FILE}" >"${updated_config}"
    mv "${updated_config}" "${CONFIG_FILE}"
  fi
fi

if [[ -n "${TENANT_ID:-}" && -n "${TIMELINE_ID:-}" ]]; then
  tenant_id=${TENANT_ID}
  timeline_id=${TIMELINE_ID}
else
  tenant_id="$(curl -sS -X GET -H "Content-Type: application/json" "http://pageserver:9898/v1/tenant" | jq -r '.[0].id')"
  if [[ -z "${tenant_id}" || "${tenant_id}" = null ]]; then
    echo "Creating tenant"
    generate_id tenant_id
    curl -sS -X PUT \
      -H "Content-Type: application/json" \
      -d '{"mode":"AttachedSingle","generation":1,"tenant_conf":{}}' \
      "http://pageserver:9898/v1/tenant/${tenant_id}/location_config" >/dev/null
  fi

  timeline_id=""
  if [[ "${RUN_PARALLEL:-false}" != "true" ]]; then
    timeline_id="$(curl -sS -X GET -H "Content-Type: application/json" "http://pageserver:9898/v1/tenant/${tenant_id}/timeline" | jq -r '.[0].timeline_id')"
  fi

  if [[ -z "${timeline_id}" || "${timeline_id}" = null ]]; then
    echo "Creating timeline"
    generate_id timeline_id
    curl -sS -X POST \
      -H "Content-Type: application/json" \
      -d "{\"new_timeline_id\":\"${timeline_id}\",\"pg_version\":${PG_VERSION}}" \
      "http://pageserver:9898/v1/tenant/${tenant_id}/timeline/" >/dev/null
  fi
fi

if [[ ${PG_VERSION} -ge 17 ]]; then
  ulid_extension=pgx_ulid
else
  ulid_extension=ulid
fi

shared_libraries=$(jq -r '.spec.cluster.settings[] | select(.name=="shared_preload_libraries").value' "${CONFIG_FILE}")
sed -i "s|${shared_libraries}|${shared_libraries},${ulid_extension}|" "${CONFIG_FILE}"
sed -i "s|TENANT_ID|${tenant_id}|" "${CONFIG_FILE}"
sed -i "s|TIMELINE_ID|${timeline_id}|" "${CONFIG_FILE}"

# Clear stale Unix socket files that can survive container restarts.
rm -f /tmp/.s.PGSQL.55433 /tmp/.s.PGSQL.55433.lock

echo "Starting compute node"
exec /usr/local/bin/compute_ctl \
  --pgdata /var/db/postgres/compute \
  -C "postgresql://cloud_admin@localhost:55433/postgres" \
  -b /usr/local/bin/postgres \
  --compute-id "compute-${RANDOM}" \
  --config "${CONFIG_FILE}" \
  --dev
