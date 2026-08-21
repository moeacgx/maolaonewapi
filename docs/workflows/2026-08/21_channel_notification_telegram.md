# 渠道启停通知改用通知中心 Telegram

日期: 2026-08-21

## 目标

自动禁用和自动恢复渠道时，不再调用用户通知偏好中的邮件、Webhook、Bark 或 Gotify 通道；两类事件统一进入通知中心，由管理员配置的 Telegram Bot 任务投递。

## 实现

- 新增核心通知事件 `channel_disabled` 和 `channel_enabled`。
- 通知中心事件定义提供渠道名称、渠道 ID、禁用原因和提及用户变量，并提供默认 Telegram 模板。
- `service.DisableChannel` 在渠道状态实际切换成功后入队 `channel_disabled`。
- `service.EnableChannel` 在渠道状态实际恢复成功后入队 `channel_enabled`。
- 入队使用通知中心已有的幂等事件、投递记录和异步 Telegram dispatcher；通知表尚未迁移时静默跳过，不影响渠道状态变更。
- 移除渠道启停路径对 `NotifyRootUser` 的调用，因此不会再通过根用户邮箱发送这两类通知。其他用户额度等通知保持原有通知偏好行为。

## 验证

- 渠道通知服务测试确认两类事件各生成通知事件和投递记录，payload 包含渠道名称、ID 和禁用原因。
- 控制器测试确认通知中心公开两类核心事件及模板变量。
- `go test ./controller ./service ./model -run 'Notification|ChannelNotification' -count=1 -timeout 120s` 通过。
