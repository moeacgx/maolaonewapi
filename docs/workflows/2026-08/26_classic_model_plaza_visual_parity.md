# Classic 模型广场概览视觉对齐

日期：2026-08-26

## 问题

模型详情页与新版模型广场的视觉契约不一致：

- “各分组性能”表的分组标签固定为蓝色，不能像新版 `GroupBadge` 一样按分组稳定着色。
- “概览”成功率在 Classic 详情页里依赖 Tailwind 文本类，实际展示容易退化为默认黑色。
- “按分组定价”的折扣 badge 固定使用 warning 橙色，和折扣语义不匹配，视觉噪声偏高。

## 修复范围

- 新增 Classic 模型广场分组视觉 helper，独立轻量实现与既有 `stringToColor` 一致的稳定色算法，按 group code 输出 Semi `Tag` 颜色与文本色。
- “各分组性能”表的分组 `Tag` 不再固定 `blue`，改为按分组稳定取色。
- “模型”信息里的分组 pill 和“按分组定价”分组名称使用同一稳定分组色。
- “概览”成功率使用内联 Semi 语义色，按新版阈值展示：`>= 90%` 绿色、`>= 70%` amber、`< 70%` danger。
- “各分组性能”的成功率数字改为内联语义色，避免 Classic 构建中动态 Tailwind 颜色类退化。
- 模型卡片的性能状态标签按整体成功率着色，但继续隐藏百分比，保持紧凑摘要布局。
- 折扣 badge 从固定 warning 橙色改成低饱和中性辅助徽标，避免把正常折扣显示成警告。

## 兼容性

- 本次只改 Classic 模型广场展示层，不修改模型 ID、分组 code、筛选、计价、折扣计算或后端 API。
- 分组显示名称仍由 `groupNames` 映射提供，颜色基于内部 group code 保持稳定。
- 动态路由、渠道、日志、用户分组绑定不在本次改动范围内。

## 验证计划

- `node --test web/classic/src/components/table/model-pricing/model-pricing-visual-contract.test.mjs`
- `node --test web/classic/src/components/table/model-pricing/groupVisuals.test.mjs`
- `node --test web/classic/src/components/table/model-pricing/performance/utils.test.mjs`
- `node --test web/classic/src/components/table/model-pricing/view/card/ModelPerformanceBadge.test.mjs`
- `git diff --check`
