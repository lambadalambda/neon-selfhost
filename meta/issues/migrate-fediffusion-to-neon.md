# Migrate fediffusion.art to Neon

## Summary

Use the smaller fediffusion.art Pleroma installation as the first production pilot for the self-hosted Neon stack.

## Requirements

- Create and verify fresh backups before changing the database runtime.
- Deploy Neon side-by-side without disrupting the existing Pleroma service.
- Preserve the PostgreSQL 16 schema, data, roles, and required extensions, including RUM.
- Keep the original PostgreSQL cluster available for rollback.
- Use conservative resource settings appropriate for a 4 GiB host.
- Record the deployment and rollback procedure without committing credentials.

## Acceptance Criteria

- The source database and Pleroma files have verified backups.
- The Neon stack is healthy and persists data across a controlled restart.
- The restored database passes extension, relation, row-count, and index validation.
- Pleroma operates against Neon after a controlled cutover.
- A rollback test or documented rollback verification confirms the original PostgreSQL service remains usable.
- The repository test suite and compose validation pass after any required code or configuration changes.

## Notes

- Source host: `root@fediffusion.art`.
- Source database: PostgreSQL 16 `pleroma`, approximately 4.24 GiB.
- Required non-core index: `public.objects_fts` using RUM 1.3.
- Keep secrets only on the deployment host in a mode-0600 environment file.
