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

### Changed
- Controller startup now uses the persistent branch store when a controller data directory is configured.
- Compose controller service now requires explicit basic auth password configuration.
- Documentation now reflects implemented auth, persistence, and operation logging behavior.
- Branch mutation/restore APIs now return explicit `storage_error` responses for persistence failures.
- Documentation now reflects concrete compose storage-plane wiring and remaining compute-orchestration gap.
