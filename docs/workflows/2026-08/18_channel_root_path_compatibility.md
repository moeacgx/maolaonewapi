# 渠道根路径兼容性修复

日期：2026-08-18

## 问题

Default 前端渠道页调用 `GET /api/channel?p=...`，后端只注册了 `GET /api/channel/`，在 Gin 当前路由配置下不会自动重定向，返回 `404 Invalid URL (GET /api/channel)`。

同一根路径还影响 `POST /api/channel` 创建渠道；Classic 前端主要使用带尾斜杠的 `/api/channel/`，因此本地固定验证未覆盖无尾斜杠路径。

## 变更范围

- 后端 `/api/channel` 根路径同时接受无尾斜杠和有尾斜杠的 `GET`、`POST`、`PUT`。
- Default 前端渠道 API 使用带尾斜杠的列表和创建路径，保持与既有 Classic 调用一致。
- 增加路由注册测试，避免根路径兼容性再次回归。

## 安全与兼容性

不改变鉴权链路，仍复用 `channelRoute.Use(middleware.AdminAuth())`。不新增配置、数据表或迁移。外部客户端可继续使用 `/api/channel/`；无尾斜杠 `/api/channel` 作为兼容路径返回同一处理结果。

## 验证计划

- 执行路由注册测试，确认 `GET/POST/PUT /api/channel` 和 `/api/channel/` 都存在。
- 复用固定本地测试环境，登录 root 后请求 `/api/channel?p=1&page_size=10` 与 `/api/channel/?p=1&page_size=10`，确认均返回 200。
- 执行 `scripts/local-test.ps1 -Action verify`。
- 执行 `git diff --check`。

## 验证结果

- `go test ./router -run TestSetApiRouterRegistersChannelRootWithAndWithoutTrailingSlash -count=1 -timeout 60s`：通过。
- `node node_modules/typescript/bin/tsc -b && node node_modules/@rsbuild/core/bin/rsbuild.js build`（`web/default/`）：通过；本机未找到 `bun`，Rsbuild 仅提示既有 `channel-observability/index.tsx` 未导出 Route。
- `scripts/local-test.ps1 -Action start`：通过，重建后端并验证 `/api/status`、classic 首页、root/demo 登录和演示数据接口。
- Default 主题浏览器验证：登录 root 打开 `http://localhost:3000/channels`，渠道表正常渲染 3 条数据；页面实际请求为 `/api/channel/?tag_mode=false&id_sort=false&p=1&page_size=20`，未再触发无尾斜杠 404。
- 登录 root 后浏览器内直接请求 `GET /api/channel?p=1&page_size=10`：返回 HTTP 200，`success=true`，`items=3`。
- 登录 root 后浏览器内直接请求 `GET /api/channel/?p=1&page_size=10`：返回 HTTP 200，`success=true`，`items=3`。
- `scripts/local-test.ps1 -Action verify`：通过。
- `git diff --check`：通过；仅输出既有 i18n locale 文件 CRLF/LF 提示。
