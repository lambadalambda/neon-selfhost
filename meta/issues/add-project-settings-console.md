# Add project settings console

## Summary

Expose useful single-project configuration and runtime facts without editing environment files blindly.

## Requirements

- Add a read-only settings summary first.
- Show version, runtime mode, Postgres version, data locations, endpoint policy, and retention settings.
- Redact credentials and secrets.
- Add mutable controls only where validation and rollback are defined.

## Acceptance Criteria

- Operators can confirm effective configuration from the console.
- Every displayed value identifies whether it is runtime, persisted, or environment-derived.
- No setting can be changed without validation and an explicit effect description.
