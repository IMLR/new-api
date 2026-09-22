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

上游偶尔把整条回答放在 `message` 字段而不是 `delta` 里，这类帧会被当成 delta 处理，不会丢内容；工具调用分片缺少 `index` 时按调用 id 归位，两个并行调用不会合并成一个。整条流结束时若既没有正文也没有工具调用，中转会写出一帧错误（`upstream stream ended without an answer`）并记录最后一帧，而不是让客户端收到一个「成功但空」的流。

### 上游连接

WorkBuddy 的聊天请求固定走 HTTP/1.1。上游网关支持 HTTP/2，但半死的 h2 流会在回答中途报 `http2: response body closed`，客户端表现为流被截断；同一部署在 HTTP/1.1 下稳定，参考实现同样显式禁用了 h2（置空 `TLSNextProto`，只设 `ForceAttemptHTTP2=false` 不够）。连接参数：建连 10 秒上限、TCP keepalive 15 秒、TLS 握手 10 秒、空闲连接 30 秒、响应头等待 120 秒（与大上下文下较慢的首个 token 留出余量）。凭证续期、模型目录、积分与账号任务使用同一个客户端；渠道配置的代理同样生效。

## 账号任务

账号任务在渠道页的额度对话框里管理，每个任务独立显示状态与开关：

| 任务 | 做什么 | 时间（北京时间） | 适用部署 |
|---|---|---|---|
| `checkin` | 每日签到领积分 | 09:00 / 21:00 | 国内版 |
| `activity` | 上报一条对话事件，点亮活跃地图与连登 | 10:00 | 两个部署 |
| `travel` | 领取到站奖励并在额度允许时再次派出 | 09:00 / 21:00 | 国内版 |
| `keepalive` | 强制轮换一次访问令牌 | 22:00 | 两个部署 |

状态分四种：**未开始**（当天还没跑）、**完成**、**跳过**（上游已经没有这个活动，或今天已经领过）、**失败**（调用出错，错误原文显示在任务下方）。某个活动一直失败时，在对话框里单独关掉它即可，其它任务不受影响；开关按渠道保存。

实现方式：

- `service/workbuddy_tasks.go` 提供注册表与调度器。调度器在主节点每 10 分钟检查一次，任务在配置的小时内、当天尚未成功、且距上次尝试超过 90 分钟时执行。
- 每个任务是一个独立文件（`service/workbuddy_task_*.go`），文件里的 `init()` 调用 `RegisterWorkBuddyTask` 完成注册。新增活动就是加一个文件，删除活动就是删掉这个文件，调度器、渠道页与开关列表自动跟随，不需要改其它代码。
- 任务状态与开关保存在渠道的 `other_info` 字段里（键 `workbuddy_tasks`），随渠道保存，不需要新建表；写入走渠道级互斥锁，避免并发覆盖。
- 上游把「活动已下线」「今天已领过」当作正常业务状态处理，这类回答记为「跳过」，不会被算成失败。

接口：

- `GET /api/channel/:id/workbuddy/quota` 的返回体新增 `tasks` 数组（含状态、消息、执行时间点与开关状态）。
- `POST /api/channel/:id/workbuddy/tasks/:task`，请求体 `{"enabled":true|false}`，需要渠道操作权限，返回更新后的任务列表。

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

## 工具调用的转发规则

上游把一次工具调用拆成多帧：首帧带调用 id 与函数名，后续帧只带参数分片，并且把函数名写成空字符串。中转按下面的规则改写后再发给客户端：

- 函数名只在首帧出现一次，后续分片里的空名字会被删掉，客户端不会收到空名字的调用。
- 首帧缺少函数名时先扣住分片，等名字到达后一起发出；名字始终不来的调用整条丢弃，因为它对客户端不可用。
- 上游偶尔会把已经用过的序号分配给第二个调用，这时按调用 id 判断并换一个空闲序号，否则第二个调用会被当成重复分片、函数名被剥掉。

只有真正转发出去的正文或带名字的调用才算答复。收尾帧（`finish_reason`、上游错误帧、`[DONE]`）先扣住不发，等答复确定可用后再发；只渲染空内容的帧（角色帧、空增量）也先扣住，等第一条有内容的帧到来时一起发出，因此被丢弃的那次尝试不会在客户端留下任何帧。

答复不可用时（例如上游只发了参数分片、首帧缺失），中转重发一次上游请求并把幸存那次的答复发给客户端，被丢弃那次的收尾帧一并丢掉；两次都不可用时写出一帧 `upstream stream ended without an answer`。每次尝试都会记录一行日志（`workbuddy relay stream: attempt=… answers=… forwarded=… calls=… held=… last=…`），便于对照客户端报的断流。非流式请求在聚合结果为空时同样重发一次。

错误帧预读（`peekStreamError`）用带缓冲的读取器读取第一帧，读取器可能已经把后续帧拉进内存；回放时这些字节一并带回，否则第一帧之后的帧会丢失。

## 账号任务与地区

签到与猫猫旅行只在国内版存在，国际版账号没有这两套体系；活跃上报与令牌续期在两边都能运行。任务定义里的 `Realms` 决定它服务哪个地区，渠道页据此把不执行的任务标成不适用并禁用开关，接口返回 `applicable` 字段。

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
