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
- Gemini 返回 HTTP 200 但无 candidates（例如上游安全拦截）时，统一交给最终错误响应边界处理，避免绕过替换规则。
- Gemini 兼容接口的流式安全拦截同样转换为上游错误并交给最终错误响应边界；原生 Gemini 协议仍保留其原始响应格式。
- Responses 流已经提交后产生的上游错误事件也替换事件中的客户端文案；HTTP 状态码仍保持已提交的状态。

## 匹配模式语义

- `contains`：候选错误文案中包含任意匹配值即命中；命中后将返回给客户端的整条文案替换为 `replace`。
- `exact`：按大小写不敏感的普通文本匹配，命中后替换消息中所有匹配到的文本；不会替换未命中的前后内容。
- `regex`：按 Go RE2 正则匹配，命中后替换所有匹配到的文本，并支持 `$1` 等捕获组引用。

规则只在最终客户端错误视图执行，且按规则顺序取首条命中；同一规则的多个匹配值为 OR，填写
`status_code` 时与文案条件为 AND。`exact` 和 `regex` 的局部替换作用于最终选中的客户端消息候选，
而 `contains` 仍使用整条消息替换。

## 验证

- `go test ./common ./controller ./middleware ./service -run 'TestErrorMessageReplacement|TestWriteRelayErrorResponse|TestRealtimeClientErrorView|TestClientErrorReplacementIgnoresInternalQuotaErrors|TestPerformanceClientErrorReplacementIgnoresInternalError|TestSensitiveFilterOpenAIErrorResponseIgnoresClientStatusReplacement|TestWritePromptAuditRelayErrorFinalClientView|TestWritePromptAuditRealtimeDecisionFinalClientView|TestPromptAuditRealtimeCyberSessionBlockStopsBeforeUpgrade' -count=1 -timeout 180s` 通过。
- 追加覆盖 Gemini 空 candidates 直写绕过和已提交 Responses 流错误事件的替换路径。

## 结论

配置 `balance` / `额度` 现在只会替换上游响应错误；本站本地预扣费或余额不足不再被改写为通用异常文案。
