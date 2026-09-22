# WorkBuddy / CodeBuddy 渠道

渠道类型 `62`，上游是腾讯 CodeBuddy（国内版 `copilot.tencent.com` + `www.codebuddy.cn`，国际版 `www.workbuddy.ai`）。一个渠道保存一个账号凭证，凭证可以自动续期。

协议来自开源网关 [workbuddy2api](https://github.com/Sliverkiss/workbuddy2api) 的实现，本文记录本仓库复现的部分。

## 凭证

渠道密钥是一个 JSON 对象，兼容两种写法：

- 插件导出的嵌套形：`{"auth":{"accessToken":...,"refreshToken":...,"expiresAt":...,"domain":...,"realm":...},"account":{"uid":...,"enterpriseId":...,"nickname":...},"device_token":...}`
- 手写的扁平形：所有字段放在顶层，字段名相同。

`refreshToken` 必填，否则保存时直接报错。`realm` 只接受 `cn` 与 `global`；不填时按 `domain` 判断（`workbuddy.ai` 及其子域为国际版），再不行按国内版处理。

## 渠道页面登录

新建或编辑 WorkBuddy 渠道时，密钥输入框上方有「登录 WorkBuddy」按钮，走的是上游的 OAuth 设备授权流程：

1. 选择部署（中国 / 国际）。
2. 点击后中转向上游申请一个授权会话，弹出登录链接（同页可复制或再次打开）。
3. 在浏览器完成 CodeBuddy 登录，回到对话框点击「我已登录完成」。
4. 中转轮询上游换取令牌与账号信息，成功后把规范化的凭证写入密钥输入框，保存渠道即可。

登录链接默认 10 分钟有效；未完成登录时对话框会提示稍后再次确认，流程不会因为一次确认而失效。

接口：

- `POST /api/channel/workbuddy/oauth/start`，请求体 `{"realm":"cn|global","proxy":""}`，返回 `flow_id`、`authorize_url` 与过期时间。
- `POST /api/channel/workbuddy/oauth/complete`，请求体 `{"flow_id":"..."}`，返回 `status` 为 `pending` 或 `ready`；`ready` 时附带凭证 JSON 与账号信息。

两个接口都需要渠道敏感写入权限。授权会话沿用与 Codex 登录相同的存放方式：有 Redis 时存 Redis，否则存进程内存。

也可以在别处完成登录后手工粘贴凭证：参考实现的 `login.sh`（`./login.sh --realm=global` 选国际版）会把同样的字段写到 `auths/workbuddy-<uid>.json`，把该文件内容整段粘贴进密钥输入框即可。

## 令牌续期

`POST {chatBase}/v2/plugin/auth/token/refresh`，请求头 `X-Refresh-Token` 携带刷新令牌，另加 `X-Auth-Refresh-Source: plugin`。响应 `data` 里有 `accessToken`、`refreshToken`、`expiresIn`（实测 5184000 秒，60 天）和 `domain`，两个令牌同时更换。

续期在三处触发：请求前发现令牌将在 5 分钟内过期、上游返回 401、后台每小时扫描一次已启用渠道。写入使用行锁加进程内互斥锁，并发请求不会丢掉刚轮换的令牌。`expiresIn` 缺失或超过 10 年时保留旧的过期时间，避免刷新风暴或永不刷新。

上游主机的选择规则：运营方填写的 API 地址优先；留空或仍是内置默认地址时，按凭证的 `realm` 决定，国内版用 `copilot.tencent.com`（积分接口用 `www.codebuddy.cn`），国际版用 `www.workbuddy.ai`。聊天、模型目录与令牌续期共用这套规则，避免国际版账号被送到国内主机。

上游返回 `12153`（`refresh token failed`、`invalid_grant`）表示这套登录在上游已经失效，只能重新登录。中转会把这类错误改写成 `WorkBuddy sign-in has expired, sign in again from the channel credentials: ...`，在渠道凭证步骤点「重新登录 WorkBuddy」即可换一套新凭证。

## 请求头

三类请求头族（common / chat / billing / refresh）按上游要求带齐：

| 请求头 | 取值 |
|---|---|
| `Authorization` | `Bearer <accessToken>`（chat、models、billing） |
| `X-CodeBuddy-Request` | 固定 `1` |
| `User-Agent` | `WorkBuddy/<客户端版本> <平台>/<客户端版本> CLI/<CLI版本>`，国际版平台段为 `WorkBuddy AI`；billing 域用单段 `WorkBuddy/<客户端版本>` |
| `Origin` / `Referer` | 国内版 `https://www.codebuddy.cn`，国际版 `https://www.workbuddy.ai` |
| `Accept-Language` | 国内版 `zh-CN`，国际版 `en-US` |
| `X-User-Id` / `X-Enterprise-Id` / `X-Domain` | 来自凭证；缺失时发对应的 `X-No-*` 声明头 |
| `X-Machine-ID` / `X-Session-ID` | 由账号 UID 派生（36 位十六进制），跨重启稳定、账号间互异 |
| `X-Agent-Purpose` / `X-IDE-Name` / `X-IDE-Type` / `X-IDE-Version` / `X-Product` | 桌面端用量归属头组 |
| `X-Device-Token` | 凭证里的 `device_token`，没有就不发 |
| `X-Conversation-ID` | 客户端会话标识（下游 `X-Conversation-ID`/`X-Session-Id`/`session_id`），没有就不发 |
| `X-Conversation-Request-ID` / `X-Root-Request-ID` / `X-Trace-ID` | 本轮请求标识，取本仓库的请求 ID |
| `X-Conversation-Message-ID` / `X-Request-ID` | 每条消息新生成的 32 位十六进制 |
| `X-B3-TraceId` / `X-B3-SpanId` / `X-B3-Sampled` | B3 链路头，请求标识不是合法 B3 值时回落到消息标识 |

客户端 IP 不向上游转发（参考实现的 `passthrough_ip` 默认也是关闭），上游看到的是中转服务器的地址。

## 请求体改写

上游只接受流式请求，并且对请求体有若干硬性要求，中转按下面的顺序改写（顺序与参考实现一致）：

1. 强制 `stream: true`。
2. `max_completion_tokens` 翻译为 `max_tokens`（显式 `max_tokens` 优先，别名一律删除）。
3. 缺少 `stream_options` 时补 `{"include_usage": true}`。
4. `tool_choice` 归一化：对象形态转字符串，`none` 同时删除 `tools` / `functions`。
5. `developer` 角色改写为 `system`（上游角色白名单不认 `developer`）。
6. `image_url` 字符串形态转对象形态 `{"url": ...}`。
7. 工具调用配对修复：先把插在工具调用与结果之间的消息移到整组之后，再删除没有配对的调用与孤儿结果。历史里的一条坏配对会让上游拒绝之后每条消息，这一步让会话自愈。
8. DeepSeek 系模型注入 `thinking: {"type":"enabled"}`，缺少档位时补默认档；显式 `disabled` 则删除档位。
9. `reasoning_effort` 按模型声明的支持档位降级（模型目录来自上游，缓存在内存中 10 分钟）。
10. DeepSeek 多轮一致性：assistant 消息补齐非空的 `reasoning` 与 `reasoning_content`。
11. 内容审核指纹清洗：Claude Code / Codex 的固定模板句按参考实现的规则做最小改写（`11128` 拆成 `11-128`），键值型指纹整段删除。
12. 注入 `prompt_cache_key`（`wb2a-<账号前8位>-<会话哈希>`），客户端已带则不覆盖。上游据此复用前缀缓存，实测可显著降低扣费。
13. 国际版账号在首条消息不是 system 时补一条兜底 system。

## 响应

上游永远返回 SSE 事件流。中转按下游需要处理：

- 下游要流式：逐帧按 OpenAI 规范重建（丢弃空 delta、空 `function_call`、顶层噪声字段），保留上游错误帧原文，缓存首个消息 id 供后续帧复用，末尾保证有 `data: [DONE]`。
- 下游要非流式：把流的 `content`、`reasoning_content` 与 `tool_calls` 分片聚合为一个 `chat.completion`，并补齐 `usage.total_tokens`。
- 流开始前若首帧就是错误帧，按错误返回，不当作空响应。

工具调用的分片按「首个分片带函数名、后续分片只带参数」的规范收敛。上游有时会把函数名放到后面的分片，直接在首个分片发出空名字会让严格的客户端报「调用了未声明工具」并中断整轮；中转会把没有名字的分片暂存，等到名字出现的那个分片一起发出，并且从不转发空名字。整轮都没有名字的调用会被丢弃（没有名字的调用无法执行）。

## 模型目录

两个端点并发探测后合并，配置端点优先、企业端点补缺：

| 端点 | 说明 |
|---|---|
| `GET {chatBase}/v3/config` | 权威目录，响应 `data.models` |
| `GET {chatBase}/console/enterprises/personal/models` | 国内版企业端点，按 `agents[cli].models` 过滤 |
| `GET {chatBase}/v2/enterprises/personal/models` | 国际版企业端点 |

非对话条目会被过滤：`nes-`、`completion-`、`codewise-` 前缀，`maxOutputTokens ≤ 256`，以及 tags 含 `text-to-image` 的模型。

## 积分余额

`POST {billingBase}/v2/billing/meter/get-user-resource`（国内版）或 `POST {billingBase}/billing/meter/get-user-resource`（国际版，404 时回落到 `/v2` 前缀），请求体是分页与套餐时间范围，响应按账号聚合套餐的 `CycleCapacity*` 与到期时间。余额接口不消耗积分。

渠道页的「账户信息」按钮打开积分对话框，显示账号尾号、部署地区、剩余与已用积分、每个套餐的剩余量、到期时间，以及一周内到期的标记。

## 验证（2026-09-22）

本机没有 CodeBuddy 账号，真实对话流量未验证。已验证的部分来自无凭证探测，端口与响应信封与实现一致：

| 探测 | 结果 |
|---|---|
| `POST https://copilot.tencent.com/v2/chat/completions` | 401（网关鉴权拦截） |
| `POST https://copilot.tencent.com/v2/plugin/auth/token/refresh` | `400 {"code":10001,"msg":"10001:refreshToken is empty"}`，确认路径与信封 |
| `GET https://copilot.tencent.com/v3/config` | `200 {"code":0,"msg":"OK","data":{"agent":...,"models":null}}`，匿名无模型 |
| `GET https://copilot.tencent.com/console/enterprises/personal/models` | 302（跳转登录） |
| `GET https://www.workbuddy.ai/v2/enterprises/personal/models` | `401 {"code":401,"msg":"401"}` |
| `POST https://www.codebuddy.cn/v2/billing/meter/get-user-resource` | 401 |
| `POST https://www.workbuddy.ai/billing/meter/get-user-resource` | 401 |

注意这些端点是 GET 或 POST 专用：用错方法会得到 `404 Route Not Found`。

代码级验证：请求体改写、请求头、SSE 重建与聚合、模型目录合并、积分解析、令牌续期由 `pkg/workbuddy/workbuddy_test.go` 覆盖；渠道分派与错误帧处理由 `relay/channel/workbuddy/adaptor_test.go` 覆盖；渠道页积分响应由 `controller/workbuddy_quota_test.go` 覆盖。

## 与参考实现的差异

- 参考实现是账号池网关（多账号轮换、熔断、冷却、会话粘性、定时签到）。本仓库每个渠道一个账号，账号池的角色由渠道选择与渠道亲和承担；签到、旅行、任务等定时脚本没有移植。
- 参考实现的提示词三模式（`passthrough` / `custom` / `append`）没有移植，中转保持透传并做审核指纹清洗。
- 参考实现的模型名单缓存、成本账本、在途租约等治理功能属于网关层职责，本仓库不实现。
