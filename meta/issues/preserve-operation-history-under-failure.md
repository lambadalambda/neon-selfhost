# Preserve operation history under failure

## Summary

Rejected requests can evict the active operation entry, and operation-store failures can silently produce stale or volatile audit history.

## Requirements

- Keep active operations finishable regardless of history trimming.
- Surface operation persistence degradation accurately.
- Avoid reporting successful persistence when terminal state was not stored.

## Acceptance Criteria

- A regression test covers a running operation with enough rejections to exceed retention.
- Store load and upsert failures are represented in health or operation results.
- Persisted operations do not remain incorrectly running after normal completion.
