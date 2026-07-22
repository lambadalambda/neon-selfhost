# Improve SQL editor workflow

## Summary

Add high-value SQL editing and result-handling ergonomics without weakening safety limits.

## Requirements

- Add new-query action and `Ctrl/Cmd+Enter` execution.
- Disable Run for empty input and expose cancellation for active queries.
- Add SQL-aware indentation, syntax highlighting, and error-position display.
- Add result copy and CSV/JSON export within existing limits.

## Acceptance Criteria

- Keyboard-first query execution is reliable and accessible.
- Cancellation releases server resources promptly.
- Exported data matches the bounded result shown in the console.
