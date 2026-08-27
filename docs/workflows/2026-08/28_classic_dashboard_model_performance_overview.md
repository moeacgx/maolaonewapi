# Classic 控制台首页性能概览补齐

日期：2026-08-28

## 目标

在 Classic 控制台首页补一个只读的模型性能概览卡，参考新版前端的
性能健康面板，让管理员可以直接看到各节点/模型的调用量、平均延迟、
吞吐量和成功率。

## 范围

- 新增 `PerformanceOverviewPanel`，独立请求 `GET /api/perf-metrics/summary`
  的 24 小时摘要数据，并从 `series` 里补出首 Token 延迟。
- 在首页 `StatsCards` 后、收入面板前展示性能概览。
- 概览卡包含四项汇总指标，以及按调用量排序的节点/模型明细表。
- 数据为空时只显示“暂无性能数据”，不改动现有告警或计费逻辑。

## 兼容性

- 不修改后端接口，不改模型定价页、性能详情页或模型卡片摘要。
- 仅管理员首页显示该面板，保持现有权限边界。
- 复用现有经典版性能格式化函数与现成翻译词条，不新增业务契约。

## 验证

- `node --test web/classic/src/pages/Dashboard/__tests__/console-shell-contract.test.mjs`
- `node --test web/classic/src/components/dashboard/__tests__/performance-overview-panel.test.mjs`
- 后续再跑 Classic 构建和相关 ESLint / Prettier 检查。
