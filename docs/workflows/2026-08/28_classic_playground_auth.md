# Classic 操练场聊天认证修复

## 目标

修复 Classic 操练场在重新登录后，聊天请求仍持续返回 `401 AUTH_UNAUTHORIZED` 的问题。请求必须继续使用后端的用户访问令牌认证，不得通过放宽路由鉴权、接受 `sk-` 令牌或关闭认证来规避。

## 现状与根因

- 操练场固定调用 `/pg/chat/completions`。
- Classic 非流式请求使用原生 `fetch`，流式请求使用 `SSE`，两者此前只发送内容类型和 `New-Api-User`，没有发送 `Authorization: Bearer ...`。
- 登录流程会把规范化后的访问令牌写入 `localStorage.user`，但操练场请求没有读取这份最新状态。
- 后端 `/pg/chat/completions` 在进入操练场控制器前经过 `UserAuth`；缺少 Bearer 头时在认证中间件层直接返回 `401 AUTH_UNAUTHORIZED`，不会进入上游转发。
- 生产入口将请求轮询到 `maolaoapi-slave-1`、`maolaoapi-slave-2` 和 `maolaoapi`，三个实例均观察到同一路径的 401，未发现单实例漂移。

## 修复范围

新增可独立测试的请求头构造函数。每次创建非流式或 SSE 请求时读取当前 `localStorage.user`，兼容 `token` 与 `access_token` 字段，并仅在存在访问令牌时加入 Bearer 头。这样重新登录或刷新后不会复用旧闭包中的值。

本次不修改后端路由、中间件、Canvas 会话认证或 relay `sk-` 令牌语义；不修改数据库和业务数据。

## 兼容性与安全边界

- 未登录状态仍不伪造凭据，后端继续按既有规则返回未授权。
- `/pg/chat/completions` 仍只接受用户访问令牌；Canvas 继续使用其独立的会话入口。
- 访问令牌只从浏览器内存中的登录状态读取，不写入 URL、日志、测试报告或文档示例。

## 验证计划

- 单元回归：首次请求使用当前令牌；登录态替换后下一次非流式和 SSE 请求使用新令牌，不捕获旧值。
- Classic 相关 Node 测试、前端构建和 ESLint/格式检查。
- 后端认证及路由 Go 测试，确认未改变 401 认证边界。
- 仅在测试通过且确认需要发布时，按 `maolaoapi-slave-1` -> `maolaoapi-slave-2` -> `maolaoapi` 顺序滚动，并逐实例检查 health、`/api/status` 与认证日志；PostgreSQL/Redis 不重启。

## 当前状态

代码与测试修复已完成，PR 待创建。生产未部署：当前任务没有单独的发布授权，且修复尚未合并到发布分支；如后续获准发布，必须按验证计划逐实例滚动并保留脱敏验收记录。
