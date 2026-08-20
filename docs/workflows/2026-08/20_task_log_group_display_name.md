# 任务日志分组显示名称修复

日期: 2026-08-20

## 问题

任务日志表格的分组列直接展示任务记录中保存的稳定分组标识。分组名称被管理员修改后，历史任务仍显示固定 code 或 alias，和其他日志、用户列表、模型广场的显示名称规则不一致。

## 修复

- 任务日志 DTO 新增 `group_name`，后端在列表响应阶段通过 `GetGroupDisplayNameMap()` 把当前 code、历史 alias 和当前名称解析为最新显示名称。
- `group` 字段继续保留内部稳定标识，用于筛选、计费、复制兼容和颜色稳定性。
- Classic 任务日志分组列优先展示 `group_name`，缺失时回退原 `group`。
- Default 任务日志已按 `group_name` 渲染，后端补齐字段后无需额外改动。

## 验证

- `go test ./controller -run 'TestTasksToDtoHidesUpstreamModelForUserView|TestTasksToDtoUsesCurrentGroupDisplayName' -count=1` 通过。
- `node --test group-display-name-integration.test.mjs` 在 `web/classic/src` 通过。
- 本地固定环境 `http://localhost:3000` + Classic `http://localhost:3001`：插入分组 code `task-log-fixed-code`、显示名 `任务日志显示名称` 和任务 `task_group_display_smoke` 后，`/api/task` 返回 `group=task-log-fixed-code`、`group_name=任务日志显示名称`；浏览器打开 Classic 任务日志确认表格分组列显示 `任务日志显示名称`。验证后已删除临时任务和分组数据。
