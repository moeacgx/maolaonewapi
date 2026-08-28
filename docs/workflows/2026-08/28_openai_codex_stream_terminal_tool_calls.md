# OpenAI/Codex 流式末块工具调用保留

## 目标

修复 OpenAI/Codex 兼容流式请求中，客户端未请求 usage 时末尾携带 usage 的工具调用块可能只返回 `[DONE]` 的问题。

## 根因

`v1.0.0-rc.10.1.10.244` 的 staged upstream integration 将 OpenAI SSE 改为暂存最后一个块后统一处理。`handleLastResponse` 在末块含有效 usage 且 `ShouldIncludeUsage=false` 时，只将文本和推理内容视为可发送输出，遗漏了 `tool_calls`。

因此，工具调用恰好位于最终 usage 块时会被过滤；工具调用位于前一块时则正常转发，表现为偶发空回复。`v1.0.0-rc.10.1.10.243` 的等价过滤逻辑已将 `tool_calls` 视为有效输出。

## 修改范围

- 在 OpenAI 末块发送判定中，将非空 `tool_calls` 恢复为有效输出。
- 增加真实 SSE 转发回归测试：未请求 usage 时，包含 `tool_calls + usage` 的最后块仍必须向客户端发送。
- 不修改 relaykit 转换、渠道并发、认证或前端。

## 兼容性

- 普通文本和推理内容的既有发送判定不变。
- `ShouldIncludeUsage=true` 的 usage 终止块行为不变；`false` 时 choices 为空的最终缓存 usage-only 末块仍按原规则过滤。
- 该判定只控制 OpenAI 最终块转发；Claude 和 Gemini 仍走各自的转换收尾路径，不受影响。
- 已核对旧版删除的前导空块暂存和流错误识别：它们属于重试、已提交错误和敏感响应边界，未参与本次末尾 valid usage 块的发送判定；恢复它们会扩大行为面，故不纳入本补丁。

## 验证计划

- 运行新增的定向 Go 回归测试，断言工具调用、`finish_reason`、`[DONE]` 和 usage 解析均保留。
- 运行 OpenAI 渠道包定向测试，确认相邻流式 usage 路径保持通过。

## 验证结果

- `go test ./relay/channel/openai -run 'TestOaiStreamHandler(KeepsFinalToolCallChunkWithUsageWhenNotRequested|DoesNotReinjectAudioUsageFromSecondLastChunk)$' -count=1 -timeout=60s` 通过。
- `go test ./relay/channel/openai -run 'TestOaiStreamHandler' -count=1 -timeout=60s` 通过。
