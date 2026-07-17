# Make controller database restore safe

## Summary

The mise restore task can overwrite databases used by a running controller and defaults to a host path that does not contain the Compose deployment's named volume data.

## Requirements

- Refuse to restore while the target controller is running.
- Provide a restore path that targets the Podman Compose controller volume.
- Avoid reporting success when no controller databases were restored.
- Document native and Compose restore usage accurately.

## Acceptance Criteria

- Automated tests cover running-controller refusal and missing database handling.
- Native restore requires explicit, valid source and target data.
- Podman Compose users have a documented task that restores the named volume safely.
