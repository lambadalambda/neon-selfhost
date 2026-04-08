# neon-selfhost

`neon-selfhost` aims to be a Docker-first self-hosting project for running open-source Neon with a simple admin web UI.

The goal is to make Neon branching and point-in-time restore practical for small deployments (for example, safe app upgrades and fast rollback).

Status: pre-alpha scaffold. A runnable controller with status and branch-management endpoints is included. The full Neon data-plane compose stack is still a placeholder template.

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
- `internal/config` contains minimal environment-based config loading.
- `internal/branch` contains the single-tenant in-memory branch model/store.
- `internal/server` contains the HTTP router, status endpoint, and branch CRUD endpoints for MVP slice 1.
- `docker-compose.yml` is a deployment skeleton. Placeholder Neon services are behind the `neon` profile until concrete images/commands are wired.
- `Dockerfile.controller` builds a minimal controller image.

## Implemented API (MVP Slice 1)

- `GET /api/v1/status`
- `GET /api/v1/branches`
- `POST /api/v1/branches`
- `DELETE /api/v1/branches/{name}` (soft-delete)

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

For the future full stack, use `docker compose --profile neon up` after replacing placeholder service images/commands.

## Operational Caveats (MVP)

- Single-node deployment is not HA. Host or disk loss can still cause data loss.
- Docker named volumes are not backups.
- Branching and PITR retention increase disk usage; in Phase 1, operations must fail safely with clear errors/logs, while proactive disk guardrails are planned for Phase 2.
- Soft-deleting a branch does not imply immediate disk reclamation.
- If exposing UI or Postgres beyond localhost, terminate TLS via a reverse proxy; basic auth alone is not Internet-grade protection.

## Repository Layout

- `AGENTS.md` - contribution and coding-agent rules.
- `docs/architecture.md` - architecture, deployment topology, and safety model.
- `docs/mvp-roadmap.md` - phased plan for delivery.

## Current Status

Planning and scaffolding phase.

See `docs/architecture.md` for the current target design.
