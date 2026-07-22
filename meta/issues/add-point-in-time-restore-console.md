# Add point-in-time restore console

## Summary

Expose the existing point-in-time restore API as a safe branch-scoped console workflow.

## Requirements

- Add a Backup & Restore branch page.
- Select source branch, RFC3339 timestamp, and optional target branch name.
- Preview the new branch name and explain that restore creates a branch rather than overwriting data.
- Show operation progress and actionable retained-history errors.

## Acceptance Criteria

- Operators can create and open a restored branch without calling the API manually.
- Invalid or unavailable timestamps fail closed without changing active branches.
- Restore results link to branch overview and connection helpers.
