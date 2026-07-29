# 屏蔽词客户端中英文错误与状态契约

## 问题

屏蔽词阻断原先只返回英文 `sensitive words detected`。部分客户端只显示错误正文，
无法直观看出这是 new-api 的本地屏蔽词规则，也无法判断 HTTP、SSE 或 Realtime
实际使用的状态。同时，模型广场旧指标链路会把上游 `cyber_policy` 拒绝记录为失败，
导致内容违规被误解为模型连接故障。

## 契约

- 稳定错误码保持 `sensitive_words_detected`，不改变客户端判断条件。
- 普通 HTTP 请求和非流式响应阻断返回 HTTP 400。
- SSE 在连接和响应头已经建立后命中时保持 HTTP 200，并发送 `event: error`。
- Realtime 发送标准错误事件，并以 WebSocket 4403 关闭。
- 错误正文同时包含英文 `Sensitive words detected` 和中文“检测到屏蔽词”，并注明
  当前传输层状态及稳定错误码。
- HTTP 与 SSE 的 OpenAI 错误对象通过 `metadata` 返回中英文描述和结构化状态；
  SSE 额外返回 `transport=sse` 与 `stream_event=error`。
- 屏蔽词错误继续标记为不可重试，不改变审计事件、计费或上游错误契约。
- `sensitive_words_detected`、`prompt_guard_blocked`、`prompt_blocked`、
  `cyber_policy` 只按稳定结构化错误码识别，继续保留安全审计，但不写入模型广场
  失败样本，也不参与渠道质量成功率。
- 普通 HTTP 400/403、鉴权失败、协议错误和真实连接故障不在过滤范围内。
- HTTP 200 中的 SSE `response.failed` 和 Realtime `cyber_policy` 事件在精确识别后
  标记整次请求，既不记模型成功，也不记渠道连接失败。

## 兼容性

改动只扩充错误 `message` 和 `metadata`，既有 `error.code` 保持不变。SSE 已写出响应头
后不能改成 HTTP 400，因此必须由客户端同时识别 `event: error` 和
`sensitive_words_detected`。SQLite、MySQL 和 PostgreSQL 不涉及数据迁移。升级前已经
写入 `perf_metrics` 的聚合桶没有错误码维度，不能无损反向拆分，会随查询和保留窗口
自然退出；新请求立即按本契约过滤。

## 验证

- 服务层测试校验 HTTP 400、SSE HTTP 200、双语正文和结构化元数据。
- HTTP 中间件测试校验前置阻断响应。
- Realtime 中间件测试校验错误事件正文与 4403 关闭码。
- 模型广场测试校验内容策略拒绝不写失败桶，真实连接失败仍正常写入。
- 渠道质量测试覆盖实时采集、流式错误优先级和历史日志回填。
