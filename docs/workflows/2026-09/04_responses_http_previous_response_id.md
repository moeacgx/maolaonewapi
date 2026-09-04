# HTTP Responses 清理 previous_response_id

## 问题

部分客户端会把 `previous_response_id` 带入下一轮 Responses 请求。该字段属于有状态
续传协议，普通 HTTP/SSE Responses 上游不支持，只在 Responses WebSocket v2 中有效。
同一类客户端历史中还可能包含缺少 `call_id` 的 `function_call_output`，该项同样会被
HTTP 上游拒绝。因此上游会返回：

```text
status_code=400, previous_response_id is only supported on Responses WebSocket v2
```

## 处理契约

- `/v1/responses` 与 `/v1/responses/compact` 的 HTTP/SSE 上游请求始终移除
  `previous_response_id`；
- `input` 中缺少 `call_id` 的 `*_call_output` 项降级为普通用户文本，避免向 HTTP
  上游发送非法工具输出；
- 参数覆盖在清理后重新注入的 `previous_response_id` 也会被移除；
- 清理只作用于网关生成的上游请求副本，不修改客户端请求对象；
- 既有 `/v1/realtime` WebSocket 路径不受影响；
- 客户端需要续接上下文时，应在 HTTP 请求的 `input` 中携带必要历史，不能依赖该字段。

## 兼容性与验证

- 不涉及数据库、配置或迁移；
- 覆盖请求副本清理、透传 body 清理、缺失 `call_id` 的输入归一化与原始请求保持不变；
- 执行 `go test ./relay -run 'Test(NormalizeHTTPResponsesInput|SanitizeHTTPResponsesBody|ResponsesHelperDropsPreviousResponseID|NewSanitizedHTTPResponsesBody)' -count=1 -timeout 60s`；
- 执行 `gofmt`、`git diff --check`。
