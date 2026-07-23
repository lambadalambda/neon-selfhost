# Development Log

## 2026-07-23

- Reframed the README as a current product overview, captured sanitized screenshots from the live Docker-mode controller, and moved development and verification workflows into `docs/development.md`. Screenshot capture must use a fresh tunnel to remote port `18080`; an existing local controller can otherwise look identical while running in no-op mode.
- Added and visually verified the responsive Tables/schema browser: fixed read-only catalog queries, bounded search/detail APIs, desktop three-pane inspection, mobile drill-down, and non-executing SQL Editor handoff.
- Replaced the SQL Editor's browser-local sample library with controller-backed saved-query CRUD and execution history in `controller.db`; browser coverage verifies reload persistence, context/project scopes, context switching, and history refresh.
- Protected SQL writes now require a two-step keyboard-accessible confirmation and reset to read-only whenever branch, database, or connection context changes.
- Added and browser-verified the point-in-time restore console workflow, including explicit non-overwrite guidance, retained-history failures that preserve branch selection, and desktop/mobile result navigation into existing connection helpers.
- Verified primary writer handoff end to end with a disposable Podman Compose stack: a lazy child compute wrote a marker, was drained and removed, primary adopted the child timeline, SQL continued through primary, and handback to `main` succeeded.
- Podman's Docker-compatible container list can omit health from `Status` even when inspect reports `State.Health.Status=healthy`; primary readiness now inspects health when list metadata is inconclusive.
- Podman dynamic compute DNS names must stay within the 63-character label limit. The guarded smoke uses a shorter unique project name so its generated branch compute remains resolvable.
- The non-root controller requires `label=disable`, and therefore runs without SELinux confinement, to use the Podman socket mounted at `/var/run/docker.sock`.
