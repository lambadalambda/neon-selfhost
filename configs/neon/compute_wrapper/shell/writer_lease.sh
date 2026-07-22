#!/usr/bin/env bash

acquire_writer_lease() {
  local tenant_id="$1"
  local timeline_id="$2"
  local lease_dir="${3:-/var/lib/neon/compute/writer-leases}"

  if [[ ! "${tenant_id}" =~ ^[0-9a-fA-F]{32}$ ]]; then
    echo "Invalid tenant ID for writer lease" >&2
    return 64
  fi
  if [[ ! "${timeline_id}" =~ ^[0-9a-fA-F]{32}$ ]]; then
    echo "Invalid timeline ID for writer lease" >&2
    return 64
  fi
  if [[ "${lease_dir}" != /* ]]; then
    echo "Writer lease directory must be absolute" >&2
    return 64
  fi
  if ! command -v flock >/dev/null 2>&1; then
    echo "flock is required for compute writer leases" >&2
    return 69
  fi

  mkdir -p "${lease_dir}"
  local lease_path="${lease_dir}/${tenant_id,,}_${timeline_id,,}.lock"
  exec {WRITER_LEASE_FD}<>"${lease_path}"
  local lock_status=0
  flock -E 75 -n "${WRITER_LEASE_FD}" || lock_status=$?
  if [[ ${lock_status} -eq 0 ]]; then
    return 0
  fi
  if [[ ${lock_status} -eq 75 ]]; then
    echo "Writer lease is already held for tenant ${tenant_id} timeline ${timeline_id}" >&2
    return 75
  fi
  echo "Failed to acquire writer lease for tenant ${tenant_id} timeline ${timeline_id}" >&2
  return 69
}
