# Persist the SQL query library

## Summary

Saved queries and run history currently disappear on page reload.

## Requirements

- Persist saved queries and bounded execution history in controller storage.
- Scope entries by branch and database while supporting project-wide lookup.
- Support create, rename, update, and delete for saved queries.
- Avoid storing result data or database credentials.

## Acceptance Criteria

- Saved queries survive browser and controller restarts.
- History retention is bounded and configurable.
- Legacy in-memory sample behavior is removed or clearly marked as an example.
