# 渠道亲和性缓存命中率排除失败尝试

## 问题

渠道亲和性详情以 `Hit / Total` 展示同一规则、入口分组和 Key 指纹的上游缓存命中率。
空用量、客户端取消和异常流虽然没有可用的上游 usage，仍会先调用统计 observer：`Total`
增加一次，而 `cached_tokens` 为零导致 `Hit` 不增加。这会把失败尝试伪装成缓存 miss。

生产只读聚合确认，示例渠道的一个活跃 Key 在截图时有 `12` 次表面 miss，其中 `9` 次是
`client_gone` 且输入、输出均为零，只有 `3` 次是存在实际用量但没有缓存命中。零用量记录在
`.282` 之前已经存在，但当时写成消费日志；`.282` 将其改为错误日志后，错误才在用户侧显式出现。

## 根因

`PostTextConsumeQuota` 在计算 `empty_response` 之前调用
`ObserveChannelAffinityUsageCacheByRelayFormat`。底层 observer 对每次调用无条件增加 `Total`，
只在 `cached_tokens` 或 `prompt_cache_hit_tokens` 大于零时增加 `Hit`，因此无法自行区分成功用量和
失败尝试。

Responses 的首字时间表示收到首个非 `[DONE]` SSE 帧，可能只是生命周期事件。客户端在最终 usage
事件前断开时，日志仍可具有正常首字时间和请求耗时，但不能据此推断上游已经返回可计费用量。

## 修复契约

- 正常结束但零用量的 `empty_response` 继续写错误日志并保留既有重试、退款和审计行为，但不进入亲和缓存统计。
- `client_gone`、非正常流终态、终态未知和带软错误的流不进入亲和缓存统计，即使已经收到部分 usage。
- 只有存在正数 token usage 且请求业务结果有效的样本进入统计；成功但真实缓存为零的请求仍是合法 miss。
- 只有工具附加费、没有 token usage 的请求继续按既有规则计费和记录消费，但不进入 token 缓存命中率。
- 底层 observer 的直接聚合能力保持不变；调用方负责只提交符合业务口径的成功样本。

## 状态迁移与兼容性

亲和 usage 统计 Redis 命名空间从 `new-api:channel_affinity_usage_cache_stats:v1` 升级为 `v2`，避免旧的
错误分母继续污染新口径，并隔离滚动发布期间的新旧容器。旧 `v1` 项目不迁移、不删除，按原 TTL 自然
过期。渠道亲和绑定缓存、规则配置、Key 指纹、日志字段和详情 API 不变。

当前 Redis 混合缓存仍采用跨实例非原子的读改写累计；这项既有限制不在本次修复中扩大处理。

## 回归测试

- `PostTextConsumeQuota` 的零用量失败返回 `empty_response` 后，统计保持 `Total=0`、`Hit=0`。
- `client_gone` 流以及没有 `StreamStatus` 终态的流，即使携带部分 token 和 cached tokens，也不进入统计。
- 正常成功且携带 cached tokens 的请求继续累计一次命中。
- 保留底层 observer 对 OpenAI、Claude、显式零值和缺失字段的既有测试。

聚焦验证命令：

```text
go test ./service -run '^(TestPostTextConsumeQuotaKeepsBillingOpenForZeroUsageRetry|TestPostTextConsumeQuotaRecordsOnlyEligibleAffinityUsage|TestObserveChannelAffinityUsageCache.*)$' -count=1 -timeout 60s
```

## 验证结果

- 聚焦回归测试通过，并实际确认修复前零用量失败、`client_gone` 部分 usage 和终态未知流会错误增加
  `Total`，修复后这些样本保持为零。
- `go test ./service -count=1 -timeout 60s`、`go test ./model -count=1 -timeout 60s` 通过。
- `go test ./controller ./relay ./middleware ./pkg/channel_metrics -count=1 -timeout 60s` 通过。
- `cd relaykit && GOWORK=off go build ./...` 通过，`relaykit` 独立构建边界未受影响。
- 本次新增、修改的文档链接目标检查和 `git diff --check` 通过。
- 当前环境未安装 Bun，无法执行仓库 Markdown 或前端格式工具；本次没有前端代码变更。
