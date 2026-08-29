# Default 模型详情分组折扣与供应商信息收敛

日期：2026-08-29

## 问题

Default 模型详情页的分组定价同时展示综合折扣标签和原始分组倍率，用户会看到
重复的“折扣/倍率”信息；供应商信息区的模型标签也会增加无关噪声。

## 修改范围

- 分组定价的动态计费卡片和普通计费表格都在分组名称右上侧显示 `X折`，折数由
  综合因子乘以 10 得到；每个分组都会显示自己的折数，不再把加价因子显示为 `X倍`。
- 折扣颜色沿用 v243 语义：综合因子 `< 0.5` 使用红色，其余使用绿色；徽标采用
  v243 Classic 的小号浅色 Tag 视觉，并使用方圆角而非大圆角胶囊。
- 删除普通计费表格的独立 Ratio 列，以及动态卡片中重复的 `x` 文本；倍率仍由已有
  价格信息承担。
- 供应商信息的 Groups 继续使用灰色 `CatalogPillList`，移除 Tags 单元格；筛选页和
  模型卡片的标签能力不受影响。
- 新增 Default 前端的纯逻辑计费因子 helper，并为七种 locale 登记对应的展示 key。

## 兼容性与边界

本次仅修改 Default `web/src` 展示层和其定价 helper/契约测试，不修改 Classic、后端
计价公式、分组编码、模型筛选或 API 数据契约。综合因子仍按
`groupRatio * (priceRate / usdExchangeRate)` 计算，展示不参与实际扣费。

## 验证计划

- `npx --yes vitest run src/features/pricing/lib/__tests__/pricing-contracts.test.ts`
- Default 类型检查、受影响文件 lint/格式检查和生产构建。
- `bun run i18n:sync`（本机无 bun 时使用等价的 `npx --yes bun run i18n:sync`）。
- `git diff --check`，并确认 Classic 路径无差异。

## 验证结果

- `npx --yes vitest run src/features/pricing/lib/__tests__/pricing-contracts.test.ts`：
  1 个文件、6 个测试通过。
- `npx --yes bun run typecheck`：通过（`tsgo -b`）。
- `npx --yes bun x oxlint -c .oxlintrc.json` 针对 3 个受影响 TS/TSX 文件：通过，
  无输出错误。
- `npx --yes oxfmt -c .oxfmtrc.json --check` 针对 3 个受影响文件：通过。
- `npx --yes bun run i18n:sync`：完成，所有 locale `missingCount` 和 `extrasCount`
  均为 0。
- `npx --yes bun run build`：通过，最近一次 Rsbuild 在 5.62 秒完成生产构建。
- `git diff --check`：通过；Classic `web/classic` 无差异。

全仓库 `format-with-protected-headers.mjs --check` 未作为本次变更的通过依据：脚本在
恢复快照时遇到并发文件锁错误（`use-chat-handler.ts`），受影响文件的定向格式检查已
通过。使用本地 mock 数据完成 Default 模型详情截图，验证三组分别显示 `2折`、
`10折`、`20折`；未执行真实 API 数据验收。

后续样式微调再次使用本地 mock 截图确认：徽标改为 v243 Classic 小号浅色 Tag 风格，
三组折数均保留，截图位于系统临时目录
`C:\\Users\\Administrator\\AppData\\Local\\Temp\\default-pricing-details.png`。
