# 多实例认证、用户创建与 Classic 令牌限流回归

## 问题

多实例升级后，Classic 收到 401 时刷新会话使用了错误的字段：服务端会话标识是
`session.sid`，旧代码读取 `session.id`，导致刷新请求无法带上期望的会话标识。
同时 Classic 操练场仍逐个调用 `/api/token/{id}/key`，令牌较多时会触发每个请求的
关键接口限流，表现为 429。

注册和后台创建用户没有传 `group`，而用户模型创建钩子会解析分组实体；空值被查询为
不存在的分组，最终返回 `record not found`。

## 修改范围

- `User.BeforeCreate` 对空分组使用 `default`，显式非法分组仍返回错误。
- 公开注册和后台创建用户显式设置默认分组；新版用户表单创建请求同步发送 `group`。
- Classic 刷新会话优先读取 `session.sid`，保留旧 `session.id` 兼容。
- Classic 获取活动令牌 key 改为一次调用 `POST /api/token/batch/keys`，不改变返回 key
  的顺序和过滤规则。

## 接口与安全边界

- 会话刷新仍由 refresh Cookie 和可选 `X-Auth-Session` 共同约束，不放宽服务端会话校验。
- 批量 key 接口仍受用户归属校验、关键接口限流和最多 100 个 ID 限制。
- 显式填写不存在的用户分组仍失败，只有省略或空白分组才回退到 `default`。
- 多实例部署仍必须共享同一主数据库、`SESSION_SECRET` 和 Redis 配置；本次修改不替代
  部署侧配置检查。

## 验证

- `go test ./model -run TestUserBeforeCreateDefaultsBlankGroup -count=1`
- `node --test web/classic/src/classic-auth-session-compat.test.mjs`
- `bun run test -- web/src/features/users/lib/user-form.test.ts`
- `git diff --check`

生产容器当前仍运行 `v1.0.0-rc.10.1.10.262`，本工作项只修改源码和测试，未执行生产重启或部署。
