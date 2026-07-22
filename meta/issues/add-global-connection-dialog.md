# Add global connection dialog

## Summary

Make branch connection information available from every console page.

## Requirements

- Add a global Connect action for the selected branch.
- Select database and show DSN, `psql`, host, port, user, and password.
- Provide explicit copy controls and readiness/error states.
- Never place credentials in URLs, logs, or analytics.

## Acceptance Criteria

- Operators can connect without navigating away from their current workflow.
- Dialog values update when branch or database context changes.
- Unpublished or unhealthy endpoints fail closed with recovery guidance.
