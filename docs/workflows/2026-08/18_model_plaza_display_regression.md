# 模型广场分组显示与状态回归修复

日期：2026-08-18

## 问题

模型广场价格详情和卡片需要展示管理员维护的分组显示名称、模型可用状态和失败样本过滤后的可用性。当前可观察回归包括：

- 价格详情中的分组可能回退展示历史兼容标识，例如 `group_3分组`，而不是当前显示名称。
- 卡片已有延迟与吞吐量摘要时，状态列仍可能显示 `—`。
- 需要确认模型广场失败过滤规则仍参与失败样本排除。

## 根因假设

分组名称映射只覆盖当前 `groups.code`，没有覆盖迁移后仍可能出现在可用分组、能力或历史指标中的 `group_aliases.alias`。前端状态条只在摘要接口返回 `series` 时渲染；当后端或缓存只提供聚合 `status_rate/success_rate` 时，状态列退化为无数据。

## 变更范围

- 后端模型广场分组名称映射补齐启用分组的历史 alias 到当前显示名称。
- Default 模型卡片在缺少分时序列但已有聚合状态率时，使用聚合状态率渲染可用状态，不再显示空状态。
- 增加回归测试覆盖 alias 显示名称映射、卡片状态聚合兜底和失败过滤规则。

## 安全与兼容性

不改变分组稳定编码、渠道选择、计费、令牌分组绑定、失败过滤规则配置或原始错误日志。历史 alias 只用于展示名称解析；模型卡片状态兜底只影响可视化，不改变性能统计口径。

## 验证计划

- 执行分组名称映射和定价控制器测试。
- 执行 Default 模型卡片状态相关测试。
- 执行失败过滤规则专项测试。
- 复用固定本地测试环境验证 `/api/status`、classic 首页、root/demo 登录和演示数据接口。
- 执行 Default 类型检查、Markdown 格式检查和 `git diff --check`。

## 验证结果

- `go test ./model ./controller ./pkg/perf_metrics -run 'Test(GetActiveGroupNameMapUsesLatestDisplayName|GetActiveGroupNameMapFallsBackToHistoricalAliases|GetActiveGroupNameMapIgnoresDisabledGroupAliases|NormalizeGroupCodeRejectsSelectorValues|BuildPricingGroupNamesUsesDisplayNameAndFallback|ShouldRecordRelayFailureConfigRules)' -count=1`：通过。
- `node --test src/features/pricing/lib/group-names.test.ts src/features/pricing/components/model-perf-badge.test.mjs src/features/performance-metrics/lib/status-segments.test.ts`（`web/default/`）：通过，10 个测试通过。
- `node node_modules/typescript/bin/tsc -b`（`web/default/`）：通过；本机 PATH 未提供 `bun`，所以未直接执行 `bun run typecheck`。
- `node node_modules/@rsbuild/core/bin/rsbuild.js build`（`web/default/`）：通过；构建仍提示既有 `channel-observability/index.tsx` 未导出 `Route` 警告，本次未修改该路由。
- 固定本地库 `tmp-local-v10101.db` 冒烟：临时启用 Default 模型广场并把 `svip`/`vip` 显示名改为 `超级会员`/`贵宾用户`，`GET /api/pricing` 返回 `group_names`，浏览器 `http://localhost:3000/pricing` 分组筛选显示当前显示名称；验证后已恢复 `HeaderNavModules`、`theme.frontend` 和分组显示名。
- `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/local-test.ps1 -Action verify`：通过，验证 `/api/status`、classic 首页、root/demo 登录和演示数据接口。
- `node web/default/node_modules/prettier/bin/prettier.cjs --check docs/developer/README.md docs/workflows/2026-08/18_model_plaza_display_regression.md docs/workflows/2026-08/18_channel_root_path_compatibility.md`：首次发现 Markdown 格式差异，已执行 `--write` 修正。
- 跨项目影响评估：`/api/pricing.group_names` 会增加历史 alias 展示名映射，已按规则通知 `tokens-pro` 与 `Sub2API` 评估兼容性：<https://github.com/moeacgx/tokens-pro/issues/12>、<https://github.com/moeacgx/sub2api/issues/4>。
