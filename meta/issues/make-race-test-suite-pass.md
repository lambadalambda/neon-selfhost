# Make the race test suite pass

## Summary

The branch endpoint idle-timeout test reads and writes tracking engine state concurrently without synchronization.

## Requirements

- Synchronize the tracking test double without masking production races.
- Avoid timing-only polling where a deterministic signal is practical.

## Acceptance Criteria

- `go test -race ./...` passes.
- The idle-timeout test still verifies the expected container stop call.
