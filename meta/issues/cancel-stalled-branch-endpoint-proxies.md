# Cancel stalled branch endpoint proxies

## Summary

The branch TCP proxy waits for both copy directions indefinitely, allowing a silent backend to retain goroutines, sockets, and active-connection capacity after a client disconnects.

## Requirements

- Ensure both proxy copy loops terminate when either side exits unexpectedly.
- Preserve normal bidirectional PostgreSQL traffic and half-close behavior where useful.
- Release active-connection accounting promptly.

## Acceptance Criteria

- A regression test reproduces a silent peer after the opposite side closes.
- The proxy returns and closes both connections within a bounded interval.
- Existing endpoint proxy tests remain green.
