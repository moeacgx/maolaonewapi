# 空用量日志与渠道并发槽位泄漏修复

## 问题现象

- 日志中的输入和输出均为 `0`，但类型显示为绿色“消费”或被性能看板计为成功。
- 渠道 `267` 的并发上限为 `18`，用户流量很少时仍出现
  `channel concurrency limit reached`。

## 根因

### 空用量

`PostTextConsumeQuota` 原先在 `usage == nil` 时把预估输入 token 写回结算摘要，随后无条件使用
`LogTypeConsume` 写日志并调用 `RecordRelaySample(..., true, ...)`。这会把“上游未返回计费信息”
表现成一次成功消费。`v243` 虽然已有“总 token 为 0 时不扣费并记错误日志”的分支，但该函数没有返回错误，
各 Relay 调用方仍然直接返回成功，因此没有触发跨渠道重试。

当前规则是：普通请求的 `input=0 && output=0` 必须返回 `empty_response`（HTTP 502），让外层选择器按既有
重试策略切换渠道；客户端已断开、响应已写出、没有剩余重试次数或指定渠道时，仍由外层门禁禁止重试。
正常的零 token 业务（例如有明确工具附加费用的 alpha search）仍按工具费用计费，不受本修复影响。

随后发现一个重试计费生命周期问题：如果第一次 0/0 尝试调用 `SettleBilling(..., 0)`，
`BillingSession` 会把本请求标记为已结算；外层切换渠道后复用同一个 session，成功重试可能不再扣费。
因此 0/0 失败不能在当前尝试调用结算，必须保留预扣会话，交由外层在最终成功时结算、在重试耗尽时退款。

### 渠道并发

渠道并发计数是每个 Go 进程的内存状态。自动渠道测试直接调用
`SetupContextForSelectedChannel`，成功后没有释放槽位；同一渠道每次测试都会留下一个占用计数。
Remix 任务入口也绕过 `Distribute`，直接设置锁定渠道，同样需要由入口负责释放。

因此 `267` 被周期性测试逐步填满后，即使真实请求不多，选择器仍会认为该实例上的渠道已满。
三个生产容器各自维护计数，当前实现也不是三实例共享的全局上限。

## 修复契约

- 上游 `usage` 缺失时，不再用预估输入伪造实际用量；该尝试记录实际额度 `0`，但不关闭预扣会话。
  外层重试成功后按最终用量结算，重试耗尽后统一退款。
- 没有可计费用量的文本、音频和实时结算日志写入 `LogTypeError`，用户侧显示为错误，不显示为消费成功。
- 普通 HTTP 文本、音频和实时结算的 `0/0` 会返回 `empty_response`，由 `Relay` 切换其他渠道；
  失败尝试不执行 `SettleBilling`，固定价格不会绕过该规则。
- 实时 WebSocket 同样返回 `empty_response`；客户端已断开时由外层 Context 门禁禁止重试，未断开时按普通可重试错误处理。
- 流式请求只有存在可计费用量且以正常结束（`done`、`eof` 或兼容的 `handler_stop`）结束时，才计入性能成功率。
- alpha search 等明确产生工具附加费用的零 token请求仍写入消费日志并计费。
- 自动渠道测试成功设置渠道后，在函数返回时按 Context 所有权释放槽位。
- Remix 锁定渠道入口成功设置后，在该请求生命周期结束时按 Context 所有权释放槽位。
- `Distribute` 统一入口原有的请求级释放逻辑保持不变；本修复不清理、不重置已有进程计数。
- `middleware.ReleaseChannelConcurrencyForContext` 是 Distribute 外部自行选渠入口的唯一释放方式；禁止按渠道 ID 直接释放，以免 setup 失败重试时误扣其他请求的槽位。

## 兼容性与运维边界

- 日志类型数值保持既有约定：消费为 `2`，错误为 `5`；旧日志不会被回写或迁移。
- 并发限制仍是单实例限制。要实现三容器共享的严格上限，需要另行设计 Redis 信号量、租约过期和实例故障回收，不能通过本修复推断为全局限制。
- 发布后旧进程中的幽灵计数不会由代码主动清零；滚动重启对应实例会重新初始化内存计数。生产操作必须逐实例进行，并在每个实例健康后再继续。
- 自动测试仍可能短暂占用一个真实渠道槽位，但测试结束后应立即释放；测试请求本身仍会访问上游。

## 回归测试

- `controller`：Remix 入口请求结束后槽位恢复可用。
- `service`：缺失 usage 不生成预估 token；无计费用量不进入成功样本；工具附加费用仍属于消费。
- `service`：普通 HTTP 和实时 WebSocket 的 `0/0` 返回 `empty_response`，固定价格 `0/0` 不收费；工具附加费用零 token 请求不返回空用量错误。
- `service`：零用量失败不会关闭 BillingSession，后续重试仍可按最终用量结算。
- `controller/relay`：`empty_response` 使用 HTTP 502，沿用既有可重试状态码和客户端断开/响应已写出门禁。
- `model`：消费日志构造器保留显式错误类型。

聚焦验证命令：

```text
go test ./controller ./service ./model -count=1 -timeout 60s
```

发布前还需执行相关后端测试、`gofmt`、`git diff --check`，并在生产逐实例观察健康状态、
并发错误数量和 `testing channel #267` 日志是否在测试结束后继续增长。

`282` 本次本地验证结果：`controller`、`service`、`model`、`middleware`、`relay` 及相关渠道包测试通过；
`cd relaykit && GOWORK=off go build ./...` 通过；`git diff --check` 通过。根目录 `go test ./...`
仅因当前工作区没有 `web/classic/dist` 的嵌入目录而在根包加载阶段失败，其他 Go 包均通过。

后续 P0 修复在 `283` 版本补充了“0/0 失败不关闭 BillingSession，重试成功后仍按最终用量结算”的入口级回归测试，
并重新通过 `go test ./service ./controller ./relay ./middleware ./model -count=1 -timeout=60s`、
`cd relaykit && GOWORK=off go build ./...` 和 `git diff --check`。
