# xAI 分组权限、操练场 400 与错误日志修复

日期：2026-09-07

## 问题

多实例部署中，xAI 令牌偶发返回“无权访问 Grok-Super 分组”，操练场使用
xAI 渠道时返回 `status_code=400, Upstream error: 400`；鉴权提前失败时后台没有
错误日志。

## 根因

- 分组权限和全局配置驻留在进程内存。配置刷新路径在持有 `OptionMapRWMutex` 时
  调用配置后处理，定价配置后处理又会读取同一把锁，形成死锁。某个实例停止刷新
  后，负载均衡会出现节点间分组权限不一致。
- 令牌分组门禁和操练场分组门禁在渠道选择前直接返回，没有调用 `RecordErrorLog`。
- 操练场使用 Chat Completions 路由；xAI 渠道的上游兼容路径需要转换为 Responses
  请求，否则上游只返回泛化的 400。

## 修改范围

- 释放 `OptionMapRWMutex` 后再执行配置更新和后处理，主题配置的回写也遵循同一锁边界。
- 令牌分组越权、弃用分组及操练场分组拒绝写入 `LogTypeError`，无上游渠道时使用
  `channel_id=0`，记录 `error_stage=authentication` 或 `distribution`。
- 操练场对 xAI 渠道默认启用 Chat→Responses 兼容转换；标准 API 仍由全局策略控制。

## 兼容性与安全边界

- 不放宽标准 `/v1/**` 的令牌绑定权限。
- 不修改上游 API Key、渠道地址或数据库结构。
- 错误日志不包含令牌明文；管理员详情保留请求路径、模型、分组和状态码。

## 验证

- `go test ./service -run '^TestShouldChatCompletionsUseResponsesForRequest$' -count=1 -timeout 60s`
- `go test ./middleware -run '^TestRecordAuthErrorLogPersistsGroupAccessFailure$' -count=1 -timeout 60s`
- `go test ./model -run '^TestUpdateOptionMap' -count=1 -timeout 60s`
- `gofmt` 与 `git diff --check`

生产实例的重启和渠道配置修改不属于本次代码变更，需单独按生产操作流程确认。
