# 异步图片任务与普通模型限流隔离

## 问题

升级后，API 令牌调用 `POST /v1/images/tasks` 以及 Canvas 异步任务时，
会收到“1 分钟内最多请求 N 次”的 429。这个响应来自 `ModelRequestRateLimit`，
不是后台 `/api` 路由使用的 `GlobalAPIRateLimit`。

## 根因

异步图片任务最初在 `e981ed42b` 中单独注册，没有普通模型请求限流。2026-08-17
的 `d3de1c0f5` 迁移 Default 模块时，将 `ModelRequestRateLimit` 加到
`/canvas/v1/images/tasks` 和 `/v1/images/tasks` 的提交链路，导致异步任务同时受到
普通模型限流和异步准入限制。

## 修改

- 移除两个异步任务提交入口上的 `ModelRequestRateLimit`。
- 保留 `TokenAuth`、Canvas 会话认证、Prompt Audit、分组和模型权限校验。
- 保留 `ImageTaskAdmissionGuard` 的专用限制：每用户每分钟 12 次、每令牌每分钟
  8 次；活动任务上限为每用户 8 个、每令牌 4 个。
- 同步 `/v1` Relay 和 Canvas 的聊天、图片、音频、视频接口继续使用
  `ModelRequestRateLimit`。

## 令牌限速边界

当前代码没有可配置的“每个 API 令牌 RPM”字段。`ModelRequestRateLimit` 的系统设置
按用户 ID 计数，并支持全局、用户组和请求分组规则；令牌上的 `ModelLimits` 是模型
访问权限，不是 RPM。此次修复恢复异步入口的历史行为，不宣称已经新增通用的令牌
RPM 配置。

## 兼容性与安全

异步任务不会绕过认证、模型权限、敏感词审计、余额预扣或渠道并发上限。异步专用的
用户/令牌速率和活动任务数仍在多实例数据库准入流程中执行。同步请求的系统模型限流
保持原有行为。

## 验证

- `go test ./router -run 'TestAsyncImageTaskRouteUsesDedicatedAdmissionInsteadOfGenericModelLimit|TestFeatureRoutesRegisterExactlyOnce|TestCanvasImageTaskRouteHidesCrossUserTasks' -count=1 -timeout 60s`
- `go test ./controller -run 'TestCanvasRouteAsyncSubmissionReplaysAndSettlesWithoutToken|TestCanvasAsyncImageTaskSubmitRunsPromptAuditBeforeTaskInsert' -count=1 -timeout 60s`
- `git diff --check`

