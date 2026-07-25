# Development Status

This file records project-level work that is implemented in this repository.
Dates use UTC.

## Completed

- 2026-07-25: Added independent ChatGPT OAuth authorization for Codex channels,
  including PKCE, one-time server-side flows, manual localhost callback input,
  channel reauthorization, multilingual UI, and regression tests.
- 2026-07-25: Synchronized the fork with upstream stable release
  `v1.0.0-rc.21` while preserving the local Codex relay and OAuth changes.

## Planned

- Keep the Codex OAuth request parameters synchronized with the current Codex
  client behavior when the upstream authorization protocol changes.
- Add an integration test backed by Redis to cover cross-instance OAuth flow
  consumption.
