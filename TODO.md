# Development Status

This file records project-level work that is implemented in this repository.
Dates use UTC.

## Completed

- 2026-09-19: Added Cline channels with JSON credential import, automatic token rotation, subscription-aware model discovery and streaming/nonstreaming chat relay.

- 2026-07-27: Added native AMD64/ARM64 GHCR builds and restricted automatic
  deployment of immutable commit images to the Hong Kong VPS.
- 2026-07-27: Synchronized the fork with upstream release `v1.0.0-rc.22`,
  adopted dynamic Codex model discovery, migrated the custom OAuth interface to
  the current frontend directory, and retained the local Codex relay behavior.
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
