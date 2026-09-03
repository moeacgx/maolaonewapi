# Classic 模型详情分组与标签展示收敛

日期：2026-08-30

## 问题

v276 已包含 Default 模型详情折扣修复，但 Classic 模型详情的供应商信息区仍把分组
按稳定语义色渲染，并继续展示模型标签。该区域需要保持低噪声的灰色分组 pill，且不
展示模型标签。

## 修改范围

- `ModelBasicInfo` 的供应商 Groups 改为普通灰色 `classic-pricing-detail-pill`，不再
  注入分组颜色变量。
- 删除 Classic 模型详情中的 `modelData.tags` 解析和“模型标签”单元格。
- 更新 Classic 视觉契约测试，保护灰色分组和隐藏标签行为。

## 兼容性与边界

本次只改 Classic 模型详情展示层和对应测试，不修改分组编码、筛选器、计价公式、
性能面板、Default 前端或后端 API。模型广场筛选仍可使用模型标签，只有详情元数据区
不再显示标签。

## 验证计划

- `node --test web/classic/src/components/table/model-pricing/model-pricing-visual-contract.test.mjs`
- `node --test web/classic/src/components/table/model-pricing/groupVisuals.test.mjs`
- `git diff --check`，并确认 PR base 为 `custom-main`。

## 验证结果

- `node --test src/components/table/model-pricing/model-pricing-visual-contract.test.mjs
src/components/table/model-pricing/groupVisuals.test.mjs
src/components/table/model-pricing/view/card/__tests__/card-layout-contract.test.mjs`：
  8 个测试通过。
- Classic 两个受影响源码/契约文件的 Prettier 检查：通过。
- `git diff --check`：通过；Default 前端未修改。
- PR #120 已于 2026-08-30 合并，merge commit 为
  `d2ce24df5a4a3335e24158c4bae5855c6eb88ad1`，v276 tag 已包含 Default 修复；本次是
  针对截图实际命中的 Classic 详情路径的补充修复。

Classic 完整构建未执行，本机当前只具备契约测试环境；未部署生产。
