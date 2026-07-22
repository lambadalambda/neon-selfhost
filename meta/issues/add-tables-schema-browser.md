# Add tables and schema browser

## Summary

Provide branch and database-scoped schema inspection without requiring hand-written catalog queries.

## Requirements

- Add a Tables page using read-only PostgreSQL catalog queries.
- Browse schemas, tables, columns, indexes, and constraints.
- Show estimated row counts and sizes when available.
- Link generated inspection queries into the SQL editor.

## Acceptance Criteria

- Operators can inspect common schema objects on any selected database.
- Catalog access remains read-only and bounded.
- Large schemas remain searchable and responsive.
