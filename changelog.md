# Changelog

## Unreleased

### Added
- HTTP basic auth support for controller API routes via `BASIC_AUTH_USER` and `BASIC_AUTH_PASSWORD`.
- Persistent branch store support via `CONTROLLER_DATA_DIR`, with branch state written to `branches.json`.
- Serialized branch mutation execution and in-memory operation logging exposed by `GET /api/v1/operations`.
- Restore endpoint scaffold at `POST /api/v1/restore` with RFC3339 validation, source-history checks, and restore-branch creation.
- Primary endpoint control API scaffold at `POST /api/v1/endpoints/primary/start|stop|switch` and `GET /api/v1/endpoints/primary/connection`.
- New tests for config loading, auth enforcement, operation logging, restore behavior, primary endpoint controls, and branch persistence.
- Persistence error classification for branch state updates, including explicit insufficient-disk-space handling.
- Concrete Neon image/command wiring in `docker-compose.yml` for storage broker, pageserver, and 3 safekeepers under the `neon` profile.
- Pageserver runtime config files under `configs/neon/pageserver` for compose-based local bootstrap.
- Compute wrapper build/runtime files under `configs/neon/compute_wrapper` and compose wiring for a local Neon compute service.
- Health endpoint at `GET /api/v1/health` with component checks for branch storage, operation manager, and primary endpoint state.
- Startup preflight checks for `CONTROLLER_DATA_DIR` path validity and writability.
- Docker-runtime primary endpoint orchestration in controller endpoints (`start`, `stop`, `switch`, `connection`) using Docker Engine API over the mounted Docker socket.
- Endpoint orchestration configuration via new environment variables (primary endpoint mode/service/connection settings and Docker socket/project settings).
- Pageserver-backed branch attachment resolver that ensures tenant/timeline mapping for endpoint start/switch operations.
- Endpoint selection persistence in compute data dir (`endpoint-selection.json`) consumed by compute startup.
- Branch store attachment metadata (`tenant_id`, `timeline_id`) persisted with branch state.
- Restore-time branch attachment resolution via pageserver timestamp-to-LSN lookup and timeline creation at `ancestor_start_lsn`.

### Changed
- Controller startup now uses the persistent branch store when a controller data directory is configured.
- Compose controller service now requires explicit basic auth password configuration.
- Documentation now reflects implemented auth, persistence, and operation logging behavior.
- Branch mutation/restore APIs now return explicit `storage_error` responses for persistence failures.
- Documentation now reflects concrete compose storage-plane wiring and remaining compute-orchestration gap.
- Documentation now reflects health/preflight behavior and current scope boundaries for Neon-service health integration.
- Documentation now reflects Docker-based compute lifecycle orchestration and the remaining branch-to-timeline attachment gap.
- Documentation now reflects implemented restore-time LSN attachment behavior and the remaining endpoint readiness diagnostics gap.
