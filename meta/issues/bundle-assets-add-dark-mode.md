# Bundle console assets and add dark mode

## Summary

Make the console fully usable on isolated networks and improve operator display preferences.

## Requirements

- Remove runtime dependencies on external font/CDN requests.
- Use bundled assets or robust system-font fallbacks.
- Add system-aware dark mode with an explicit persisted override.
- Preserve contrast, focus visibility, and mobile behavior in both themes.

## Acceptance Criteria

- The console renders correctly with no internet access.
- Light and dark themes meet accessible contrast expectations.
- Theme selection does not flash or reset unexpectedly.
