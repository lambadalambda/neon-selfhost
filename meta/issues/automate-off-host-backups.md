# Automate off-host backups

## Summary

Add scheduled, verified backups that survive loss of the single Neon host.

## Requirements

- Back up controller metadata and required Neon state to configurable off-host storage.
- Define retention, encryption, credentials handling, and failure reporting.
- Verify archives automatically and perform documented restore drills.
- Take a mandatory backup before upgrades.

## Acceptance Criteria

- Host or volume loss has a tested recovery path.
- Backup success is not reported until integrity verification completes.
- Restore documentation includes recovery-point and recovery-time expectations.
