# OpenCode Go 渠道

渠道类型 `61`，上游地址默认为 `https://opencode.ai/zen/go`。OpenCode Go 是 OpenCode 官方提供的编程模型订阅（每月 10 美元），一个 API 密钥对应一份订阅。

## 密钥

一个渠道保存一个 API 密钥，不支持多密钥。密钥有两种写法：

- 直接粘贴密钥，形如 `sk-...`。
- 粘贴 OpenCode 凭证文件 `auth.json` 里的 `opencode-go` 条目，例如 `{"opencode-go":{"type":"api","key":"sk-..."}}`，保存时自动取出其中的密钥。

`opencode`（Zen）条目属于按量付费的另一套密钥，导入时会明确报错，避免把 Zen 密钥当成 Go 订阅使用。

## 模型列表

先填写密钥，再点击「获取模型」，渠道从 `GET /v1/models` 读取目录。这个接口是公开的，密钥无效时依然返回完整目录，密钥轮换期间不影响获取模型。2026-09-22 实测返回 40 个模型。

OpenCode 客户端配置里的模型 ID 带 `opencode-go/` 前缀，例如 `opencode-go/kimi-k3`。中转时前缀会被去掉，下游用带前缀或不带前缀的写法都能命中同一个上游模型。

## 上游接口

OpenCode Go 按模型分成三个接口，同一个订阅的密钥通用：

| 接口 | 认证 | 模型 |
|------|------|------|
| `/v1/chat/completions` | `Authorization: Bearer` | GLM、Kimi、LongCat、DeepSeek、MiMo、Hy3、Hy4 preview 等其余模型 |
| `/v1/messages` | `x-api-key` | Qwen3.8 Max / Flash、Qwen3.7 Max / Plus、Qwen3.6 Plus、MiniMax M3 / M2.7 / M2.5 |
| `/v1/responses` | `Authorization: Bearer` | Grok 4.7 / 4.6 / 4.5、GPT 5.6 Luna、Muse Spark 1.3 / 1.2 Contributor |

上表与 OpenCode 官方文档（`packages/web/src/content/docs/go.mdx`）一致。把模型发到不属于它的接口时，上游返回 `Model <id> is not supported for format openai`（或 `format anthropic`），所以模型映射会同时决定请求发往哪个接口。

`/v1/messages` 只认 `x-api-key`，用 Bearer 会得到 `Missing API key.`。

下游可以用三种格式里任意一种调用：OpenAI Chat Completions、Claude Messages、OpenAI Responses。发往上游时按模型转换，上游用另一种格式回答时再转换回下游格式，例如下游用 Chat Completions 请求 Grok，上游用 Responses API 的流式事件回答，中转会把这些事件转成 Chat Completions 的流式响应。

渠道模型映射（把下游模型名映射到上游模型 ID）同样决定接口选择，映射到 Grok 就发 Responses，映射到 Qwen 就发 Messages。

## 会话标识

OpenCode 要求客户端为每个会话发送稳定的会话 ID（`x-opencode-session`），上游用它做路由优化和提示缓存。客户端带了这个请求头时，中转原样转发；代码类客户端带的 `session_id` 也一并转发。客户端不带时上游仍可服务，只是这类请求无法复用会话路由与缓存。

## 订阅限额

`GET /v1/usage` 用同一个密钥读取订阅用量，读取本身不占用订阅额度。返回体按窗口分组：

```json
{
  "usage": {
    "rolling": { "status": "ok", "percent": 12.5, "resetsAt": "2026-09-22T12:00:00.000Z" },
    "weekly": { "status": "ok", "percent": 40, "resetsAt": "2026-09-25T00:00:00.000Z" },
    "monthly": { "status": "ok", "percent": 80, "resetsAt": "2026-10-01T00:00:00.000Z" }
  }
}
```

- `percent` 已经是 0–100 的百分数，不需要再乘 100。
- `status` 为 `ok` 或 `rate-limited`。
- `resetsAt` 是服务端算好的绝对时间，包含毫秒。
- 三个窗口分别是滚动 5 小时、每周和每月：5 小时窗口用掉月度额度的 20%，每周窗口用掉 50%，每月窗口用掉 100%。每月窗口按订阅纪念日重置，不是 30 天滚动窗口。

解析同时兼容旧版字段（`resetInSec` 倒计时、只有 `used` 和 `limit` 的写法），上游格式变化时渠道页仍能显示。

常见错误：`401 AuthError`（缺少或无效密钥）、`403 EntitlementError`（密钥有效但没有 Go 订阅）、`403 RegionError`（模型不在当前地区开放，例如 Muse Spark）。

## 渠道页面查看限额

渠道列表里 OpenCode Go 渠道的「账户信息」按钮打开额度对话框，显示：

- 密钥尾号（用于区分同一个订阅下的多个渠道）。
- 三个订阅窗口的已用百分比、剩余百分比、重置时间和重置倒计时。
- 渠道上已配置的模型，以及每个模型的上游接口路径。

接口：

- `GET /api/channel/:id/opencode-go/quota` 读取密钥、订阅窗口和模型接口，不消耗上游额度。

与 Cline 渠道的额度检测不同，OpenCode Go 提供查询用量的接口，所以不需要发一次真实请求来探测额度。

## 验证（2026-09-22）

本次改动没有可用的 Go 订阅密钥，因此真实订阅流量（转发请求、读取用量）未在本仓库验证。已验证的部分：

- `GET /v1/models` 无需密钥，实测返回 40 个模型；带无效密钥同样返回 200。
- 无密钥或无效密钥时 `/v1/usage`、`/v1/chat/completions`、`/v1/messages` 返回 `401 {"type":"error","error":{"type":"AuthError","message":"Missing API key."}}`，与本仓库的错误解析一致。
- 把模型发到不匹配的接口会得到 `Model <id> is not supported for format openai/anthropic`，与上表的接口划分一致。
- 用量返回体的字段（`status`、`percent`、`resetsAt`）来自上游源码 `packages/console/app/src/routes/zen/go/v1/usage.ts`。

请求转换、接口分派、鉴权请求头和会话标识转发由 `relay/channel/opencode/adaptor_test.go` 覆盖；用量解析由 `pkg/opencode/usage_test.go` 覆盖；额度接口由 `controller/opencode_quota_test.go` 和 `service/opencode_test.go` 覆盖。
