# neon-selfhost

`neon-selfhost` aims to be a Docker-first self-hosting project for running open-source Neon with a simple admin web UI.

The goal is to make Neon branching and point-in-time restore practical for small deployments (for example, safe app upgrades and fast rollback).

Status: pre-alpha scaffold. A runnable controller with status, branch-management, restore, and endpoint lifecycle endpoints is included. Docker compose wires concrete storage broker/pageserver/safekeeper/compute services, and endpoint switch/start now resolve branch tenant/timeline attachments through pageserver APIs.

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
- `internal/server` contains the HTTP router, status/health endpoints, branch and restore endpoints, primary endpoint lifecycle endpoints, and operation log endpoint for MVP slice 1.
- `docker-compose.yml` wires controller + storage broker/pageserver/safekeepers/compute under the `neon` profile.
- `configs/neon/pageserver` contains the pageserver config mounted into the Neon container runtime.
- `configs/neon/compute_wrapper` contains the compute wrapper image/build files used by compose for local compute startup.
- `Dockerfile.controller` builds a minimal controller image.

## Implemented API (MVP Slice 1)

- `GET /api/v1/status`
- `GET /api/v1/health`
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

Primary endpoint start/stop/switch and connection APIs orchestrate the compose `compute` container lifecycle through the Docker socket. Start/switch resolve branch attachment metadata (tenant/timeline) via pageserver APIs, persist endpoint selection under `COMPUTE_DATA_DIR`, and restart compute against that selection.

Endpoint switch currently branches from parent timeline head. Timestamp-to-LSN-backed restore attachment remains planned.

Branch mutation and restore APIs return `storage_error` responses when controller state persistence fails (including disk-full conditions).

Controller startup runs a preflight check for `CONTROLLER_DATA_DIR` writability and exits early on invalid paths.

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

To bring up the controller plus Neon storage/compute services, set `BASIC_AUTH_PASSWORD` and run:

```bash
BASIC_AUTH_PASSWORD=change-me docker compose --profile neon up
```

Override `NEON_IMAGE`, `NEON_COMPUTE_IMAGE`, or `NEON_COMPUTE_TAG` if you need specific image tags.
The compose controller runs with `PRIMARY_ENDPOINT_MODE=docker`, uses `/var/run/docker.sock` to orchestrate the `compute` service lifecycle, and uses `PAGESERVER_API` to resolve branch attachment metadata.

## Operational Caveats (MVP)

- Single-node deployment is not HA. Host or disk loss can still cause data loss.
- Docker named volumes are not backups.
- Branching and PITR retention increase disk usage; in Phase 1, operations must fail safely with clear errors/logs, while proactive disk guardrails are planned for Phase 2.
- Soft-deleting a branch does not imply immediate disk reclamation.
- If exposing UI or Postgres beyond localhost, terminate TLS via a reverse proxy; basic auth alone is not Internet-grade protection.

## Repository Layout

- `AGENTS.md` - contribution and coding-agent rules.
- `configs/neon/pageserver` - pageserver runtime config used by `docker compose --profile neon`.
- `configs/neon/compute_wrapper` - compute wrapper build/runtime files used by `docker compose --profile neon`.
- `docs/architecture.md` - architecture, deployment topology, and safety model.
- `docs/mvp-roadmap.md` - phased plan for delivery.

## Current Status

Planning and scaffolding phase.

See `docs/architecture.md` for the current target design.
