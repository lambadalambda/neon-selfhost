# Keep wide SQL results readable

## Summary

Prevent wide SQL query results from compressing columns into vertically stacked characters.

## Requirements

- Size result tables from their content instead of forcing every column into the editor width.
- Keep headers and ordinary scalar values readable without arbitrary character-level wrapping.
- Preserve horizontal and vertical scrolling inside the bounded result area on desktop and mobile.
- Continue wrapping genuinely long values at a reasonable maximum column width.

## Acceptance Criteria

- A wide query such as `SELECT * FROM users LIMIT 1` renders readable column names and values.
- Wide result sets scroll horizontally instead of expanding or crushing the page layout.
- Long individual values remain bounded and readable.
- UI tests, Go tests, race tests, and Compose validation pass.
- The production fediffusion console is browser-verified after deployment.

## Outcome

- Result tables use content-sized columns with an 8rem minimum and 32rem maximum value width.
- Wide and tall results scroll inside a bounded, keyboard-focusable region without expanding the page.
- The named results region supports keyboard scrolling and preserves semantic column headers.
- An opt-in Chromium regression covers desktop and mobile containment, both-axis overflow, long values, and keyboard scrolling.
- Standard tests, race tests, vet, browser tests, and Compose validation pass.
- The production fediffusion console renders `SELECT * FROM users LIMIT 1` with readable bounded columns, internal horizontal scrolling, keyboard scrolling, and no desktop or mobile page overflow.
