# 客户端取消流式请求的 500 展示修复

日期：2026-08-09

## 问题

流式请求在下游请求上下文已经结束后,写入或刷新 SSE 数据会返回:

```text
request context done: context canceled
```

部分适配路径会用 `types.NewOpenAIError` 包装该错误。旧实现把包装后的错误转换成新的字符串错误,丢失了 `context.Canceled` cause,导致外层无法识别这是请求上下文结束,最终可能对外或日志展示为:

```text
status_code=500, request context done: context canceled
```

该文案容易被误解为上游 500。实际含义是服务端在写回客户端时发现请求已经被取消。

## 语义

`context canceled` 只说明请求上下文已经取消,常见来源包括:

- 用户或调用方主动停止生成、关闭页面、关闭 HTTP 连接;
- SDK、浏览器、反向代理或上游网关的客户端侧超时;
- 网络断开导致下游连接消失。

它不能单独证明是人手动点了停止,也不是上游模型服务返回的 500。

## 修复

`types.NewOpenAIError` 现在保留原始 `err` 作为 `cause`,使 `errors.Is(relayErr, context.Canceled)` 和 `errors.Is(relayErr, context.DeadlineExceeded)` 在包装后仍然有效。Relay 终态写错和错误日志已有请求上下文结束判断,因此能继续跳过这类客户端取消,不再把它当成普通 500 上游失败写回。

## 验证

- `go test ./types -run TestNewOpenAIErrorPreservesCause -count=1`
- `go test ./controller -run 'TestWriteRelayErrorResponseSkipsCommittedStreamAndCancellation|TestShouldRetryWithReasonStopsWhenRequestContextEnds' -count=1`

## 回滚

回滚 `types/error.go` 与对应测试即可恢复旧行为;回滚后,经 `NewOpenAIError` 包装的流式写出取消会再次丢失 cause,外层可能重新显示 `status_code=500, request context done: context canceled`。
