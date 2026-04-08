# AGENTS.md

This repository is for a Docker-first self-host control plane and web UI for Neon.

## Working Rules

1. Practice TDD for all non-trivial behavior.
   - Write a failing test first.
   - Implement the smallest change to pass.
   - Refactor while tests stay green.

2. Keep commits small and topical.
   - One concern per commit.
   - Prefer many small, reviewable commits over large mixed commits.

3. Write the backend web service in Go.
   - Use the Go standard library first.
   - Add dependencies only when they materially reduce complexity.

4. Prioritize operational safety over feature breadth.
   - Conservative defaults.
   - Explicit error handling.
   - Clear logging for operators.

5. Keep the architecture simple and Docker-first.
   - Favor single-node, single-tenant defaults.
   - Avoid optional complexity until the core snapshot/restore/switch flow is solid.

6. Keep documentation aligned with current maturity.
   - Do not describe unimplemented features as production-ready.
   - Prefer explicit labels like "planned", "experimental", or "pre-alpha" when appropriate.

7. Keep delivery traceable.
   - Commit after finishing each feature or meaningful docs slice.
   - Keep `changelog.md` updated in the same commit with user-facing changes.
