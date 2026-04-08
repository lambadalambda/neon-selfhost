# neon-selfhost

`neon-selfhost` aims to be a Docker-first self-hosting project for running open-source Neon with a simple admin web UI.

The goal is to make Neon branching and point-in-time restore practical for small deployments (for example, safe app upgrades and fast rollback).

Status: pre-alpha planning scaffold. No runnable implementation is committed yet.

## What This Project Is

- A thin control plane and web console for self-hosted Neon.
- Focused on single-node, single-tenant use cases.
- Optimized for simple operations and safe defaults.

## What This Project Is Not

- A full replacement for the managed `neon.com` platform.
- A multi-tenant cloud control plane.
- A "run everything everywhere" orchestration layer.

## Planned MVP Capabilities

- Bring up Neon components with Docker Compose.
- Manage branches/timelines from a web UI.
- Restore to a past timestamp by creating a branch at a resolved LSN.
- Start/stop/switch a primary compute endpoint.

## Operational Caveats (MVP)

- Single-node deployment is not HA. Host or disk loss can still cause data loss.
- Docker named volumes are not backups.
- Branching and PITR retention increase disk usage; in Phase 1, operations must fail safely with clear errors/logs, while proactive disk guardrails are planned for Phase 2.
- Soft-deleting a branch does not imply immediate disk reclamation.
- If exposing UI or Postgres beyond localhost, terminate TLS via a reverse proxy; basic auth alone is not Internet-grade protection.

## Repository Layout

- `AGENTS.md` - contribution and coding-agent rules.
- `docs/architecture.md` - architecture, deployment topology, and safety model.
- `docs/mvp-roadmap.md` - phased plan for delivery.

## Current Status

Planning and scaffolding phase.

See `docs/architecture.md` for the current target design.
