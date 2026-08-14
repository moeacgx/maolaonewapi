# 官方风控封禁窗口重置与自动处置

> 后续变更：2026-08-14 起新增可选的 `cyber_policy` 会话屏蔽。本文仍记录自动封禁
> 窗口重置问题的修复；其中“不写会话标记、不前置 403、不关闭后续 Realtime 帧”的
> 结论只适用于会话屏蔽开关关闭的默认兼容模式。当前契约见
> [官方风控 cyber_policy 会话屏蔽](14_security_audit_cyber_session_block.md)。

## 问题

`cyber_policy` 自动封禁原先直接统计滚动窗口内的全部历史事件。Root 手动恢复用户后，
上一次封禁前的事件仍在窗口内，用户下一次命中可能立即再次被禁用。

上游明确拒绝当前请求或 Realtime 帧后，网关只应保留本次事件证据，并按配置执行可选的
自动禁用用户；不应猜测或持久化客户端会话身份，也不应在后续同会话请求前置阻断。

## 方案

- `users.cyber_policy_count_reset_event_id` 保存最近一次自动封禁成功时的累计重置事件 ID。
- 禁用普通用户的同一条条件更新同时写入状态和重置点；重置点取本次累计查询中最新的
  已计入事件 ID，并发命中只有成功完成状态迁移的执行者能够更新重置点和记录处置日志。
- 审计事件继续完整保留。单用户自动处置和事件列表批量计数都只统计重置点之后的新
  事件，仍叠加当前滚动窗口、渠道或分组范围及自动封禁白名单。
- 精确识别到上游 `cyber_policy` 后，只写入当前请求或当前 Realtime 帧的官方风控事件，
  并只在事件持久化成功后参与可选自动禁用用户。
- 不读取、哈希、缓存或记录 `prompt_cache_key`、`conversation_id`、`conversation.id`、
  Codex/Claude 会话头或正文近似值作为后续阻断依据；请求 ID 和 `previous_response_id`
  同样只作为普通请求元数据处理。
- 后续同用户、同稳定会话请求不会因为历史 `cyber_policy` 事件在选渠、并发占用或计费
  前返回本地 403；Realtime 同连接后续帧也不会因为之前的上游 `cyber_policy` 帧被本地
  直接关闭。

## 兼容性与边界

用户表通过 GORM 新增普通整数事件游标列，默认值为 0，兼容 SQLite、MySQL 和 PostgreSQL；
旧用户保持原有统计语义，首次自动封禁后才建立重置点。

官方风控作用范围只约束事件是否落库及自动禁用累计，不再控制任何会话级缓存。上游
`cyber_policy` 的原始 HTTP/SSE/Realtime 响应继续按原样返回客户端；本地只记录结构化
事件、内容策略标记和可选自动处置结果。

## 验证

- 官方风控事件仍按全部渠道、指定渠道或指定分组范围写入审计。
- 自动禁用用户仍应用滚动窗口、重置点和分组白名单。
- 会话屏蔽开关关闭时，后续同会话请求不会因为历史 `cyber_policy` 事件前置返回 403。
- 会话屏蔽开关关闭时，Realtime 连接收到上游 `cyber_policy` 帧后，后续客户端帧不会被本地直接关闭。
- `go test -count=1 -timeout 60s ./model ./service ./controller ./relay/channel/openai`。
- `go vet ./model ./service ./controller ./relay/channel/openai`。
