# HTTP Responses 清理 previous_response_id

## 问题

部分客户端会把 `previous_response_id` 带入下一轮 Responses 请求。该字段属于有状态
续传协议，普通 HTTP/SSE Responses 上游不支持，只在 Responses WebSocket v2 中有效，
因此上游会返回：

```text
status_code=400, previous_response_id is only supported on Responses WebSocket v2
```

## 处理契约

- `/v1/responses` 与 `/v1/responses/compact` 的 HTTP/SSE 上游请求始终移除
  `previous_response_id`；
- 清理只作用于网关生成的上游请求副本，不修改客户端请求对象；
- 既有 `/v1/realtime` WebSocket 路径不受影响；
- 客户端需要续接上下文时，应在 HTTP 请求的 `input` 中携带必要历史，不能依赖该字段。

## 兼容性与验证

- 不涉及数据库、配置或迁移；
- 覆盖请求副本清理与原始请求保持不变；
- 执行 `go test ./relay -run TestResponsesHelperDropsPreviousResponseIDForHTTPRelay -count=1 -timeout 60s`；
- 执行 `gofmt`、`git diff --check`。
