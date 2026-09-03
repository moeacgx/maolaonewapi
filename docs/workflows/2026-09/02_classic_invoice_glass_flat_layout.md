# Classic 发票中心玻璃卡片与全宽表格

## 问题

Classic `/console/invoice` 复用了页面级 `classic-flat-page`，导致发票中心缺少业务模块的
透明玻璃卡片边界；表格使用 `scroll.x='max-content'`，在宽屏上只占内容宽度，页面右侧留出
大块空白，操作列也没有稳定贴边。

## 变更

- 发票中心页面改用 `classic-glass-card`，增加半透明背景、边框、阴影和 `backdrop-filter`，
  同时保持业务卡片与页面外壳的层级。
- 发票页面容器增加 `min-width: 0`，卡片宽度为 `100%`。
- 发票表格改用 `scroll={{ x: '100%' }}`；用户取消待支付和管理员处理操作列均固定到右侧。
- Default 模板未修改：本次截图对应 Classic `/console/invoice`，两套模板入口和样式独立。

## 兼容性与边界

- 不改变发票 API、支付方式、状态流转、权限或数据结构。
- 窄屏仍由 Semi Table 的横向滚动承载，不强行压缩订单号、时间和操作列。
- 仅新增 Classic 页面样式类，不改变其他使用 `classic-flat-page` 的通知中心和安全审计页面。

## 验证

- Classic 发票页面契约测试：7/7 通过。
- Classic 发票支付契约测试：4/4 通过。
- Classic 发票页面 ESLint：通过。
- Classic 发票页面及契约测试 Prettier：通过。
- Classic Vite 生产构建：通过。
- `git diff --check`：通过。
