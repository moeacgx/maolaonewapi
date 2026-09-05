# 中间件提前返回错误日志落库

## 问题

渠道分发发生在 `Distribute` 中间件。无可用渠道、渠道并发达到上限等错误会直接
`Abort`，不会进入 `controller.processChannelError`，因此客户端收到 503 时后台可能没有
对应的错误日志。

## 处理契约

- 分发阶段错误统一通过 `abortDistributorError` 处理。
- 默认启用错误日志；显式设置 `ERROR_LOG_ENABLED=false` 时仍可关闭。
- 认证请求写入一条 `LogTypeError`，保留请求路径、模型、分组、状态码、错误码和
  `error_stage=distribution`。
- 尚未选出具体渠道时使用 `channel_id=0`，不伪造上游渠道信息。
- 不改变 HTTP 错误响应、渠道选择或 Relay 重试行为。

## 验证

- `go test ./middleware -run '^$' -count=1 -timeout 60s`
- `go test ./controller -run '^TestSettledEmptyUsageRecordsOnlyOneErrorLog$' -count=1 -timeout 60s`
- `git diff --check`
