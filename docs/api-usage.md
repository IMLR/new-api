# 界面与业务使用的接口

本文列出当前前端实际调用的后端接口，按界面分组。路径里的 `:参数` 表示路径参数；同一路径出现多个方法表示分别用于读取与写入。

统计：后端注册接口 347 条，界面与业务使用 133 条；未使用的接口见 [`docs/api-unused.md`](./api-unused.md)。

数据来源：扫描 `web/src` 下的接口封装与直接调用，统计时间 2026-09-22；已经没有人引用的封装不计入本表。

转发接口（`/v1/*`、`/v1beta/*`、`/mj/*`、`/pg/*`、`/suno/*`、`/kling/*`、即梦、账单兼容）面向 API 客户端，不由界面调用，因此列在 [`docs/api-unused.md`](./api-unused.md) 的第一节。

## 一、登录与初始化

| 方法 | 路径 | 用途 |
|---|---|---|
| GET | `/api/oauth/:provider` | OAuth 登录回调（GitHub、Discord、OIDC 等） |
| POST | `/api/oauth/state` | 生成 OAuth 登录状态码 |
| GET | `/api/oauth/telegram/login` | Telegram 登录回调 |
| GET | `/api/oauth/wechat` | 微信登录回调 |
| GET | `/api/setup` | 读取初始化状态 |
| POST | `/api/setup` | 提交初始化，创建超级管理员 |
| GET | `/api/status` | 站点状态、版本、显示货币与功能开关 |
| POST | `/api/user/auth/logout` | 退出登录 |
| POST | `/api/user/auth/refresh` | 刷新访问令牌 |
| POST | `/api/user/login` | 用户名密码登录 |
| POST | `/api/user/login/2fa` | 提交两步验证码完成登录 |
| POST | `/api/user/passkey/login/begin` | 通行密钥登录，开始 |
| POST | `/api/user/passkey/login/finish` | 通行密钥登录，完成 |
| POST | `/api/user/passkey/verify/begin` | 通行密钥二次验证，开始 |
| POST | `/api/user/passkey/verify/finish` | 通行密钥二次验证，完成 |
| POST | `/api/verify` | 敏感操作前的安全验证 |

## 二、个人资料

| 方法 | 路径 | 用途 |
|---|---|---|
| POST | `/api/user/2fa/backup_codes` | 重新生成备用码 |
| POST | `/api/user/2fa/disable` | 关闭两步验证 |
| POST | `/api/user/2fa/enable` | 启用两步验证 |
| POST | `/api/user/2fa/setup` | 生成两步验证密钥与二维码 |
| GET | `/api/user/2fa/status` | 读取两步验证状态 |
| GET | `/api/user/models` | 读取可用模型（密钥的模型限制） |
| DELETE | `/api/user/passkey` | 删除通行密钥 |
| GET | `/api/user/passkey` | 读取已注册的通行密钥 |
| POST | `/api/user/passkey/register/begin` | 注册通行密钥，开始 |
| POST | `/api/user/passkey/register/finish` | 注册通行密钥，完成 |
| GET | `/api/user/self` | 读取当前用户资料 |
| PUT | `/api/user/self` | 更新当前用户资料（用户名、语言） |
| GET | `/api/user/self/groups` | 读取可用分组（密钥表单、资料页） |
| GET | `/api/user/sessions` | 读取登录会话列表 |
| DELETE | `/api/user/sessions/:sid` | 撤销指定登录会话 |
| POST | `/api/user/sessions/revoke-others` | 撤销其他登录会话 |
| PUT | `/api/user/setting` | 更新通知方式与额度预警 |
| GET | `/api/user/token` | 生成访问令牌 |

## 三、数据看板

| 方法 | 路径 | 用途 |
|---|---|---|
| GET | `/api/data` | 数据看板：全部用户的调用量与消耗 |
| GET | `/api/data/self` | 数据看板：当前用户的调用量与消耗 |
| GET | `/api/perf-metrics/summary` | 数据看板：模型性能汇总 |

## 四、渠道

| 方法 | 路径 | 用途 |
|---|---|---|
| GET | `/api/channel` | 渠道列表 |
| POST | `/api/channel` | 新建渠道 |
| PUT | `/api/channel` | 更新渠道 |
| DELETE | `/api/channel/:id` | 删除渠道 |
| GET | `/api/channel/:id` | 读取单个渠道详情 |
| GET | `/api/channel/:id/cline/quota` | 读取 Cline 渠道免费额度 |
| POST | `/api/channel/:id/cline/quota/probe` | 探测 Cline 渠道额度 |
| POST | `/api/channel/:id/codex/oauth/complete` | Codex 渠道设备码授权，完成 |
| POST | `/api/channel/:id/codex/oauth/start` | Codex 渠道设备码授权，开始 |
| POST | `/api/channel/:id/codex/refresh` | 刷新 Codex 渠道凭证 |
| GET | `/api/channel/:id/codex/usage` | 读取 Codex 用量与重置额度 |
| POST | `/api/channel/:id/codex/usage/reset` | 重置 Codex 用量 |
| GET | `/api/channel/:id/codex/usage/reset-credits` | 读取 Codex 重置次数 |
| POST | `/api/channel/:id/key` | 查看渠道密钥 |
| GET | `/api/channel/:id/opencode-go/quota` | 读取 OpenCode Go 订阅额度 |
| POST | `/api/channel/:id/status` | 启用或停用单个渠道 |
| GET | `/api/channel/:id/workbuddy/quota` | 读取 WorkBuddy 积分与账号任务 |
| POST | `/api/channel/:id/workbuddy/tasks/:task` | 启用或停用 WorkBuddy 账号任务 |
| POST | `/api/channel/batch` | 批量删除渠道 |
| POST | `/api/channel/batch/tag` | 批量设置渠道标签 |
| POST | `/api/channel/codex/oauth/complete` | Codex 授权（新建渠道），完成 |
| POST | `/api/channel/codex/oauth/start` | Codex 授权（新建渠道），开始 |
| POST | `/api/channel/copy/:id` | 复制渠道 |
| DELETE | `/api/channel/disabled` | 删除全部已停用渠道 |
| POST | `/api/channel/fetch_models` | 从上游拉取模型（新建渠道） |
| GET | `/api/channel/fetch_models/:id` | 从上游拉取模型（已有渠道） |
| GET | `/api/channel/fingerprint/:id` | 读取渠道指纹采样结果 |
| POST | `/api/channel/fingerprint/:id` | 对渠道做指纹采样 |
| POST | `/api/channel/fix` | 修复渠道与模型的对应关系 |
| GET | `/api/channel/models` | 读取全部渠道的模型 |
| POST | `/api/channel/multi_key/manage` | 多密钥的查询、启停与删除 |
| DELETE | `/api/channel/ollama/delete` | 删除 Ollama 模型 |
| POST | `/api/channel/ollama/pull/stream` | Ollama 模型拉取（流式进度） |
| GET | `/api/channel/ops` | 读取渠道操作配置 |
| GET | `/api/channel/search` | 按条件搜索渠道 |
| POST | `/api/channel/status/batch` | 批量启用或停用渠道 |
| PUT | `/api/channel/tag` | 编辑渠道标签 |
| POST | `/api/channel/tag/disabled` | 按标签批量停用渠道 |
| POST | `/api/channel/tag/enabled` | 按标签批量启用渠道 |
| GET | `/api/channel/tag/models` | 读取某个标签下的模型 |
| GET | `/api/channel/test` | 测试全部渠道 |
| GET | `/api/channel/test/:id` | 测试单个渠道 |
| POST | `/api/channel/test/:id` | 用自定义提示词测试单个渠道，返回模型回复 |
| GET | `/api/channel/update_balance` | 更新全部渠道余额 |
| GET | `/api/channel/update_balance/:id` | 更新单个渠道余额 |
| POST | `/api/channel/upstream_updates/apply` | 应用单个渠道的上游模型变化 |
| POST | `/api/channel/upstream_updates/apply_all` | 应用全部渠道的上游模型变化 |
| POST | `/api/channel/upstream_updates/detect` | 检测单个渠道的上游模型变化 |
| POST | `/api/channel/upstream_updates/detect_all` | 检测全部渠道的上游模型变化 |
| POST | `/api/channel/workbuddy/oauth/complete` | WorkBuddy 账号登录，完成确认 |
| POST | `/api/channel/workbuddy/oauth/start` | WorkBuddy 账号登录，获取登录链接 |
| GET | `/api/group` | 分组列表（渠道与密钥表单） |

## 五、模型

| 方法 | 路径 | 用途 |
|---|---|---|
| GET | `/api/models` | 模型列表 |
| POST | `/api/models` | 新建模型 |
| PUT | `/api/models` | 更新模型 |
| DELETE | `/api/models/:id` | 删除模型 |
| GET | `/api/models/:id` | 读取单个模型 |
| GET | `/api/models/missing` | 列出缺失元数据的模型 |
| GET | `/api/models/search` | 按条件搜索模型 |
| POST | `/api/models/sync_upstream` | 从上游同步模型元数据 |
| GET | `/api/models/sync_upstream/preview` | 预览上游同步差异 |
| GET | `/api/prefill_group` | 预填分组列表 |
| POST | `/api/prefill_group` | 新建预填分组 |
| PUT | `/api/prefill_group` | 更新预填分组 |
| DELETE | `/api/prefill_group/:id` | 删除预填分组 |
| GET | `/api/vendors` | 供应商列表 |
| POST | `/api/vendors` | 新建供应商 |
| PUT | `/api/vendors` | 更新供应商 |
| DELETE | `/api/vendors/:id` | 删除供应商 |

## 六、API 密钥

| 方法 | 路径 | 用途 |
|---|---|---|
| GET | `/api/token` | 密钥列表 |
| POST | `/api/token` | 新建密钥 |
| PUT | `/api/token` | 更新密钥 |
| DELETE | `/api/token/:id` | 删除密钥 |
| GET | `/api/token/:id` | 读取单个密钥 |
| POST | `/api/token/:id/key` | 查看密钥明文 |
| POST | `/api/token/batch` | 批量删除密钥 |
| POST | `/api/token/batch/keys` | 批量读取密钥明文 |
| GET | `/api/token/search` | 按条件搜索密钥 |

## 七、使用日志

| 方法 | 路径 | 用途 |
|---|---|---|
| GET | `/api/log` | 使用日志：全部用户（管理员范围） |
| GET | `/api/log/self` | 使用日志：仅自己 |
| GET | `/api/log/self/stat` | 使用日志：仅自己统计 |
| GET | `/api/log/stat` | 使用日志：全部用户统计 |
| GET | `/api/user/:id` | 使用日志里查看调用用户信息 |

## 八、高级配置

| 方法 | 路径 | 用途 |
|---|---|---|
| GET | `/api/log/channel_affinity_usage_cache` | 渠道亲和性：读取规则命中缓存 |
| GET | `/api/option` | 读取系统设置（设置页与模型价格） |
| PUT | `/api/option` | 保存系统设置 |
| DELETE | `/api/option/channel_affinity_cache` | 渠道亲和性：清空缓存 |
| GET | `/api/option/channel_affinity_cache` | 渠道亲和性：读取缓存统计 |

## 九、日志与监控

| 方法 | 路径 | 用途 |
|---|---|---|
| DELETE | `/api/performance/logs` | 日志与监控：清理日志文件 |
| GET | `/api/performance/logs` | 日志与监控：读取服务器日志文件信息 |
| GET | `/api/system-task/:task_id` | 按任务号读取任务进度 |
| GET | `/api/system-task/current` | 当前清理任务进度 |
| GET | `/api/system-task/list` | 系统任务列表 |
| POST | `/api/system-task/log-cleanup` | 日志与监控：启动历史日志清理任务 |

## 十、系统信息（高级配置页面底部）

| 方法 | 路径 | 用途 |
|---|---|---|
| GET | `/api/system-info/instances` | 系统信息：实例列表 |
| DELETE | `/api/system-info/instances/:node_name` | 系统信息：删除单个失效实例 |
| DELETE | `/api/system-info/stale-instances` | 系统信息：清理全部失效实例 |
