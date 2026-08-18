# Classic 面板会话认证兼容

## 问题

`.244` 后端登录接口返回短期 `access_token`、嵌套 `user` 和 refresh Cookie；
Classic 面板仍按旧协议读取顶层用户字段，并继续调用已移除的
`GET /api/user/logout`。结果是登录后用户角色和认证头为空，后台反复回到登录页，
旧退出接口还会产生 404。

## 修改范围

- 统一把新旧登录响应归一化为 Classic 用户对象，保留 `token`、角色和会话信息。
- Classic API 请求携带 refresh Cookie 和 Bearer access token。
- 收到 401 时只对原请求执行一次会话刷新，再重试原请求，避免刷新请求递归。
- 退出改用 `POST /api/user/auth/logout`。
- 2FA 登录提交后端要求的 `flow_token`。

## 安全边界与兼容性

刷新 Cookie 仅通过 HTTPS 同源请求发送；刷新失败不会伪造用户状态，也不会无限重试。
旧格式登录响应仍可被归一化，因此不影响仍返回顶层用户字段的部署。

## 验证计划

- Classic 前端构建和 ESLint。
- 认证归一化、登录、2FA 和刷新请求的定向测试。
- 合并到 `dev` 后，仅更新 `zzapi` 容器验证登录、后台保持和 401 刷新。
