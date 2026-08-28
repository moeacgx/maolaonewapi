# Classic 控制台首页移除模型性能概览

日期：2026-08-28

## 背景

`e989342da` 在恢复模型广场与控制台回归时重新挂载了
`PerformanceOverviewPanel`。该面板会在管理员控制台首页额外展示模型性能指标，
与用户需要的简洁控制台布局不符，也不是模型广场详情页的必要功能。

## 目标

移除 Classic 控制台首页的模型性能概览，恢复统计卡片后直接展示管理员收入面板的布局。

## 修改范围

- 从控制台首页移除 `PerformanceOverviewPanel` 的导入和 JSX 挂载。
- 保留模型广场详情页的性能概览、性能指标接口和系统性能设置，不改变其数据契约。
- 保留管理员收入面板、统计卡片、控制台侧栏和移动端顶栏行为。
- 回归测试改为约束控制台首页不得重新挂载该面板。

## 兼容性与边界

这是 Classic 前端首页布局调整，不修改后端接口、数据库、权限、计费或限流逻辑。
`PerformanceOverviewPanel.jsx` 暂保留，供模型广场或后续明确需求复用；当前首页不会加载其性能摘要请求。

## 验证

- `node --test web/classic/src/components/dashboard/__tests__/performance-overview-panel.test.mjs`
- `node --test web/classic/src/pages/Dashboard/__tests__/console-shell-contract.test.mjs`
- `git diff --check`

