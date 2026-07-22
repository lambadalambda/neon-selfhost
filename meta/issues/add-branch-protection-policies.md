# Add branch protection policies

## Summary

Replace hard-coded `main` protection with explicit persisted branch policy.

## Requirements

- Persist a protected flag and conservative defaults for the primary branch.
- Gate reset, delete, write-enabled SQL, switch, and other destructive actions.
- Add protect/unprotect controls with confirmation and operation history.
- Preserve protection across controller restarts.

## Acceptance Criteria

- Protected branches reject destructive API calls as well as hiding UI actions.
- Protection changes are auditable.
- Existing `main` deployments migrate to protected without operator action.
