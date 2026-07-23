# Document repository delivery cadence

## Summary

Record the required issue-to-commit workflow for repository changes and the explicit exception for direct fediffusion.art deployment operations.

## Requirements

- Require an existing or newly written repository issue before work begins.
- Require TDD for implementation work, followed by review, issue archival, and a topical commit.
- Exempt direct deployment and verification operations on fediffusion.art from the repository cadence.
- Keep repository changes made for production subject to the normal cadence.
- Record the fediffusion.art operational exception in Obsidian.

## Acceptance Criteria

- `AGENTS.md` states the cadence and production-operations exception unambiguously.
- The existing fediffusion.art Obsidian note records the exception without secrets.
- The policy change is reviewed with no unresolved findings.

## Notes

- This is a documentation-only change, so behavior-level TDD is not applicable.
