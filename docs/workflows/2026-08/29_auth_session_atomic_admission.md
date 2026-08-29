# 登录会话上限原子准入

## 问题

登录流程原先分别统计活动会话和签发窗口数量，再调用 `CreateUserSession`
写入新行。并发登录可能同时读到同一个未满上限的计数，随后都成功写入，
突破 `USER_SESSION_ACTIVE_LIMIT` 或 `USER_SESSION_ISSUANCE_LIMIT`。

## 修改范围

- 新增 `model.CreateUserSessionWithLimits`，在同一数据库事务中锁定用户行，
  检查活动会话、签发窗口并创建新会话。
- 登录服务改用原子准入接口；普通 `CreateUserSession` 保持用于已有管理、
  清理和测试场景的无上限底层写入能力。
- SQLite 不使用不兼容的 `FOR UPDATE`；通过单写者事务及同进程用户级互斥
  保证并发请求不会重复通过计数。MySQL/PostgreSQL 使用 GORM v2 行锁，
  不依赖数据库专用语法。

## 接口与安全边界

- 会话上限、签发窗口配置和错误码不变；达到上限仍分别返回
  `AUTH_SESSION_LIMIT` 或 `AUTH_SESSION_ISSUANCE_LIMIT`。
- 用户状态和 `auth_version` 在锁内再次校验，密码重置或安全设置变更不会
  签发旧版本会话。
- 新行提交成功后才发布缓存；缓存失败不会回滚已提交的会话记录。

## 验证

- `go test ./service -run TestCreateLoginSessionConcurrentAdmissionHonorsHardLimits -count=1 -timeout=60s`
- `go test ./service -run 'Test(CreateLoginSession|PasswordReset|LoginSession|UserAuthVersion)' -count=1 -timeout=60s`
- `gofmt` 和 `git diff --check`

本工作项只修改源码、测试和开发记录，未执行生产部署或重启。
