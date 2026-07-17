# Harden Docker access and database credentials

## Summary

The default deployment combines a root controller, unrestricted Docker socket access, and a known fallback database credential.

## Requirements

- Remove known database password defaults and require explicit Compose configuration.
- Reduce controller privileges or constrain Docker API access without breaking endpoint orchestration.
- Keep local-first deployment behavior documented accurately.

## Acceptance Criteria

- Compose validation fails when the endpoint password is absent.
- Configuration tests reject missing Docker-mode endpoint credentials.
- The controller no longer needs unrestricted root execution solely to access Docker.
- Documentation describes the remaining Docker trust boundary.
