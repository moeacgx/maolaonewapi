# 本项目二次开发能力

这份清单记录当前二开分支已落地的业务能力和继续开发的边界。

## 已落地能力

- 双前端与主题切换：Default、Classic 共用后端 API，发票、订阅、返利、通知中心和扩展都有入口。代码见 web/default/src/routes/\_authenticated、web/classic/src/App.jsx。
- 多支付网关充值：EPay、Stripe、Creem、Waffo、Waffo Pancake、BEPUSDT、OKPay，含回调、重试和订单快照。代码见 controller/topup\_\*.go、model/topup.go。
- 订阅计费和套餐：套餐 CRUD、启停、购买、周期配额重置、限制、退款和多渠道付款。路由为 /api/subscription/_、/api/subscription/admin/_。
- 发票中心：支付时申请、历史订单合并申请、服务费支付、个人/企业和普票/专票状态流转；后台支持保留审计数据的单条和批量软删除。订单首次进入待开票状态产生 invoice_pending。代码见 model/invoice.go、model/invoice_order.go、model/invoice_payment.go。
- 返利和提现：邀请关系、分级返佣、成熟期结算、提现账户、审批、风控冻结和追回。路由为 /api/affiliate/_、/api/affiliate/admin/_。
- 邀请制注册：新增默认关闭的 `InvitationRegisterEnabled`。公开注册关闭后，密码注册与 OAuth 新用户只需携带仍有效的返利邀请码；创建事务内会再次确认邀请人状态与返利风控状态。邀请码熵较低，持有者或猜中者均可尝试注册，这是取消额外签名后的明确安全取舍。浏览器只在当前标签页临时保存邀请码。密码注册仍受 `PasswordRegisterEnabled` 约束，已有 OAuth 用户登录不受影响。完整契约见 [邀请制注册](invitation-registration.md)。
- 通知中心：Telegram Bot、多个任务和目标、提及、自定义模板、事务 outbox、429 重试、死信和每任务最新五条历史。管理路由为 /api/notification/\*，仅 root 可管理。
- 自定义 OAuth/OIDC：数据库动态注册、Discovery、字段映射、绑定和解绑。路由为 /api/custom-oauth-provider/\*、/api/oauth/:provider。
- 异步图片任务：POST/GET /v1/images/tasks，复用渠道、限流和计费链路并按 TTL 清理；任务日志列表跳过图片正文，图片预览按需加载首张结果；最终失败会稳定进入错误使用日志，并与 Relay 已有日志去重。代码见 controller/canvas_image_task.go、controller/task.go，失败日志契约见[异步图片失败使用日志补齐](../workflows/2026-07/31_async_image_failure_usage_log.md)。
- 图片模型端点兼容：图片模型误用 `/v1/chat/completions` 或 `/v1/responses` 时，网关会在 Relay 内部改写到 `/v1/images/generations`，并从 `input`、`messages` 或 `instructions` 提取 `prompt`。内置图片模型继续自动识别；自定义模型仅在唯一端点为 `image-generation` 时参与改写，Codex 渠道、`-openai-compact`、自定义多端点模型、Responses compact、multipart 编辑和普通文本模型保持客户端原路径。自动改写后的跨渠道重试会继续排除 Codex。完整边界见[自动路由工作记录](../workflows/2026-07/29_image_model_endpoint_auto_route.md)。
- 动态计费表达式：覆盖预消费、结算、日志和版本快照；新增变量或函数前必须阅读 pkg/billingexpr/expr.md。
- 用户资料与计费隔离：设置、管理访问令牌、身份绑定及管理资料使用显式列或字段白名单更新；通用 `User.Update` 只允许用户名、显示名和显式密码。设置 JSON 通过字节精确旧值 CAS 合并并发修改，管理员资料以目标旧角色为 CAS 守卫，角色和分组更新也检查旧值。Redis 用户快照使用每用户 generation 栅栏并保留带有效用户标记的实时额度；缓存缺失时通过跨实例回退锁把待处理额度立即落库。额度接口区分可中止预扣与已承诺结算/退款，后者落库失败会进入受保护队列并按已接受处理，Redis 关闭时也禁止读取本实例尚未落库的旧额度。四个高频自助入口按认证用户 ID 原子限流，`PUT /api/user/self` 另有 64 KiB 请求体和 16 KiB 侧边栏 JSON 限制。完整边界见[用户自助资料更新覆盖计费字段修复](../workflows/2026-08/05_user_self_accounting_lost_update.md)。
- 官方上游修复采用按功能手工回移策略，保留自定义补全倍率和动态计费作为管理员定价事实来源，
  不按 GPT-5.6 等模型名称强制覆盖配置。当前选择性回移范围和验证边界见
  [官方上游修复选择性回移](../workflows/2026-08/04_official_upstream_selected_backports.md)。
- 渠道运维：趋势、稳定性、状态码、失败链路、并发限制、亲和缓存和上游模型变更检测。单渠道并发上限调低时从当前在途并发开始，在一分钟内线性收敛；调高或改为不限制仍立即生效。多分组令牌按配置顺序跨组重试，每个分组最多发起一次上游请求，失败后立即进入下一组；每一次选渠道只使用当前重试分组的渠道候选集。数据库路径中的 `abilities` 是模型与分组候选的兼容索引，最终选渠与渠道亲和必须以结构化 `channel_groups` 当前绑定为准；启动回填完成后关联表是唯一权威，绑定缺失采用拒绝策略，历史能力残留不得扩大路由范围。MySQL 的能力分组复合主键使用大小写敏感排序规则，分组编码在三种数据库中均保持大小写敏感。完整边界见 [数据库选渠与渠道当前分组一致性](../workflows/2026-07/30_channel_retry_current_group_consistency.md)。分组可标记为独立，独立分组只能单独绑定令牌，历史冲突绑定在请求时返回 503；旧客户端缺失独立字段时保留原值，请求热路径使用可失效快照而不每次查库。结构化接口新建分组时，后端取得真实 ID 后以 ID 的十进制文本作为最终兼容 code；旧分组通过管理员显式预检和事务迁移切换为数字 code，并同步渠道、令牌、用户、能力、订阅、选项、屏蔽词分组规则、安全审计分组范围与白名单及相关缓存，`default` 保持固定标识；事实快照保留原编码并在查询时通过历史别名归并。令牌分组迁移遇到已存在的目标绑定时只去重该令牌内部的分组，不删除独立令牌记录。OpenAI/Codex 流中的前导事件会立即发送和刷新，避免慢模型首字节饥饿；只有首个上游错误发生且 Gin Writer 尚未提交时才透明切换渠道，已经写出任何事件后不在同一连接重放。官方模型容量错误仍可在未提交时绕过亲和失败跳过规则并排除当前渠道；普通 500 按管理员状态码范围处理，策略上可重试的失败会淘汰本次实际命中的精确亲和绑定。完整边界见[上游容量错误跨渠道重试](../workflows/2026-08/01_upstream_capacity_cross_channel_retry.md)和[流内 500 重试与失败亲和淘汰](../workflows/2026-08/04_stream_retry_affinity_eviction.md)。
- 模型广场失败排除：`perf_metrics_setting.failure_filter_rules` 支持按状态码、错误码、错误正文或完整错误响应配置包含、精确匹配和 RE2 正则；每条规则可保存多个独立匹配值，Enter 添加内容、Shift+Enter 保留正文换行，旧单值 `value` 配置继续兼容读取。命中只排除模型广场失败样本，不改变客户端响应、安全审计、重试、计费或渠道质量统计。完整契约见 [模型广场失败过滤规则](../workflows/2026-07/29_model_plaza_failure_filter_rules.md)。
- 游戏钱包和预测玩法：主链路存在，但 JudgeProvider 尚未实现，自动判题会回落人工，标记为实验性。
- 站点与导航定制：Logo、页脚、公告、FAQ、自定义链接、分区、图标和排序。
- CC Switch 一键导入：聊天设置提供独立的 `CCSwitchAPIAddress` API 根地址；Default、Classic 的令牌页按钮和聊天 `ccswitch` 入口共用该配置。留空回退网站服务器地址，Claude/Gemini 使用根地址，Codex 幂等追加 `/v1`，导入信息中的官网地址仍使用网站主域名。完整契约见 [CC Switch 自定义 API 地址](../workflows/2026-07/30_ccswitch_custom_api_address.md)。
- 安全审计：内置 Root 独立页面，统一管理既有屏蔽词过滤、无需 Guard 的上游 `cyber_policy` 事后事件，以及 Qwen3Guard 异步观察和同步阻断；屏蔽词规则可逐条选择全部渠道，或在同一指定范围中同时选择多个渠道和多个业务分组，命中任一目标即生效。分组按渠道实际绑定的稳定分组编码匹配，不把 `Channel.Tag`、用户分组或关键词预填组当成新规则目标。审计事件页面展示本次实际渠道、实际路由分组和事件发生时的令牌绑定分组快照；显式多分组展示全部绑定，`auto` 令牌显示 `auto`，渠道绑定分组快照仅保留在接口中用于兼容。审计事件可按事件发生时固化的用户名进行忽略大小写的部分匹配，列表、删除预览和确认删除复用同一条件。上游官方风控自动封禁支持业务分组白名单，白名单事件继续留痕但不参与窗口累计或处置。支持加密事件原文、无密钥元数据事件、持久任务队列、Guard 节点池及 Realtime 文本门禁。Guard 默认关闭，本地屏蔽词与上游策略事件可独立运行；管理路由为 /api/security-audit/\*，完整设计见 [安全审计](prompt-security-audit.md)。
- 完整请求归档：安全审计页内的 Root 能力，可选择归档全部符合条件的请求，或只在审计事件成功落库后归档对应原始请求。事件模式可同时按最终实际渠道、实际业务分组稳定编码和审计来源筛选，同维度按任一匹配、不同非空维度按同时匹配；来源可限定为 Prompt Guard、屏蔽词或官方风控。HTTP 原始正文与 Realtime 客户端帧进入跨数据库持久队列后，异步投递到多个可切换的本地、S3 兼容或 Cloudflare R2 目标；Realtime 事件模式只归档直接触发客户端审计的帧，或上游风控发生时最近的相关客户端帧，不回溯整条连接的历史音频。支持任务数与字节双容量、租约重试、精确版本清理、配置 CAS 和脱敏运行状态；归档失败不影响 Relay。管理路由为 /api/security-audit/request-archive/\*，配置稳定性标记为实验性，完整契约见 [安全审计](prompt-security-audit.md#完整请求归档)。

## 扩展宿主

完整说明见[扩展模块开发](extensions.md)。当前提供 static、http 两种运行时、动态导航、兼容 iframe、`native v1` 宿主原生渲染槽、宿主上下文、模块代理、上传/启停/卸载，以及按 manifest 能力发布通知事件。原生模块仍由 zip 交付，业务页面不注册进主程序固定路由：

- /api/extensions/host/me
- /api/extensions/:id/proxy/\*path
- /api/extensions/:id/native/:pageKey/:target/:asset
- /api/extensions/:id/notification-events
- /api/extension-admin/\*

## 继续开发的接口

- 新 AI 渠道：relay/channel/\* 适配器和渠道类型注册；支持 StreamOptions 时加入 streamSupportedChannels。
- 新异步任务：复用 service.TaskPollingAdaptor 和主节点调度器，明确租约、超时、清理、多节点行为。
- 新 OAuth：实现 oauth.Provider 并接入注册表。
- 新通知：外部模块声明 notifications.events；内置 Go 业务在事务中调用 model.EnqueueNotificationEventTx。
- 新计费：先阅读 pkg/billingexpr/expr.md。
- 模型端点覆盖：使用 ModelMeta.Endpoints。对于非内置图片模型，唯一的 `image-generation` 端点会作为图片自动路由信号；自定义多端点模型保持客户端原路径。

## 必须遵守的约束

- JSON 使用 common.Marshal、common.Unmarshal、common.DecodeJson 等封装。
- 数据库同时兼容 SQLite、MySQL、PostgreSQL，优先 GORM。
- 通知事件和订单状态变更使用同一事务。
- 密钥不能写入 manifest、静态前端或日志。
- 新页面同时考虑 Default 和 Classic；模块页面必须有加载、空、错误状态。
- 可信的一方页面、看板和后台工具默认使用 `native v1` 并同时提供 Default、Classic 入口；仍受信任但需要独立 DOM、样式或框架运行时的第三方/遗留页面，以及远程 HTTP UI 才使用 iframe。iframe 不是安全沙箱，不可信模块禁止安装，也不得用主题变量桥接冒充原生 UI。
- 默认使用 static 轻量模块，需要常驻进程、队列或长连接才使用 http。
- 不得修改受保护的项目标识、署名和归属信息。

## 本地验证

固定使用后端 3000、Classic 3001 和 tmp-local-v10101.db：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/local-test.ps1 -Action status
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/local-test.ps1 -Action start
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/local-test.ps1 -Action verify
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/local-test.ps1 -Action stop
```
