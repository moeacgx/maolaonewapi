# Classic 数据看板全宽平铺修复

日期：2026-08-30

## 问题

Classic 数据看板在 2K 宽屏下仍然被 `classic-console-page-container`
的 `max-width: 1440px` 和 `margin: 0 auto` 居中收窄，所以主内容只占
中间一块，左侧留下明显空白。

## 修改

- `web/classic/src/index.css` 中的 `.classic-console-dashboard-container`
  改为 `max-width: none` 和 `margin: 0`。
- 保留通用 `classic-console-page` 外壳与 8px 水平内边距，不影响其他 Classic
  控制台页面。
- 更新 Dashboard 契约测试，明确校验它不再被 1440px 宽度和自动居中限制住。
- 版本号随本次修复提升到 `v1.0.0-rc.10.1.10.279`。

## 兼容性与边界

- 只影响 Classic 的 `Dashboard` 页面。
- 不改后端接口、数据库、权限或其他 Classic 页面布局。

## 验证计划

- 执行 Dashboard 相关契约测试。
- 执行 `web/classic` 的 lint 检查。
- 重新发版并在 zzapi 上验证页面宽度变化。
