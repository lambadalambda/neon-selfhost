# Drain HTTP requests before closing resources

## Summary

Controller shutdown closes branch, endpoint, and operation resources before the HTTP server stops accepting and drains requests.

## Requirements

- Stop accepting new requests and drain in-flight handlers before closing their dependencies.
- Preserve bounded shutdown behavior.
- Log shutdown failures without silently skipping remaining cleanup.

## Acceptance Criteria

- A test demonstrates an in-flight handler can finish before its backing resource closes.
- Shutdown remains bounded by a timeout.
- All controller resources are closed after HTTP draining completes.
