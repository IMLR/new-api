# Repository Details

## Main modules

- `router/`: HTTP route registration and permission requirements.
- `controller/`: request parsing, authorization boundaries, API responses, and
  audit events.
- `service/`: reusable business operations and upstream integrations.
- `model/`: GORM models, persistence, and caches.
- `relay/`: provider adapters and protocol conversion.
- `middleware/`: authentication, permissions, rate limiting, and request
  context.
- `common/`: shared JSON, Redis, HTTP, logging, and utility functions.
- `dto/`, `constant/`, `types/`: API contracts and shared types.
- `web/`: React 19 administrator and user interface.
- `docs/`: focused design and operator documentation for complex features.

## Feature documents

- [`docs/cline.md`](./docs/cline.md): Cline credential import, independent renewal, subscription-aware model discovery and OpenAI chat relay.

- [`docs/codex-oauth.md`](./docs/codex-oauth.md): independent ChatGPT OAuth
  authorization for Codex channels, including endpoints, state storage,
  security boundaries, and operator workflow.
- [`docs/hk-deployment.md`](./docs/hk-deployment.md): native multi-architecture
  GHCR builds, restricted SSH deployment, runtime configuration, health
  verification, and rollback behavior for the Hong Kong VPS.
