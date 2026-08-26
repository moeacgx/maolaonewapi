# Classic 模型广场卡片高度统一

日期：2026-08-26

## 目标

让 Classic 模型广场卡片在同一网格中保持稳定的信息高度，避免模型配置的
端点、能力标签或价格调整标签数量影响卡片布局。

## 根因

新版模板迁移后，`PricingCardView` 在卡片标题旁渲染价格调整 `Tag`，并在页脚
额外渲染端点、模型标签、隐藏数量和计费单位。后者会随模型的能力和分组配置换行，
使不同卡片的内容高度不一致。

## 修改范围

- 移除卡片标题旁的价格调整标签，以及卡片页脚的端点、能力标签和数量摘要。
- 模型介绍下只保留实际计价分组的显示名称、按量/按次（或动态）计费说明，以及
  `ModelPerformanceBadge` 的延迟、吞吐和可用状态。
- 卡片分组名称仍优先使用 `group_names` 映射，缺失时由
  `getGroupDisplayName` 回退稳定 code；保留按 group code 计算的稳定颜色。
- 卡片只展示当前实际计价分组；`all`、`auto` 等伪值不会显示。无法从计价结果获得
  真实分组时，展示第一个真实启用分组；没有真实分组则隐藏该位置，不重新引入多分组
  标签或 `+N` 计数。
- `ModelBasicInfo` 和详情抽屉继续展示分组、端点和模型标签，不受卡片收敛影响。
- 成功率语义色继续由 `ModelPerformanceBadge` 与现有性能 helper 决定。
- 当性能摘要只有 `status_rate` 或 `success_rate`、没有有效时间序列时，卡片以固定
  三段信号显示同一语义色，并保留状态百分比的可访问名称和悬浮标题。

## 兼容性与边界

- 仅修改 Classic 模型广场卡片展示层；不改动态路由、后端接口、模型 ID、分组 code、
  计费计算、性能统计或详情抽屉的数据契约。伪分组过滤不参与价格计算。
- Classic 当前定向测试环境以 `node --test` 读取 JSX 展示契约，未配置 React DOM
  组件测试运行时。因此回归用例直接验证分组显示名 helper 的真实回退行为，并对卡片
  与详情的 JSX 边界做结构契约校验。

## 验证结果

- `node --test src/components/table/model-pricing/view/card/__tests__/card-layout-contract.test.mjs src/components/table/model-pricing/groupVisuals.test.mjs src/components/table/model-pricing/view/card/ModelPerformanceBadge.test.mjs`：8 项通过。
- `git diff --check`：通过。
- 本机未安装 `bun`，且 `web/classic/node_modules` 不存在；因此无法执行 Classic 的
  Prettier 检查和 Vite 构建，未安装或变更依赖。
