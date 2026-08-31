# Responses 缺失 usage 的本地计费、空流重试与错误日志收敛

## 问题

部分 Responses 上游返回了可见输出，但没有 `usage` 或只返回 `0/0`。
旧路径会把这类响应当作 `empty_response`（HTTP 502），导致已经产生输出的
请求不计费；同一结算失败还可能同时写入计费错误日志和外层渠道错误日志，
形成中英文混杂或重复的两条记录。

## 根因

- 非流 Responses 只读取上游 `usage`；usage 整体缺失或只缺输入/输出一侧时，
  都没有根据实际 `output` 补齐缺失字段。
- 流式 Responses 主要收集 `response.output_text.delta`；如果文本只出现在
  `response.completed`/`response.incomplete` 的完整 output，或者客户端只收到
  `output_item.done`、content/reasoning part、reasoning/function/custom tool，终态
  仍可能被当作零用量。
- 合法 refusal、`response.output_text.done` 等完整 done 事件没有进入计数；空
  delta/done 事件却会提前提交 Writer，仍可造成“客户端已收到空 SSE、外层不能
  换渠”。仅含 `total_tokens` 而缺少输入/输出拆分的 usage 也有相同问题。
- `response.failed`/`response.error`/`error` 没有按上游 API 错误解析，只含错误
  的 SSE 会被当作空生命周期流丢弃，最终错误被改写为通用 `empty_response` 502。
- 原生 Responses 收到 `response.created`/`response.in_progress` 后立即写给
  客户端；即使最终是空响应，`Writer.Written()` 也会阻止外层按 502 换渠。
- Codex 亲和规则的 `SkipRetryOnFailure` 在当前合并基线中错误地覆盖了已明确
  配置为可重试的 502，与 v243 的重试优先级不一致。
- `PostTextConsumeQuota` 已写入 `LogTypeError` 计费审计后，返回的
  `empty_response` 又被外层 `processChannelError` 记录一次。

## 现场证据

2026-08-31 对生产集群执行了只读核查，没有部署、重启或修改配置/数据：

- 截图中的两条 zola 错误行具有相同 `request_id`，分别是详细计费错误和外层
  通用错误，不是两次请求，也不是一次重试后的两次尝试。
- 同秒的 `2352532` 消费行具有另一个 `request_id`，是独立成功请求，不能拿来
  证明 zola 请求成功或被重试。
- zola 请求只有一次渠道尝试，`use_channel=[601]`、`retry_planned=false`；现场
  `RetryTimes=2` 且 502 在自动重试状态码范围内，因此无重试预算不是根因。
- 该请求的上游状态和客户端状态均为 200，流终态为正常 `eof`，失败事件标记
  `partial_response=true`；这只能证明网关已写出 SSE 数据，不能证明其中包含普通
  文本。旧实现没有归档 reasoning/tool-call 内容，无法从历史记录反推客户看到的
  精确语义输出。
- 最终 usage、日志 quota 均为 0，预扣订阅额度随后执行退款，因此该请求当时
  没有产生本地计费。

## 处理契约

- 非流 Responses 在上游 usage 整体缺失或只缺一侧且存在实际输出时，基于输出
  文本、推理摘要、function/custom tool 名称与协议对应参数构造本地计数文本；
  保留上游已提供的一侧，只在本地补齐缺失侧，继续向客户端返回并正常结算。
- 流式 Responses 收集 output text、`reasoning_summary_text` 与
  `reasoning_summary` 两套 summary 事件、reasoning text、function
  arguments、custom tool input、refusal 的 delta/done 与 content/reasoning part；
  content/reasoning part 的 added/done、meaningful `output_item.added` 也进入聚合。
  `output_item.done` 按 `output_index` 覆盖同一项的 done/delta，若终态带完整
  output，则再以终态 output 覆盖事件累积，避免重复计数。缺失索引的兼容 item
  只有在 ID 或内容能证明属于最近输出时才继承索引；不同 item 会独立累加。
- 非流 output、流式 `output_item.done` 和终态完整 output 统一识别 web/file/function/
  custom/tool_use 调用；流式终态按调用 ID 或协议内容去重，既补齐 terminal-only
  工具附加费，也避免 item done 与 terminal output 对同一次调用重复收费。
- `response.incomplete`/`response.cancelled`/`response.canceled` 携带的有效 usage
  或部分 output 会进入同一计费聚合；`response.failed`/`response.error`/`error`
  则恢复为真实上游错误，响应未提交时保留外层重试能力，不再伪装成空用量 502。
- `response.created`、`response.in_progress`、空 output item/part 和真正空的终态
  最多暂存 16 个事件或 1 MiB。首次出现实际输出或有效 usage 时按原顺序下发已
  保留的前导/结构帧和当前事件；边界内整条流只有生命周期事件时丢弃暂存，让
  外层在响应未提交状态下按 502 换渠。第 17 个事件或单帧/累计超过 1 MiB 时立即
  按普通流提交，停止透明换渠，避免无界缓冲或静默丢弃协议事件。
- 空文本 delta/done 与仅有 `total_tokens`、没有输入/输出拆分及实际 output 的
  终态同样按空流暂存，不把无法可靠拆分计价的总量当作有效 usage。
- 没有文本、工具附加费或其他可计费用量的响应仍返回
  `empty_response`（HTTP 502），不向客户端写入伪造响应，保留预扣会话供外层
  重试或最终退款。
- 已由 `PostTextConsumeQuota`/音频结算写入详细错误审计的空用量错误抑制外层
  重复错误日志；预检阶段的空响应仍由外层记录一条错误日志。
- 空用量错误按请求语言返回简体中文、繁体中文或英文，不再固定写英文。
- HTTP 502 本身仍属于默认可重试状态；恢复 v243 的优先级后，明确配置为可重试
  的状态码可覆盖亲和规则 `SkipRetryOnFailure`。流式响应一旦已写出实际输出或
  ping，`Writer.Written()` 仍禁止换渠，避免客户端收到两份响应；指定渠道、
  显式 `skipRetry`、客户端断开和 `RetryTimes=0` 等门禁不变。
- 无效或缺失的 HTTP 状态码不属于“明确配置状态码”，因此仍受亲和失败粘滞
  限制；没有亲和限制时继续保留既有的传输错误重试行为。
- 未提交流进入下一次尝试或最终错误响应前，清理 SSE 与 Codex 响应头，避免前一
  渠道的头信息污染下一渠道或 JSON 错误体。

## 兼容性与计费边界

- 本地估算设置 `local_count_tokens`，usage 日志会标明 `usage_billing_path=local`；
  后续 `PostTextConsumeQuota` 继续走统一预扣、结算和退款流程。
- `relaykit/dto.ResponsesOutput` 保留 reasoning `summary`、custom tool `input` 与
  refusal；流事件 DTO 保留 text/refusal/arguments/input done 字段。根模块的本地
  计数不依赖原始 JSON 字符串拼接；`relaykit` 仍保持独立构建。
- 工具调用附加费仍由 `PriceData.OtherRatios`/工具计费路径单独决定；无文本但
  有合法工具附加费的请求不被误判为空用量。
- 不把预估输入 token 当作“上游实际 usage”；只有检测到真实的文本、推理或
  工具输出时才使用本地输出估算。

## 回归验证

```text
go test ./relay/channel/openai ./relay/helper -count=1 -timeout 60s
go test ./service ./controller ./setting/operation_setting -count=1 -timeout 60s
cd relaykit && GOWORK=off go build ./... && GOWORK=off go test ./... -count=1 -timeout 60s
git diff --check
```

本次变更未修改 `web/src` 或 `web/classic` 前端模板；错误语言统一属于后端
Responses/计费错误链路，前端模板边界不受影响。
