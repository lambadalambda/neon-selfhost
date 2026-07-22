# Expose endpoint lifecycle controls

## Summary

Surface existing primary and branch endpoint lifecycle APIs in the console.

## Requirements

- Show start, stop, switch, publish, and unpublish controls where supported.
- Display readiness, runtime state/message, active connections, and last error.
- Confirm disruptive actions and prevent conflicting operations.
- Preserve automatic publication as the conservative default.

## Acceptance Criteria

- Operators can recover or switch endpoints without direct API calls.
- Controls reflect actual runtime state and disable unsafe transitions.
- Failures link to operation details and preserve the previous healthy endpoint.
