# 模型匹配与渠道选择

模型管理使用文字条件匹配渠道中已经配置的模型名。客户端使用模型管理中的模型名称发起请求时，已保存的匹配规则会把这些渠道模型一起加入路由候选，并自动使用所选渠道的模型名称请求上游。规则也用于目录元信息和候选渠道预览。

## 操作

1. 打开模型管理，新增或编辑模型。
2. 在“必须包含全部”中输入文字，每项按 Enter 添加。例如 `claude`、`sonnet` 会匹配同时包含两项的模型，文字顺序不限。
3. 在“排除文字”中添加排除项。例如 `preview`、`thinking` 会排除包含任意一项的模型。
4. 按需打开“区分大小写”；默认不区分大小写，包含项和排除项使用同一设置。
5. 预览按模型名列出渠道。取消勾选只停用该渠道的这个模型，影响渠道的全部分组；重新勾选可恢复。
6. 保存后应用条件和渠道选择。关闭表单会放弃尚未保存的选择。

例如包含 `claude`、`sonnet`，排除 `preview`，不区分大小写：

| 渠道模型名 | 结果 |
| --- | --- |
| `claude-sonnet-4` | 匹配 |
| `CLAUDE-SONNET-4` | 匹配 |
| `sonnet-claude-custom` | 匹配 |
| `claude-opus-4` | 缺少 `sonnet` |
| `claude-sonnet-preview` | 命中排除项 |

预览展示已有模型及其候选渠道，停用的整个渠道会显示对应状态。最终请求还受请求分组、接口端点、渠道优先级及现有路由策略约束。

例如模型名称为 `deepseek-v4-flash`，包含 `deepseek`、`flash`：Cline 的 `deepseek-v4-flash` 和 WorkBuddy 的 `deepseek-v4.1-flash` 都会参与优先级选择。WorkBuddy 优先级更高时，会向它发送 `deepseek-v4.1-flash`。客户端模型名称仍用于普通请求的价格与使用日志。

渠道设置中的显式模型映射仍然有效。配置了客户端模型名的映射时优先使用该映射；否则先自动选出匹配的渠道模型，再应用该模型的显式映射。多个映射可以组成转换顺序，循环映射会被拒绝。

价格配置仍以模型名称为键，不会批量修改文字条件匹配到的全部模型价格。表单会注明当前价格对应的模型名称。

## 保存与路由

- `models.match_rule` 使用 TEXT 保存包含项、排除项和大小写设置，支持 SQLite、MySQL 和 PostgreSQL。
- 后端对每项文字进行正则转义，自动生成独立的正则表达式；逐项检查包含条件，再检查排除条件。`.`、`*`、`[` 等符号作为普通文字处理，不接受用户正则。
- 包含项至少一项；包含项和排除项各最多 32 项，每项最多 128 个 UTF-8 字节。保存时去除首尾空白和重复项。
- `channel_model_exclusions` 独立保存被停用的渠道/模型组合，同时更新全部分组的 `abilities.enabled`。模型配置和渠道选择在同一数据库事务中提交。
- 内存缓存和数据库路由都遵守这些停用项。更新渠道、重新启用整个渠道、修改标签状态或重建能力表后，停用项仍然保留。
- 只有请求模型名称对应的已启用元数据规则参与路由，其他模型的重叠规则不会介入。尚未设置文字规则的模型保留原有路由行为。
- 匹配到的模型按分组、接口、渠道/模型冷却和本次请求已尝试的渠道过滤，再选择最高可用优先级。一个渠道匹配到多个模型时只计算一次渠道权重；同渠道优先使用完整名称相同的模型，否则按模型名称顺序选择第一个可用项。
- 重试重新选择可用渠道和对应模型名；不会把同一个账号的其他模型变体当作新账号反复尝试。自动名称替换不会修改持久渠道映射或共享缓存对象。
- 请求体原样转发模式同样应用模型名称替换，只替换已有 JSON `model` 字段，其余字段及用于重试的客户端请求体保留原值。模型名称位于 URL 中的协议不会新增请求体字段。
- 新增、修改、停用和删除模型规则后刷新路由缓存，单独修改规则无需再次修改渠道勾选项。
- 删除模型元数据不会恢复渠道模型；重新勾选并保存才会恢复。删除渠道会清理该渠道的能力记录和停用项。
- 尚未编辑的历史模型记录保留原来的匹配范围。编辑并保存后使用新的文字条件，界面不再提供前缀、后缀或正则输入。

## 接口与模块

管理员预览接口：`POST /api/models/match_preview`。

```json
{
  "match_rule": {
    "include": ["claude", "sonnet"],
    "exclude": ["preview"],
    "case_sensitive": false
  }
}
```

响应按模型名返回 `channels`，包含渠道 ID、名称、类型、分组、整渠道启用状态、模型选择状态和优先级。预览是只读请求，不生成管理员写操作审计。

原有模型新增、更新接口接受同样的 `match_rule`，以及仅包含本次选择变更的 `channel_selections`：

```json
{
  "channel_selections": [
    { "channel_id": 1, "model": "claude-sonnet-4", "selected": false }
  ]
}
```

- `model/model_matching.go`：文字条件、验证和共享匹配器。
- `model/model_routing.go`：匹配规则的路由候选、原生模型选择与数据库查询。
- `model/channel_model_selection.go`：渠道预览、持久选择及事务保存。
- `controller/model_matching.go`：管理员预览接口。
- `web/src/features/models/components/model-matching-section.tsx`：文字输入、大小写开关、渠道预览和选择。
- `web/src/features/models/lib/model-matching.ts`：表单验证和选择草稿。

## 验证

后端运行 `go test ./model ./controller ./middleware ./relay/helper`。测试覆盖文字匹配、目录元数据、事务回滚、缓存/数据库路由、原生模型名称替换、流式与非流式重试，以及重建后的停用效果。

前端运行 `bun run typecheck`、相关文件 lint、`bun run build`，以及 `bun test src/features/models/lib/__tests__/selection.test.ts`。

浏览器测试使用独立初始化的测试服务和管理员账号，创建真实的临时渠道和模型，不调用上游。运行前使用 `bunx playwright install chromium` 准备浏览器：

```sh
MODEL_MATCHING_E2E_URL=http://127.0.0.1:3379 \
MODEL_MATCHING_E2E_USER=matchtest \
MODEL_MATCHING_E2E_PASSWORD='<test-password>' \
bun test --timeout 60000 src/features/models/components/__tests__/matching.test.mjs
```

测试实际输入多项条件、切换大小写、排除模型，通过点击和键盘取消选择，保存和刷新后检查持久状态，并在手机尺寸下检查长文字溢出与恢复选择。
