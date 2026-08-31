# 任务日志分组显示名称修复

日期: 2026-08-20；更新: 2026-08-21

## 问题

任务日志表格的分组列直接展示任务记录中保存的稳定分组标识。分组名称被管理员修改后，历史任务仍显示固定 code 或 alias，和其他日志、用户列表、模型广场的显示名称规则不一致。

## 修复

- 任务日志 DTO 新增 `group_name`，后端在列表响应阶段通过 `GetGroupDisplayNameMap()` 把当前 code、历史 alias 和当前名称解析为最新显示名称。
- `group` 字段继续保留内部稳定标识，用于筛选、计费、复制兼容和颜色稳定性。
- Classic 任务日志分组列优先展示 `group_name`，缺失时回退原 `group`。
- Default 任务日志已按 `group_name` 渲染，后端补齐字段后无需额外改动。
- 兼容仍保留旧版 `UserUsableGroups` 的历史标识：当旧 key 未进入 `group_aliases` 且当前 code/alias 映射未命中时，使用旧 key 对应的配置标签作为展示回退，不改变权限、筛选或计费解析。
- 任务日志筛选补齐模型名和管理员用户名参数；管理员可按请求模型或实际上游模型查询，普通用户只按可见模型查询，避免泄露隐藏上游模型。
- 多档计费日志补齐实际结算 token 维度、请求倍率、实际扣费和预估/实际档位差异，Default 详情弹窗按后端结算轨迹展示过程。

## 验证

- `go test ./controller -run 'TestTasksToDtoUsesCurrentGroupDisplayName' -count=1 -timeout 120s` 通过。
- `go test ./model -run 'TestTaskLogFiltersUsernameAndModelName|TestTasksToDtoUsesCurrentGroupDisplayName' -count=1 -timeout 120s` 通过。
- `go test ./service -run 'TestInjectTieredBillingInfoIncludesActualSettlementTrace|TestComposeTieredTextQuotaKeepsToolCallSurcharges|TestTryTieredSettleNoClampInRange' -count=1 -timeout 120s` 通过。
- `cmd /c node_modules/.bin/vitest.cmd run src/features/usage-logs/lib/filter.test.ts src/features/usage-logs/lib/billing-details.test.ts` 通过。
- `cmd /c node_modules/.bin/tsgo.cmd -b` 通过。
- `node --test classic/src/group-display-name-integration.test.mjs` 在 `web` 通过。
- `go test ./controller -run 'TestTasksToDtoUses(LegacyUserUsableGroupDisplayName|CurrentGroupDisplayName)' -count=1 -timeout 60s` 通过，覆盖旧版 `Codex-Plus.group_2` 标签到 `codex-basic` 的兼容显示。
- `cmd /c node scripts/sync-i18n.mjs` 完成，`_sync-report.json` 显示所有 locale `missingCount=0`、`extrasCount=0`。
- 本地固定环境 `http://localhost:3000` + Classic `http://localhost:3001`：插入分组 code `task-log-fixed-code`、显示名 `任务日志显示名称` 和任务 `task_group_display_smoke` 后，`/api/task` 返回 `group=task-log-fixed-code`、`group_name=任务日志显示名称`；浏览器打开 Classic 任务日志确认表格分组列显示 `任务日志显示名称`。验证后已删除临时任务和分组数据。
