# Development Log

## 2026-07-23

- Verified primary writer handoff end to end with a disposable Podman Compose stack: a lazy child compute wrote a marker, was drained and removed, primary adopted the child timeline, SQL continued through primary, and handback to `main` succeeded.
- Podman's Docker-compatible container list can omit health from `Status` even when inspect reports `State.Health.Status=healthy`; primary readiness now inspects health when list metadata is inconclusive.
- Podman dynamic compute DNS names must stay within the 63-character label limit. The guarded smoke uses a shorter unique project name so its generated branch compute remains resolvable.
- The non-root controller requires `label=disable`, and therefore runs without SELinux confinement, to use the Podman socket mounted at `/var/run/docker.sock`.
