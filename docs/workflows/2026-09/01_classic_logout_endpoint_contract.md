# Classic 退出登录接口契约修复

## 问题

Classic 顶栏退出登录仍请求历史接口 `GET /api/user/logout`。当前认证会话实现只提供
`POST /api/user/auth/logout`，因此退出操作无法进入统一的会话撤销流程，页面会显示请求失败。

## 修改范围

- 仅修改 Classic 模板顶栏退出登录调用，将请求改为 `POST /api/user/auth/logout`。
- 保留后端 `SessionCookieOriginGuard`、Refresh Cookie 和 Bearer 会话撤销边界，不恢复旧的 GET 路由。
- 新增契约测试，确保 Classic 不再调用已退役的旧路径。

## 兼容性与安全边界

退出请求继续携带现有认证客户端的凭据，并由后端撤销当前登录会话和匹配的 Refresh Cookie。
Secure Cookie 模式下仍须满足服务端配置的 Origin 校验；本次修复不放宽该安全检查。
Default 模板已使用当前 POST 接口，不在本次改动范围内。

## 验证

- 使用 Classic 专属 Node 原生测试验证请求方法和路径。
- 执行 `git diff --check`。
- 运行 Classic 代码格式检查，确认修改文件符合现有格式。
