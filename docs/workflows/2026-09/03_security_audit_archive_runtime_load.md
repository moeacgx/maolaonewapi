# 安全审计与对话归档页面持续打不开修复

## 问题

`.300` 发布后，Classic 对话归档和安全审计路由外壳可以返回 `200`，但登录后的原生入口或页面主体仍可能空白，表现为“页面打不开”。

## 根因

- Classic 原生 SDK 将登录前创建的 `API` 实例按值冻结到模块清单。登录刷新后 `updateAPI()` 替换实例，扩展仍使用旧实例，导致请求携带失效或空令牌。
- 对话归档页用 `Promise.all` 等待配置和分组；分组接口失败会阻塞全部页面。
- 安全审计页用 `Promise.all` 等待配置、运行状态和分组；非核心接口失败或挂起会阻塞主体配置和其他 Tab 的渲染。

## 修改范围

- Classic 原生 SDK 通过 getter 和 `getAPI()` 每次读取当前 `API` 实例。
- 对话归档配置作为唯一阻断项；分组失败显示可见错误但不阻塞归档列表和配置，并改用扩展专用 `/groups` 接口。
- 安全审计配置返回后立即渲染；运行状态和分组以独立 Promise 后台补齐，失败分别显示降级状态和可见提示。
- 配置刷新、运行状态轮询和组件卸载均受请求序号保护，旧响应不能覆盖新配置或编辑中的草稿；核心配置失败会明确提示分组未刷新。
- 新增 Node 原生契约测试，覆盖 SDK 实例更新、归档专用分组接口/降级和安全审计非阻断加载。

## 兼容性与安全边界

Default 前端、后端路由、数据模型和权限未修改。Classic 请求仍使用既有会话刷新、RootAuth 和 `skipErrorHandler` 契约；本次只修复客户端实例生命周期与加载隔离。

## 验证

- `web/classic`：`node --test scripts/conversation-archive-native.test.mjs scripts/native-sdk-live-api.test.mjs scripts/security-audit-load-data.test.mjs`
- `git diff --check`
- zzapi server `52`：三个应用容器健康运行 `.300`；根路径、`/console/security-audit` 和归档路由外壳均返回 `200`。未使用或展示生产凭据，受保护 API 的未授权响应为 `401`。

## 已知限制

当前环境未安装 Bun，无法在本机执行 Classic Vite 生产构建；发布前需在 CI 或含 Bun 依赖的环境执行 `bun install --frozen-lockfile && bun run build`。
