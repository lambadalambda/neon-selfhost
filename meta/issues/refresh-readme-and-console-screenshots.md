# Refresh README and console screenshots

## Summary

Update the project landing page to show the current console and accurately describe the implemented single-node Neon control plane without mixing product orientation with contributor workflows.

## Requirements

- Replace stale console screenshots with current views containing representative data.
- Rewrite the README around the implemented product state and supported full-stack quickstart.
- Remove defensive "what it is not" framing and stale scaffold/MVP-slice language.
- Move development setup, testing, seeded fixtures, API inventory, and repository layout details into a dedicated document under `docs/`.
- Keep essential maturity and operational safety caveats visible without presenting planned features as implemented.
- Keep architecture and changelog references aligned with the documentation update.

## Acceptance Criteria

- The README shows current Dashboard, Tables, and SQL Editor screenshots with no exposed credentials or sensitive application data.
- The README lists all implemented console workflows and accurately labels the project maturity and supported topology.
- `docs/development.md` contains the extracted contributor setup, testing, fixture, API, and repository guidance.
- Documentation links resolve and Markdown formatting checks pass.
- A code/documentation review has no unresolved findings.

## Notes

- Use the deployed `fediffusion.art` controller only for read-only screenshot capture.
- Do not include connection strings, passwords, private user data, or other credentials in screenshots.
