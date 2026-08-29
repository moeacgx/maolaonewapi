# Classic 登录会话上限恢复路径修复

## 目标

修复 v244 Classic 登录遇到 `409 AUTH_SESSION_LIMIT` 时只显示通用错误、且
Axios 全局拦截器与登录页重复弹窗的问题。保留服务端会话上限和认证边界，
为有效账号提供可操作的安全恢复指引。

## 根因

- 服务端在活动登录会话达到 `USER_SESSION_ACTIVE_LIMIT` 时拒绝签发新
  Session，并返回 HTTP 409、`code=AUTH_SESSION_LIMIT`。
- Classic 登录请求未跳过全局错误处理；Axios 拦截器先将 409 显示为
  `Request failed with status code 409`。
- `LoginForm` 的 catch 随后固定显示“登录失败，请重试”，覆盖了业务原因。
- 现有新前端已经按错误码给出恢复建议，但 Classic 没有对应映射。

## 修复范围

- Classic 密码登录请求设置 `skipErrorHandler`，避免登录页与 Axios 拦截器
  重复弹窗。
- 新增稳定错误码映射：Classic 显示带有 `AUTH_SESSION_LIMIT` 的明确提示，
  同时说明两条恢复路径：在仍登录的设备上退出其他会话；没有旧设备时，
  通过“忘记密码？”进入邮箱重置流程。
- `AUTH_SESSION_ISSUANCE_LIMIT` 同步提供滚动窗口提示，避免将另一种 429
  限制误报为密码错误。
- 保持后端 409 和错误码不变，不提高或取消会话上限，不自动踢出旧设备，
  不接受未认证请求管理会话。

## 安全恢复契约

“忘记密码？”路径使用现有 `/reset` → 邮件验证码 → `/api/user/reset` 流程。
只有持有账号邮箱的有效重置凭据才能完成密码重置；服务端重置密码时递增
`auth_version` 并撤销该用户全部旧登录会话，随后新密码登录会在原活动上限
下正常签发一个新 Session。密码重置不会清空签发窗口计数，因此不能借此
绕过短时间签发限流。

## 兼容性

- Classic 继续兼容旧的顶层用户字段和新的 `access_token`/`session.sid`
  认证包。
- 新前端认证契约、`AUTH_SESSION_LIMIT` HTTP 状态和响应字段不变。
- 非会话限制的登录错误仍使用原有通用提示。

## 验证

- `go test ./service -run 'Test(CreateLoginSessionEnforcesActiveLimitAcrossAuthVersions|PasswordResetRecoversLoginAfterActiveSessionLimit|PasswordResetDoesNotClearSessionIssuanceHistory)' -count=1 -timeout=60s`
- `node --test web/classic/src/classic-auth-session-compat.test.mjs`
- Classic 所有 locale JSON 解析检查。
- Prettier 检查通过（`npx --no-install prettier --check`）。当前工作树没有
  `web/classic/node_modules`，且未安装 Bun，因此无法执行 Classic 构建和依赖
  项目工具链的 ESLint；该限制已在交付说明中记录。
- `git diff --check`

## 当前状态

源码和行为级回归测试已完成，未执行生产部署或重启。PR 仅提交本工作项文件，
目标分支为 `custom-main`。

PR #112 在仓库保持私有时首次触发的 GitHub Actions 检查未实际启动；三个 job
的 annotation 均报告仓库账户近期付款失败或支出限额不足。经用户明确授权，
仓库临时公开后重跑的 Frontend typecheck/test（含 Classic build）和 Backend
vet/build/test（含 relaykit 独立构建）均通过；验证完成后仓库已立即恢复私有。
