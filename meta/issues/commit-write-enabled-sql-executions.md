# Commit write-enabled SQL executions

## Summary

Successful SQL executions with write mode enabled currently return success but are rolled back.

## Requirements

- Commit a successful read-write transaction before returning success.
- Keep read-only execution and all existing timeout, row, byte, and statement limits.
- Roll back failed, canceled, timed-out, or partially consumed executions.
- Return a clear error when commit fails.

## Acceptance Criteria

- A successful write remains visible to a later connection.
- Failed writes leave no partial changes.
- Read-only requests still reject writes.
- Unit, integration, race, and Compose checks pass.
- A disposable production probe is committed, observed, and removed after deployment.

## Outcome

- Successful read-write executions close their result stream and commit through a bounded context.
- Truncated writes, read-only writes, query failures, and pre-commit cancellation roll back without committing.
- Definitive commit rollback and unknown commit outcome errors have distinct API responses.
- Unit, API, integration, race, vet, and Compose checks pass.
