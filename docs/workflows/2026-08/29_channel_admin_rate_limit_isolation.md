# 渠道管理 API 限流隔离

## 目标

外部管理员、Root 和管理 PAT 高频调用 `/api/channel` 写接口时，不再与普通 API
请求共享按 IP 的全局 GA 桶；未认证和普通用户仍必须经过现有鉴权与权限校验。

## 行为

- 仅已注册的 `/api/channel` 管理写路由使用现有 Dashboard 凭证分类器确认用户角色，
  包括 CRUD、状态、标签、批处理、Codex/Ollama、模型抓取、复制、多密钥和上游更新。
  只有已确认的 Admin/Root 凭证跳过 GA；无效、缺失或普通用户凭证继续进入 GA 限流，
  随后由 `AdminAuth`/`RequirePermission` 返回 401/403。
- `/api/channel/:id/key` 的 RootAuth、SecureVerification 和 `CriticalRateLimit` 语义不变。
- 唯一例外是 `POST /api/channel/:id/key`，它虽使用 POST 但语义上只是读取密钥，继续
  经过 GA 保护；未注册或未知路径同样不会获得豁免。
- `/v1` Relay 的 `ModelRequestRateLimit` 不变，普通 `/api` 请求仍共享 GA。

## 兼容性与风险

该隔离作用于已注册的渠道管理写路由，不改变渠道读接口、未知路径和
`POST /api/channel/:id/key` 的 GA 保护，也不按路径放行未认证流量。管理凭证分类会在
`AdminAuth` 前执行一次，鉴权失败不会获得豁免。多实例部署仍应使用共享 Redis；未启用
Redis 时，普通 API 的既有进程内限流行为保持不变。

## 验证

- `go test ./middleware -run TestChannelAdminBypassIsScopedToAuthenticatedWrites -count=1 -timeout 60s`
- `go test ./middleware -run 'TestChannelAdminBypassCoversManagementWritesButProtectsKeyRead|TestChannelAdminBypassRejectsUnprivilegedCredentials' -count=1 -timeout 60s`
- `go test ./router -run 'TestChannelStatusRoutesUseOperatePermission|TestSetApiRouterRegistersChannelRootWithAndWithoutTrailingSlash' -count=1 -timeout 60s`
- `go test ./middleware -run 'TestRedis.*RateLimiter|TestGlobalWebRateLimit' -count=1 -timeout 60s`
- `gofmt` 和 `git diff --check`
