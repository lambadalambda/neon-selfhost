# Clean up orphaned pageserver timelines

## Summary

Branch create, restore, and reset failure paths can leave pageserver timelines that are not tracked by controller state.

## Requirements

- Add pageserver timeline deletion support with conservative error handling.
- Compensate when persistence or publishing fails after timeline creation.
- Do not hide the original failure when cleanup also fails.

## Acceptance Criteria

- Restoring to an existing branch name does not leak a timeline.
- Auto-publish and persistence failures attempt timeline cleanup.
- Tests cover cleanup success and cleanup failure reporting.
