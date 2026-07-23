# Architecture

## Overview

`neon-selfhost` is a Podman-first, operator-friendly control plane around open-source Neon with an embedded web console.

Current maturity: experimental. The implementation includes branch and endpoint lifecycle orchestration, point-in-time restore, guarded SQL execution, a persistent SQL library, schema browsing, health/status APIs, and SQLite controller state. The included Compose stack wires the controller to the storage broker, pageserver, safekeeper, primary compute, and on-demand branch computes through Podman's Docker-compatible API.

Supported topology:

- One admin user.
- One tenant.
- One primary database endpoint plus optional published branch endpoints.
- Branching and restore workflows that are safe and easy.

## Terminology

- Branch: user-facing name for a Neon timeline.
- Timeline ID: canonical internal identifier backing a branch.
- Endpoint: a compute instance serving PostgreSQL traffic.
- Tenant: Neon storage namespace; the supported topology uses a single tenant.

## High-Level Components

1. Controller (Go web service + web UI)
   - Public entrypoint for all admin actions.
   - Owns configuration, persistent operation logs, and orchestration jobs.
   - Exposes a small HTTP API consumed by the UI.

2. Neon data-plane services
   - Pageserver for timeline/page history.
   - Safekeeper(s) for WAL durability.
   - Compute endpoint for primary PostgreSQL traffic plus branch-specific on-demand endpoints.
   - Broker if required by the selected Neon runtime wiring (Neon internal coordination service used by some control/runtime paths).

3. Persistent storage
   - Named Podman volumes by default, with a bind-backed storage override for pageserver, safekeeper, compute, and controller state.

## Podman Topology

- Exposed ports (bind to localhost by default):
  - `8080` -> Controller UI/API
  - `55433` -> Primary PostgreSQL endpoint
  - `56000-56049` -> Published branch PostgreSQL endpoints (configurable)
- If exposing beyond localhost, terminate TLS in a reverse proxy and do not treat basic auth alone as Internet-grade security.
- Internal-only services:
  - Storage broker gRPC port
  - Pageserver HTTP and page service ports
  - Safekeeper ports
- Controller runtime mount:
  - Podman API socket, mounted at `/var/run/docker.sock` for compatibility with the controller's engine client
  - The controller runs as non-root UID `65532` with socket-group access; the socket still grants engine-level authority and is not a security boundary
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

4. Publish branch endpoint
   - Resolve branch tenant/timeline attachment.
   - Allocate a branch endpoint host port from configured range.
   - Start controller-side TCP gateway listener.
   - Lazily start branch compute on first client connection.
   - Return branch-scoped connection details without switching primary.

## Controller API

Implemented routes:

- `GET /` (controller web console)
- `GET /api/v1/status`
- `GET /api/v1/health`
- `POST /validate` (internal pageserver generation-validation upcall)
- `GET /api/v1/branches`
- `POST /api/v1/branches`
- `POST /api/v1/branches/{name}/reset`
- `POST /api/v1/branches/{name}/publish`
- `POST /api/v1/branches/{name}/unpublish`
- `GET /api/v1/branches/{name}/connection`
- `GET /api/v1/branches/{name}/databases`
- `GET /api/v1/branches/{name}/schema`
- `GET /api/v1/branches/{name}/schema/table`
- `POST /api/v1/branches/{name}/sql/execute`
- `DELETE /api/v1/branches/{name}` (soft-delete)
- `GET|POST /api/v1/sql/saved-queries`
- `PATCH|DELETE /api/v1/sql/saved-queries/{id}`
- `GET /api/v1/sql/history`
- `POST /api/v1/restore`
- `POST /api/v1/endpoints/primary/start`
- `POST /api/v1/endpoints/primary/stop`
- `POST /api/v1/endpoints/primary/switch`
- `GET /api/v1/endpoints/primary/connection`
- `GET /api/v1/endpoints` (published branch endpoint list)
- `GET /api/v1/operations`

Current API behavior notes:

- Branch operations are backed by a single-process store; when `CONTROLLER_DATA_DIR` is set, branch state persists to SQLite.
- `GET /` serves a single-page controller console with Dashboard, Branches, branch Overview, SQL Editor, Tables, and Backup & Restore views. Primary lifecycle controls, explicit publish/unpublish, and operation details remain API-driven.
- `DELETE /api/v1/branches/{name}` marks branches as deleted; it does not remove storage.
- `POST /api/v1/restore` validates RFC3339 timestamps, rejects future timestamps, and rejects timestamps before source-branch history.
- `POST /api/v1/restore` resolves timestamp-to-LSN via pageserver APIs and creates a restore timeline using `ancestor_start_lsn`.
- `POST /api/v1/restore` fails closed with `restore_unavailable` when pageserver-backed restore integration is unavailable.
- `POST /api/v1/branches/{name}/reset` creates a fresh child timeline from the branch parent head and re-attaches the branch to that timeline.
- Branch endpoint publish/unpublish/list/connection APIs are implemented; publish allocates a per-branch host port and unpublish tears down listener/container state.
- Branch endpoint publish state (`published` + `port`) is persisted with branch metadata.
- Published branch endpoints are available in container-engine mode and use a controller TCP gateway that lazy-starts branch compute on first client connection.
- Primary endpoint start/stop/switch APIs orchestrate the compose `compute` container through Podman's Docker-compatible API via the controller's socket mount.
- `GET /api/v1/endpoints/primary/connection` reflects compute runtime state plus controller-held branch selection and connection metadata.
- Endpoint start/switch resolve branch tenant/timeline attachment via pageserver APIs, persist endpoint selection in compute data dir, and restart compute against that selection.
- Branch reset refreshes published branch endpoint selection metadata; branch delete unpublishes branch endpoint state before soft-delete.
- Branch creation and reset attach at a parent timeline head; restore attaches at the timestamp-resolved LSN; primary switch reattaches compute to the selected branch's existing timeline.
- Endpoint connection responses include readiness diagnostics (`ready`, `runtime_state`, `runtime_message`) sourced from container runtime state, report `status=starting` during health-check warmup, and `status=unhealthy` when runtime is running but unhealthy.
- Endpoint connection responses include endpoint credential metadata (`user`, `password`) alongside runtime diagnostics.
- Branch credentials are branch-specific and controller-managed; create/restore operations assign random passwords persisted with branch state.
- Endpoint connection DSN is emitted only when `ready=true`.
- The web console exposes one-click connection helpers for branch DSNs, `psql` commands, and passwords.
- Saved SQL queries and bounded execution history are stored in `controller.db`, filterable by branch/database or project-wide. Operator-entered SQL is stored verbatim; result data and controller-managed endpoint credentials/DSNs are never stored, so operators must not embed secrets in SQL they save or execute through the editor.
- Schema browsing uses fixed parameterized catalog queries in explicit read-only transactions, pages relations before size calculation, caps all list/detail collections, and applies a five-second statement timeout.
- `SQL_HISTORY_RETENTION_LIMIT` controls physical execution-history retention and defaults to `200`.
- Branch create/delete/restore operations return explicit `storage_error` responses when controller state persistence fails, including insufficient-disk-space failures.
- `GET /api/v1/health` reports controller component health checks for branch storage, SQL query storage, operation manager, and primary endpoint state, and marks primary endpoint health as degraded while runtime is up but not yet ready.
- Startup performs a preflight writability check for `CONTROLLER_DATA_DIR` and fails fast on invalid/unwritable paths.
- Validation and JSON parse failures return stable JSON envelopes with `error.code` and `error.message`.
- When `BASIC_AUTH_USER` and `BASIC_AUTH_PASSWORD` are configured, the web console and `/api/v1/*` routes require HTTP basic auth. The internal `POST /validate` pageserver upcall is deliberately unauthenticated and fails closed unless its configured tenant-generation allowlist validates the request.
- State-changing branch operations are serialized through a controller operation lock; each attempt is recorded in the persistent SQLite operation log exposed at `GET /api/v1/operations`.

## Safety Principles

- Conservative retention defaults for PITR.
- Soft-delete branches in early versions.
- Serialize admin operations through a controller job lock.
- Never expose internal Neon ports publicly by default.
- Keep explicit operator logs for every state-changing action.

## Operational Caveats

- Single-node deployment does not provide host-level high availability.
- Named Podman volumes improve persistence but are not a backup strategy.
- Off-host backups are required for meaningful disaster recovery.
- PITR/branch retention and branch fan-out increase disk usage; in Phase 1, fail safely with clear errors/logs on disk pressure, with proactive warning/guardrail automation planned for Phase 2.
- Soft-delete and reset do not delete the previous pageserver timelines. They may consume storage indefinitely until an operator performs and verifies explicit timeline cleanup.

See the [MVP roadmap](mvp-roadmap.md) for implemented baseline work and remaining hardening priorities.
