# Prevent concurrent writers on branch timelines

## Summary

Opening the branch SQL endpoint for the branch already served by the primary compute can start a second writable compute on the same timeline. Safekeeper fencing then crashes one or both computes and interrupts the application.

## Requirements

- Never start two writable computes for the same tenant/timeline.
- Reuse the running primary compute for its selected branch, or fail closed until safe routing is available.
- Keep branch endpoints on distinct child timelines independently startable.
- Reconcile stale branch compute containers before starting or switching the primary.
- Log and surface conflicts without repeatedly restarting computes.

## Acceptance Criteria

- SQL access to the active primary branch does not create a branch compute container.
- Repeated read and write queries do not change the primary compute process uptime.
- Non-primary branch endpoints still start lazily on their own timelines.
- Switching the primary cannot leave the previous and target timelines with conflicting writers.
- Unit, race, Compose, and production smoke tests pass.

## Incident

- On `2026-07-22`, a production SQL API probe for `main` started `neon-selfhost-branch-main-*` while the Pleroma primary compute was active.
- Safekeeper rejected the older writer term; the branch compute panicked and the primary compute restarted.
- The probe table was removed and the public Pleroma API remained healthy after recovery.

## Subissues

- [Enforce one compute writer per timeline](enforce-one-compute-writer-per-timeline.md)
- [Route the primary branch endpoint through primary compute](route-primary-branch-through-primary-compute.md)
- Coordinate primary start/switch with branch compute lifecycle and reconcile stale containers.
