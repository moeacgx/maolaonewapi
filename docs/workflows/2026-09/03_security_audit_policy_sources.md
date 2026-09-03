# 安全审计上游策略来源泛化

## 变更目标

会话屏蔽与达到阈值自动禁用原先只针对 `cyber_policy`。现支持在
`policy_action_sources` 中选择 `cyber_policy`、`biological_risk`，两项处置
均按所选来源累计。

## API 与数据契约

- `PromptAuditConfig` 和内置策略接口新增 `policy_action_sources: string[]`。
- 配置值持久化为 `prompt_audit_configs.policy_action_sources` 的 JSON TEXT；
  旧数据库为空时按 `["cyber_policy"]` 兼容。
- 生物风险事件使用 `source=biological_risk`、`error_code=biological_risk`。
  `cyber_policy` 事件继续使用历史 `source=upstream_policy`。
- 会话屏蔽响应统一返回 `session_blocked_by_security_policy`；内容策略识别仍兼容
  历史 `session_blocked_by_cyber_policy`。
- 请求归档审计来源筛选接受 `biological_risk`。

## 识别与安全边界

生物风险仅在上游响应明确包含 `status_code=500` 且包含短语
`flagged for possible biological risk` 时记录；普通 500 或仅包含 `risk` 的文案
不会触发策略处置。结构化 `cyber_policy` 错误路径保持不变。

## 兼容与验证

保留旧 `cyber_session_block_enabled`、`cyber_policy_auto_ban_enabled`、阈值和
窗口字段。旧字段继续作为动作总开关，`policy_action_sources` 决定来源集合。
模型层新增的 TEXT 列可由三数据库 `AutoMigrate` 添加，空值由读取逻辑回退默认。
测试覆盖生物风险识别、状态码边界以及多来源累计统计。
