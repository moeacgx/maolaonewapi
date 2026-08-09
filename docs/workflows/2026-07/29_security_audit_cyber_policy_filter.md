# 安全审计官方风控 cyber_policy 筛选标识

## 问题

后端已经只对结构化 `cyber_policy` 错误码生成上游策略事件，但两套管理页面仍把
来源显示为“上游策略”，管理员无法在筛选器中快速确认这是官方风控提交。

## 变更

- Default 和 Classic 的事件来源标签、详情字段和来源筛选项统一显示
  “官方风控（cyber_policy）”。
- 保留现有数据契约：数据库使用 `source=upstream_policy`、`error_code=cyber_policy`，
  不改动自动封禁统计、历史事件或 API 查询参数。
- 后端识别保持严格路径：仅接受 `error.code` 或 `response.error.code` 的精确值，
  不扫描错误文案，也不把普通 400 误判为官方风控事件。
- Default 与 Classic 的新增文案均接入本地化资源。

## 验证

- `go test -count=1 ./service ./model ./controller`
- Default 安全审计单测与类型检查/构建
- Classic 安全审计 ESLint、单测与构建
- `git diff --check`
