# 渠道模型冷却

上游账号某个模型的额度或频率用尽时，报错里通常会写明恢复时间。New API 读取这个时间，把「渠道 × 模型」记为冷却，冷却期内渠道选取跳过它，时间到期后自动恢复参与选取，不需要人工启用。同一账号的其他模型不受影响。

## 识别范围

识别只看报错内容，不限状态码：429、400、503，或 WorkBuddy 在 HTTP 200 的 SSE 首帧里返回的错误都一样处理。解析逻辑在 `pkg/resethint`，所有渠道类型共用。

| 形式 | 例子 |
|---|---|
| 相对时间（英文） | `Try again in 20h 4m`、`Retry after 55s`、`Please retry in 1 day and 2 hours`、`try again in 1m30.5s`、`try again in 20ms` |
| 相对时间（中文） | `请 30 秒后重试`、`1小时20分钟后恢复` |
| 绝对时间 | `your usage will reset at 2026-10-17 18:09:40 UTC+8`、`resets at 2026-10-17T10:09:40Z`、`available after 2026/10/17 18:09 +0800`、`将于 2026-10-17 18:09:40 重置` |
| 响应头 | `Retry-After: 55`、`Retry-After: Fri, 17 Oct 2026 10:09:40 GMT` |

规则：

- 绝对时间前后必须有 reset、retry、available、until、重置、恢复、重试等词，报错里普通的请求时间戳不会被当成恢复时间。
- 带 `UTC+8`、`+08:00`、`Z`、`UTC` 的时间按写明的时区换算；没有时区的时间按服务器时区解释，香港部署默认 `Asia/Shanghai`。
- 正文和响应头都给出时间时取较晚的一个；已经过去的恢复时间忽略；所有窗口最多 7 天。
- 并发请求返回较短的速率限制时，不会缩短已经记录的较长冷却。

## 重试与渠道测试

报错带有恢复时间时，本次请求会继续尝试其他未冷却的渠道，即使这个状态码不在自动重试范围内（例如 400）。指定单个渠道、客户端取消、已经开始输出，以及设置里的「始终不重试错误码」仍然会结束重试。

渠道页面的单渠道测试遇到同样的报错也会记录冷却，因此测试发现的限额会直接影响后续的渠道选取。

冷却记录保存在当前进程内，服务重启后重新积累。

## 渠道专属规则

适配器只在通用规则之外补充上游特有的规则，并通过上游错误响应的 `Retry-After` 头交给通用逻辑记录：

- Cline：每日免费额度（`INFERENCE_CAP_ERROR` 或 `Daily free limit`）没有时间提示时冷却一小时，最短一分钟。见 [Cline 渠道](./cline.md)。
- WorkBuddy：SSE 首帧里的 `6004`（模型频率限制）还原成 HTTP 429。见 [WorkBuddy 渠道](./workbuddy.md)。

## 验证

`pkg/resethint/resethint_test.go` 覆盖上表各种写法以及不应识别的情况；`controller/workbuddy_retry_test.go` 中的 `TestRelayResetHintsCoolChannelModel` 用本地 HTTP 测试服务验证：HTTP 429 的 `6004`、SSE 首帧的 `6004` 和 400 中文相对时间三个账号依次失败后切换到备用渠道，第二次请求直接选择备用渠道；`controller/relay_retry_test.go` 覆盖 Cline 的每日额度与 `Retry-After`。到期自动恢复由 `model/channel_cooldown_test.go` 覆盖。
