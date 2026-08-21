# 客户端错误替换仅作用于上游错误

日期: 2026-08-21

## 问题

客户端错误消息替换规则原先在最终响应边界按状态码和文案匹配所有错误。管理员配置 `balance`、`额度` 等关键词用于隐藏上游余额不足时，本地预扣费不足也可能命中同一规则，例如：

- `用户额度不足, 剩余额度: ...`
- `预扣费额度失败, 用户剩余额度: ..., 需要预扣费额度: ...`

这些错误来自本站本地计费校验，不是上游返回；被替换后会把真实的用户额度不足展示成通用“异常波动”类 403，误导排障。

## 修复

- 新增 `types.IsUpstreamReturnedError`，只把上游响应错误类型和上游响应错误码族视为可替换来源。
- `clientOpenAIError`、`clientClaudeError` 和普通 relay 错误响应仅在 `IsUpstreamReturnedError` 为真时应用客户端错误消息替换。
- 性能保护、安全审计、敏感词和 cyber_policy 会话本地拦截不再应用该替换规则。
- 预扣费不足仍返回原始本地额度文案和原状态码，便于用户和管理员判断是真余额不足还是上游余额不足。

## 验证

- `go test ./common ./controller ./middleware ./service -run 'TestErrorMessageReplacement|TestWriteRelayErrorResponse|TestRealtimeClientErrorView|TestClientErrorReplacementIgnoresInternalQuotaErrors|TestPerformanceClientErrorReplacementIgnoresInternalError|TestSensitiveFilterOpenAIErrorResponseIgnoresClientStatusReplacement|TestWritePromptAuditRelayErrorFinalClientView|TestWritePromptAuditRealtimeDecisionFinalClientView|TestPromptAuditRealtimeCyberSessionBlockStopsBeforeUpgrade' -count=1 -timeout 180s` 通过。

## 结论

配置 `balance` / `额度` 现在只会替换上游响应错误；本站本地预扣费或余额不足不再被改写为通用异常文案。
