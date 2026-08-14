# 渠道映射 compact 别名端点修正

## 问题与目标

渠道模型重定向中允许把对外别名映射到真实上游模型，例如 `gpt-5.5-openai-compact` -> `gpt-5.5`。旧的渠道测试自动端点判定先读取对外别名的 `-openai-compact` 后缀，再执行模型重定向，导致自动测试走 `/v1/responses/compact`，而请求体模型最终变成 `gpt-5.5`。正式请求中，`ModelMappedHelper` 在 compact 模式下也会先剥离后缀再查映射；显式配置的 `gpt-5.5-openai-compact` 映射不会被视为命中，最终仍用 compact 路由请求上游。对不支持 compact 路由的 Codex 上游，这会返回 404 `Not Found`。

目标：自动测试和正式 Responses 请求都先尊重显式 compact 别名映射。若 `gpt-5.5-openai-compact` 被明确映射到非 compact 目标 `gpt-5.5`，则按普通 Responses 请求发送到上游；未显式映射的 compact 变体继续保留原 `/v1/responses/compact` 语义。

## 实现契约

- `normalizeChannelTestEndpoint` 保留显式 `endpoint_type` 的最高优先级。
- 自动端点判定新增 `resolveChannelTestEndpointModelName`：只读取当前渠道 `model_mapping`，按映射链解析对外测试模型名，用解析后的模型名判断 compact 后缀。
- `ModelMappedHelper` 在 Responses compact 模式下先尝试精确匹配原始 compact 别名；没有显式 compact 别名映射时，才保留旧行为：去掉 `-openai-compact` 后缀再查基础模型映射。
- 显式 compact 别名映射到非 compact 目标时，正式请求会把 `RelayMode` 和 `RequestURLPath` 降级为普通 Responses，并把请求体模型改为映射目标。
- 映射 JSON 为空、`{}` 或解析失败时，自动端点判定回退到原始测试模型；后续正式 `ModelMappedHelper` 仍负责返回映射错误。
- 未配置显式映射的 `*-openai-compact` 模型继续使用 `/v1/responses/compact`。
- Codex 渠道的普通映射别名继续默认使用 `/v1/responses`。
- 图片模型判定同时保留原始模型名和映射目标模型名，避免破坏已有图片测试自动端点。

## 兼容性

不新增数据库字段、配置项或 API 参数。渠道测试的自动端点选择改为按映射目标判定；正式请求只在“显式 compact 别名映射到非 compact 目标”时降级上游路由。未配置显式映射的 compact 模型、基础模型映射后的 compact 变体和显式选择的测试端点保持旧行为。

## 验证

- `go test ./common ./controller -run "TestGetEndpointTypesByChannelTypeCodexCompactUsesResponsesCompact|TestNormalizeChannelTestEndpoint"`
- `go test ./relay/channel/codex -run TestAdaptorInheritsNewAPIResponsesCompactSupport`
- `go test ./relay/helper -run TestModelMappedHelper`
- `go test ./relay -run TestResponsesHelperMappedCompactAliasUsesResponsesUpstreamRoute`
