# 营销福利重构 Task 10 验收记录

## 范围与边界

- 基线：`ea09ff531`。
- 范围：Default 与 Classic 两套前端 i18n、长期开发文档、全量验证、真实路由视觉验收和源码合同复核。
- 文件边界：仅修改 Task 10 指定 locale、`static-keys.ts`、开发文档、工作流记录和三张最终截图。
- 禁止部署：本次未部署 `maolaoapi`、`zzapi` 或其他实例，未推送、未创建 PR、未合并。

## i18n

- 执行 `cd web; bun run i18n:sync`：失败，环境未安装 `bun`（PowerShell 报 “bun is not recognized”）。
- 执行 `cd web; node scripts/sync-i18n.mjs`：通过，生成报告后以基线文件为底补齐 Task 1-9 新增键。
- Default locale：`en`、`zh`、`zh-TW`、`fr`、`ru`、`ja`、`vi` 均覆盖 Task 4-6 字面量。
- Classic locale：`en`、`zh`、`zh-CN`、`zh-TW`、`fr`、`ru`、`ja`、`vi` 均覆盖 Task 7-9 字面量。
- 插值变量核对：Task 键占位符集合逐 locale 比较，未发现不匹配；Tokens、品牌和 API 标识保留原文。

## 验证命令

以下结果在完成后按实际退出码更新：

| 阶段 | 命令 | 结果 |
| --- | --- | --- |
| 后端定向 | `go test ./model ./controller ./router -count=1 -timeout 60s` | 通过：model/controller/router 均通过 |
| 前端构建 | `npm run build`、`npx --no-install vite build` | 失败：Default 缺少 `rsbuild`，Classic 缺少 `vite`/node_modules |
| 构建后全量 Go | `go test ./... -count=1 -timeout 60s` | 失败：根包 embed 报 `pattern web/classic/dist: no matching files found`；其余已执行包通过 |
| Default 全量 | `npm run test -- --run`、`npm run typecheck`、`npm run lint`、`npm run format:check`、`npm run copyright:check` | 失败：vitest/tsgo/oxlint 不存在；format 检查退出 1 无输出；copyright 检查发现 52 个基线业务文件头待更新（未越界修改） |
| Classic 定向 | `node --test ...` | 通过：48/48 |
| Classic ESLint | `npx --no-install eslint ...` | 失败：npx 提供 ESLint 10，仓库仅有 `.eslintrc`，缺少 `eslint.config.*` |
| Classic Prettier | `npx --no-install prettier --check ...` | 失败：7 个既有文件格式不符合（测试、EditRedemptionModal.jsx、index.css 等；未修改业务实现） |
| Classic i18n | `npx --no-install i18next-cli lint` | 失败：本地无 `i18next-cli` |
| 最终差异 | `git diff --check` | 通过 |

单次 Go 测试超时上限为 60 秒。若出现基线无关失败，记录精确文件、错误和影响范围，不修改业务实现。

## 真实页面验收

尝试使用本地 Default/Classic 服务和 Playwright 真实路由 DOM 验证，但环境阻塞：

- `go run .` 无法启动，根包 embed 缺少 `web/classic/dist`。
- Playwright wrapper 无法执行，Windows WSL 报 `/bin/bash` 不存在。
- 因无服务和构建产物，未生成或伪造截图；以下计划路径目前不存在：

- Default：`/benefits`、`/redemption-codes`。
- Classic：`/console/benefits`、`/console/redemption`。
- 视口：1440x900、768x1024、390x844；覆盖明暗主题、USD/CNY/CUSTOM/TOKENS、用户福利、管理员活动/报表、券表/流水、兑换码/优惠码、选择/取消、批量栏、确认框和空态。
- 检查 `body.scrollWidth/clientWidth`、元素边界重叠和移动端可达性。
- 最终截图：
  - `output/playwright/marketing-benefits-default-1440.png`
  - `output/playwright/marketing-benefits-classic-1440.png`
  - `output/playwright/marketing-benefits-mobile-390.png`

## 源码合同复核

逐项复核金额换算使用内部 quota 真值、迁移只填充空字段且幂等、删除事务和并发锁、优惠码
reservation 回调、用户流水所有权、Default/Classic API 与状态矩阵一致、移动端和暗色主题。
发现业务缺陷时只记录 blocker，不越界修改 Go/TS/TSX/JS/JSX 实现。

### Blocker

1. `model/benefit_voucher.go:501-523` 的 `RefundBenefitVoucherAdditional` 未检查券是否已
   `voided`/`expired`，直接恢复余额并设置 `status=active`，可能在终止或过期后复活券。
2. `model/benefit_voucher.go:1368-1375` 的 `benefitActivityOverlapTx` 只统计其他活动，
   发布/恢复事务只锁当前活动；两个并发事务可能同时通过重叠检查，违反同组时间窗口约束。

上述 blocker 属 Task 1-9 业务实现，按本任务文件所有权未修改；在修复并补充并发/终态回归测试前，
不能判定发布就绪。

## 交付结论

本记录随 Task 10 最终提交一并交付；实际提交 SHA 以交付摘要为准。实际测试输出、截图尺寸/路径、阻塞项与残余风险已在上文补录。由于前端依赖/构建
产物缺失、真实路由无法启动、三张截图未生成，且存在上述两个高风险业务 blocker，本次不具备
发布条件；不包含任何部署授权。

## Task 10 最终复验（049a2de3d 之后）

本节覆盖后端修复提交 `049a2de3d` 后的最终验收，取代上文基于旧基线的阻塞结论。

### Blocker 状态

- 已修复：终态 `voided`/`expired` 福利券的 additional refund 与 settlement rollback 只写零额度审计流水，不恢复余额、已用额度或 `active` 状态；相关溢出保护和幂等回归测试已随 `049a2de3d` 合入。
- 已修复：福利活动发布与恢复事务通过 `lockBenefitActivityGroupTx` 锁定分组行，串行化同组重叠检查，避免并发事务同时通过窗口校验。
- 残余风险：上述行锁语义仅在本地 SQLite 测试中实际执行；本次没有可用的 MySQL 或 PostgreSQL 实例，未完成 live 并发验证。

### 本轮命令与结果

- `go test ./model ./controller ./router -count=1 -timeout 60s`：通过。
- `npm run test -- --run`（Default）：通过，91 个文件、414 个测试。
- `npm run typecheck`（Default/`tsgo -b`）：通过。
- `oxlint -c .oxlintrc.json src/features/benefits src/features/redemption-codes`：失败，仅有两个既有实现错误：`redemptions-mutate-drawer.tsx:105` 与 `promo-codes-panel.tsx:124` 的 `react(set-state-in-effect)`；本任务禁止修改业务实现。
- `oxfmt --check src`：失败，21 个既有业务文件格式不符合；未涉及允许写入的 locale/文档。
- `npm run copyright:check`：失败，52 个既有业务文件头需要更新；未越界修改。
- `npm run build`（Default Rsbuild）：通过。
- Classic 福利/营销码定向 `node --test`：通过，82/82。
- Classic scoped ESLint：通过（退出码 0）。
- Classic scoped Prettier：失败，6 个既有文件格式问题；未修改业务实现。
- `npm run build`（Classic Vite）：通过，16794 modules transformed。
- 两套前端构建后 `go test ./... -count=1 -timeout 60s`：通过。
- `git diff --check`：通过。

### i18n 复核

以 `fffda54f` 相对父提交新增键为集合：Default 140 键、Classic 149 键；每个列出的 locale 均为缺键 0、插值变量 0。已修正中文流水提示及 fr/ru/ja/vi 的混杂机器翻译，复核保留 Tokens、品牌名和 API 标识。同步脚本 `node scripts/sync-i18n.mjs` 成功执行，生成的临时报告目录已清理。Default `static-keys.ts` 与 locale 合同保持一致。

### 真实浏览器验收

使用 Paseo 原生 `browser_new_tab`、`browser_navigate`、`browser_snapshot`、`browser_click`、`browser_fill`、`browser_resize`、`browser_evaluate`、`browser_screenshot`，未使用 WSL/Playwright wrapper。临时本地 Go+SQLite 实例监听 `127.0.0.1:3000`，通过真实管理员会话渲染 Default：

- `/benefits`：真实空态、福利摘要、可用额度和活动福利 DOM；1440x900 检查 `scrollWidth=1440/clientWidth=1440`，标题边界无重叠。
- `/redemption-codes`：真实兑换码/优惠码/时效额度券标签、筛选、表格和空态 DOM。
- Classic dev server `http://127.0.0.1:4174/console/benefits`：真实活动福利 DOM；1440x900 无水平溢出。390x844 下在浅色与深色主题均渲染摘要、空态和移动导航；深色背景为 `rgb(22,22,26)`，`scrollWidth=375/clientWidth=375`，标题边界无重叠。
- Classic `/console/redemption` 在该次会话已过期并真实重定向 `/login?expired=true`，因此未将登录页冒充兑换码路由通过；这是浏览器验收残余 blocker。Default 代理登录曾受限流/会话差异影响，最终使用后端同源管理员会话完成真实 Default 路由验收。
- 本轮仅保存三张非空 PNG：`output/playwright/marketing-benefits-default-1440.png`（1440x900）、`output/playwright/marketing-benefits-classic-1440.png`（1440x900）、`output/playwright/marketing-benefits-mobile-390.png`（390x844）。

### 源码合同结论与发布边界

已复核金额换算以内部 quota 为真值、迁移幂等、软删除矩阵、三类营销码删除接口、支付 reservation 回调、用户流水权限、双模板 API/状态合同，以及移动端和暗色主题。发现 `web/src/features/benefits/lib/labels.ts:113` 的未知活动删除 reason fallback 会插入原始 `reason` 代码，可能对管理员显示内部代码；该业务实现不在 Task 10 写入所有权内，列为残余风险。未部署、未推送、未创建 PR、未合并；在补齐 Classic 兑换码会话验收、跨数据库 live 并发验证并处理上述基线 lint/格式问题前，不具备发布条件。
