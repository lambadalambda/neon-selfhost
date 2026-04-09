# MVP Roadmap

## Phase 1 - Single-Node Baseline

Goal: `docker compose up` and complete snapshot/restore/switch workflows from UI.

- Compose stack for controller + Neon services; controller/storage/compute wiring is implemented for local compose runtime.
- Controller web console route (`GET /`) is implemented with branch list/create/delete/switch, restore, endpoint controls, and connection-command display.
- Basic auth for one admin user.
- Branch list/create/delete (soft delete).
- Restore to timestamp (timestamp -> LSN -> branch); implemented via pageserver timestamp-to-LSN lookup and restore timeline creation with persisted branch attachment.
- Primary endpoint start/stop/switch actions; compose compute lifecycle orchestration plus branch attachment resolution is implemented, with runtime readiness diagnostics and unhealthy-state reporting exposed through the primary connection API.
- Operation log with clear failure messages.
- Fail-safe behavior on disk pressure (clear errors, no silent corruption or implicit destructive cleanup); storage-error API handling is implemented, proactive warning/guardrail automation remains.

## Phase 2 - Hardening

Goal: safer operations and recovery.

- Default 3 safekeepers (even on one host) where feasible, to reduce single-process durability risk (not host-level HA).
- Backup automation and documented off-host backup path.
- Health checks and startup preflight checks; controller-level `GET /api/v1/health` and data-dir preflight checks are implemented, deeper Neon-service health integration remains.
- Upgrade flow with mandatory pre-upgrade snapshot.
- Disk pressure warnings and guardrails.

## Phase 3 - Optional Advanced Features

Goal: add power without compromising baseline simplicity.

- Optional preview endpoints.
- Expanded branch policy controls.
- Enhanced observability and diagnostics bundle.
- Evaluate multi-project support only after stable single-project operations.
