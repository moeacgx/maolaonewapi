# Classic 控制台全局全宽平铺修复

日期：2026-08-30

## 问题

上一次只把 Dashboard 的 `classic-console-dashboard-container` 改成全宽，
但公共 `classic-console-page-container` 仍保留 `max-width: 1440px` 和
`margin: 0 auto`。个人设置、通知中心、发票中心、安全审计、兑换和营销福利等
共享外壳页面仍会在宽屏下被居中收窄。渠道可观测性中心没有接入公共外壳，
仍使用 `mx-auto mt-[60px] max-w-[1600px]`，同样不符合全局平铺要求。

## 修改

- `web/classic/src/index.css`：公共 `.classic-console-page-container` 改为
  `max-width: none` 和 `margin: 0`，保留 8px 水平内边距。
- `web/classic/src/pages/ChannelObservability/index.jsx`：接入
  `classic-console-page` 与 `classic-console-page-container`，移除页面自带的
  居中、顶部偏移和 1600px 最大宽度。
- 更新 Dashboard 与页面平铺契约测试，覆盖渠道可观测性中心，并断言共享容器
  本身就是全宽契约。

## 范围

- 仅修改 Classic 模板。
- 不修改 Default 模板、后端接口、数据库、权限、计费、生产配置或凭据。
- 已接入公共外壳的页面会全局生效：Dashboard、个人设置、通知中心、发票中心、
  安全审计、兑换、营销福利和渠道可观测性中心。

## 兼容性

这是布局层修复，不改变业务组件、接口参数、数据请求或权限判断。
内部业务 Card、Tabs、表格横向滚动、Banner 和 Modal 行为保持由各页面原组件负责。

## 验证

- `web/classic` 下执行 `node --test`：178/178 通过。
- `npx prettier@3.6.2 --check ...`：本次修改文件格式检查通过。
- 受影响 JSX 和测试文件执行 ESLint：通过；当前 worktree 未安装 `node_modules`，
  使用同批 worktree 的 Classic 依赖目录作为 `--resolve-plugins-relative-to`。
- `git diff --check`：通过。
