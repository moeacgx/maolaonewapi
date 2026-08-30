# 客户端取消与零用量错误分类

## 问题

Responses 流已向客户端发送部分 SSE 后，Codex 客户端可能在最终
`response.completed` usage 到达前主动断开。NewAPI 会正确停止上游请求，tokens-pro
因此只能记录 `input_tokens=0`、`output_tokens=0`。旧逻辑把这类已知的
`client_gone/context canceled` 继续包装成 `empty_response`，并写成“上游没有返回计费信息”，
容易误判为上游或渠道故障。

## 根因证据

- 601 渠道样本在 NewAPI 中均记录 `stream ended ... reason=client_gone`、`end_error=context canceled`。
- 同一请求 ID 在 OVH tokens-pro 的 `request_logs` 中为 HTTP 200，但 input/output/cache usage 全为 0，
  与客户端断开时间一致。
- 失败样本与相邻成功样本发往 ChatGPT 的请求头均为同样的 13 个名称；入口 Nginx 的读写超时为 600 秒，
  样本持续 2-9 秒，不符合请求头过多或固定反代超时特征。

## 处理契约

- 对于没有任何可计费用量的 `client_gone` 或流式请求上下文取消：停止上游、保持预扣会话未结算，
  由外层统一退款；使用 `do_request_failed` + `context.Canceled` 作为内部取消错误，禁止重试和渠道自动禁用。
- 无可计费用量的客户端取消不再写“上游没有返回计费信息”，改写为“客户端已断开连接，本次请求未计费”，
  保留错误日志的审计行和 `stream_status`，但不记录性能失败样本。
- 如果客户端取消前已经收到有效 token usage，则按已消耗量正常结算，但仍不进入缓存命中率和性能成功样本。
- 正常结束但确实没有 token usage 时，继续使用 `empty_response`，允许外层按既有规则重试并在最终失败时退款。
- 工具附加费仍按既有 `hasBillableUsage` 规则结算；没有 token usage 的工具请求不进入 token 缓存命中率。

## 兼容性边界

- 不修改渠道绑定缓存、规则 + 分组 + Key 指纹、上游并发共享计数或 tokens-pro 协议。
- 亲和缓存统计继续只接受正数 token usage 且正常结束的流；统计 Redis 使用 `v2` 命名空间，旧口径不迁移。
- `client_gone` 仍由 `StreamScannerHandler` 依据入口请求上下文识别；有明确 `StreamStatus` 时以终态为准，
  只有终态缺失才使用请求上下文取消作为兜底。没有终态的流也不会进入亲和统计。

## 回归测试

- `TestPostTextConsumeQuotaTreatsClientGoneAsNonBillableCancellation`：客户端取消返回内部取消错误，预扣保持可退款，日志不再写上游缺 usage 文案。
- `TestRecordRelayFailureExcludesClientCancellation`：客户端取消不进入旧性能失败样本。
- `TestIsClientCancelledStreamHonorsExplicitTerminalStatus`：正常终态后发生的客户端断开不被倒推为取消。
- 亲和统计回归继续覆盖零用量失败、正常 miss、命中、终态未知流和 `client_gone`。

## 验证结果

- `go test ./service -count=1 -timeout 60s` 通过。
- `go test ./model -count=1 -timeout 60s` 通过。
- `go test ./controller ./relay ./middleware ./pkg/channel_metrics -count=1 -timeout 60s` 通过。
- `go test ./pkg/perf_metrics -count=1 -timeout 60s` 通过。
- `cd relaykit && GOWORK=off go build ./...` 通过。
- `git diff --check` 通过；当前环境未安装 Bun，本次没有前端代码变更。
