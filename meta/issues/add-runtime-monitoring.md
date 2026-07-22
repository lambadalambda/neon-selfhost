# Add runtime monitoring

## Summary

Replace the decorative monitoring placeholder with real single-node operational metrics.

## Requirements

- Collect compute CPU, memory, connections, and uptime.
- Collect pageserver and safekeeper health, disk, and WAL indicators.
- Provide branch/compute selection and bounded time ranges.
- Keep collection lightweight and resilient when a component is unavailable.

## Acceptance Criteria

- Dashboard charts display sourced data rather than animation.
- Operators can identify stopped, idle, overloaded, or storage-constrained components.
- Metrics retention and disk cost are documented and bounded.
