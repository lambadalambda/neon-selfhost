# Select the SQL editor database

## Summary

Allow SQL editor queries to target any connectable database on the selected branch.

## Requirements

- List connectable, non-template databases through a branch-scoped API.
- Allow SQL execution requests to select a database while preserving the branch endpoint database as the default.
- Add an accessible database selector to the SQL editor.
- Preserve the selected database independently for each branch while the page is open.

## Acceptance Criteria

- The selector lists both `postgres` and `pleroma` on fediffusion.
- Selecting `pleroma` allows the SQL editor to query `public.activities`.
- Existing clients that omit a database retain current behavior.
- Backend, frontend, and Compose tests pass.

## Outcome

- The deployed fediffusion console lists `postgres` and `pleroma`, preserving `postgres` as the default.
- A browser-driven read-only query against `pleroma.public.activities` completed successfully.
- Branch and database transitions clear write mode; stale connection responses and unavailable remembered databases fail closed.
- API tests cover the default database behavior for requests that omit `database`.
- Go tests, race tests, and Compose validation pass.
