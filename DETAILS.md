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
- `web/default/`: React 19 administrator and user interface.
- `web/classic/`: legacy React administrator and user interface.
- `docs/`: focused design and operator documentation for complex features.

## Feature documents

- [`docs/codex-oauth.md`](./docs/codex-oauth.md): independent ChatGPT OAuth
  authorization for Codex channels, including endpoints, state storage,
  security boundaries, and operator workflow.
