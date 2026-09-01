# 营销福利重构 Task 10 最终验收记录

## 范围与禁止事项

- 基线：`ea09ff531`；后端修复已包含 `049a2de3d`，本分支未重复提交该修复。
- 本轮等价提交：`14152589c` cherry-pick 生成 `daa05a242`；未知 reason 文案、版权头和
  Classic 格式化分别为 `f5410fe0b`、`212a0ab49`、`aa9083a20`。
- 范围：Default/Classic locale、开发文档、工作流记录、全量验证和真实路由浏览器验收。
- 未部署、未推送、未创建 PR、未合并；未连接 `zzapi`、`maolaoapi` 或任何真实数据库。

## i18n 复核

按 `i18n-translate` skill 执行。所有 locale 写入均由临时
`web/scripts/add-missing-keys.mjs` 完成，随后执行 `node scripts/sync-i18n.mjs`；临时脚本和
同步报告目录已删除，未直接编辑 locale JSON。

用户补充的实际英文 `t('literal')` 调用点已在生产福利/营销码模块中复现：Default 各自调用点
缺失 8 个，Classic 各自调用点缺失 15 个。同步到双模板全部 locale 后，两套相对
`fffda54f1` 的新增键集合各为 23 个；其中 Default 补齐 Classic 已有的 15 个交叉模板键，
Classic 补齐 Default 已有的 8 个交叉模板键（含 `zh-CN`），并复核插值变量和专有名词。

- 实际调用点缺键：Default `0`、Classic `0`。
- Task 10 新增键集合插值不一致：Default `0`、Classic `0`。
- Task 10 新增键集合缺键：Default `0`、Classic `0`。
- 相对 `fffda54f1` 的新增键集合：Default `23`、Classic `23`；其中 Default 本轮补齐了
  Classic 已有的 `15` 个交叉模板调用键，Classic 本轮补齐了 Default 已有的 `8` 个交叉模板
  调用键。`static-keys.ts` 与 Default locale 合同保持一致。
- 全文件历史扫描仍会命中既有非任务 key 的插值差异（Default 6 条、Classic 342 条，含
  i18next `_one/_other` 复数键和旧中文直写 key）；它们不属于本轮新增集合，未当作通过项。

## 命令验证

所有 Go 测试均使用 `-timeout 60s`。命令使用工作树现有 node_modules，未安装新包或修改锁文件。

### 后端

- `go test ./model ./controller ./router -count=1 -timeout 60s`：通过（19.134s、3.148s、
  3.101s）。
- 两套构建后 `go test ./... -count=1 -timeout 60s`：全包通过。

### Default

- redemption-codes Vitest：9 个文件、50 个测试通过。
- benefits Vitest：6 个文件、44 个测试通过。
- 全量 Vitest：91 个文件、420 个测试通过。
- `tsgo -b`：通过；scoped `oxlint`：0 error/0 warning；scoped `oxfmt --check`：50 个
  TS/TSX 文件通过。
- Rsbuild：通过，12.2s。
- copyright check：退出码 1，仅报告仓库既有 31 个范围外文件头；福利版权头已由
  `212a0ab49` 覆盖，检查未写文件。

### Classic

- 福利、兑换码、优惠码明确目录执行 `node --test`：82 个测试通过。
- scoped ESLint：通过。
- scoped Prettier：退出码 1，恰为 5 个 `.mjs` 测试文件的 CRLF checkout 行尾报警：
  `user-benefits-contract.test.mjs`、`benefit-contract.test.mjs`、
  `activity-management-contract.test.mjs`、`bulk-delete-contract.test.mjs`、
  `usePromoCodesData-contract.test.mjs`。`core.autocrlf=true` 下源码 token/格式无差异，未越界修改。
- Vite build：通过（16794 modules transformed；仅有既有 lottie `eval`、chunk size 和
  Browserslist 提示）。
- 首次模糊路径筛选误将工作树名 `marketing-benefits-final-qa` 匹配为测试范围，混入两个
  headerbar 测试，结果 274 通过/2 个文件加载失败；改用明确目录后 82/82 通过。

### 收口检查

- 两套 build 后 `go test ./... -count=1 -timeout 60s`：全包通过。
- `git diff --check`：通过。

## 真实浏览器验收

使用 Playwright CLI（Windows 原生 Node，未用 WSL wrapper）。两套前端均运行在
`http://localhost:5173`，后端为临时二进制 + SQLite，管理员为临时 `task10admin`。
Classic 最终 URL 为真实登录路由，不是 `/login?expired=true`。

### 真实后端会话与空态

- Default `/benefits`：真实会话加载福利摘要、我的福利券、可领取活动空态；1440×900、
  768×1024、390×844 均满足 `body.scrollWidth === clientWidth`。
- Default `/redemption-codes`：真实路由加载兑换码、优惠码、时效额度券标签和空态；真实创建
  抽屉可打开、Escape 关闭并重新打开。
- Classic `/console/benefits`：真实 SQLite 会话显示福利摘要和两块空态面板，桌面/移动无溢出。
- Classic `/console/redemption`：真实会话显示营销福利空态；“更多操作”菜单、创建兑换码抽屉
  可打开并由 Escape 关闭。

### 浏览器内 API fixture

Default 兑换码页面另外使用 `page.route` 注入两行 `Preview USD`/`Preview CNY`，仅用于稳定
覆盖真实 React 路由组件的选择/取消、批量栏和确认框，不代表真实后端数据联调，未写入 SQLite。

- fixture 页面显示 `$10` 和 `$2`；选择后出现批量栏，删除确认框显示“相关充值日志、到账
  额度和返佣来源不会受到影响”，随后取消。
- 三个营销标签切换成功；Classic 真实空态覆盖失效清理聚合菜单。
- USD/CNY/CUSTOM/TOKENS 精度、当前展示单位与 Tokens 整数约束由 Default 44/50 测试及
  Classic 82 个合同测试证明；fixture 不伪造后端换算结果。

### 主题、视口与截图

- Default：1440×900、768×1024、390×844；深色移动背景 `lab(11.26 0 0)`，均无水平溢出。
- Classic：1440×900、390×844；浅/深主题均验证，深色背景 `rgb(22, 22, 26)`；移动
  `390/390`、桌面 `1440/1440`。
- 三张真实组件截图均由 `page.screenshot` 生成，并用标准图像查看器复核可读、非黑屏、无扫描线：
  - `output/playwright/marketing-benefits-default-1440.png`：1440×900。
  - `output/playwright/marketing-benefits-classic-1440.png`：1440×900。
  - `output/playwright/marketing-benefits-mobile-390.png`：390×844。

## 源码合同与风险

- 内部 quota 是计费真值；页面按当前 USD/CNY/CUSTOM/TOKENS 展示，货币 0.01、Tokens
  整数；领取门槛为 CNY 实付快照，API/表单按当前单位回显。
- 固定乘积、随机区间、迁移幂等、软删除矩阵、三类删除接口、payment reservation 回调、
  用户流水所有权和管理员字段隔离均已由源码与测试复核。
- `049a2de3d` 已修复终态券 additional refund/settlement rollback 不恢复余额，以及 Group
  行锁串行 publish/resume；定向测试通过。
- 未知 reason 使用固定文案；双模板 API/状态/权限合同一致，移动和暗色有浏览器证据。
- 残余风险：未连接 MySQL/PostgreSQL，Group 行锁没有 live 跨数据库并发验证；Classic
  5 个 `.mjs` Prettier 报警为 checkout 行尾；Default copyright 为范围外基线问题。

## 发布边界

本 Task 仅提交允许范围内 i18n、文档和截图；没有部署、推送、PR 或合并。因 MySQL/PostgreSQL
live 并发验证尚未完成，当前结论是“代码与本地验证通过，但不具备直接发布条件”。
