# neon-selfhost

`neon-selfhost` aims to be a Docker-first self-hosting project for running open-source Neon with a simple admin web UI.

The goal is to make Neon branching and point-in-time restore practical for small deployments (for example, safe app upgrades and fast rollback).

Status: pre-alpha scaffold. A runnable controller with status and branch-management endpoints is included. Docker compose now wires concrete storage broker/pageserver/safekeeper services, while compute orchestration remains a scaffold.

## What This Project Is

- A thin control plane and web console for self-hosted Neon.
- Focused on single-node, single-tenant use cases.
- Optimized for simple operations and safe defaults.

## What This Project Is Not

- A full replacement for the managed `neon.com` platform.
- A multi-tenant cloud control plane.
- A "run everything everywhere" orchestration layer.

## Planned MVP Capabilities

- Bring up Neon components with Docker Compose.
- Manage branches/timelines from a web UI.
- Restore to a past timestamp by creating a branch at a resolved LSN.
- Start/stop/switch a primary compute endpoint.

## Current Scaffold

- `cmd/controller` contains the Go controller entrypoint.
- `internal/config` contains environment-based config loading, including basic auth credentials.
- `internal/branch` contains the single-tenant branch model/store with optional on-disk persistence.
- `internal/server` contains the HTTP router, status endpoint, branch CRUD endpoints, and operation log endpoint for MVP slice 1.
- `docker-compose.yml` wires controller + storage broker/pageserver/safekeepers under the `neon` profile.
- `configs/neon/pageserver` contains the pageserver config mounted into the Neon container runtime.
- `Dockerfile.controller` builds a minimal controller image.

## Implemented API (MVP Slice 1)

- `GET /api/v1/status`
- `GET /api/v1/branches`
- `POST /api/v1/branches`
- `DELETE /api/v1/branches/{name}` (soft-delete)
- `POST /api/v1/restore`
- `POST /api/v1/endpoints/primary/start`
- `POST /api/v1/endpoints/primary/stop`
- `POST /api/v1/endpoints/primary/switch`
- `GET /api/v1/endpoints/primary/connection`
- `GET /api/v1/operations`

When `BASIC_AUTH_USER` and `BASIC_AUTH_PASSWORD` are set, API routes require HTTP basic auth.

When `CONTROLLER_DATA_DIR` is set, branch state persists to `branches.json` under that directory.

`POST /api/v1/restore` currently validates timestamp semantics and creates a restore branch using a scaffold LSN resolver; Neon data-plane timestamp-to-LSN wiring remains planned.

Primary endpoint start/stop/switch and connection APIs currently operate on controller-local endpoint state for workflow development; Neon compute orchestration wiring remains planned.

Branch mutation and restore APIs return `storage_error` responses when controller state persistence fails (including disk-full conditions).

Validation and JSON parsing failures return stable JSON error envelopes:

```json
{
  "error": {
    "code": "validation_error",
    "message": "branch name is required"
  }
}
```

## Quickstart (Controller Dev)

```bash
mise exec -- go test ./...
mise exec -- go run ./cmd/controller
```

Then open `http://127.0.0.1:8080/api/v1/status`.

To run with basic auth enabled:

```bash
BASIC_AUTH_USER=admin BASIC_AUTH_PASSWORD=change-me mise exec -- go run ./cmd/controller
curl -u admin:change-me http://127.0.0.1:8080/api/v1/status
```

To bring up the controller plus Neon storage-plane services, set `BASIC_AUTH_PASSWORD` and run:

```bash
BASIC_AUTH_PASSWORD=change-me docker compose --profile neon up
```

Override `NEON_IMAGE` if you need a specific image tag.

## Operational Caveats (MVP)

- Single-node deployment is not HA. Host or disk loss can still cause data loss.
- Docker named volumes are not backups.
- Branching and PITR retention increase disk usage; in Phase 1, operations must fail safely with clear errors/logs, while proactive disk guardrails are planned for Phase 2.
- Soft-deleting a branch does not imply immediate disk reclamation.
- If exposing UI or Postgres beyond localhost, terminate TLS via a reverse proxy; basic auth alone is not Internet-grade protection.

## Repository Layout

- `AGENTS.md` - contribution and coding-agent rules.
- `configs/neon/pageserver` - pageserver runtime config used by `docker compose --profile neon`.
- `docs/architecture.md` - architecture, deployment topology, and safety model.
- `docs/mvp-roadmap.md` - phased plan for delivery.

## Current Status

Planning and scaffolding phase.

See `docs/architecture.md` for the current target design.
