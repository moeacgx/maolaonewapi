# 登录签发次数恢复 v243 兼容默认

## 问题

当前生产中部分自动化客户端没有复用登录后的 Session，持续使用密码登录，
在 24 小时内达到原默认 100 次后触发 `AUTH_SESSION_ISSUANCE_LIMIT`。这条限制
不是 v243 的行为，也不是活跃会话自动淘汰可以解决的问题。

## 方案

- `USER_SESSION_ISSUANCE_LIMIT` 默认值改为 `0`，表示关闭签发窗口限流，恢复
  v243 的默认登录行为。
- 配置正数时保留原有滚动窗口计数、全部状态计数和
  `AUTH_SESSION_ISSUANCE_LIMIT` 错误码，便于需要风控的部署显式启用。
- 活跃会话上限、最早会话自动淘汰、Redis deny fence、Refresh Token 轮换和
  密码变更后的旧会话失效逻辑均不回退。
- 负数配置仍回退到默认值 `0`；显式 `0` 合法且不会记录错误。

## 兼容性与边界

该变更只恢复 v243 的“默认不限制签发次数”语义，不恢复 v243 的旧 Cookie
Session 实现。当前服务端 Session 表和设备撤销能力继续保留。配置正数时，
签发窗口仍不得超过 revoked Session 保留期；窗口超长钳制是启动时无条件执行的
清理保护策略，即使限流关闭也会生效。

## 存储增长与清理边界

默认 `USER_SESSION_ISSUANCE_LIMIT=0` 只关闭“窗口内签发次数”的拒绝条件，
不会设置数据库总量上限，也不会停止 Session 行写入。持续重复登录的客户端仍
可能使 `user_sessions` 增长；服务端依靠过期/撤销 Session 的定时分批清理控制
长期存储边界。`USER_SESSION_ISSUANCE_WINDOW_SECONDS` 始终作为清理保护窗口：
清理不会删除仍处于有效窗口内的过期行，或尚未达到 revoked 保留期的撤销行。
因此默认关闭限流不等于无限期保留记录，也不等于数据库增长完全不受控；清理
任务的执行频率、批量大小和数据库维护能力仍会影响实际增长速度。

## 部署与回滚注意事项

部署前应确认自动化客户端已经复用登录后的 Session/Refresh Token。若客户端仍
每次请求都用密码登录，默认关闭限流会使其在服务端拒绝前持续创建记录，增加
存储和清理压力；客户端修复属于部署前风险缓解，不在本工作项内改造。

若需要回滚到旧默认 `USER_SESSION_ISSUANCE_LIMIT=100`，窗口内已积累的历史
Session 创建记录仍会计数，回滚后用户可能立即触发
`AUTH_SESSION_ISSUANCE_LIMIT`，并不会因配置回滚而清零。建议在回滚前显式设置
足够高的临时限额，或先等待签发窗口滚出旧记录，再恢复为 `100`；同时监控
`user_sessions` 清理积压和登录失败率，必要时分批错峰处理受影响用户。

窗口变量具有双重用途：它在 `USER_SESSION_ISSUANCE_LIMIT` 为正数时定义签发
计数窗口；无论限流是否启用，都定义过期/撤销 Session 清理保护边界。超出
`USER_SESSION_REVOKED_RETENTION_DAYS` 时启动会记录告警并无条件钳制实际窗口。

## 验证

- `go test ./service -run 'TestCreateLoginSession(V243DefaultAllowsIssuancePastLegacyLimit|EnforcesIssuanceLimitAcrossAllStatuses|IssuanceLimitDoesNotEvictActiveSession)' -count=1 -timeout=60s`
- `go test ./common -run TestInitUserSessionSettingsUsesPositiveFallbacksAndClampsWindow -count=1 -timeout=60s`
- `git diff --check`

本工作项未执行生产部署或重启。生产部署前应先确认自动化客户端是否改为复用
Session/Refresh Token，避免默认关闭限流后继续无限创建会话记录。
