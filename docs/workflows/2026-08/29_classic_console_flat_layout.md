# Classic 控制台独立页面主内容平铺修复

## 目标

统一 Classic 控制台独立页面的页面外壳，使固定顶栏下的主内容具有一致的顶部占位和水平内边距。2026-08-30 后续修正已取消公共容器的最大宽度和居中限制，避免宽屏下只铺在中间区域。

## 范围

- `web/classic/src/index.css`：新增 `.classic-console-page` 和 `.classic-console-page-container` 共用外壳。
- `Dashboard`、`NotificationCenter`、`Invoice`、`SecurityAudit`、`Redemption`、`Affiliate`、`PersonalSetting`：统一使用页面外壳与内容容器。
- 通知中心、发票中心和安全审计继续保留原有业务 `Card`、`Tabs` 及弹窗层级。
- 未修改 `web/src`、后端、生产配置、凭据或其他项目文件。

## 根因

各页面分别维护 `mt-[60px]`、`max-w-7xl`、`max-w-[1600px]`、`mx-auto` 和 `px-2` 等外层工具类，导致内容宽度和顶栏占位不一致。个人设置页还同时使用 `flex justify-center` 与内层 `mx-auto`，形成重复居中。PageLayout 的 Content 已提供控制台内边距，因此页面自身只需统一外壳职责。

## 布局契约

- `.classic-console-page` 以固定 64px Header 为基准。PageLayout Content 已提供桌面 24px、移动端 5px 内边距，因此页面外壳使用 `padding-top: calc(64px - Content padding)`：桌面实际为 40px、移动端实际为 59px，合计始终为 64px，不产生双重或不足留白。桌面端底部留白 24px，移动端底部留白 20px。
- `.classic-console-page-container` 使用 `width: 100%`、`max-width: none`、`margin: 0` 和 8px 水平内边距，让个人设置、通知中心等共享外壳页面按主内容区域全宽铺开。
- 页面业务内容放入容器；弹窗保持为页面外壳的兄弟节点，避免改变 Semi UI 的弹层行为。
- 内部业务卡片与 Tabs 不因外壳统一而拆除，既有数据请求、权限和交互契约保持不变。

## 兼容性与风险

这是 Classic 前端的布局层变更，不改变 API、请求参数、权限、计费或后端数据。审计页不再被外层最大宽度收敛；其内部 Card、表格和 Tabs 保持不变，超宽内容仍由组件自身滚动策略处理。

## 验证

- `node --test src/pages/Dashboard/__tests__/flat-layout-contract.test.mjs`：覆盖 64px Header 与 PageLayout Content 内边距协调契约，以及七页外壳/业务 Card 保留，单文件 3/3 通过。
- `node --test` 执行全部 Classic `*.test.mjs`：166/166 通过；Passkey 契约匹配允许 Prettier 合法的单行与多行调用格式，不再把空白排版误判为认证回归。
- `prettier --check ...`：本次涉及的 Classic 文件格式检查通过。
- `git diff --check`：通过。
- Classic Vite 构建仍受当前依赖树阻断：`vite-plugin-semi` 引用的嵌套 `@douyinfe/semi-icons` 无法解析 `@douyinfe/semi-theme-default/scss/index.scss`。该作业未通过，不能作为本次布局变更的构建成功证据。
