# 单渠道监控开关定时测试修复

## 背景

`zzapi` 渠道编辑页支持在 `settings` 中保存单渠道监控覆盖项，例如
`monitor_enabled=false`。后台定时 `channel_test` 任务此前只按渠道状态、
全局渠道测试模式和自动禁用开关筛选渠道，没有读取该覆盖项，导致已关闭监控的
渠道仍会产生“模型测试”消费日志。

## 范围

- 定时渠道测试：计划任务运行时跳过 `monitor_enabled=false` 的渠道。
- 手动批量测试：管理员主动触发“测试所有渠道”时仍按手动语义测试全部非手动禁用渠道。
- 渠道设置契约：`ChannelOtherSettings` 保留监控相关字段，避免后端读写设置时丢失前端保存的覆盖项。

## 行为契约

- `settings.monitor_enabled` 缺省或为 `true`：渠道继续纳入定时渠道测试。
- `settings.monitor_enabled=false`：渠道不纳入后台定时 `channel_test`。
- 手动 `GET /api/channel/test` 创建的批量测试任务不受 `monitor_enabled=false` 限制。
- 手动 `GET /api/channel/test/{id}` 单渠道测试不受该字段限制。

## 兼容性

该变更不调整数据库结构，只扩展 `settings` JSON 的 Go 结构定义。已有渠道缺省不受影响。

## 验证计划

- `go test ./controller -run TestSelectChannelsForAutomaticTest`
- `cd relaykit && GOWORK=off go build ./...`
- `git diff --check`
