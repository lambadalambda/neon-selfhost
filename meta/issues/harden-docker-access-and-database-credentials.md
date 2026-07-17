# Harden Podman access and database credentials

## Summary

The default deployment combines engine-level Podman socket access with a known fallback database credential.

## Requirements

- Remove known database password defaults and require explicit Compose configuration.
- Reduce controller privileges or constrain Docker API access without breaking endpoint orchestration.
- Keep local-first deployment behavior documented accurately.

## Acceptance Criteria

- Compose validation fails when the endpoint password is absent.
- Configuration tests reject missing Docker-compatible endpoint credentials.
- The controller no longer runs as root solely to access Podman.
- Documentation describes the remaining Podman socket trust boundary.
