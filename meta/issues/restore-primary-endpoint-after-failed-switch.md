# Restore primary endpoint after failed switch

## Summary

A failed primary endpoint branch switch can leave the previously running endpoint stopped even though controller state still identifies the previous branch.

## Requirements

- Restore the previous endpoint selection when switching fails after the old runtime is stopped.
- Restart the previous runtime when it was running before the switch.
- Preserve useful errors when both the switch and rollback fail.

## Acceptance Criteria

- A failing selection write restores service on the previous branch.
- A failing runtime start restores the previous selection and restarts the previous runtime.
- Tests cover successful rollback and rollback failure.
