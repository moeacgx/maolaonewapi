# 登录会话达到活跃上限时自动撤销最早会话

## 目标

当用户登录会话达到 `USER_SESSION_ACTIVE_LIMIT` 时，普通满员登录不再直接
返回 `AUTH_SESSION_LIMIT`。服务端自动撤销同一用户最早的未过期 active 会话，
再为当前登录签发新会话，使用户无需先回到旧设备手动退出。

## 行为契约

- 踢出候选严格限定为同一 `user_id`、`status=active`、`expires_at > now` 的
  会话；按 `created_at ASC` 排序，并以 `sid ASC` 作为稳定的并列排序键。
- 候选筛选不限制 `user_auth_version`。密码重置或安全设置变更后遗留的旧鉴权
  版本 active 行仍会占用活跃上限，也可以被本流程自动清理。
- 自动撤销写入 `status=revoked`、`revoked_reason=login_session_auto_evicted`。
  在数据库提交新会话前，必须先为被踢会话发布 Redis deny fence；deny fence
  发布失败时本次登录失败且不创建新会话，维持 fail-closed 语义。
- 新会话缓存写入继续沿用既有登录语义。缓存写入失败不会额外放宽活跃上限，
  也不会把已成功提交的会话回滚成未签发状态。
- `USER_SESSION_ISSUANCE_LIMIT` 默认为 `0`，兼容 v243 的不限制签发次数行为。
  配置为正数后，按 `USER_SESSION_ISSUANCE_WINDOW_SECONDS` 的有效窗口统计全部
  创建记录（含 revoked 和旧鉴权版本），达到上限时返回
  `429 AUTH_SESSION_ISSUANCE_LIMIT`。自动撤销不能绕过已启用的签发窗口限制。
  `USER_SESSION_ISSUANCE_WINDOW_SECONDS` 即使在限流关闭时也用于过期/撤销
  Session 清理保护；启动时超出 revoked 保留期会无条件钳制实际窗口。
- `AUTH_SESSION_LIMIT` 仅保留为没有足够候选可原子撤销等极端准入失败路径；
  deny fence 发布失败按缓存故障返回，且不创建新 Session。普通活跃会话满员
  不应再返回 `AUTH_SESSION_LIMIT`。

## 并发与数据库兼容性

活跃计数、候选选择、旧会话撤销和新会话插入在同一用户级准入锁/事务内完成。
SQLite 使用单写者事务和进程内用户锁，MySQL/PostgreSQL 使用现有
`lockForUpdate(tx)` 行锁；不引入数据库专用 SQL。并发登录可以有多个请求最终
成功，但数据库提交后的未过期 active 行数始终不超过配置上限；只有在签发限额
配置为正数时，总创建量才受签发窗口限制。窗口值本身始终参与清理保护，超长
钳制不以限流是否启用为条件。

## 安全边界与已知风险

Redis deny fence 是旧会话失效的缓存先行屏障。若已启用 Redis 但 fence 发布失败，
服务端宁可拒绝本次登录，也不在旧会话仍可能被缓存接受时创建新会话；未启用
Redis 时会话校验直接回源数据库，不需要该缓存屏障。跨版本升级或集群滚动
期间偶发重新登录属于独立运维现象；本工作项不执行生产排障、服务器登录、容器
重启或部署。

## 验证计划

- `USER_SESSION_ACTIVE_LIMIT=1` 时第二次登录成功，第一次会话变为 revoked，
  原因是 `login_session_auto_evicted`。
- 跨 `user_auth_version` 的旧 active 残留会被纳入候选并清理。
- 并发登录最终 active 数不超过上限；签发窗口在配置正数时限制短时间内的总创建量，
  限流关闭时仍保留窗口驱动的清理保护。
- deny fence 或缓存故障时不创建新会话，且 active 数不超过上限。
- 后端行为级测试、SQLite/MySQL/PostgreSQL 兼容路径、独立 relaykit 构建和
  `git diff --check` 均按交付命令记录。

本工作项不修改 Classic 视觉修复、支付渠道或 PR #123，也未执行生产部署。
