# Codex Responses Lite 并行工具调用兼容

## 背景

Codex CLI 在部分请求中会透传 `X-OpenAI-Internal-Codex-Responses-Lite: true`。
该上游内部模式要求请求体里的 `parallel_tool_calls` 必须明确为 `false`。如果客户端发送
`true`，上游会返回：

```text
X-OpenAI-Internal-Codex-Responses-Lite requires parallel_tool_calls to be false.
```

该问题只在 Codex 渠道、Responses 接口、Responses Lite 请求头与并行工具调用同时出现时触发，
因此属于低频兼容性问题。

## 变更范围

- `relay/channel/codex/adaptor.go`
  - Codex Responses 转换时检测 `X-OpenAI-Internal-Codex-Responses-Lite: true`。
  - 命中后将 `parallel_tool_calls` 明确写为 `false`，不删除字段。
- `relay/responses_handler.go`
  - 请求体透传开启时，同样在 Codex Responses Lite 请求体里写入 `parallel_tool_calls: false`。
- `relay/channel/codex/adaptor_test.go`
  - 覆盖 Responses Lite 请求会强制关闭并行工具调用。
  - 覆盖普通 Codex Responses 请求仍保留客户端原始 `parallel_tool_calls`。
- `relay/responses_handler_test.go`
  - 覆盖透传请求体归一化会补齐或覆盖 `parallel_tool_calls: false`。

## 兼容性说明

该修复只影响带 Responses Lite 请求头的 Codex 渠道请求。普通 Codex Responses 请求不被改写。
请求体透传模式下也会应用该兼容修正，避免绕过适配器转换后继续把 `true` 原样发给上游。

这里选择写入 `false`，而不是删除字段，是为了避免上游在字段缺失时按默认并行工具调用处理。

## 验证计划

```bash
go test ./relay
go test ./relay/channel/codex
git diff --check
```
