# Development

This guide covers controller development, test workflows, the local fixture dataset, API routes, and controller metadata recovery. The main [README](../README.md) stays focused on the current product and full-stack quickstart.

## Toolchain

- Go 1.24 or newer (`mise` currently selects Go 1.25.7).
- Podman with Podman Compose for full-stack and smoke testing.
- `mise` for the repository task shortcuts.
- `agent-browser` for browser-tagged console tests.
- `psql`, `curl`, and `jq` for fixture and smoke scripts.

Install the configured toolchain and run the default test suite:

```bash
mise install
mise run test
```

## Controller-Only Development

Run the native controller without the Neon data plane:

```bash
mise exec -- go run ./cmd/controller
```

The console is available at <http://127.0.0.1:8080/>. This default mode is useful for controller and UI work, but database discovery, SQL execution, schema browsing, point-in-time restore, and published branch endpoints require the full container-engine stack.

Enable basic auth when exercising authentication paths:

```bash
BASIC_AUTH_USER=admin \
BASIC_AUTH_PASSWORD=change-me \
mise exec -- go run ./cmd/controller
```

Controller metadata persists when `CONTROLLER_DATA_DIR` is set. The controller creates `controller.db` for branch and SQL-library state and `operations.db` for operation history.

## Full Stack

Start the controller and Neon services:

```bash
PRIMARY_ENDPOINT_PASSWORD=replace-me \
BASIC_AUTH_PASSWORD=change-me \
mise run stack:up
```

Useful lifecycle commands:

```bash
mise run stack:ps
mise run stack:logs
mise run stack:down
```

The default local ports are:

| Port | Purpose |
| --- | --- |
| `8080` | Controller UI and API |
| `55433` | Primary PostgreSQL endpoint |
| `56000-56049` | Published branch PostgreSQL endpoints |

Set `CONTROLLER_HOST_PORT` when port `8080` is occupied. For a non-Podman Docker socket, set `CONTAINER_ENGINE_SOCKET` and its numeric `CONTAINER_ENGINE_GID`. If `COMPOSE_PROJECT_NAME` is changed, set `DOCKER_COMPOSE_PROJECT` to the same value so the controller can resolve Compose resources.

For a dedicated data filesystem, prepare `controller`, `compute`, `compute-cache`, `pageserver`, and `safekeeper1` under the selected root. Assign `controller` and `compute` to UID `65532`, and the remaining directories to UID `1000`, then run:

```bash
COMPOSE_FILE=docker-compose.yml:docker-compose.storage.yml \
NEON_DATA_ROOT=/mnt/neon \
PRIMARY_ENDPOINT_PASSWORD=replace-me \
BASIC_AUTH_PASSWORD=change-me \
podman compose --profile neon up -d --build
```

The root initialization service validates the shared compute-state layout and creates the writer-lease directory before the controller and compute start.

## Tests

Run the standard unit and package tests:

```bash
mise run test
```

CI runs the same `go test ./...` command. The race suite is also expected to pass:

```bash
mise exec -- go test -race ./...
```

Browser tests require `agent-browser` and use the `browser` build tag:

```bash
mise exec -- go test -tags=browser ./internal/server
```

SQL execution and schema catalog integration tests require a disposable PostgreSQL database. They create and remove temporary schemas:

```bash
SQL_EXECUTION_TEST_DATABASE_URL='postgresql://user:password@127.0.0.1:5432/database' \
mise exec -- go test -tags=integration ./internal/server
```

## Smoke Tests

Run the API smoke test against an active local stack:

```bash
mise run smoke
```

Start and stop a clean stack around the smoke test:

```bash
PRIMARY_ENDPOINT_PASSWORD=replace-me mise run smoke:fresh
```

Verify lazy branch compute to primary-writer handoff in an isolated disposable Compose project:

```bash
PRIMARY_ENDPOINT_PASSWORD=replace-me mise run smoke:writer-handoff
```

Writer-handoff mode refuses non-loopback targets, creates an unoverrideable temporary project and volumes, verifies writes on both sides of the handoff, and retains the stack for inspection if handback fails. Do not point it at a persistent or production stack.

## Branch Fixture

Reset and seed the `branch_lab` database on `main` for manual console checks:

```bash
mise run db:reset-seed
```

The fixture creates schema `app` with:

- `app.accounts`: `acme` (`pro`), `globex` (`starter`), and `initech` (`enterprise`).
- `app.documents`: four documents distributed across the three accounts.
- A default search path of `app, public`.

Verify branch isolation by creating a temporary branch, changing its data, and confirming `main` remains unchanged:

```bash
mise run db:verify
```

Run the same verification with managed stack startup and teardown:

```bash
PRIMARY_ENDPOINT_PASSWORD=replace-me mise run db:verify:fresh
```

The backing script is `scripts/reset_seed_data.sh`. It refuses non-local controller URLs unless explicitly forced because seed mode drops and recreates `branch_lab`.

## API Inventory

The web console and `/api/v1/*` routes are under the authenticated controller origin when basic auth is configured. The internal `POST /validate` pageserver upcall is intentionally exempt from basic auth and uses its own fail-closed tenant-generation validation.

### Controller

- `GET /api/v1/status`
- `GET /api/v1/health`
- `GET /api/v1/operations`

### Branches and Restore

- `GET /api/v1/branches`
- `POST /api/v1/branches`
- `POST /api/v1/branches/{name}/reset`
- `DELETE /api/v1/branches/{name}`
- `POST /api/v1/restore`

Branch deletion is a soft-delete of controller state. Restore accepts an RFC3339 timestamp and creates a new timeline-backed branch at the pageserver-resolved LSN.

### Branch Endpoints and Data

- `POST /api/v1/branches/{name}/publish`
- `POST /api/v1/branches/{name}/unpublish`
- `GET /api/v1/branches/{name}/connection`
- `GET /api/v1/branches/{name}/databases`
- `GET /api/v1/branches/{name}/schema`
- `GET /api/v1/branches/{name}/schema/table`
- `POST /api/v1/branches/{name}/sql/execute`
- `GET /api/v1/endpoints`

SQL execution accepts one statement, defaults to an explicit read-only transaction, and applies query, timeout, row, result-size, and cell-size limits. Write mode requires `allow_writes=true`; writes to `main` additionally require `confirm_protected_writes=true`.

### SQL Library

- `GET|POST /api/v1/sql/saved-queries`
- `PATCH|DELETE /api/v1/sql/saved-queries/{id}`
- `GET /api/v1/sql/history`

Saved SQL and history store operator-entered query text verbatim. They do not store result rows or controller-managed DSNs, but secrets typed into SQL are still persisted and should be avoided.

### Primary Endpoint

- `POST /api/v1/endpoints/primary/start`
- `POST /api/v1/endpoints/primary/stop`
- `POST /api/v1/endpoints/primary/switch`
- `GET /api/v1/endpoints/primary/connection`

The connection response includes readiness diagnostics and returns a DSN only when the runtime is ready. Primary lifecycle actions are currently API-driven rather than console controls.

### Pageserver Upcall

- `POST /validate`

Standalone pageserver deployments can call this internal route to validate tenant generations against `PAGESERVER_VALID_TENANT_GENERATIONS`. It is deliberately outside basic auth because pageserver does not send operator credentials, and it rejects requests unless the configured tenant and generation match.

## Controller Metadata Backup

These workflows back up controller metadata only. They do not back up pageserver layers, safekeeper WAL, or compute data and are not a Neon disaster-recovery solution.

For a stopped native controller:

```bash
CONTROLLER_DATA_DIR=.data/controller mise run db:backup
CONTROLLER_DATA_DIR=.data/controller \
BACKUP_DIR=/path/to/backup-dir \
mise run db:restore
```

For Compose, stop the controller before exporting or replacing the controller-state volume:

```bash
export BASIC_AUTH_PASSWORD=change-me
export PRIMARY_ENDPOINT_PASSWORD=replace-me
podman compose stop controller
mise run db:backup:compose
BACKUP_DIR=/path/to/backup-dir mise run db:restore:compose
podman compose --profile neon up -d
```

Restore validates both SQLite databases and exports rollback state before replacing the volume. The default volume is `neon-selfhost_controller_state`; set `CONTROLLER_VOLUME` if the Compose provider uses another name.

## Repository Layout

- `cmd/controller`: controller entrypoint.
- `internal/branch`: branch model and SQLite persistence.
- `internal/config`: environment configuration and validation.
- `internal/controllerdb`: shared SQLite migration and backup primitives.
- `internal/server`: HTTP API, embedded console, endpoint orchestration, SQL execution, and schema browsing.
- `configs/neon/pageserver`: pageserver runtime configuration.
- `configs/neon/compute_wrapper`: wrapped compute image and writer-lease startup logic.
- `scripts`: smoke, fixture, and controller metadata recovery tools.
- `docs`: architecture, roadmap, and development documentation.
- `meta`: repository-local issue tracker.

## Delivery Workflow

Repository changes follow the cadence documented in `AGENTS.md`:

1. Create or select a matching issue in `meta/issues.md`.
2. Use red-green-refactor TDD for testable behavior.
3. Run relevant unit, race, browser, integration, or smoke verification.
4. Get a code or documentation review and resolve findings.
5. Update `changelog.md`, archive the issue, and make a small topical commit.
