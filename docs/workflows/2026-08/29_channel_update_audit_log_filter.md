# 渠道同步审计日志过滤

## 目标

ApiPanelWatch 周期性通过管理接口同步渠道配置时，会产生大量 `channel.update`
管理审计记录，干扰后台日志观察。该同步属于管理操作日志，不参与模型消费的
RPM/TPM 统计，但持续写入数据库没有排障价值。

## 方案

在 `RecordOperationAuditLog` 写入入口精确过滤 action 为 `channel.update` 的记录。
渠道创建、删除、启停、标签操作和其他管理审计保持原有行为；渠道更新接口本身
以及鉴权、权限和消费日志不受影响。

## 兼容性与边界

- 不修改 `LogTypeConsume`，RPM/TPM 统计口径保持不变。
- 不删除历史日志，仅阻止新的渠道同步审计写入。
- 仅匹配稳定 action 标识，不按内容文本或用户/IP 过滤，避免误伤其他审计。

## 验证

- 回归测试确认 `channel.update` 不落库。
- 回归测试确认 `channel.create` 仍正常落库。
- 运行 `go test ./model -run TestRecordOperationAuditLogSuppressesChannelUpdates -count=1 -timeout 60s`。
- 运行 `git diff --check`。
