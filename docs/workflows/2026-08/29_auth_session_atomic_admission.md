# 登录会话原子准入与旧会话淘汰

## 问题

登录流程原先分别统计活动会话和签发窗口数量，再调用 `CreateUserSession`
写入新行。并发登录可能同时读到同一个未满上限的计数，随后都成功写入，
突破 `USER_SESSION_ACTIVE_LIMIT` 或 `USER_SESSION_ISSUANCE_LIMIT`。同时，活跃会话
达到容量时直接拒绝新登录，导致用户必须手动找设备或重置密码恢复。

## 修改范围

- `model.CreateUserSessionWithLimits` 在同一数据库事务中锁定用户行，检查签发窗口；
  活跃会话达到容量时，按最后活跃时间、创建时间和 SID 的稳定顺序淘汰最旧会话，
  再创建新会话。
- 登录服务改用原子准入接口；普通 `CreateUserSession` 保持用于已有管理、
  清理和测试场景的无上限底层写入能力。
- SQLite 不使用不兼容的 `FOR UPDATE`；通过单写者事务及同进程用户级互斥
  保证并发请求不会重复通过计数。MySQL/PostgreSQL 使用 GORM v2 行锁，
  不依赖数据库专用语法。

## 接口与安全边界

- `USER_SESSION_ACTIVE_LIMIT` 仍限制同时保留的活跃会话数量，但不再产生
  `AUTH_SESSION_LIMIT`；达到容量时新登录会自动撤销最旧会话。
- `AUTH_SESSION_ISSUANCE_LIMIT` 保持不变，仍用于限制滚动窗口内的签发次数。
- 被淘汰会话使用 `revoked_reason=login_session_evicted`，并在 Redis 中发布撤销
  tombstone，避免旧设备继续使用缓存鉴权。
- 用户状态和 `auth_version` 在锁内再次校验，密码重置或安全设置变更不会
  签发旧版本会话。
- 新行提交成功后才发布缓存；缓存失败不会回滚已提交的会话记录。

## 验证

- `go test ./service -run 'Test(CreateLoginSession|PasswordReset|LoginSession|UserAuthVersion)' -count=1 -timeout=60s`
- `gofmt` 和 `git diff --check`

本工作项只修改源码、测试和开发记录，未执行生产部署或重启。
