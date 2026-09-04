# Responses 工具输出 call_id 兼容

## 问题

HTTP `/v1/responses` 请求在多渠道重试时，客户端历史可能包含缺少 `call_id` 的
`function_call_output`。Codex 上游会将其与 HTTP 续传限制合并返回：

```text
function_call_output requires call_id on HTTP requests; continuation via previous_response_id is only supported on Responses WebSocket v2
```

`previous_response_id` 的清理已由上一项工作完成；本次剩余失败点是非法工具输出仍可能
从原生 Responses 输入或透传 body 直达上游。

## 处理契约

- HTTP `/v1/responses` 与 `/v1/responses/compact` 继续移除顶层
  `previous_response_id`；
- `input` 中 `*_call_output` 必须已有非空 `call_id` 才保留；缺少该字段时，即使存在
  输出项 `id` 也不冒充 `call_id`，而是降级为普通用户文本；
- 普通转换、参数覆盖后请求体和透传 body 均执行同一归一化；
- `/v1/realtime` WebSocket 路径不受影响；
- 原始客户端请求不会被修改。

## 验证

- `go test ./relay -run 'Test(NormalizeHTTPResponsesInput|SanitizeHTTPResponsesBody|ResponsesHelperDropsPreviousResponseID|NewSanitizedHTTPResponsesBody)' -count=1 -timeout 60s`；
- `go test ./relay -count=1 -timeout 60s`；
- `go vet ./relay`；
- `gofmt` 与 `git diff --check`。
