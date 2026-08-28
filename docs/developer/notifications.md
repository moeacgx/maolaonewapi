# 通知中心与模块事件

通知中心只负责发送。业务模块产生事件，通知中心根据任务、Bot、接收目标和模板完成投递。模块不能读取 Bot Token，也不能覆盖接收人和模板；一个 Bot 可以被多个任务复用。

## 内置事件

内置事件定义必须同时提供事件值、显示名称、默认模板、变量白名单和示例负载。当前核心事件包括：

- `invoice_pending`：变量包括 `mention`、`invoice_id`、`source_type`、`source_id`、`user_id`、`title`、`total_amount`、`create_time`。
- `channel_disabled`：变量包括 `mention`、`channel_name`、`channel_id`、`status_code`、`error_code`、`error_message`、`reason`。
- `channel_enabled`：变量包括 `mention`、`channel_name`、`channel_id`。

渠道事件的负载只提供渠道状态字段，不提供发票金额字段。模板校验和发送渲染都按事件负载执行，未知变量必须拒绝。

## 模板生命周期

模板使用 `{{variable}}` 语法，所有负载值在发送前转义为 Telegram HTML。创建和更新任务时，后端使用该事件的示例负载校验模板；异步发送时再次按实际事件负载渲染，避免绕过管理端校验的历史数据发送错误消息。

通知中心前端在切换事件类型时，如果当前模板为空或仍等于原事件默认模板，会替换为目标事件默认模板；用户已经自定义的模板会保留。

早期前端曾在从 `invoice_pending` 切换到渠道事件时保留发票默认模板。升级后的后端对核心渠道事件的空模板和这一个完整的历史默认模板做按事件归一化：渠道禁用和渠道启用分别替换为各自默认模板；发票事件仍使用包含 `total_amount` 的发票默认模板。该兼容处理覆盖保存、管理端加载和异步发送，但不会放行任意自定义的 `{{total_amount}}` 或其他未知变量。

## 渠道事件接入

渠道状态实际改变后，在业务代码中调用通知事件入队。`channel_disabled` 负载应包含渠道名称、渠道 ID、状态码、错误码、错误消息和禁用原因；`channel_enabled` 负载包含渠道名称和渠道 ID。通知表尚未迁移时，入队应静默跳过，不影响渠道状态变更。

投递由通知中心统一处理 Telegram 调用、429 重试、失败状态和历史清理。投递请求使用 `chat_id`、HTML `text`、`parse_mode: HTML` 和 `disable_web_page_preview: true`，Bot Token 不出现在响应、日志或事件负载中。

## 模块事件

扩展模块通过宿主声明变量白名单和默认模板，再由受信任的服务端事件入口发布事件。模块事件变量必须与声明的负载字段一致，事件 ID 使用小写字母、数字、短横线和下划线，完整事件名最多 64 个字符。模块不得把 Bot Token、Access Token 或密码放入负载。

## 变更与验证

涉及事件变量、默认模板、请求字段或发送行为的改动，必须同时更新本专题文档和 `docs/workflows/YYYY-MM/` 工作记录，并覆盖保存校验、历史模板兼容、未知变量拒绝及 Telegram 请求负载测试。
