# Add operations and diagnostics console

## Summary

Expose persisted operation history and controller diagnostics to operators.

## Requirements

- Add an operations view with status, type, branch, timestamps, and error detail.
- Support existing status/type filters and pagination.
- Show persistence mode, schema versions, and component health details.
- Redact credentials and sensitive paths where appropriate.

## Acceptance Criteria

- Failed branch, restore, and endpoint operations are diagnosable from the console.
- In-flight and terminal states update without reloading the whole page.
- Sensitive values are never rendered.
