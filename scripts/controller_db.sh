#!/usr/bin/env bash
set -euo pipefail

ACTION=${1:-}
PODMAN=${PODMAN:-podman}
CONTROLLER_VOLUME=${CONTROLLER_VOLUME:-neon-selfhost_controller_state}
export BASIC_AUTH_PASSWORD=${BASIC_AUTH_PASSWORD:-change-me}
PRESERVE_ROLLBACK=0
RESTORE_DESTRUCTIVE_STARTED=0
RESTORE_RESOLVED=0
ROLLBACK_DIR=""
ROLLBACK_ARCHIVE=""
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPOSITORY_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)

die() {
  printf '%s\n' "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

require_backup_file() {
  [[ -f "$1" ]] || die "missing $1"
  [[ -s "$1" ]] || die "backup file is empty: $1"
}

compose_controller_is_running() {
  local running
  if ! running=$("$PODMAN" compose ps --status running -q controller); then
    die "failed to inspect Podman Compose controller state"
  fi
  [[ -n "$running" ]]
}

validate_volume_archive() {
  local archive=$1
  require_backup_file "$archive"
  require_command go
  (cd "$REPOSITORY_ROOT" && go run ./cmd/controller-db validate-archive "$archive")
}

restore_previous_volume() {
  "$PODMAN" volume rm "$CONTROLLER_VOLUME" >/dev/null 2>&1 || true
  if ! "$PODMAN" volume create "$CONTROLLER_VOLUME" >/dev/null; then
    return 1
  fi
  if ! "$PODMAN" volume import "$CONTROLLER_VOLUME" "$ROLLBACK_ARCHIVE"; then
    return 1
  fi
  return 0
}

finish_compose_restore() {
  if [[ "$RESTORE_DESTRUCTIVE_STARTED" == "1" && "$RESTORE_RESOLVED" != "1" ]]; then
    if restore_previous_volume; then
      RESTORE_RESOLVED=1
    else
      PRESERVE_ROLLBACK=1
      printf 'automatic rollback failed; rollback archive preserved at %s\n' "$ROLLBACK_ARCHIVE" >&2
    fi
  fi
  if [[ "$PRESERVE_ROLLBACK" != "1" && -n "$ROLLBACK_DIR" ]]; then
    rm -rf "$ROLLBACK_DIR"
  fi
}

interrupt_compose_restore() {
  PRESERVE_ROLLBACK=1
  exit 130
}

backup_native() {
  require_command go
  (cd "$REPOSITORY_ROOT" && go run ./cmd/controller-db backup)
}

restore_native() {
  require_command go
  (cd "$REPOSITORY_ROOT" && go run ./cmd/controller-db restore)
}

backup_compose() {
  require_command "$PODMAN"
  if compose_controller_is_running; then
    die "Podman Compose controller is running; stop it before backing up its volume"
  fi

  local stamp backup_dir archive
  stamp=$(date +%Y%m%d-%H%M%S)
  backup_dir=${BACKUP_DIR:-.data/controller-backups/backup-$stamp}
  archive="$backup_dir/controller-state.tar"
  mkdir -p "$backup_dir"
  "$PODMAN" volume export "$CONTROLLER_VOLUME" --output "$archive"
  validate_volume_archive "$archive"
  printf 'Podman volume backup created at %s\n' "$backup_dir"
}

restore_compose() {
  [[ -n "${BACKUP_DIR:-}" ]] || die "set BACKUP_DIR=/path/to/backup-dir"
  local archive="$BACKUP_DIR/controller-state.tar"
  validate_volume_archive "$archive"
  require_command "$PODMAN"
  if compose_controller_is_running; then
    die "Podman Compose controller is running; stop it before restoring its volume"
  fi

  ROLLBACK_DIR=$(mktemp -d)
  ROLLBACK_ARCHIVE="$ROLLBACK_DIR/controller-state.tar"
  trap finish_compose_restore EXIT
  trap interrupt_compose_restore INT TERM
  if ! "$PODMAN" volume export "$CONTROLLER_VOLUME" --output "$ROLLBACK_ARCHIVE"; then
    die "failed to export current controller volume before restore"
  fi
  validate_volume_archive "$ROLLBACK_ARCHIVE"

  "$PODMAN" compose down
  RESTORE_DESTRUCTIVE_STARTED=1
  if ! "$PODMAN" volume rm "$CONTROLLER_VOLUME" >/dev/null; then
    die "failed to remove existing controller volume; automatic rollback will be attempted"
  fi
  if ! "$PODMAN" volume create "$CONTROLLER_VOLUME" >/dev/null; then
    if restore_previous_volume; then
      RESTORE_RESOLVED=1
      die "failed to create replacement controller volume; previous controller volume was restored"
    fi
    PRESERVE_ROLLBACK=1
    die "failed to create replacement controller volume and rollback failed; rollback archive: $ROLLBACK_ARCHIVE"
  fi
  if ! "$PODMAN" volume import "$CONTROLLER_VOLUME" "$archive"; then
    if restore_previous_volume; then
      RESTORE_RESOLVED=1
      die "failed to import requested controller volume; previous controller volume was restored"
    fi
    PRESERVE_ROLLBACK=1
    die "failed to import requested controller volume and rollback failed; rollback archive: $ROLLBACK_ARCHIVE"
  fi
  RESTORE_RESOLVED=1
  trap - EXIT INT TERM
  rm -rf "$ROLLBACK_DIR"
  printf 'Podman volume restore completed from %s; restart the stack when ready\n' "$BACKUP_DIR"
}

case "$ACTION" in
  backup)
    backup_native
    ;;
  restore)
    restore_native
    ;;
  backup-compose)
    backup_compose
    ;;
  restore-compose)
    restore_compose
    ;;
  *)
    die "usage: $0 {backup|restore|backup-compose|restore-compose}"
    ;;
esac
