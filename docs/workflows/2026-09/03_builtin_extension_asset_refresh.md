# 内置扩展同版本资源刷新修复

## 问题

`conversation-archive` 的内置扩展清单版本保持 `0.1.0`，但其原生入口资源已更新。
容器启动时，内置模块安装器仅比较 `manifest.json` 的版本；版本相同即跳过安装，导致
`/data/data/modules/conversation-archive` 继续提供旧入口。镜像内的源码与运行时扩展资源
因此不一致，Default/Classic 宿主可能加载到不匹配的 SDK 依赖并报“Required host SDK
modules are unavailable”。

## 修复

- 保留清单版本和现有模块启用状态的兼容契约。
- 内置模块在同版本且静态根目录、健康页均有效时，逐文件比对嵌入资源与安装目录。
- 任一内置文件缺失、不是常规文件或内容不同时，沿用既有临时目录和重命名替换流程刷新整个模块。
- 不匹配时不会直接覆盖运行目录；替换失败仍保留原目录。

## 验证

- `TestInstallBuiltinModulesRefreshesChangedAssetsAtSameVersion` 先将已安装的
  `conversation-archive/public/native/classic.mjs` 改为旧内容，确认安装器会恢复与嵌入资源
  完全相同的文件。
- 后续发布需在 `zzapi` 的三个应用容器完成滚动更新后，使用 Root 登录态分别打开 Default
  和 Classic 的对话归档页，确认原生入口加载和归档配置请求均成功。
