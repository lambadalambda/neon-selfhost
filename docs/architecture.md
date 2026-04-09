# Architecture

## Overview

`neon-selfhost` is planned as a Docker-first, operator-friendly setup around open-source Neon with a minimal web console.

Current maturity: pre-alpha. The current implementation includes a runnable controller with status/health, branch-management, restore, and endpoint lifecycle endpoints backed by a single-tenant branch store that can persist state to disk, plus compose wiring for storage broker/pageserver/safekeepers/compute and Docker-backed compute lifecycle orchestration.

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
  - `55433` -> Primary PostgreSQL endpoint
- If exposing beyond localhost, terminate TLS in a reverse proxy and do not treat basic auth alone as Internet-grade security.
- Internal-only services:
  - Storage broker gRPC port
  - Pageserver HTTP and page service ports
  - Safekeeper ports
- Controller runtime mount:
  - `/var/run/docker.sock` for compute lifecycle orchestration via Docker Engine API
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

- `GET /` (controller web console)
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

Planned for later slices:

- Deeper endpoint readiness and startup diagnostics sourced directly from Neon runtime APIs.

Current API behavior notes:

- Branch operations are backed by a single-process store; when `CONTROLLER_DATA_DIR` is set, branch state persists to a local JSON state file.
- `GET /` serves a single-page controller console for branch, restore, endpoint, and operation-log workflows.
- `DELETE /api/v1/branches/{name}` marks branches as deleted; it does not remove storage.
- `POST /api/v1/restore` validates RFC3339 timestamps, rejects future timestamps, and rejects timestamps before source-branch history.
- `POST /api/v1/restore` resolves timestamp-to-LSN via pageserver APIs and creates a restore timeline using `ancestor_start_lsn`.
- `POST /api/v1/restore` fails closed with `restore_unavailable` when pageserver-backed restore integration is unavailable.
- Primary endpoint start/stop/switch APIs orchestrate the compose `compute` container through Docker Engine API calls via the controller's Docker socket mount.
- `GET /api/v1/endpoints/primary/connection` reflects compute runtime state plus controller-held branch selection and connection metadata.
- Endpoint start/switch resolve branch tenant/timeline attachment via pageserver APIs, persist endpoint selection in compute data dir, and restart compute against that selection.
- Switch-time branching attaches at parent timeline head; restore-time branching attaches at the timestamp-resolved LSN.
- Endpoint connection responses include readiness diagnostics (`ready`, `runtime_state`, `runtime_message`) sourced from Docker runtime state, report `status=starting` during health-check warmup, and `status=unhealthy` when runtime is running but unhealthy.
- Endpoint connection DSN is emitted only when `ready=true`.
- The web console exposes one-click connection helpers (`psql` command copy, DSN copy, and `DATABASE_URL` snippet copy) for the current primary branch endpoint.
- Branch create/delete/restore operations return explicit `storage_error` responses when controller state persistence fails, including insufficient-disk-space failures.
- `GET /api/v1/health` reports controller component health checks for branch storage, operation manager, and primary endpoint state, and marks primary endpoint health as degraded while runtime is up but not yet ready.
- Startup performs a preflight writability check for `CONTROLLER_DATA_DIR` and fails fast on invalid/unwritable paths.
- Validation and JSON parse failures return stable JSON envelopes with `error.code` and `error.message`.
- When `BASIC_AUTH_USER` and `BASIC_AUTH_PASSWORD` are configured, the web console and API routes require HTTP basic auth.
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
