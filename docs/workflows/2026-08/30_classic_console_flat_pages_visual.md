# Classic 控制台页面视觉平铺

日期：2026-08-30

## 根因

此前 Classic 控制台统一了页面外层 `classic-console-page` 和
`classic-console-page-container`，但通知中心、发票中心和安全审计仍在共享容器内
渲染单个覆盖整页的 Semi `Card`。这些页面因此继续显示大面积卡片背景、边框和阴影，
与已平铺的 Dashboard 视觉契约不一致。

## 变更范围

- `NotificationCenter`、`Invoice`、`SecurityAudit` 的页面级 Card 增加
  `classic-flat-page` 专用类。
- Classic CSS 仅对 `.classic-flat-page.semi-card` 移除背景、边框和阴影，并对其
  根 Card 的直接 body 保留 16px 内容间距。
- 页面内部业务 Card、Tabs、表格横向滚动、Banner、保存按钮和 Modal Portal 结构
  保持不变。
- Dashboard、Redemption、Affiliate、PersonalSetting 的统一控制台外壳保持不变。
- 未修改 `web/src`、后端、生产配置或凭据。

## 复查修复：body 内边距选择器收窄

代码审查发现首版 `.classic-flat-page .semi-card-body { padding: 16px; }` 有两个问题：

1. **层叠中不生效**：`index.css` "semi-ui 组件自定义样式" 分区已有全局规则
   `.semi-card-header, .semi-card-body { padding: 10px !important; }`。该规则带
   `!important` 而新规则不带，无论声明顺序如何 10px 总是胜出；三个页面上同时设置的
   `bodyStyle={{ padding: 16 }}` 内联样式同样会被这条全局 `!important` 规则压过。
2. **后代选择器泄漏**：`.classic-flat-page .semi-card-body` 是后代选择器，会匹配
   `.classic-flat-page` 内部任意深度的 `.semi-card-body`。NotificationCenter 在同一个
   `classic-flat-page` Card 内的 Tabs 中渲染了任务、Bot 和最近通知三组独立的 Semi
   `Card`，旧规则会把这些内部业务卡片的 body 也一并改成 16px。

修复为直接子代选择器并加 `!important`：

```css
.classic-flat-page.semi-card > .semi-card-body {
  padding: 16px !important;
}
```

验证依据（`@douyinfe/semi-ui` `card/index.js` 源码）：传入 `Card` 的 `className` 与
内置 `semi-card` 一起挂在同一个根 `div`，`renderBody()` 产出的 `.semi-card-body` 是
该根 `div` 的直接子节点。因此新选择器结构上精确命中页面级根 Card 的 body；三个类
组成的特异性（0-3-0）高于全局规则的单类特异性（0-1-0），两者都带 `!important` 时
按特异性比较，新规则胜出且不依赖声明顺序；`>` 直接子代组合器保证不会再匹配
NotificationCenter 内部的任务/Bot/最近通知卡片。

## 视觉契约与兼容性

Dashboard 作为平铺参考页面；七个控制台页面继续使用统一的页面和容器外壳。
专用类只匹配页面级根 Card 的直接 body，不会覆盖其他 Classic 页面或内部业务 Card
（含 NotificationCenter 内嵌的任务/Bot/最近通知卡片）。发票表格仍使用
`scroll.x = 'max-content'`，通知中心和安全审计的 Tabs、Banner、Modal 以及各自的
内部 Card 继续由原组件负责布局。

## 测试与验证

- Node 契约测试已重命名为
  `web/classic/src/pages/__tests__/page-card-flat-contract.test.mjs`（原名
  `flat-layout-contract.test.mjs` 与 `pages/Dashboard/__tests__/` 下的同名测试冲突）。
  新增断言：body 覆盖规则必须是 `.classic-flat-page.semi-card > .semi-card-body`
  直接子代选择器、必须带 `!important`，且不得再出现 `.classic-flat-page` 与
  `.semi-card-body` 之间以空格连接的宽泛后代选择器。单文件运行 5/5 通过（较修复前
  4 个测试新增 1 个）。
- 相关回归：该契约测试 + Dashboard 两个契约测试 + NotificationCenter 两个测试 +
  Invoice 两个测试 + SecurityAudit 五个测试，`node --test` 合计 65/65 通过（其余
  60 个既有测试数量不变，65 = 60 + 本次 5；修复前基线为 64 = 60 + 4）。
- `git diff --check` 通过。
- Prettier 格式化已执行。验证时复用了同一仓库 `musing-quail` worktree 已安装的
  `web/classic/node_modules`；`index.css` 格式化前后无变化，测试文件按规则重排后
  仍 5/5 通过。
- Classic 标准生产构建已执行并转换 1985 个模块，但借用依赖中的
  `vite-plugin-semi` 无法解析 `semi-theme-default/scss/index.scss`，构建在依赖主题
  编译阶段失败。该失败发生在本次 CSS/JSX 变更之外，不能作为生产构建通过证据。
- 真实 Chromium 运行时验证已覆盖 1280x800 的通知中心、发票中心、安全审计，
  以及 375x812 的通知中心。四个视口的页面级根 Card 计算样式均为透明背景、
  `0px` 边框、`box-shadow: none`、body `padding: 16px`；页面均无横向溢出。
  截图使用本地模拟登录态，不包含真实账号、令牌或生产数据。

## 已知限制

- 本地截图使用模拟登录态且后端 API 未运行，因此只验证页面外壳、响应式几何和
  计算样式；真实业务数据、Modal Portal 层级和接口交互仍需在集成环境验收。
- 标准 Vite 生产构建尚未通过；需要在依赖完整且 Semi 主题解析正常的 CI 或干净
  安装环境中复验。
