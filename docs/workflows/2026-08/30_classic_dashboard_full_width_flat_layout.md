# Classic 数据看板全宽平铺修复

日期：2026-08-30

## 问题

Classic 数据看板在 2K 宽屏下仍然被 `classic-console-page-container`
的 `max-width: 1440px` 和 `margin: 0 auto` 居中收窄，所以主内容只占
中间一块，左侧留下明显空白。

## 修改

- `web/classic/src/index.css` 中的 `.classic-console-dashboard-container`
  改为 `max-width: none` 和 `margin: 0`。
- 保留通用 `classic-console-page` 外壳与 8px 水平内边距。
- 2026-08-30 后续修正已将公共 `classic-console-page-container` 同步改为全宽，
  Dashboard 不再依赖单页特例，个人设置、通知中心等共享外壳页面也不再居中收窄。
- 更新契约测试，明确校验 Classic 控制台公共容器不再被 1440px 宽度和自动居中限制住。
- 版本号随本次修复提升到 `v1.0.0-rc.10.1.10.279`。

## 兼容性与边界

- 影响 Classic 的共享控制台外壳页面，包括 Dashboard、个人设置、通知中心、
  发票中心、安全审计、兑换、营销福利和渠道可观测性中心。
- 不改 Default 模板、后端接口、数据库、权限、计费或生产配置。

## 验证计划

- 执行 Dashboard 相关契约测试。
- 执行 `web/classic` 的 lint 检查。
- 重新发版并在 zzapi 上验证页面宽度变化。
