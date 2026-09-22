# 未使用的接口

统计：后端注册接口 347 条，界面与业务使用 133 条，本文件覆盖剩下的 214 条。按用途分四类：

1. 客户端转发接口 70 条：给 API 客户端（Codex、Cherry Studio、脚本等）调用，界面不调用，必须保留。
2. 协议与运维接口 3 条：保留。
3. 界面已移除、接口仍在 120 条：删除候选，按已经删掉的功能分组。
4. 前端有封装但界面未调用 21 条：前端封装可以删除，后端接口是否删除取决于是否保留该能力。

统计时间 2026-09-22。使用中的接口见 [`docs/api-usage.md`](./api-usage.md)。

## 一、客户端转发接口（保留）

这些接口面向 LLM 客户端与第三方工具，不是界面调用。

### OpenAI 兼容接口（38）

| 方法 | 路径 |
|---|---|
| POST | `/v1/alpha/search` |
| POST | `/v1/audio/speech` |
| POST | `/v1/audio/transcriptions` |
| POST | `/v1/audio/translations` |
| POST | `/v1/chat/completions` |
| POST | `/v1/completions` |
| POST | `/v1/edits` |
| POST | `/v1/embeddings` |
| POST | `/v1/engines/:model/embeddings` |
| GET | `/v1/files` |
| POST | `/v1/files` |
| DELETE | `/v1/files/:id` |
| GET | `/v1/files/:id` |
| GET | `/v1/files/:id/content` |
| GET | `/v1/fine-tunes` |
| POST | `/v1/fine-tunes` |
| GET | `/v1/fine-tunes/:id` |
| POST | `/v1/fine-tunes/:id/cancel` |
| GET | `/v1/fine-tunes/:id/events` |
| POST | `/v1/images/edits` |
| POST | `/v1/images/generations` |
| POST | `/v1/images/variations` |
| POST | `/v1/messages` |
| GET | `/v1/models` |
| POST | `/v1/models/*path` |
| DELETE | `/v1/models/:model` |
| GET | `/v1/models/:model` |
| POST | `/v1/moderations` |
| GET | `/v1/realtime` |
| POST | `/v1/rerank` |
| POST | `/v1/responses` |
| POST | `/v1/responses/compact` |
| POST | `/v1/video/generations` |
| GET | `/v1/video/generations/:task_id` |
| POST | `/v1/videos` |
| GET | `/v1/videos/:task_id` |
| GET | `/v1/videos/:task_id/content` |
| POST | `/v1/videos/:video_id/remix` |

### Gemini 兼容接口（3）

| 方法 | 路径 |
|---|---|
| GET | `/v1beta/models` |
| POST | `/v1beta/models/*path` |
| GET | `/v1beta/openai/models` |

### Midjourney（16）

| 方法 | 路径 |
|---|---|
| GET | `/mj/image/:id` |
| POST | `/mj/insight-face/swap` |
| POST | `/mj/submit/action` |
| POST | `/mj/submit/blend` |
| POST | `/mj/submit/change` |
| POST | `/mj/submit/describe` |
| POST | `/mj/submit/edits` |
| POST | `/mj/submit/imagine` |
| POST | `/mj/submit/modal` |
| POST | `/mj/submit/shorten` |
| POST | `/mj/submit/simple-change` |
| POST | `/mj/submit/upload-discord-images` |
| POST | `/mj/submit/video` |
| GET | `/mj/task/:id/fetch` |
| GET | `/mj/task/:id/image-seed` |
| POST | `/mj/task/list-by-condition` |

### Suno（3）

| 方法 | 路径 |
|---|---|
| POST | `/suno/fetch` |
| GET | `/suno/fetch/:id` |
| POST | `/suno/submit/:action` |

### Kling（4）

| 方法 | 路径 |
|---|---|
| POST | `/kling/v1/videos/image2video` |
| GET | `/kling/v1/videos/image2video/:task_id` |
| POST | `/kling/v1/videos/text2video` |
| GET | `/kling/v1/videos/text2video/:task_id` |

### 即梦（1）

| 方法 | 路径 |
|---|---|
| POST | `jimeng` |

### Playground（1）

| 方法 | 路径 |
|---|---|
| POST | `/pg/chat/completions` |

### 账单兼容接口（4）

| 方法 | 路径 |
|---|---|
| GET | `/dashboard/billing/subscription` |
| GET | `/dashboard/billing/usage` |
| GET | `/v1/dashboard/billing/subscription` |
| GET | `/v1/dashboard/billing/usage` |

## 二、协议与运维接口（保留）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/authz/catalog` | 权限目录，供授权管理读取 |
| GET | `/api/ratio_config` | 倍率配置导出，供其它实例做上游同步 |
| GET | `/api/status/test` | 管理员自检接口 |

## 三、界面已移除、接口仍在（删除候选）

这些接口对应的页面与设置已经删除，保留它们只是为了兼容旧客户端或以后恢复功能。确认不再需要时可以连同控制器一起删除。

### 用户管理（11）

| 方法 | 路径 |
|---|---|
| GET | `/api/user` |
| POST | `/api/user` |
| PUT | `/api/user` |
| GET | `/api/user/2fa/stats` |
| DELETE | `/api/user/:id` |
| DELETE | `/api/user/:id/2fa` |
| DELETE | `/api/user/:id/bindings/:binding_type` |
| DELETE | `/api/user/:id/reset_passkey` |
| GET | `/api/user/groups` |
| POST | `/api/user/manage` |
| GET | `/api/user/search` |

### 账号绑定（3）

| 方法 | 路径 |
|---|---|
| GET | `/api/oauth/telegram/bind/:flow_token` |
| GET | `/api/user/:id/oauth/bindings` |
| DELETE | `/api/user/:id/oauth/bindings/:provider_id` |

### 注册与邮件找回（5）

| 方法 | 路径 |
|---|---|
| POST | `/api/oauth/email/bind` |
| GET | `/api/reset_password` |
| POST | `/api/user/register` |
| POST | `/api/user/reset` |
| GET | `/api/verification` |

### 钱包与充值（16）

| 方法 | 路径 |
|---|---|
| POST | `/api/user/amount` |
| POST | `/api/user/creem/pay` |
| GET | `/api/user/epay/notify` |
| POST | `/api/user/epay/notify` |
| POST | `/api/user/pay` |
| POST | `/api/user/stripe/amount` |
| POST | `/api/user/stripe/pay` |
| GET | `/api/user/topup` |
| POST | `/api/user/topup` |
| POST | `/api/user/topup/complete` |
| GET | `/api/user/topup/info` |
| GET | `/api/user/topup/self` |
| POST | `/api/user/waffo-pancake/amount` |
| POST | `/api/user/waffo-pancake/pay` |
| POST | `/api/user/waffo/amount` |
| POST | `/api/user/waffo/pay` |

### 支付回调与支付对接（9）

| 方法 | 路径 |
|---|---|
| POST | `/api/creem/webhook` |
| GET | `/api/option/waffo-pancake/catalog` |
| POST | `/api/option/waffo-pancake/pair` |
| POST | `/api/option/waffo-pancake/save` |
| POST | `/api/option/waffo-pancake/subscription-product` |
| GET | `/api/option/waffo-pancake/subscription-product-options` |
| POST | `/api/stripe/webhook` |
| POST | `/api/waffo-pancake/webhook/:env` |
| POST | `/api/waffo/webhook` |

### 邀请（2）

| 方法 | 路径 |
|---|---|
| GET | `/api/user/aff` |
| POST | `/api/user/aff_transfer` |

### 兑换码（7）

| 方法 | 路径 |
|---|---|
| GET | `/api/redemption` |
| POST | `/api/redemption` |
| PUT | `/api/redemption` |
| DELETE | `/api/redemption/:id` |
| GET | `/api/redemption/:id` |
| DELETE | `/api/redemption/invalid` |
| GET | `/api/redemption/search` |

### 订阅计划（23）

| 方法 | 路径 |
|---|---|
| POST | `/api/subscription/admin/bind` |
| GET | `/api/subscription/admin/plans` |
| POST | `/api/subscription/admin/plans` |
| PATCH | `/api/subscription/admin/plans/:id` |
| PUT | `/api/subscription/admin/plans/:id` |
| POST | `/api/subscription/admin/plans/:id/subscriptions/reset` |
| DELETE | `/api/subscription/admin/user_subscriptions/:id` |
| POST | `/api/subscription/admin/user_subscriptions/:id/invalidate` |
| GET | `/api/subscription/admin/users/:id/subscriptions` |
| POST | `/api/subscription/admin/users/:id/subscriptions` |
| POST | `/api/subscription/admin/users/:id/subscriptions/reset` |
| POST | `/api/subscription/balance/pay` |
| POST | `/api/subscription/creem/pay` |
| GET | `/api/subscription/epay/notify` |
| POST | `/api/subscription/epay/notify` |
| POST | `/api/subscription/epay/pay` |
| GET | `/api/subscription/epay/return` |
| POST | `/api/subscription/epay/return` |
| GET | `/api/subscription/plans` |
| GET | `/api/subscription/self` |
| PUT | `/api/subscription/self/preference` |
| POST | `/api/subscription/stripe/pay` |
| POST | `/api/subscription/waffo-pancake/pay` |

### 自定义 OAuth 提供商（6）

| 方法 | 路径 |
|---|---|
| GET | `/api/custom-oauth-provider` |
| POST | `/api/custom-oauth-provider` |
| DELETE | `/api/custom-oauth-provider/:id` |
| GET | `/api/custom-oauth-provider/:id` |
| PUT | `/api/custom-oauth-provider/:id` |
| POST | `/api/custom-oauth-provider/discovery` |

### 模型部署（io.net）（19）

| 方法 | 路径 |
|---|---|
| GET | `/api/deployments` |
| POST | `/api/deployments` |
| DELETE | `/api/deployments/:id` |
| GET | `/api/deployments/:id` |
| PUT | `/api/deployments/:id` |
| GET | `/api/deployments/:id/containers` |
| GET | `/api/deployments/:id/containers/:container_id` |
| POST | `/api/deployments/:id/extend` |
| GET | `/api/deployments/:id/logs` |
| PUT | `/api/deployments/:id/name` |
| GET | `/api/deployments/available-replicas` |
| GET | `/api/deployments/check-name` |
| GET | `/api/deployments/hardware-types` |
| GET | `/api/deployments/locations` |
| POST | `/api/deployments/price-estimation` |
| GET | `/api/deployments/search` |
| GET | `/api/deployments/settings` |
| POST | `/api/deployments/settings/test-connection` |
| POST | `/api/deployments/test-connection` |

### 绘图与任务日志（7）

| 方法 | 路径 |
|---|---|
| GET | `/api/log/search` |
| GET | `/api/log/self/search` |
| GET | `/api/log/token` |
| GET | `/api/mj` |
| GET | `/api/mj/self` |
| GET | `/api/task` |
| GET | `/api/task/self` |

### 性能页面（4）

| 方法 | 路径 |
|---|---|
| DELETE | `/api/performance/disk_cache` |
| POST | `/api/performance/gc` |
| POST | `/api/performance/reset_stats` |
| GET | `/api/performance/stats` |

### 公开页面（6）

| 方法 | 路径 |
|---|---|
| GET | `/api/about` |
| GET | `/api/home_page_content` |
| GET | `/api/pricing` |
| GET | `/api/privacy-policy` |
| GET | `/api/rankings` |
| GET | `/api/user-agreement` |

### 其它已移除功能（2）

| 方法 | 路径 |
|---|---|
| POST | `/api/channel/ollama/pull` |
| GET | `/api/usage/token` |

## 四、前端有封装但界面未调用

前端代码里还留着这些接口封装，但没有界面引用。删掉封装即可；后端接口如果确定不用，也可以按第三节处理。路径为前端写法，`:param` 表示拼接进去的参数。

| 前端封装 | 接口 |
|---|---|
| `features/channels/api.ts` → `getEnabledModels` | `/api/channel/models_enabled` |
| `features/channels/api.ts` → `getOllamaVersion` | `/api/channel/ollama/version/:param` |
| `features/dashboard/api.ts` → `getUserQuotaDataByUsers` | `/api/data/users` |
| `features/dashboard/api.ts` → `getFlowQuotaDates` | `/api/data/flow`、`/api/data/flow/self` |
| `features/dashboard/api.ts` → `getUptimeStatus` | `/api/uptime/status` |
| `features/models/api.ts` → `searchVendors` | `/api/vendors/search` |
| `features/models/api.ts` → `getVendor` | `/api/vendors/:param` |
| `features/performance-metrics/api.ts` → `getPerfMetrics` | `/api/perf-metrics` |
| `features/profile/api.ts` → `deleteUserAccount` | `/api/user/self` |
| `features/profile/api.ts` → `bindWeChat` | `/api/oauth/wechat/bind` |
| `features/profile/api.ts` → `startTelegramBind` | `/api/oauth/telegram/bind/start` |
| `features/profile/api.ts` → `getSelfOAuthBindings` | `/api/user/oauth/bindings` |
| `features/profile/api.ts` → `unbindCustomOAuth` | `/api/user/oauth/bindings/:param` |
| `features/profile/api.ts` → `getCheckinStatus` | `/api/user/checkin` |
| `features/profile/api.ts` → `performCheckin` | `/api/user/checkin` |
| `features/system-settings/api.ts` → `confirmPaymentCompliance` | `/api/option/payment_compliance` |
| `features/system-settings/api.ts` → `resetModelRatios` | `/api/option/rest_model_ratio` |
| `features/system-settings/api.ts` → `getUpstreamChannels` | `/api/ratio_sync/channels` |
| `features/system-settings/api.ts` → `fetchUpstreamRatios` | `/api/ratio_sync/fetch` |
| `lib/api.ts` → `getSelf` | `/api/user/self` |
| `lib/api.ts` → `getNotice` | `/api/notice` |

