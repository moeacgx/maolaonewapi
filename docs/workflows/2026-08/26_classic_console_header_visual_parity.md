# Classic 控制台首页顶栏视觉对齐

日期：2026-08-26

## 问题

Classic `/console` 首页一直沿用全宽、左贴边的旧顶栏，而模型广场使用居中
最大宽度、浅色留白和暗色主题兼容的模板顶栏。两处入口在同一产品中表现为两套
视觉外壳。

## 修改范围

- `/console` 与 `/console/` 复用现有 `PricingTemplateHeader` 的品牌、导航、宽度和
  主题外壳；其他动态控制台路由继续走原顶栏。
- 模板顶栏接收独立的控制台侧栏入口。模板导航仍只由自身的 `mobileOpen` 管理，
  控制台侧栏状态由 `PageLayout` 唯一持有：移动端使用 `drawerOpen`，桌面端使用
  `collapsed`。两个汉堡按钮不会互相覆盖。
- Dashboard 使用 `classic-console-dashboard-*` 专属容器与 hero 样式，保留问候语、
  搜索、刷新、统计卡片和管理员面板，不复用模型广场页面结构或搜索框。

## 兼容性与边界

- 不修改动态路由、后端、API、权限判断、生产环境或远程配置。
- 首页模板分支显式允许桌面侧栏按钮；其他 `/console/*` 旧顶栏仍只在移动端展示
  抽屉入口。
- 控制台侧栏在桌面可折叠，在移动端继续打开/关闭抽屉；模板导航菜单保持独立。
- 此改动只涉及 Classic 展示层，不改变用户数据、统计数据或管理员内容的加载契约。

## 验证计划

- `node --test web/classic/src/components/layout/headerbar/__tests__/console-header-contract.test.mjs web/classic/src/pages/Dashboard/__tests__/console-shell-contract.test.mjs`
- `git diff --check`

Classic 的 Bun 依赖未安装，因此未运行需要 JSX/组件运行时的测试、Prettier 检查或
Vite 构建；本次未安装或变更依赖。
