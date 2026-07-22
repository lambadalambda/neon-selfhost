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

## Migration Log

### 2026-07-22

- Created and checksum-verified a 3.5 GiB backup at
  `/var/backups/neon-migration-20260722T100402Z` before changing the target.
- Deployed the Neon state directories on the dedicated 100 GiB filesystem at
  `/mnt/HC_Volume_106436269` and left the native PostgreSQL source online.
- Restored all 46 public tables and completed the post-data restore after
  deferring `activities_visibility_index`. Building that non-unique expression
  index scanned `public.activities` too slowly for the initial restore; all
  other indexes, including the RUM `objects_fts` index, completed successfully.
- Final target validation reports 160 public indexes, 99 constraints, two user
  triggers, no invalid indexes, and the same extension versions as the source.
  `non_negative_priority` is intentionally `NOT VALID` on both databases.
- The source continued accepting writes during the initial restore, so a
  logical subscription replaced the stale target data from a consistent source
  snapshot and streamed subsequent changes.
- Enabled logical WAL with a three-second source PostgreSQL restart. Pleroma
  reconnected automatically and its public NodeInfo endpoint remained healthy.
- Completed an initial-copy logical subscription for 45 durable public tables.
  PostgreSQL intentionally excludes the unlogged `oban_peers` coordination
  table; it was left empty for Oban to repopulate and now contains its target
  coordination row.
- A Neon checkpointer panic after post-data cleanup exposed that the Compose
  services had no restart policy. The compute recovered with unchanged row and
  index validation, and the deployment now uses `unless-stopped` policies.
- Set target-only PITR retention to zero during the import and garbage-collected
  99 obsolete layers. A generation-checked controller upcall replaced the
  placeholder control-plane URL so queued local-fs remote deletions can execute.
  The permanent implementation is commit `c10c1d6`; unknown or unconfigured
  tenant generations return 503 so deletion work is retried rather than lost.
- Completed the final zero-lag sequence synchronization and switched Pleroma to
  Neon on port 55433 at 2026-07-22 13:07 UTC. Public NodeInfo, instance, and
  public-timeline APIs returned HTTP 200; new rows appeared only on Neon.
- Removed the migration subscription, source slot, publication, role, proxy,
  and temporary firewall rule after post-cutover validation.

## Completed Procedure

1. Enabled logical WAL with a controlled three-second source restart.
2. Copied a consistent snapshot and streamed changes through a restricted
   all-tables publication and subscription.
3. Verified all durable table counts, extension versions, constraints, triggers,
   and indexes, then built `activities_visibility_index` concurrently.
4. Stopped Pleroma, waited for zero lag, synchronized all 38 sequences, disabled
   replication, and changed only the configured database port to 55433.
5. Verified Pleroma database sessions and public APIs before removing migration
   replication infrastructure. The source PostgreSQL cluster remains active on
   port 5432 with no Pleroma sessions.

## Rollback

The pre-cutover Pleroma configuration is preserved at
`/etc/pleroma/config.exs.pre-neon-20260722T130612Z`.

1. Stop Pleroma with `systemctl stop pleroma`.
2. Reconcile or explicitly accept the loss of writes committed to Neon after
   2026-07-22 13:07 UTC; logical replication was one-way and is now removed.
3. Restore the preserved config over `/etc/pleroma/config.exs` while retaining
   its original ownership and mode.
4. Start Pleroma with `systemctl start pleroma` and verify it has sessions on
   native PostgreSQL port 5432 before checking public APIs.

## Outcome

- Verified backups remain available and the native PostgreSQL cluster is intact.
- Neon persisted through controlled component restarts on the dedicated volume.
- Schema, data, roles, extensions, indexes, sequences, and application login
  were validated before cutover.
- Pleroma is serving production traffic from Neon, with fresh writes confirmed
  on the target and no source application sessions.
- The documented rollback path preserves the original cluster while explicitly
  accounting for post-cutover write divergence.
- The single-pageserver deployment still uses Neon's emergency attachment mode;
  startup logs this explicitly, and no second pageserver may attach to the same
  local-fs remote storage.
