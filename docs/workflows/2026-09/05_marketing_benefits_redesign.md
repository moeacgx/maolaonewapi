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
