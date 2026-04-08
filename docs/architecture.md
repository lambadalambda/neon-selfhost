# Architecture

## Overview

`neon-selfhost` is planned as a Docker-first, operator-friendly setup around open-source Neon with a minimal web console.

Current maturity: pre-alpha. The current implementation includes a runnable controller with status and branch-management endpoints backed by a single-tenant branch store that can persist state to disk.

Design target:

- One admin user.
- One tenant.
- One primary database endpoint.
- Branching and restore workflows that are safe and easy.

## Terminology

- Branch: user-facing name for a Neon timeline.
- Timeline ID: canonical internal identifier backing a branch.
- Endpoint: a compute instance serving PostgreSQL traffic.
- Tenant: Neon storage namespace; MVP assumes a single tenant.

## High-Level Components

1. Controller (Go web service + web UI)
   - Public entrypoint for all admin actions.
   - Owns configuration, operation logs, and orchestration jobs.
   - Exposes a small HTTP API consumed by the UI.

2. Neon data-plane services
   - Pageserver for timeline/page history.
   - Safekeeper(s) for WAL durability.
   - Compute endpoint for PostgreSQL client traffic.
   - Broker if required by the selected Neon runtime wiring (Neon internal coordination service used by some control/runtime paths).

3. Persistent storage
   - Named Docker volumes for pageserver, safekeepers, compute state, and controller state.

## Docker Topology (MVP)

- Exposed ports (bind to localhost by default):
  - `8080` -> Controller UI/API
  - `5432` -> Primary PostgreSQL endpoint
- If exposing beyond localhost, terminate TLS in a reverse proxy and do not treat basic auth alone as Internet-grade security.
- Internal-only services:
  - Pageserver HTTP and page service ports
  - Safekeeper ports
  - Broker ports (if enabled)
- Networks:
  - One internal network for service-to-service communication

## Core User Flows

1. Create snapshot branch
   - Branch from current endpoint head.
   - Tag with timestamped name for rollback.

2. Restore to timestamp
   - Accept RFC3339 timestamps and normalize to UTC.
   - Resolve timestamp -> LSN.
   - Create a new branch at the resolved LSN.
   - Semantics: restore to the latest commit at or before the requested timestamp.
   - Fail clearly when the timestamp is outside retained history or required WAL/page history is unavailable.

3. Switch primary endpoint
   - Stop endpoint.
   - Reattach/start on target branch.
   - Return fresh connection details.

## Controller API

Implemented in MVP slice 1:

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

Planned for later slices:

- Neon data-plane integration for endpoint lifecycle actions.
- Neon-backed connection details for active compute endpoints.

Current API behavior notes:

- Branch operations are backed by a single-process store; when `CONTROLLER_DATA_DIR` is set, branch state persists to a local JSON state file.
- `DELETE /api/v1/branches/{name}` marks branches as deleted; it does not remove storage.
- `POST /api/v1/restore` validates RFC3339 timestamps, rejects future timestamps, and rejects timestamps before source-branch history.
- `POST /api/v1/restore` currently uses a scaffold timestamp-to-LSN resolver for controller development; Neon data-plane resolution wiring is still planned.
- Primary endpoint start/stop/switch APIs currently manage controller-local endpoint state and serialize transitions through the operation lock.
- `GET /api/v1/endpoints/primary/connection` returns scaffold connection details from controller state.
- Validation and JSON parse failures return stable JSON envelopes with `error.code` and `error.message`.
- When `BASIC_AUTH_USER` and `BASIC_AUTH_PASSWORD` are configured, API routes require HTTP basic auth.
- State-changing branch operations are serialized through a controller operation lock; each attempt is recorded in an in-memory operation log exposed at `GET /api/v1/operations`.

## Safety Principles

- Conservative retention defaults for PITR.
- Soft-delete branches in early versions.
- Serialize admin operations through a controller job lock.
- Never expose internal Neon ports publicly by default.
- Keep explicit operator logs for every state-changing action.

## Operational Caveats

- Single-node deployment does not provide host-level high availability.
- Named Docker volumes improve persistence but are not a backup strategy.
- Off-host backups are required for meaningful disaster recovery.
- PITR/branch retention and branch fan-out increase disk usage; in Phase 1, fail safely with clear errors/logs on disk pressure, with proactive warning/guardrail automation planned for Phase 2.
- Soft-deleted branches may continue consuming storage until cleanup/GC conditions are met.

## Non-Goals (MVP)

- Multi-tenant UX.
- Multi-node orchestration.
- Full parity with Neon cloud control-plane features.

## Evolution Path

- Phase 1: Single-node MVP with reliable branch/restore/switch flow.
- Phase 2: Hardening (backup automation, better health checks, safer upgrades).
- Phase 3: Optional advanced features (preview endpoints, richer policies).
