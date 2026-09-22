# 个人自用版前端组件清单

重建前的组件文档。范围限定为单用户自建、自己使用、不对外营业的场景：后端接口保持不变，重建后的前端要覆盖当前全部可用功能，布局会重新设计。

因此组件按职责分层，不写死当前页面位置，不写死数据获取方式。表格中“接口”一列是功能覆盖的校验依据，重建时按这一列核对即可。

## 一、分层约定

| 层 | 职责 | 允许依赖 |
|---|---|---|
| 原子层 | 无业务含义的控件与展示单元 | 设计变量、工具函数 |
| 组合层 | 表格、表单、筛选、对话框、卡片等结构性组件 | 原子层 |
| 业务区块层 | 单个业务对象的一段界面 | 组合层、数据 hooks |
| 页面层 | 组合区块、处理路由参数、页面标题与动作区 | 业务区块层 |
| 应用层 | 提供者、鉴权、国际化、主题、请求客户端 | 全部层 |

三条硬性约束：

1. 原子层不出现业务名词。“渠道”“令牌”这类词只能出现在业务区块层的属性里。
2. 业务区块不直接调用网络请求，数据由 hooks 传入。这样同一个区块可以放进抽屉、对话框、整页三种容器。
3. 页面层不写业务规则，只负责组合区块与读取路由参数。

## 二、功能范围

重建后需要覆盖的界面，共 12 项：

| 界面 | 作用 | 当前路由 |
|---|---|---|
| 登录与身份验证 | 登录、两步验证、通行密钥、找回与重置密码、注册（可关闭）、OAuth 回调、初始化向导 | `/sign-in`、`/otp`、`/forgot-password`、`/reset`、`/setup`、`/oauth` |
| 数据看板 | 模型调用量与消耗统计、性能概览 | `/dashboard/models` |
| 渠道 | 上游渠道增删改查、测活、额度查询、指纹采样 | `/channels` |
| 模型 | 模型元数据、供应商、价格与倍率、上游同步 | `/models/metadata` |
| API 密钥 | 令牌增删改查、额度与限制 | `/keys` |
| 使用日志 | 调用记录查询、统计、详情 | `/usage-logs/common` |
| 高级配置 | 模型行为、重试、渠道自动停用、渠道亲和性 | `/system-settings/models/advanced` |
| 日志与监控 | 性能指标设置、日志开关、日志清理 | `/system-settings/operations/monitoring` |
| 系统信息 | 实例、系统任务、版本更新 | `/system-info` |
| 个人资料 | 资料、通知方式、安全、登录会话、通行密钥、两步验证、语言 | `/profile` |
| 错误页 | 401、403、404、500、503 | `/(errors)/*` |
| 全局框架 | 侧边导航、头部、命令搜索、通知、语言、主题 | 所有已登录页面 |

## 三、原子层

### 3.1 输入与选择

| 组件 | 职责 | 关键属性 |
|---|---|---|
| Button | 主动作、次动作、危险动作、图标按钮 | variant、size、loading、disabled、icon |
| IconButton | 只有图标的按钮 | aria-label 必填、tooltip |
| ButtonGroup | 动作组或分段控件 | 选中项、受控切换 |
| Input | 单行文本 | 前缀、后缀、错误状态 |
| NumberInput | 数字输入 | 最小值、最大值、步进、单位后缀 |
| Textarea | 多行文本 | 自动高度、字数上限 |
| PasswordInput | 密码输入 | 可见性切换、强度提示位 |
| SecretInput | 密钥输入 | 默认脱敏、显示切换、复制 |
| Select | 固定列表单选 | 选项、空选项、禁用项 |
| Combobox | 可搜索单选 | 远程选项、输入防抖、自定义空状态 |
| MultiSelect | 多选 | 已选数量、全选、搜索 |
| Checkbox、RadioGroup、Switch、Slider | 基础选择控件 | 受控、禁用、说明文字 |
| SegmentedControl | 少量选项的横向切换 | 选项、受控值 |
| InputOTP | 一次性验证码 | 固定长度、自动跳格、粘贴 |
| DatePicker、DateTimePicker | 日期与日期时间 | 时间戳与字符串互转 |
| TimeRangePicker | 时间范围 | 快捷预设、自定义区间、时区提示 |
| TagInput | 标签输入 | 已有标签建议、去重、上限 |
| KeyValueEditor | 键值对列表 | 自定义请求头、增删行 |
| JsonEditor | 带校验的 JSON 文本 | 语法高亮、格式化、错误提示 |
| StatusCodeListEditor | 状态码列表 | 数字与区间混输、去重 |
| ExpressionInput | 计费表达式 | 变量提示、语法校验（分层定价用） |

### 3.2 展示

| 组件 | 职责 | 备注 |
|---|---|---|
| Text、LongText | 普通文本与超长文本 | 超长时折叠展开 |
| CopyButton | 复制到剪贴板 | 复制成功反馈 |
| CopyableId | 可复制的标识 | 请求 ID、渠道 ID 这类短标识 |
| MaskedValue | 脱敏值 | 显示切换、复制 |
| Badge | 通用标记 | 颜色变体 |
| StatusBadge | 状态到颜色与文案的映射 | 成功、警告、危险、中性 |
| IconBadge | 图标底板 | 卡片标题使用 |
| TagBadge | 标签样式 | 可关闭、可点击筛选 |
| Avatar | 用户头像 | 无头像时的文字与配色 |
| ProviderBadge | 渠道类型徽标 | 提供商图标与名称 |
| ModelBadge、ChannelBadge、GroupBadge、VendorBadge | 业务对象徽标 | 名称加类型，尽量可点击跳转 |
| Tooltip | 悬浮说明 | 触屏上要有点按替代 |
| HoverCard | 悬浮预览 | 模型、渠道摘要 |
| Separator、SectionDivider | 分隔 | 横向与带标题两种 |
| Skeleton | 加载占位 | 与最终布局尺寸接近 |
| Spinner | 行内加载 | 按钮与小区域 |
| Progress | 进度 | 任务进度、日志清理进度 |
| Kbd | 快捷键提示 | 命令面板 |
| CodeBlock | 代码块 | 请求与响应原文 |
| MarkdownContent | 描述渲染 | 模型描述、发布说明 |
| JsonViewer | 只读 JSON | 折叠、复制、大文本虚拟滚动 |
| Alert | 提示条 | 信息、警告、错误、成功 |
| EmptyState | 空数据 | 图标、说明、可选动作 |
| ErrorState | 加载失败 | 错误信息、重试 |
| LoadingState | 整块加载 | 与 Skeleton 二选一，按场景 |
| QuotaDisplay | 配额与货币显示 | 统一换算入口 |
| TokenCount | Token 数量 | 缩写与全量提示 |
| LatencyText | 耗时显示 | 按阈值着色 |

### 3.3 反馈与浮层

| 组件 | 职责 | 备注 |
|---|---|---|
| Dialog | 通用对话框 | 标题、说明、动作区、可滚动内容 |
| AlertDialog | 确认与二次确认 | 危险动作必须使用 |
| Drawer | 侧边抽屉 | 表单主容器，宽窄两档 |
| Sheet | 移动端底部面板 | 与 Drawer 共用内容组件 |
| Popover | 轻量浮层 | 筛选、偏好设置 |
| DropdownMenu | 下拉菜单 | 行操作、用户菜单 |
| ContextMenu | 右键菜单 | 列表可选用 |
| Command | 命令面板 | 全局搜索与跳转 |
| Toast | 操作结果提示 | 成功、失败、进行中 |
| RiskAcknowledgementDialog | 高风险配置确认 | 例如高风险状态码、自动停用规则 |

### 3.4 布局

| 组件 | 职责 | 备注 |
|---|---|---|
| PageShell | 页面骨架 | 标题、说明、动作区、内容区四个槽位 |
| SectionCard、TitledCard | 区块卡片 | 标题、说明、图标、底部动作 |
| Stack、Grid | 间距与栅格 | 只在布局层使用间距变量 |
| SplitPane | 主从布局 | 日志详情这类左右结构 |
| ScrollArea | 滚动容器 | 统一滚动条样式 |
| StickyActionBar | 底部固定动作条 | 保存、重置、未保存提示 |
| CollapsibleSection | 可折叠区块 | 设置页的高级项 |
| ResponsiveRender | 断点切换渲染 | 表格与卡片两种视图共用数据 |

### 3.5 图表

| 组件 | 职责 | 备注 |
|---|---|---|
| ChartContainer | 图表外壳 | 统一颜色、字体、加载、空状态 |
| LineChart、AreaChart | 趋势 | 调用量、消耗、延迟 |
| BarChart | 对比 | 模型排行、渠道分布 |
| DonutChart | 占比 | 消耗分布 |
| Sparkline | 迷你趋势 | 卡片内嵌 |
| ChartAxis、ChartLegend、ChartTooltip | 图表零件 | 数值格式化统一走格式工具 |
| ChartPreferencePanel | 图表偏好 | 默认范围、粒度、图形类型，保存在本地 |

## 四、组合层

### 4.1 表格族

| 组件 | 职责 | 关键点 |
|---|---|---|
| DataTable | 表格主体 | 列定义、排序、行选择、加载骨架、空状态 |
| DataTableToolbar | 表格工具条 | 搜索、筛选入口、列显示、密度、刷新 |
| DataTableColumnHeader | 可排序表头 | 排序方向与清除 |
| DataTablePagination | 分页 | 页码、每页数量、总数 |
| DataTableRowActions | 行操作 | 下拉菜单，支持分组与危险项 |
| DataTableBulkActions | 批量操作 | 选中数量与批量动作 |
| DataTableMobileList | 移动端卡片列表 | 复用同一份数据与列定义 |
| DataTableCardRow | 卡片行 | 主标题、副标题、状态、数值、动作 |
| useTableState | 表格状态 | 分页、排序、筛选与 URL 同步 |
| useTableCompactMode | 密度偏好 | 本地存储 |

### 4.2 表单族

| 组件 | 职责 | 关键点 |
|---|---|---|
| FormField | 字段容器 | 标签、说明、必填标记、错误信息 |
| FormSection | 字段分组 | 分组标题与说明，抽屉与整页共用 |
| FormDialog | 对话框表单 | 表单本体与容器分离 |
| FormDrawer | 抽屉表单 | 宽窄两档，移动端全屏 |
| FormActions | 保存动作条 | 保存、重置、脏标记、离开提醒 |
| useFormDirtyGuard | 未保存提醒 | 关闭抽屉与切换路由时提示 |
| SecretInputField | 密钥字段 | 编辑时不回填旧值，只提交新值 |
| QuotaInput | 额度输入 | 货币与 Token 双单位切换 |
| DurationInput | 时长输入 | 秒、分钟、小时换算 |
| PercentInput | 百分比输入 | 小数与百分号互转 |
| SwitchRow | 带说明的开关行 | 设置页大量使用 |
| OptionForm | 选项读写表单 | 对接 `/api/option/`，批量对比差异后提交 |
| JsonOptionField | JSON 类型的选项字段 | 校验失败阻止保存 |
| NumberWithUnitField | 带单位的数字 | 状态码、秒数、天数 |
| TagListField | 字符串列表 | 关键字黑名单这类选项 |

### 4.3 筛选族

| 组件 | 职责 | 关键点 |
|---|---|---|
| FilterBar | 筛选容器 | 折叠、已选数量、重置 |
| SearchInput | 搜索输入 | 防抖、清除 |
| SelectFilter、MultiSelectFilter | 下拉筛选 | 选项来自接口或常量 |
| TimeRangeFilter | 时间筛选 | 预设与自定义 |
| FilterChip | 已选条件 | 单个移除 |
| FilterResetButton | 重置 | 保留搜索词的可选开关 |

### 4.4 指标与详情

| 组件 | 职责 | 关键点 |
|---|---|---|
| MetricCard | 单个指标 | 数值、单位、说明、加载 |
| MetricGrid | 指标组 | 响应式列数 |
| StatsStrip | 紧凑统计条 | 日志页顶部 |
| KeyValueList | 键值明细 | 详情弹窗 |
| DescriptionList | 描述列表 | 只读配置展示 |
| DetailDialog | 详情弹窗外壳 | 标题、动作、滚动区、JSON 段 |
| JsonDiffView | JSON 差异 | 上游同步、参数覆盖对比 |
| TimingMetricsCell | 耗时明细 | 总耗时、首字时间、按阈值着色 |
| CostDisplay | 费用明细 | 含工具调用附加费标记 |

## 五、业务区块层

### 5.1 渠道

列表部分：

| 组件 | 职责 |
|---|---|
| ChannelTable | 列表：ID、名称、类型、状态、模型数、分组、标签、优先级、权重、已用与剩余额度、响应时间、最近测试、操作 |
| ChannelCardList | 移动端卡片列表 |
| ChannelFilters | 状态、类型、标签、分组、关键字 |
| ChannelBulkActions | 批量启用、停用、删除、测活、编辑标签、复制、拉取模型 |
| ChannelStatusToggle | 单独启用或停用，带乐观更新 |
| ChannelRetryBadge | 展示当前最大重试次数，可跳转高级配置 |

编辑器（抽屉或整页，同一个表单组件）：

| 组件 | 职责 |
|---|---|
| ChannelTypePicker | 提供商类型选择，可搜索、带图标 |
| ChannelBasicFields | 名称、类型、状态、备注 |
| ChannelCredentialEditor | 密钥输入；多密钥模式（单条、批量、多合一的随机与轮询）、批量导入命名前缀、密钥表格（逐条启停与用量统计） |
| ChannelProviderAuthFields | 提供商专属鉴权：Vertex 的 JSON 与 API Key、AWS 的 AK/SK 与 API Key、Codex 的 OAuth 设备码流程、Cline 凭证导入、OpenCode Go 密钥解析 |
| ChannelConnectionFields | 接口地址、代理、自定义请求头、请求 UA |
| ChannelModelsEditor | 上游拉取模型、手工模型列表、启用模型子集、模型重映射表格、Ollama 模型版本读取 |
| ChannelGroupFields | 分组多选、预填分组、标签 |
| ChannelRoutingFields | 优先级、权重、自动测试间隔 |
| ChannelAdvancedEditor | 参数覆盖 JSON、自定义路由编辑器（模板、鉴权方式、格式转换、路径）、渠道检测设置、风险状态码确认 |
| ChannelBalancePanel | 余额查询配置、更新余额动作、Cline 与 Codex 额度读取 |

对话框与面板：

| 组件 | 职责 |
|---|---|
| ChannelTestDialog | 单渠道测试与批量测试，展示流式结果 |
| FetchModelsDialog | 从上游拉取模型并选择写入 |
| MissingModelsDialog | 列出缺失元数据的模型并补建 |
| CopyChannelDialog | 复制渠道，选择目标分组与名称 |
| TagBatchEditDialog | 批量编辑标签 |
| MultiKeyManageDialog | 多密钥统计、清理失效密钥 |
| ParamOverrideEditorDialog | 参数覆盖编辑 |
| AdvancedCustomEditorDialog | 自定义路由编辑 |
| BalanceQueryDialog | 余额查询结果 |
| ChannelFingerprintPanel | 指纹采样进度、逐次结果、候选排名 |
| UpstreamUpdateDialog | 上游更新检查与采纳 |
| ClineCredentialImportDialog | Cline 凭证导入与订阅识别 |
| CodexOAuthDialog | 设备码授权流程 |
| CodexUsageDialog | 用量与重置额度 |
| OpenCodeGoQuotaDialog | 订阅额度 |
| ChannelDetectionSettings | 按渠道的检测开关与端点配置 |

### 5.2 模型

| 组件 | 职责 |
|---|---|
| ModelTable | 列表：ID、模型名与匹配方式、状态、供应商、描述、标签、端点、绑定渠道、可用分组、计费类型、官方同步、时间 |
| ModelEditorForm | 模型名、匹配方式（精确与正则）、供应商、描述、标签、端点、可用分组、计费类型、绑定渠道 |
| ModelChannelBinder | 选择绑定渠道，带搜索与批量选择 |
| VendorManager | 供应商增删改查，图标与描述 |
| MissingModelsPanel | 缺失元数据的模型列表与一键补建 |
| PrefillGroupManager | 预填分组管理，供渠道与模型表单使用 |
| RatioTable | 模型倍率表格，支持行内编辑与批量设置 |
| RatioVisualEditor | 按阶梯批量设置倍率 |
| TieredPricingEditor | 分层定价表达式编辑 |
| ToolPriceTable | 工具调用价格 |
| GroupRatioTable、GroupSpecialUsableEditor | 分组倍率与分组可用范围 |
| UpstreamRatioSyncTable | 从上游同步倍率，对比后采纳 |
| ConflictConfirmDialog | 同步冲突确认 |
| ModelPricingPanel | 模型页面内嵌的价格面板，标签页：模型倍率、未设置模型、工具价格、上游同步 |

### 5.3 API 密钥

| 组件 | 职责 |
|---|---|
| ApiKeyTable | 列表：名称、状态、密钥（脱敏与复制）、剩余额度、分组、模型限制、IP 限制、创建时间、最近使用、过期时间 |
| ApiKeyEditorForm | 名称、分组与跨分组重试、过期时间（预设与自定义）、创建数量、额度（货币与 Token 双单位）、无限额度开关、模型限制、IP 白名单 |
| ApiKeyCell | 密钥脱敏显示、复制、查看 |
| ApiKeyBatchCopyDialog | 批量创建的密钥批量复制 |
| CcSwitchImportDialog | 生成 CC Switch 导入链接，选择应用、名称、主模型 |
| ApiKeyStatusToggle | 单条启用与停用 |

### 5.4 使用日志

| 组件 | 职责 |
|---|---|
| LogFilterBar | 时间范围、日志类型、模型、分组、令牌名、用户名、渠道 ID、请求 ID、上游请求 ID、敏感信息显示开关 |
| LogStatsStrip | 消耗额度、每分钟请求数、每分钟 Token 数 |
| LogsTable | 列表：时间、渠道、用户、令牌、模型、流式标记、Token 拆分、费用、耗时明细、内容摘要 |
| LogMobileCard | 移动端日志卡片 |
| LogDetailsDialog | 请求与响应原文、计费明细、重试信息、耗时拆解 |
| FailReasonDialog | 失败原因 |
| PromptDialog | 提示词内容 |
| ImagePreviewDialog、AudioPreviewDialog | 多媒体结果预览 |
| UserInfoDialog | 管理员查看调用用户信息 |
| LogScopeTabs | 全部日志与仅本人切换 |
| SensitiveVisibilityToggle | 隐藏或显示敏感字段 |

### 5.5 设置（选项驱动）

| 组件 | 职责 |
|---|---|
| SettingsPage | 分区注册表驱动的设置页，支持锚点 |
| SettingsSection | 单个设置分区，标题、说明、内容、保存条 |
| OptionForm | 读写 `/api/option/`，提交前对比差异 |
| GlobalModelSettingsForm | 请求原样转发开关、思考模型黑名单、Chat Completions 到 Responses 的转换策略、连通性检测间隔 |
| RoutingReliabilityForm | 最大重试次数、渠道自动停用阈值与关键字、自动启用、自动重试状态码、自动测试渠道（开关、间隔、测试模式） |
| ChannelAffinityForm | 开关、最大条目、默认存活时间、成功切换、渠道停用时保留 |
| AffinityRuleTable | 亲和性规则列表 |
| AffinityRuleEditor | 单条规则编辑 |
| AffinityVisualEditor | 规则可视化编辑，支持模板填充与 JSON 批量导入 |
| AffinityCachePanel | 缓存统计、刷新、清空 |
| PerfMetricsForm | 性能指标开关、写入间隔、聚合粒度、保留天数 |
| LogConsumeSwitch | 是否记录额度消耗 |
| LogCleanupPanel | 按时间点清理历史日志，进度轮询 |
| ServerLogFilesPanel | 日志目录、文件数量、总大小、日期范围、按数量或天数清理 |
| SystemTaskList | 系统任务列表与进度 |

### 5.6 系统信息

| 组件 | 职责 |
|---|---|
| InstanceTable | 实例列表，删除失效实例 |
| SystemTaskTable | 任务列表、状态、进度 |
| UpdateCheckPanel | 当前版本、最新版本、检查更新、发布说明 |

### 5.7 个人资料

| 组件 | 职责 |
|---|---|
| ProfileHeader | 头像、用户名、角色、分组 |
| ProfileBasicForm | 用户名、邮箱绑定 |
| PasswordChangeDialog | 旧密码、新密码、确认 |
| AccessTokenPanel | 生成、复制、删除访问令牌 |
| LanguageCard | 界面语言 |
| NotificationChannelForm | 通知方式（Webhook、Bark、Gotify）、额度预警阈值、各自的服务地址与令牌 |
| LoginSessionsPanel | 会话列表、当前会话标记、撤销单个、撤销其他 |
| PasskeyPanel | 注册通行密钥、列表、删除、浏览器不支持提示 |
| TwoFactorPanel | 启用向导、二维码、验证码校验、备用码、关闭 |

### 5.8 登录与初始化

| 组件 | 职责 |
|---|---|
| AuthLayout | 登录类页面的外壳，品牌区、语言切换、页脚 |
| LoginForm | 用户名与密码、记住登录、提交后进入两步验证分支 |
| TwoFactorForm | 六位验证码、重发倒计时 |
| PasskeyLoginButton | 通行密钥登录 |
| OAuthButtons | 按后端返回的可用提供商渲染，没有配置时不显示 |
| ForgotPasswordForm、ResetPasswordForm | 发送重置邮件、提交新密码 |
| RegisterForm | 注册开关关闭时不渲染 |
| SetupWizard | 首次初始化：环境检查、创建超级管理员账号 |
| SessionExpiredNotice | 会话过期后的提示与跳转 |

## 六、页面层清单

| 页面 | 组合的区块 | 接口 |
|---|---|---|
| 数据看板 | 时间与粒度筛选、统计卡、趋势图、消耗分布、模型排行、性能概览、图表偏好 | `/api/data`、`/api/data/self`、`/api/perf-metrics/summary`、`/api/status` |
| 渠道 | 渠道表或卡片、筛选、批量动作、渠道编辑器、各类诊断对话框 | `/api/channel`、`/api/channel/search`、`/api/channel/${id}`、`/api/channel/${id}/status`、`/api/channel/batch`、`/api/channel/copy/${id}`、`/api/channel/tag`、`/api/channel/models`、`/api/channel/disabled`、`/api/channel/ops`、`/api/channel/test`、`/api/channel/fetch_models`、`/api/channel/update_balance`、`/api/channel/multi_key/manage`、`/api/channel/fingerprint/${id}`、`/api/channel/codex/*`、`/api/channel/${id}/cline/quota`、`/api/channel/${id}/opencode-go/quota`、`/api/group/`、`/api/prefill_group` |
| 模型 | 模型表、模型编辑器、供应商管理、缺失模型、预填分组、价格面板 | `/api/models/`、`/api/models/search`、`/api/models/missing`、`/api/models/sync_upstream`、`/api/vendors/`、`/api/option/`、`/api/ratio_sync/channels`、`/api/ratio_sync/fetch` |
| API 密钥 | 密钥表、密钥编辑器、批量复制、CC Switch 导入 | `/api/token/`、`/api/token/search`、`/api/token/batch`、`/api/token/batch/keys`、`/api/token/${id}/key`、`/api/group/` |
| 使用日志 | 筛选栏、统计条、日志表或卡片、详情与各类预览弹窗 | `/api/log`、`/api/log/stat`、`/api/user/${userId}` |
| 高级配置 | 全局模型行为、路由可靠性、渠道亲和性、保存动作条 | `/api/option/` |
| 日志与监控 | 性能指标、日志开关、日志清理、服务器日志文件、任务列表 | `/api/option/`、`/api/system-task/log-cleanup`、`/api/system-task/list`、`/api/system-task/current` |
| 系统信息 | 实例列表、任务列表、更新检查 | `/api/system-info/instances`、`/api/system-info/stale-instances`、`/api/status` |
| 个人资料 | 资料、通知、安全、会话、通行密钥、两步验证、语言 | `/api/user/self`、`/api/user/setting`、`/api/user/token`、`/api/user/sessions`、`/api/user/passkey/*`、`/api/user/login/2fa`、`/api/oauth/email/bind`、`/api/verification` |
| 登录与初始化 | 登录、两步验证、找回与重置密码、通行密钥登录、初始化向导 | `/api/user/login`、`/api/user/login/2fa`、`/api/user/reset`、`/api/reset_password`、`/api/setup`、`/api/status` |
| 全局框架 | 侧边导航、头部、命令搜索、通知、语言、主题、用户菜单 | `/api/status`、`/api/user/self`、`/api/notice` |

## 七、应用层

| 组件或模块 | 职责 |
|---|---|
| QueryProvider | 查询客户端、重试策略、缓存时间 |
| AppErrorBoundary | 路由级与区块级错误兜底 |
| AuthProvider | 当前用户、角色、分组、登录与退出、会话过期处理 |
| HttpProvider | 统一请求前缀、携带凭据、注入令牌、401 自动刷新、错误信息归一化 |
| I18nProvider | 七种语言（简体中文、繁体中文、英语、法语、日语、俄语、越南语）、缺失键回退 |
| ThemeProvider | 亮色、暗色、圆角变量 |
| ToastProvider | 全局提示 |
| RouteGuard | 未登录跳转、无权限提示 |
| LocalPreferenceStore | 侧边栏折叠、表格密度、图表默认项 |
| PermissionHelper | 超级管理员与普通用户的分支判断 |
| FormatUtils | 配额、货币、Token、时间、速率、百分比的换算与显示 |
| LegacyRouteRedirect | 旧地址重定向到新地址 |

## 八、单位与格式

后端用整数存储配额，界面显示货币或 Token。换算规则集中在工具函数里，页面里不允许出现除以固定数字这类写法。

| 工具函数 | 用途 | 数据来源 |
|---|---|---|
| quotaToCurrency | 配额转货币 | `/api/status` 的单位与汇率 |
| currencyToQuota | 货币转配额 | 同上 |
| formatCurrency | 货币显示 | 同上，含符号与缩写 |
| formatTokens | Token 数量显示 | 千与百万缩写 |
| formatLatency | 耗时显示 | 毫秒与秒 |
| formatPercent | 百分比显示 | 成功率的显示 |
| formatDateTime、formatRelativeTime | 时间显示 | 统一时区处理 |
| formatDuration | 时长显示 | 任务与日志耗时 |

## 九、状态与交互矩阵

每个业务区块都要处理下面这些状态，避免重建后出现空白页：

| 状态 | 处理方式 |
|---|---|
| 首次加载 | 与最终布局尺寸一致的骨架 |
| 刷新中 | 保留旧数据，工具条显示加载指示 |
| 空数据 | 空状态组件，带说明与合理动作 |
| 加载失败 | 错误状态组件，带错误信息与重试 |
| 部分失败 | 局部错误提示，其余数据正常展示 |
| 无权限 | 隐藏入口或在页面内提示，不显示空白 |
| 参数非法 | 表单内联错误，阻止提交 |
| 长文本 | 折叠展开与复制 |
| 大数据量 | 服务端分页，表格关闭全量渲染 |
| 危险动作 | 二次确认，必要时要求输入确认文字 |

## 十、移动端适配

| 项目 | 处理方式 |
|---|---|
| 导航 | 底部标签栏，四项到五项，其余入口放进“更多” |
| 表格 | 同一份数据渲染成卡片列表，卡片上只保留两到三个关键数值 |
| 抽屉 | 全屏抽屉，动作按钮固定在底部 |
| 触控 | 可点区域不小于 44 像素，滑动手势不承载唯一入口 |
| 图表 | 宽度自适应，点按显示数值，图例可折叠 |
| 输入 | 键盘弹出时滚动到焦点字段，提交按钮跟随 |
| 安全区域 | 底部标签栏与动作条留出安全区 |
| 表单 | 单列布局，字段说明放在输入框下方 |

## 十一、布局无关约束

重建会更换布局，所以组件要满足以下约束，换布局时不需要改组件内部实现：

1. 组件不读取路由参数，状态由页面通过属性传入。
2. 表单组件同时支持抽屉、对话框、整页三种容器，容器只负责尺寸与转场。
3. 区块不渲染页面标题，标题由页面层提供。
4. 组件宽度由容器决定，使用容器查询而不是窗口查询。
5. 表格与卡片列表共用列定义，切换视图不复制数据处理逻辑。
6. 间距使用统一变量，不在组件内写死像素值。

## 十二、已确认删除，重建时不要恢复

以下功能在当前的精简过程中已经移除，重建时不要带回：

- 多用户相关：用户管理、用户额度与分组编辑、排行榜、注册奖励与邀请。
- 商业化相关：钱包、充值、兑换码、订阅计划、支付网关配置、合规确认。
- 公开站点相关：落地页、公开定价页、关于、用户协议与隐私政策、页脚导航。
- 聊天集成：聊天预设跳转、Cherry Studio 与 AionUI 这类外部入口、在线聊天页面与游乐场。
- 系统设置中的站点品牌、系统公告编辑、侧边栏模块、身份验证提供商配置、敏感词、令牌限制、Worker 代理、性能页面、限速设置、SMTP 邮件、生图与任务日志。
- 概览页的公告、常见问题、接口信息、在线状态面板。

## 十三、需要单独确认的边界项

| 项目 | 当前状态 | 建议处理 |
|---|---|---|
| 通知弹窗 | 头部保留组件，数据来自站点公告 | 保留，个人使用时用于查看后端发布说明 |
| 系统信息页面 | 页面存在，导航中没有入口 | 移入个人资料或高级配置页面底部，避免出现无入口的页面 |
| 注册、找回密码 | 页面存在，后端开关控制 | 保留页面，注册开关关闭时不渲染入口 |
| 绘图日志、任务日志 | 数据接口与列定义存在，导航已移除 | 保留接口映射，界面只做普通日志，需要时再加类型筛选项 |
| 邮箱绑定 | 个人资料中保留 | 保留，仅作为找回方式 |

## 十四、实现顺序

顺序按依赖关系排列，前面的层稳定后后面的层才能开始：

1. 应用层：请求客户端、鉴权、国际化、主题、格式化工具。
2. 原子层：输入与选择、展示、反馈与浮层、布局。
3. 组合层：表格族、表单族、筛选族、指标与详情。
4. 图表封装。
5. 业务区块：渠道、模型、密钥、日志。
6. 业务区块：设置、系统信息、个人资料。
7. 页面层：看板、各管理页面、错误页。
8. 移动端视图与底部标签栏。

这个顺序由依赖关系决定：应用层与原子层是全部页面的共同基础，渠道与模型的区块最复杂，先做这两个可以让表格族与表单族的缺陷尽早出现。
