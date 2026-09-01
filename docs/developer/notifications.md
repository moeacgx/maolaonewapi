# 通知中心与模块事件

通知中心只负责发送。业务模块产生事件，通知中心根据任务、Bot、接收目标和模板完成投递。模块不能读取 Bot Token，也不能覆盖接收人和模板；一个 Bot 可以被多个任务复用。

## 内置事件

内置事件定义必须同时提供事件值、显示名称、默认模板、变量白名单和示例负载。当前核心事件包括：

- `invoice_pending`：变量包括 `mention`、`invoice_id`、`source_type`、`source_id`、`user_id`、`title`、`total_amount`、`create_time`。
- `channel_disabled`：变量包括 `mention`、`channel_name`、`channel_id`、`status_code`、`error_code`、`error_message`、`reason`。
- `channel_enabled`：变量包括 `mention`、`channel_name`、`channel_id`。

`invoice_pending` 在发票记录事务内进入 `pending`（待开票）状态时入队，事件键为
`invoice:<invoice_id>`。单笔充值/订阅发票、零服务费的合并发票，以及
合并外部支付都会在创建时触发；服务费尚未支付的外部合并申请保持
`payment_pending`，不提前通知，待支付成功转为 `pending` 时触发一次。合并发票的
`source_type` 为 `batch`，`source_id` 为合并发票号，`total_amount` 为所有来源订单
开票金额与服务费的合计。

渠道事件的负载只提供渠道状态字段，不提供发票金额字段。模板校验和发送渲染都按事件负载执行，未知变量必须拒绝。

## 模板生命周期

模板使用 `{{variable}}` 语法，所有负载值在发送前转义为 Telegram HTML。创建和更新任务时，后端使用该事件的示例负载校验模板；异步发送时再次按实际事件负载渲染，避免绕过管理端校验的历史数据发送错误消息。

通知中心前端在切换事件类型时，如果当前模板为空或仍等于原事件默认模板，会替换为目标事件默认模板；用户已经自定义的模板会保留。

早期前端曾在从 `invoice_pending` 切换到渠道事件时保留发票默认模板。升级后的后端对核心渠道事件的空模板和这一个完整的历史默认模板做按事件归一化：渠道禁用和渠道启用分别替换为各自默认模板；发票事件仍使用包含 `total_amount` 的发票默认模板。该兼容处理覆盖保存、管理端加载和异步发送，但不会放行任意自定义的 `{{total_amount}}` 或其他未知变量。

## 渠道事件接入

渠道状态实际改变后，在业务代码中调用通知事件入队。`channel_disabled` 负载应包含渠道名称、渠道 ID、状态码、错误码、错误消息和禁用原因；`channel_enabled` 负载包含渠道名称和渠道 ID。通知表尚未迁移时，入队应静默跳过，不影响渠道状态变更。

投递由通知中心统一处理 Telegram 调用、429 重试、失败状态和历史清理。投递请求使用 `chat_id`、HTML `text`、`parse_mode: HTML` 和 `disable_web_page_preview: true`，Bot Token 不出现在响应、日志或事件负载中。

## 渠道禁用筛选与 Classic 编辑

`filter_config` 只对 `channel_disabled` 任务生效；其他事件类型的任务请求不得携带该字段。配置为空时不写入筛选 JSON。`status_codes` 是由服务端校验的 HTTP 状态码或范围字符串；`error_keywords` 会去除首尾空白并忽略空值，最多 64 项、每项最多 256 个 Unicode 字符。

关键词匹配 `error_message` 或 `reason` 字段，多个关键词之间是 OR 关系；状态码和关键词同时填写时是 AND 关系。服务端匹配使用不区分大小写的 `strings.ToLower` 语义。

Classic 通知任务编辑器使用 TextArea，每行一个报错关键词；打开已有任务时将 `error_keywords` 数组按换行回显，输入使用 `split(/\r?\n/)` 处理 LF 和 CRLF。保存时由 `normalizeNotificationFilterConfig` 去空行、trim 并按非 locale 的小写身份去重，保留首次出现的原始拼写。任务 Modal 的 class 挂在 Portal 外层，窄屏宽度规则直接约束其内层 `.semi-modal`，并对 `.semi-modal-content`、`.semi-modal-body-wrapper`、`.semi-modal-body` 和任务 body 设置盒模型收缩边界；目标卡片和输入宽度规则也均限定在该 class 下，footer 保持可达。

## 模块事件

扩展模块通过宿主声明变量白名单和默认模板，再由受信任的服务端事件入口发布事件。模块事件变量必须与声明的负载字段一致，事件 ID 使用小写字母、数字、短横线和下划线，完整事件名最多 64 个字符。模块不得把 Bot Token、Access Token 或密码放入负载。

## 变更与验证

涉及事件变量、默认模板、请求字段或发送行为的改动，必须同时更新本专题文档和 `docs/workflows/YYYY-MM/` 工作记录，并覆盖保存校验、历史模板兼容、未知变量拒绝及 Telegram 请求负载测试。
