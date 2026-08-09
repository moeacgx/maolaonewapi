# Classic 动态分块静态资源兜底边界

## 问题

Classic 安全审计页通过动态导入加载页面分块。其构建产物可能使用
`/assets/index-<hash>.js` 名称，而网页路由原先把任意未命中的
`index-*.js` 都视作过期主入口并返回当前主题主入口脚本。

浏览器因此把主入口内容当成安全审计动态模块加载，页面持续停留在
Suspense 加载状态，也不会发起安全审计 API 请求。

## 根因

静态资源兜底只按文件名前缀和后缀判断，没有区分：

- Default 与 Classic HTML 明确引用的主入口；
- Vite 生成的动态模块分块；
- 不再存在的未知旧资源。

`index` 不是可靠的主入口标识，不能用于宽泛改写。

## 修改

- 从两套内置 HTML 中解析各自明确引用的主入口 JS/CSS。
- 当前主题为 Classic 时，仅把 Default 的已知主入口替换成 Classic 主入口。
- 当前主题为 Default 时，仅把 Classic 的已知主入口替换成 Default 主入口。
- 当前主题入口、未知 `index-*.js`、动态分块和许可证文件均不回退改写。

该修改不新增接口、配置或数据库变更。

## 验证

- 单元测试覆盖 Default 到 Classic、Classic 到 Default 的 JS/CSS 入口替换。
- 单元测试覆盖未知安全审计动态分块、当前主题入口和 LICENSE 文件不替换。
- 两套前端生产构建后，通过固定本地流程验证
  `/console/security-audit` 能完成动态加载并请求 Root-only API。

## 回滚

回滚只涉及 `web-router.go` 的入口判断和对应测试。不要恢复按
`/assets/index-*` 前缀宽泛替换的行为，否则会再次破坏动态模块加载。
