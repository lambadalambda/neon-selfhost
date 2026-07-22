# Add disk pressure guardrails

## Summary

Detect and prevent unsafe operations before the single-node storage volume fills.

## Requirements

- Monitor filesystem free space and major Neon storage consumers.
- Warn at configurable thresholds and block space-amplifying operations at a critical threshold.
- Surface pageserver GC and safekeeper WAL recycling health.
- Never delete layers or WAL files directly as an automated response.

## Acceptance Criteria

- Operators receive actionable warnings before service failure.
- Branch, restore, and snapshot-like operations fail safely under critical pressure.
- Guardrail decisions and overrides are logged.
