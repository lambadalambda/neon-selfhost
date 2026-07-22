# Warn before writes to protected branches

## Summary

Make production-impacting SQL writes visibly distinct and require deliberate confirmation.

## Requirements

- Show a persistent protected-branch warning for `main` and future protected branches.
- Keep SQL read-only by default.
- Require an explicit confirmation after enabling writes on a protected branch.
- Clear write mode whenever branch, database, or connection context changes.

## Acceptance Criteria

- Protected-branch writes cannot run through an accidental single toggle or stale selection.
- Read-only queries remain low-friction.
- Warning and confirmation controls are keyboard and screen-reader accessible.
