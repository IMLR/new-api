# Development Status

This file records project-level work that is implemented in this repository.
Dates use UTC.

## Completed

- 2026-09-22: Added OpenCode Go channels (type 61) with credential import,
  model discovery from the public catalog, per-model routing across the chat,
  Messages and Responses endpoints, session id forwarding, and subscription
  usage queries in the channel list.
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

### 2026-09-20 渠道指纹检测与模型端点

- [x] 按渠道保存中转站检测开关和 OpenAI 模型端点偏好。
- [x] 复用渠道测试执行三次指纹采样，展示候选及逐次排名。
- [x] 添加评分一致性、端点优先级、配置校验和前端保存/交互回归用例。
- [x] 配置 GitHub CI/CD 验证与发布步骤（按用户要求不在 VPS 编译测试；每次提交的实际结果以 Actions 为准）。
- [ ] 后续研究跨家族联合评分及行为/拒绝特征融合，不将现有库内排名解释为身份概率。

- [x] 2026-09-20：指纹采样改为三个请求并发，支持服务端后台任务、重新打开读取结果和最高候选入口；本地不编译测试，由 CI 验证。

- [x] 2026-09-20：修复接口错误缺少 title 时出现空白提示，指纹详情显示实际错误原因；增加 CI 回归。
