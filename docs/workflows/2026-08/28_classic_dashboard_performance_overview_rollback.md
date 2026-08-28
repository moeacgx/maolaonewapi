# Classic 控制台首页性能概览恢复

日期：2026-08-28

## 背景

`daed310c1` 曾明确回滚 `f57d2890d` 引入的 `PerformanceOverviewPanel`，
原因是当时把模型性能汇总误解成多节点负载均衡面板。本次回归核对确认，
Classic 首页需要恢复历史的模型性能概览；该面板只展示性能摘要，不替代
系统实例健康与负载均衡面板。

## 目标

在 Classic 控制台首页 `StatsCards` 后、收入面板前恢复管理员可见的性能
概览平铺面板，展示成功率、平均延迟、首 Token 延迟、吞吐量和模型明细。

## 修改范围

- 恢复 `PerformanceOverviewPanel`，请求 `GET /api/perf-metrics/summary` 的
  24 小时摘要，并从 `series` 计算模型首 Token 延迟。
- 仅在管理员首页挂载面板，保持控制台侧栏、导航和其他首页面板行为不变。
- 加载阶段显示稳定的骨架布局；成功但无模型时显示“暂无性能数据”；API
  返回失败或请求异常时显示“加载失败”，不创建或填充测试数据。
- 模型明细优先按接口提供的调用量排序；调用量缺失或相同时按模型名稳定排序，
  并复用 Classic 既有性能格式化函数和成功率语义色。

## 兼容性与边界

- 不改后端接口、数据库、计费逻辑或系统实例负载均衡实现。
- 不改变模型广场卡片、定价页和性能详情页的数据契约。
- API 请求使用可取消的 `AbortController`，组件卸载后不更新状态。

## 验证计划

- `node --test web/classic/src/components/dashboard/__tests__/performance-overview-panel.test.mjs`
- `node --test web/classic/src/pages/Dashboard/__tests__/console-shell-contract.test.mjs`
- `git diff --check`
- 如环境提供 Bun，再执行 Classic 前端构建。

## 验证结果

- 性能概览结构契约测试通过。
- Classic 控制台外壳契约测试通过。
- `git diff --check` 通过。
- 当前环境未提供 `bun`，未执行 Classic 前端构建。
