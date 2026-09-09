# Codex Channel OAuth

## Purpose

NewAPI can authorize a Codex channel with a ChatGPT account without importing
credentials from a machine that is already signed in to Codex. The channel
gets its own OAuth credential through an independent browser sign-in.

The flow follows the current Codex browser authorization protocol:

- authorization issuer: `https://auth.openai.com`
- callback: `http://localhost:1455/auth/callback`
- authorization code grant with PKCE (`S256`)
- scopes: `openid profile email offline_access api.connectors.read
  api.connectors.invoke`
- originator: `codex_cli_rs`

## Administrator workflow

1. Open a new or existing Codex channel.
2. Select **Authorize with ChatGPT** or **Reauthorize with ChatGPT**.
3. Create and open the authorization link.
4. Sign in with the ChatGPT account assigned to the channel.
5. The browser may report that `localhost:1455` is unavailable. Copy the full
   URL from the address bar.
6. Paste that URL into NewAPI and complete authorization.
7. For a new channel, save the channel form. For an existing channel, the new
   credential is saved immediately.

## API

All endpoints require administrator authentication and the
`ChannelSensitiveWrite` permission.

| Operation | New channel | Existing channel |
| --- | --- | --- |
| Start | `POST /api/channel/codex/oauth/start` | `POST /api/channel/:id/codex/oauth/start` |
| Complete | `POST /api/channel/codex/oauth/complete` | `POST /api/channel/:id/codex/oauth/complete` |

The start request accepts the new channel's optional proxy setting. Existing
channels always use the proxy stored on the server.

The completion request contains the returned `flow_id` and the full localhost
callback URL. Existing-channel completion writes the credential directly to
that channel. New-channel completion returns the encoded credential for the
unsaved form.

## State and deployment

OAuth flow records expire after 10 minutes and are consumed atomically on the
first completion attempt. A record is bound to both the administrator ID and
the target channel ID.

When Redis is enabled, flow state is stored in Redis and can be completed by a
different NewAPI instance behind the same load balancer. Without Redis, state
is kept in process memory, so start and completion must reach the same server
process.

## Security boundaries

- NewAPI does not read the local Codex credential store.
- OAuth state, PKCE verifier, authorization codes, and tokens are not written
  to application logs.
- Completion accepts only the exact localhost callback scheme, host, port, and
  path used by the authorization request.
- The state value is compared in constant time.
- A completion attempt consumes the flow even when ownership, state, or token
  exchange validation fails.
- Existing-channel reauthorization is recorded in the administrator audit log.

### Chat Completions 流式转换排查

当 Codex 上游返回 SSE 正文但 Content-Type 被代理标为 application/json 时，Chat Completions 转换仍须使用 SSE 解析器。不能仅在客户端非流式时识别 Codex 上游流式，否则客户端 stream=true 会误入 JSON 解析，出现 `bad_response_body: invalid character 'e' looking for beginning of value`。回归测试覆盖了该响应头异常及原有非流式聚合行为。此服务端修正需部署网关后生效。
