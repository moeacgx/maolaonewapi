# xAI Chat Completions Usage 缓存字段归一化

日期：2026-09-08

## 问题

xAI Chat Completions 的非流式响应可以在标准的
`usage.prompt_tokens_details.cached_tokens` 返回缓存命中 token，也可以在
顶层 `usage.prompt_cache_hit_tokens` 返回同类信息。服务计费和渠道指标统一读取
`PromptTokensDetails.CachedTokens`，因此仅返回顶层别名时缓存命中数会丢失。

流式响应的最后一个 usage chunk 原本只复制 prompt、completion 和 total 三个总量到
计费用的 `Usage`，没有复制缓存详情；上游 chunk 虽然仍转发给客户端，结算使用的
usage 却没有缓存命中数。

## 修改范围

- 在 xAI relay 中增加 usage 归一化：标准嵌套 `cached_tokens` 优先，其次兼容
  `InputTokensDetails.cached_tokens`，最后将 `prompt_cache_hit_tokens` 回填到
  `PromptTokensDetails.CachedTokens`，并同步保留字段存在性标记。
- xAI 非流式处理在返回 usage 前执行归一化。
- xAI 流式处理按字段存在性合并多个 usage chunk，再执行同一归一化，保留前一块的
  累计 token 与后一块的缓存详情；缓存 token 会归一化为非负值，并在完整 prompt 已知
  时限制在 prompt 总量以内。
- 不改变 relaykit 已有的 `Usage` JSON 字段；客户端仍收到上游原始 usage 字段。

## 兼容性与安全边界

- 同时存在标准嵌套字段和顶层别名时，以标准嵌套字段为准；显式的零值字段不会被
  别名覆盖。
- 没有任何缓存字段时保持原有 token 结果和响应形状，不凭空生成缓存值。
- 流式 usage 后续块缺少 prompt/total 时不会覆盖已有累计值；显式零值仍按上游字段存在性保留。
- 不涉及数据库、配置、迁移、API Key 或生产部署；不改变 xAI Responses 路径。
- 计费继续使用 `PromptTokensDetails.CachedTokens`，因此渠道测试、使用日志和缓存
  亲和统计可读取同一规范字段。

## 回归测试与验证

- 非流式覆盖标准 `cached_tokens`、仅 `prompt_cache_hit_tokens`、两者并存优先级和
  无缓存字段兼容行为。
- 流式覆盖最后 usage chunk 的标准字段、顶层别名回填和无缓存字段兼容行为。

```text
go test ./relay/channel/xai -run 'TestXAI(HandlerMapsCacheUsageForBilling|StreamHandlerPreservesFinalUsageCacheFields)' -count=1 -timeout=60s
go test ./relay/channel/xai -count=1 -timeout=60s
cd relaykit && GOWORK=off go build ./...
gofmt -w relay/channel/xai/text.go relay/channel/xai/usage_test.go
git diff --check
```

生产实例重启、渠道配置修改和 CloudSSH 操作不属于本次代码变更。
