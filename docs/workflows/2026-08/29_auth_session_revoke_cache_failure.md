# 密码重置会话撤销缓存故障修复

## 问题

密码重置先递增 `auth_version`，随后批量撤销用户会话。批量撤销为每个
活动会话写 Redis deny fence；当 Redis 写入失败时，旧实现立即返回，数据库
撤销事务未执行，活动行继续占用会话上限。

## 修改范围

- `revokeUserSessions` 保留逐会话 deny fence 优先顺序，但累计 fence 和
  撤销 tombstone 的缓存错误，仍继续执行当前批次的数据库撤销事务及后续批次。
- 返回值继续报告已撤销行的累计数量；缓存故障通过带 SID 的包装错误返回，
  便于调用方和日志审计，不静默吞掉错误。
- 密码重置回归测试模拟 deny fence Redis 故障，确认活动行变为 `revoked`、
  原因保持 `password_reset`，且错误不回显新密码。

## 兼容性与安全边界

数据库写入继续使用 GORM 事务和现有锁定查询，未引入 SQLite、MySQL 或
PostgreSQL 专用 SQL。deny fence 仍在数据库撤销前尝试，以优先建立 fail-closed
缓存状态；缓存不可用时，数据库撤销和错误返回共同保证会话不再计入活动上限，
但调用方应按错误处理缓存恢复或重试。

## 验证

- `go test ./model -run 'TestResetUserPasswordRevokesSessionsWhen(DenyFence|AuthCache)Fails|TestRevokeUserSessionsReturnsCumulativeProgressAndSupportsRetry' -count=1 -timeout=60s`
- `gofmt -w model/user_session.go model/user_update_test.go`
- `git diff --check`

本工作项只修改后端源码、测试和开发工作记录，未执行生产部署或重启。
