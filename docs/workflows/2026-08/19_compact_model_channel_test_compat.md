# Compact 模型重定向与渠道测试兼容

日期：2026-08-19

## 问题

通过模型重定向把 `gpt-5.4-openai-compact`、`gpt-5.5-openai-compact` 等别名映射到普通模型后，渠道模型测试报 `invalid response request type`。

## 根因

渠道测试先按 Compact 端点创建 `OpenAIResponsesCompactionRequest`。模型重定向成功后，后端将 RelayMode 和上游路径切换为普通 Responses，但测试请求对象仍保持 Compact 类型，导致普通 Responses 转换器的类型断言失败。

## 修改范围

- 仅在渠道测试中，当 Compact 别名重定向后 RelayMode 变为普通 Responses 时，将请求转换为 `OpenAIResponsesRequest`。
- 保留输入、指令、历史响应 ID、工具、推理和缓存字段。
- 真实 Compact 请求和远程压缩链路不变。

## 兼容性与安全边界

- 不改变模型重定向配置含义，不把普通模型发送到 Compact 端点。
- 不改变真实 `/v1/responses/compact` 请求。
- 不涉及数据库迁移、计费口径或上游请求认证。

## 验证

- `go test ./controller -run 'TestNormalizeChannelTestRequest' -count=1 -timeout 60s`
- `go test ./relay/helper -run 'TestModelMappedHelperResponsesCompact' -count=1 -timeout 60s`
- `git diff --check`
