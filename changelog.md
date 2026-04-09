# Changelog

## Unreleased

### Added
- HTTP basic auth support for controller API routes via `BASIC_AUTH_USER` and `BASIC_AUTH_PASSWORD`.
- Persistent branch store support via `CONTROLLER_DATA_DIR`, with branch state written to `branches.json`.
- Serialized branch mutation execution and in-memory operation logging exposed by `GET /api/v1/operations`.
- Restore endpoint scaffold at `POST /api/v1/restore` with RFC3339 validation, source-history checks, and restore-branch creation.
- Primary endpoint control API scaffold at `POST /api/v1/endpoints/primary/start|stop|switch` and `GET /api/v1/endpoints/primary/connection`.
- Controller web console at `GET /` with endpoint status, copyable connection helpers (`psql`, DSN, `DATABASE_URL` snippet), branch create/switch/delete actions, restore form, and operation log view.
- Smoke test automation script at `scripts/smoke.sh` for status/health, branch lifecycle, restore, and operation-log verification.
- Mise task shortcuts for stack lifecycle (`stack:up`, `stack:down`, `stack:ps`, `stack:logs`) and smoke runs (`smoke`, `smoke:fresh`).
- Database reset/seed script at `scripts/reset_seed_data.sh` for repeatable branch-testing fixtures on `main` (`branch_lab`).
- Branch-isolation verification mode in `scripts/reset_seed_data.sh` that mutates data on a temporary branch and confirms `main` remains unchanged.
- Mise task shortcuts for dataset reset/verification (`db:reset-seed`, `db:verify`, `db:verify:fresh`).
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
- Primary endpoint connection readiness diagnostics (`ready`, `runtime_state`, `runtime_message`) sourced from Docker runtime state, including `status=starting` during health-check warmup.
- Restore safety hardening: timestamp-to-LSN requests now send correct query parameters, unknown pageserver timestamp kinds are rejected, and restore branches are created atomically with attachment metadata.
- Primary endpoint status hardening: connection payload now clamps `ready=false` when runtime is stopped and returns `status=unhealthy` for running-but-unhealthy runtime state.
- Primary endpoint connection payload now includes endpoint password metadata, and the web console exposes password-aware connection helpers.
- Branch-scoped credential generation with random passwords for create/restore flows, persisted in branch state and applied on endpoint start/switch.
- Branch reset endpoint at `POST /api/v1/branches/{name}/reset` to recreate branch attachment from parent timeline head.
- Branch endpoint APIs for per-branch direct access: `POST /api/v1/branches/{name}/publish`, `POST /api/v1/branches/{name}/unpublish`, `GET /api/v1/branches/{name}/connection`, and `GET /api/v1/endpoints`.
- Docker-backed branch endpoint controller with persisted publish metadata, per-branch host-port allocation, and lazy branch-compute startup on first client connection.
- Branch endpoint controller wiring in the controller runtime, including Docker-mode initialization and branch endpoint config via `BRANCH_ENDPOINT_BIND_HOST`, `BRANCH_ENDPOINT_PORT_START`, and `BRANCH_ENDPOINT_PORT_END`.
- Branch endpoint API tests covering publish/unpublish/list/connection behavior and reset/delete integration points.
- Console branch management UI now includes branch endpoint publish/unpublish controls, branch DSN copy actions, published endpoint status list, and branch filtering in a Neon-style branch/compute layout.
- Dashboard view updates in the console with a project-overview header, storage/branch summary cards, a monitoring placeholder panel, and a branch summary list inspired by Neon dashboard structure.
- Console navigation now exposes only implemented pages (`Dashboard`, `Branches`), with a dedicated branches page showing parent-aware branch lineage and endpoint actions.
- Branch auto-publish behavior for docker/pageserver mode: active branches are auto-published on startup, newly created/restored branches are auto-published by default, and branch delete continues to unpublish before soft-delete.
- Console connection UX now removes primary-endpoint controls in favor of branch-first workflows (copy branch DSN from branch lists/endpoints and rely on auto-published branch endpoints).
- Console now includes a dedicated branch-overview page (basic metadata + connection details), driven by a left-sidebar branch selector that automatically opens overview when branch selection changes.
- Branch-scoped SQL execution API at `POST /api/v1/branches/{name}/sql/execute` with single-statement validation, read-only execution defaults, timeout/size limits, and structured result payloads (columns/rows/metadata).
- Console SQL editor now executes queries through the branch-scoped SQL API, renders result tables, and records branch-local run history alongside saved snippets.

### Fixed
- Compose pageserver startup now mounts only `identity.toml` and `pageserver.toml` as read-only files, keeps `/data/.neon` writable for runtime tenant state, and configures local-fs remote storage for current Neon runtime requirements.
- Compose controller now runs as root in Docker mode so it can access the mounted Docker socket for endpoint start/stop/switch orchestration.
- Compose primary endpoint defaults now use host port `55433` (instead of `5432`) to avoid conflicts with local PostgreSQL instances.
- Endpoint selection file writes now use cross-container-readable permissions so compute can consume updated branch attachment metadata.
- Compute wrapper startup now clears stale local Postgres socket lock files before launching compute to avoid restart-time lock collisions after branch switches.
- Reset/seed tooling now pins compose operations to this repository, adds HTTP timeout + transport-failure handling, and adds a non-local target safety gate for destructive resets.
- Compute wrapper image now bakes in `/shell/compute.sh` and compute config assets so dynamically created branch compute containers can boot without host bind mounts.
- Docker endpoint stop orchestration now uses a longer Docker Engine API client timeout to avoid premature timeout errors during primary endpoint branch switches.
- Branch names can now be reused after soft-delete; recreating a deleted branch key starts a fresh active branch record instead of returning `branch already exists`.
- Branch endpoint publish flow now avoids persisting `published=true` on failed listener/selection setup and rolls back listener state on persistence failures.
- Controller startup now tolerates branch-endpoint listener restore failures per branch (recorded as endpoint errors) instead of failing the whole process.
- Branch endpoint runtime IDs (selection paths/container names) now include a deterministic branch-name hash suffix to avoid slug-collision cross-branch routing.

### Changed
- Controller startup now uses the persistent branch store when a controller data directory is configured.
- Compose controller service now requires explicit basic auth password configuration.
- Documentation now reflects implemented auth, persistence, and operation logging behavior.
- Documentation now describes the seeded `branch_lab` fixture dataset and includes `psql` queries for manual branch-isolation checks.
- Branch mutation/restore APIs now return explicit `storage_error` responses for persistence failures.
- Documentation now reflects concrete compose storage-plane wiring and remaining compute-orchestration gap.
- Documentation now reflects health/preflight behavior and current scope boundaries for Neon-service health integration.
- Documentation now reflects Docker-based compute lifecycle orchestration and the remaining branch-to-timeline attachment gap.
- Documentation now reflects implemented endpoint readiness diagnostics and the remaining deeper Neon-runtime diagnostics gap.
- Restore now fails closed with `restore_unavailable` when pageserver-backed restore integration is unavailable.
- Documentation now clarifies readiness-based DSN emission and unhealthy primary endpoint status behavior.
- Reset/seed workflow now sets `branch_lab` default search path to `app, public` so seeded tables are visible with `\d`/`\dt` in `psql`.
- Branch reset now refreshes published branch endpoint attachment selection, and branch delete now unpublishes branch endpoint state before soft-delete.
- Compose controller now exposes a localhost branch endpoint port range (`56000-56049` by default) for published branch connections.
- Branch endpoint unpublish now maps branch persistence failures to `storage_error` responses consistently with other branch mutation handlers.
- README now includes embedded console screenshots (dashboard and SQL editor) for quick visual context on GitHub.
- README top-level framing now includes newcomer-friendly Neon context, a clear "What This Is" and "Features" section, and updated current-state language aligned with branch-first console workflows.
