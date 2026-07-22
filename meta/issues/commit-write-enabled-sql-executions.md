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
