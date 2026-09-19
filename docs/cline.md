# Cline 渠道

渠道类型 `60`，上游地址默认为 `https://api.cline.bot`。支持 OpenAI Chat Completions、流式输出、工具调用和非流式 JSON 响应。

## 导入

在渠道管理中新建 **Cline** 渠道。凭证可以粘贴 JSON 或通过「导入 Cline 凭证」选择 JSON 文件：

- Desktop 的 `~/.cline/data/settings/providers.json`，仅提取 `providers.cline.settings.auth`，不会导入其他提供商的密钥。
- `{ "auth": { ... } }`。
- 直接凭证对象，支持 camelCase 和 snake_case。

```json
{
  "accessToken": "...",
  "refreshToken": "...",
  "expiresAt": 1800000000000
}
```

`refreshToken` 必填，`accessToken` 和 `expiresAt` 可省略；缺少时自动刷新。到期时间接受 Unix 秒、毫秒和 RFC3339 字符串，也可从 JWT 的 `exp` 推导。一个渠道保存一份账号凭证，不支持将 JSON 按行拆成多 Key。

导入后点击「获取模型」，选择模型并保存渠道。新建表单中的凭证如需刷新，接口将新凭证回写表单，应保存后再关闭。已有渠道获取模型时，刷新后的凭证直接写入数据库。

使用独立登录会话作为服务器凭证。Refresh Token 会轮换，Desktop 和服务器不应同时刷新同一份会话；迁移凭证后由 New API 维护。

## 模型列表

先请求 `/api/v1/users/me/plan` 检查账号，再请求带 Desktop 标识的 `/api/v1/ai/cline/recommended-models`：

- 无订阅：仅返回 `free`。
- 有效 ClinePass 订阅：合并 `free` 和 `clinePass`，去重。
- 已过期、已失效、其他类型套餐：不加入 ClinePass 模型。
- 套餐查询失败：明确报错；仅官方 `404 / no plan history found for user` 视为无订阅。

New API 对外计费沿用现有模型定价，需按现有流程配置；上游免费不会自动改变下游价格。

不会把按量付费的 `recommended` 目录当作套餐可用模型。已有渠道可使用 New API 原有上游模型更新功能同步目录。

## 自动维护

主节点每分钟扫描启用及自动禁用渠道，剩余有效期不足五分钟时续期；实际调用和获取模型前也检查有效期。刷新接口为 `/api/v1/auth/refresh`。401 响应在输出开始前触发一次刷新重试；并发请求会复用已经轮换的新凭据。

刷新成功后，在同一数据库事务中更新 Access Token、Refresh Token 和到期时间。MySQL/PostgreSQL 使用行锁，SQLite 部署应使用单实例。网络及上游错误保留原凭证；授权失效时导入新的凭证。后台不会打印令牌。

上游统一用 SSE 并请求 usage，非流式调用将文本、推理、工具参数和 token 用量聚合成标准 JSON。截断或异常流报错，不返回伪成功。后台凭证维护独立于 Desktop，不需要安装桌面程序。

## 验证（2026-09-19）

已使用真实无订阅账号通过本项目 Go 实现完成：刷新并保存轮换凭证、获取六个免费模型、Kimi K3 流式请求和非流式完整响应。有效套餐、过期套餐、无订阅、认证/网络错误、并发刷新和工具参数拼接由针对性自动化测试覆盖；未使用真实 ClinePass 订阅账号验证。

可选真实集成测试：

```bash
CLINE_LIVE_CREDENTIAL_FILE=/absolute/path/private-credential.json \
  go test ./relay/channel/cline -run TestClineLive -count=1 -v
```

此测试会实际轮换凭据并写回指定文件，使用无订阅账号，发起少量免费模型请求。普通测试不访问真实账号。
