# TokensPro overview 渠道并发对齐

渠道可选择消费 TokensPro 只读 overview，把该渠道并发上限写成当前 Key 分到的 `concurrency.allowed`。本能力不新增 TokensPro 协议类型，也不改 TokensPro。

## 合同

- 请求：`GET {base}/api/v1/downstream/overview?platform=openai`
- 鉴权：渠道现有主 Key，`Authorization: Bearer <key>`
- 成功：HTTP 200，使用 `concurrency.allowed`（整数，含 0）
- 不要回写 `concurrency.available`
- `allowed = 0` 表示当前没有可广告并发，必须按 0 处理
- 非 200（含 401/403）不改并发、不改渠道状态
- Key 需要 TokensPro `downstream:read`（后台「号池内部信息」）
- 忽略未知字段

渠道 Base URL 若以 `/v1` 结尾，对齐请求会去掉该后缀后再拼接 overview 路径，兼容把 OpenAI 兼容前缀写进 Base URL 的渠道。

## 开关与周期

渠道 `settings` 字段：

| 字段 | 含义 |
| --- | --- |
| `tokenspro_overview_sync_enabled` | 显式 `true` 才同步；缺省或 `false` 跳过 |
| `tokenspro_overview_sync_interval_seconds` | 成功后的最小间隔，缺省 60，最小值 60 |
| `tokenspro_overview_last_success_time` | 上次成功 Unix 秒 |
| `tokenspro_overview_last_allowed` | 上次写入的 `allowed` |
| `tokenspro_overview_last_error` | 上次失败摘要，不含密钥 |
| `tokenspro_overview_disabled_by_zero` | 是否因 `allowed=0` 自动禁用 |

调度任务 `tokenspro_overview` 每分钟在持有系统任务租约的实例上跑一轮。从未成功的渠道下一分钟重试；成功后按渠道间隔跳过。可用环境变量 `TOKENSPRO_OVERVIEW_SYNC_TASK_ENABLED=false` 关闭任务。

## NewAPI 并发 0 语义

`concurrency_limit = 0` 表示不限制。因此：

- 成功写入 `allowed` 到 `concurrency_limit`
- `allowed = 0` 时先自动禁用，禁用成功后才把并发写成 0；禁用失败则保持原并发和启用状态
- 份额恢复且 `allowed > 0` 时，只恢复带 `tokenspro_overview_disabled_by_zero` 的自动禁用渠道
- 人工禁用渠道不会被自动打开
- 监控自动恢复会跳过带该标记的渠道，以及同步开启且上次 `allowed` 为 0 的渠道，避免把无并发渠道重新打开成无限打

## 安全边界

- 使用渠道已有 Key，不另开密钥槽位
- 请求走渠道代理设置，Base URL 按运营配置的上游处理，不走任意用户 URL 的 SSRF 客户端
- 失败日志与 `last_error` 截断，不记录密钥
- 不调用 pool-health / 服务监控接口

## 前端

Classic 与 Default 渠道编辑页都提供开关、间隔和上次同步信息。保存时保留后端写入的上次成功/错误字段。

## 验证

- `go test ./service -run TokensProOverview -count=1`
- `cd relaykit && GOWORK=off go build ./...`
- `cd web && bun test src/features/channels/lib/__tests__/retained-channel-contracts.test.ts`
