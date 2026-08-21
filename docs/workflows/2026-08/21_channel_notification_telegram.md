# 渠道启停通知改用通知中心 Telegram

日期: 2026-08-21

## 目标

自动禁用和自动恢复渠道时，不再调用用户通知偏好中的邮件、Webhook、Bark 或 Gotify 通道；两类事件统一进入通知中心，由管理员配置的 Telegram Bot 任务投递。

## 实现

- 渠道禁用事件 payload 现在包含结构化 `status_code`、`error_code`、`error_message`，同时保留原 `reason` 兼容旧模板。
- 通知任务可针对 `channel_disabled` 配置状态码范围和报错关键词；状态码与关键词同时填写时按 AND 关系判断，多个关键词按 OR 关系判断。
- 筛选在通知事件入队阶段执行，不匹配的通知任务不会创建投递记录。

## 验证

- 渠道通知服务测试确认两类事件各生成通知事件和投递记录，payload 包含渠道名称、ID、状态码、错误码、错误消息和禁用原因。
- 通知模型测试覆盖状态码范围、大小写不敏感关键词、状态码与关键词 AND 关系，以及入队阶段只创建匹配任务投递记录。
- 控制器测试确认通知中心公开两类核心事件及模板变量，并校验渠道筛选配置。
- `go test ./controller ./service ./model -run 'Notification|ChannelNotification' -count=1 -timeout 120s` 通过。
- Default 通知中心类型检查和前端测试通过。
