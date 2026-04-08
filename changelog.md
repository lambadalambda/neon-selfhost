# Changelog

## Unreleased

### Added
- HTTP basic auth support for controller API routes via `BASIC_AUTH_USER` and `BASIC_AUTH_PASSWORD`.
- Persistent branch store support via `CONTROLLER_DATA_DIR`, with branch state written to `branches.json`.
- Serialized branch mutation execution and in-memory operation logging exposed by `GET /api/v1/operations`.
- New tests for config loading, auth enforcement, operation logging, and branch persistence.

### Changed
- Controller startup now uses the persistent branch store when a controller data directory is configured.
- Compose controller service now requires explicit basic auth password configuration.
- Documentation now reflects implemented auth, persistence, and operation logging behavior.
