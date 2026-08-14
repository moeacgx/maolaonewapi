# 官方风控 cyber_policy 会话屏蔽

## 问题

上游返回精确 `cyber_policy` 后，new-api 已能记录官方风控事件并可选自动禁用普通用户，但同一客户端显式会话继续发送后续轮次时仍会再次进入选渠、预扣费和上游连接。Sub2API 的风控中心提供的是官方上游返回驱动的会话级后置屏蔽，不是本地语义模型判断；需要把这一路径移植到 new-api，并保留现有官方事件和自动处置边界。

## 方案

- 在 `prompt_audit_configs` 增加 `cyber_session_block_enabled` 和 `cyber_session_block_ttl_seconds`，默认关闭、默认 TTL 3600 秒、上限 31536000 秒。
- 后端管理 API、Default 和 Classic 内置安全策略页同时暴露两个字段；启用会话屏蔽时必须启用上游安全策略事件记录，前端联动开关，服务端也做一致性校验。
- 只使用显式会话标识生成屏蔽键：会话类请求头（`X-Claude-Code-Session-Id`、`X-Codex-Session-Id`、`X-Session-Affinity`、`X-Session-Id`、`X-OpenCode-Session`、`X-Conversation-ID`、`conversation_id`、`session_id`）或 JSON 顶层 `prompt_cache_key`。不使用用户 ID、API Key 本身、请求 ID、`previous_response_id`、正文哈希或模糊内容回退。
- 屏蔽键以 API Key ID 隔离后做 SHA-256，存储名为 `cyber_session_block:<sha256>`；Redis 可用时写 Redis TTL，写失败退回本机内存 TTL；读取 Redis 失败按 fail-open 处理。
- 只有官方上游 `cyber_policy` 事件成功落库后才写入会话屏蔽；范围外事件、事件落库失败、本地屏蔽词和 Guard 阻断都不会写屏蔽键。
- HTTP 后续请求在 `PromptAudit` 中、渠道分配前读取同一显式会话键；命中时返回 OpenAI JSON 403，错误码 `session_blocked_by_cyber_policy`。
- Realtime 握手按请求头做同样检查；已建立连接中若上游返回 `cyber_policy`，本连接后续客户端帧直接返回 Realtime error 并以 4403 关闭。
- 本地会话屏蔽只标记内容策略拒绝，不调用 `RecordUpstreamPolicy*`，不写 `source=upstream_policy` 事件，不增加用户官方风控窗口累计，也不触发自动禁用用户。

## 兼容性与边界

- 数据库字段通过 GORM `AutoMigrate` 增加，使用普通 bool/int 字段，兼容 SQLite、MySQL 和 PostgreSQL。
- 旧配置默认关闭会话屏蔽，保持升级前行为；旧配置 TTL 为 0 时运行态归一为 3600 秒。
- 同一会话标识在不同 API Key 下互相隔离；没有显式会话标识时 fail-open。
- 官方风控事件识别仍只接受 `error.code=cyber_policy` 或 `response.error.code=cyber_policy`，不扫描错误文案。
- 自动禁用用户能力保留，继续只基于已成功持久化的官方事件、滚动窗口、重置点和分组白名单；本地会话屏蔽不会参与该计数。

## 验证

- `go test ./service ./middleware ./model ./types ./controller ./relay/channel/openai -run "TestCyberSession|TestPromptAuditRejectsCyberSession|TestRecordUpstream|TestSavePromptAuditBuiltinPolicy|TestCyberPolicyAutoBan|TestContentPolicy" -count=1 -timeout=120s`
