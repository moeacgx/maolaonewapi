# Classic 仪表盘性能概览回滚

日期：2026-08-28

## 目标

撤回误加到 Classic 管理首页的模型性能概览，避免把单模型性能统计
误当成多节点负载均衡面板。

## 根因

前一次改动把 `PerformanceOverviewPanel` 挂到了 Dashboard 首页，并直接读取
`/api/perf-metrics/summary`。这展示的是模型性能汇总，不是负载均衡下的节点
健康/吞吐情况，和当前需求不一致。

## 修改范围

- 回滚 Classic Dashboard 首页中的性能概览挂载。
- 删除 `PerformanceOverviewPanel` 及其测试。
- 回退相关工作记录，保留本次回滚说明。
- 将版本号提升到 `v1.0.0-rc.10.1.10.261`，作为回滚版发版号。

## 兼容性与边界

- 不改后端接口、不改计费、不改节点负载均衡实现。
- 仅撤回前端展示层的误加内容，避免继续向用户传递错误的指标语义。

## 验证计划

- 重新检查 Classic Dashboard 首页是否已不再渲染性能概览。
- 重新执行本次涉及的前端构建和必要的回归检查。

## 验证结果

- `node --test src/pages/Dashboard/__tests__/console-shell-contract.test.mjs`：通过。
- `git diff --check`：通过。
- `npx --yes bun install`：通过，补齐了 Classic 构建依赖。
- `npm run build`（`web/classic`）：通过。
