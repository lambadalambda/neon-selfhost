# Verify primary writer handoff

## Summary

Exercise primary switching against a disposable stack with a running lazy target compute so writer drain, removal, and handoff behavior is verified end to end.

## Requirements

- Require an explicit destructive smoke flag and managed disposable stack lifecycle.
- Start a child branch compute through its SQL endpoint before switching primary.
- Verify the lazy target compute exists before the switch and is removed during the switch.
- Verify target SQL routes through the healthy primary after handoff without recreating a branch compute.
- Switch back to the original branch and clean up all smoke resources.

## Acceptance Criteria

- The guarded smoke test passes against a fresh disposable Compose stack.
- Primary and target timelines never have simultaneous branch and primary compute writers.
- Unit, race, shell syntax, vet, and Compose checks pass.
