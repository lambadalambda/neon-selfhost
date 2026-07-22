# Report real storage usage

## Summary

Replace the misleading controller-metadata size card with actual Neon storage information.

## Requirements

- Collect pageserver tenant/timeline storage metrics safely.
- Report project total and branch/timeline storage where meaningful.
- Distinguish local layers, retained history, remote storage, and controller metadata.
- Clearly mark unavailable or estimated values.

## Acceptance Criteria

- Dashboard storage never presents metadata JSON size as database storage.
- Values have defined units, collection time, and source.
- Collection failure degrades observability without affecting database availability.
