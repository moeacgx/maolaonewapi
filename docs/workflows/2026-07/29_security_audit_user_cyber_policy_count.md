# 安全审计用户官方风控窗口累计次数

## 问题

安全审计事件列表只显示单次事件，Root 无法直接判断同一用户在自动封禁滚动窗口内
已经累计多少次官方 `cyber_policy` 命中。

## 方案

- `GET /api/security-audit/events` 的每个列表项新增 `user_cyber_policy_count`。
- 每个列表项同时返回 `cyber_policy_window_hours`，明确累计窗口。
- 统计范围以接口响应时刻为窗口终点，向前回溯当前安全审计配置中的
  `cyber_policy_violation_window_hours`。
- 只统计 `source=upstream_policy` 且 `error_code=cyber_policy` 的事件；普通 400、
  其他来源、其他错误码以及窗口外事件都不计入。
- 同一页用户通过一次 `GROUP BY user_id` 查询聚合，避免逐行查询。

## 兼容性

查询使用 GORM 的 `IN`、`GROUP BY` 和参数绑定，同时支持 SQLite、MySQL 和 PostgreSQL。
新增字段不改变现有筛选、分页和事件存储结构，旧客户端可以忽略。

## 验证

- `go test -count=1 ./model ./controller ./service`
- 覆盖重复用户 ID、时间窗口两端、窗口外事件、错误来源和错误错误码。
