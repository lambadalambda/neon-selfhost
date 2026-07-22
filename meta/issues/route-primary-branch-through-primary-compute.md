# Route the primary branch endpoint through primary compute

## Summary

The published endpoint for the branch selected as primary must proxy to the existing primary compute rather than starting a branch compute on the same timeline.

## Requirements

- Resolve the selected primary branch and attachment through the primary endpoint controller.
- Route its branch listener to an explicit internal primary backend address.
- Return the primary compute's actual database credentials through branch connection APIs.
- Fail closed when primary state, attachment, readiness, or backend routing cannot be verified.
- Keep different child timelines on the existing lazy branch-compute path.
- Never fall back to creating a branch compute after a primary-route failure.

## Acceptance Criteria

- Repeated primary-branch reads and committed writes create no branch compute container.
- Primary container ID and PostgreSQL start time remain unchanged.
- Stopped, starting, unhealthy, mismatched, or unavailable primary state fails closed.
- A different child branch still starts a distinct lazy compute.
- Unit, API, race, Compose, and production smoke tests pass.
