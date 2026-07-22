# Enforce one compute writer per timeline

## Summary

Add a data-plane lease that prevents two compute processes sharing the compute-state volume from writing the same tenant/timeline.

## Requirements

- Acquire a nonblocking filesystem lease keyed by validated tenant and timeline IDs before starting `compute_ctl`.
- Hold the lease file descriptor across `exec` for the compute process lifetime.
- Allow different timelines to run concurrently.
- Fail startup clearly when the lease is already held or the IDs are invalid.
- Keep the lease directory on the shared compute-state volume.

## Acceptance Criteria

- A second compute for the same tenant/timeline exits before contacting safekeeper.
- A compute for a different timeline can start concurrently.
- Killing or stopping the owner releases the lease.
- Deployment tests, shell syntax checks, Compose validation, and production fencing smoke tests pass.

## Outcome

- Compute startup acquires a validated tenant/timeline `flock` lease before `compute_ctl` can contact safekeeper.
- A root init service prepares the shared volume and sticky lease directory before controller or compute startup.
- Primary, init, and dynamic branch computes use the same configured wrapper image; stopped stale branch containers are recreated and running stale containers fail closed.
- Linux behavioral tests cover lease inheritance across `exec`, contention, distinct timelines, invalid IDs, and release.
- Go, race, shell syntax, and plain/profile Compose checks pass.
