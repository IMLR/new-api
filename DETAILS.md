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

### 渠道指纹与端点配置（2026-09-20）

- `pkg/modelfingerprint/`：内嵌参考统计、数字特征提取及候选排名，附 ModelTrace MIT 许可与 Python 评分快照。
- `controller/channel_fingerprint.go`：受渠道操作权限和渠道开关约束的有界指纹采样。
- `service/openai_chat_responses_mode.go`：渠道模型端点优先于全局转换策略。
- `web/src/features/channels/components/channel-detection-settings.tsx`：按渠道的开关及模型端点配置。
- `web/src/features/channels/components/dialogs/channel-fingerprint-button.tsx`：指纹按钮、采样状态和逐次结果。
- `docs/channel-fingerprints.md`：功能操作、参考库来源及已知限制。
