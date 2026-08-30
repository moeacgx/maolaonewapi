# 返佣异常检测排除支付 IP

日期：2026-08-30

## 问题

返佣分成设置的异常检测会从 `user_ip_records` 计算邀请人与被邀请人的共享 IP。
该表包含 `action` 字段，但 `GetIPOverlap` 和 `GetIPOverlapBatch` 查询没有限制动作类型。
历史数据中如果存在 `payment`、`topup` 等支付动作，同一个支付平台 IP 会被误判为
共享用户 IP，并持久化到 `affiliate_fraud_alerts.shared_ips`。

深度扫描的业务日志查询已经排除 `LogTypeTopup`，本次补齐的是
`user_ip_records` 数据源的同等边界。

## 修改

- 返佣异常检测只使用 `login` 和 `register` 动作的 IP 记录。
- 登录与注册控制器复用 `UserIPActionLogin`、`UserIPActionRegister` 常量，避免动作名
  与检测白名单漂移。
- `payment`、`topup` 及其他非身份动作不参与单用户或批量 IP 重叠计算，也不会写入
  新的异常警报。
- 普通扫描与深度扫描都会使用该过滤。重新扫描时，仍处于 `detected` 状态的历史警报
  会按当前有效证据刷新；仅由支付动作产生的警报会被删除。

## 页面边界

Classic 与 Default 的返佣异常检测页面共用
`/api/affiliate/admin/fraud-alerts` 及扫描接口，因此无需分别修改前端。页面继续展示真实
登录/注册共享 IP。已经 `resolved` 或 `dismissed` 的历史记录属于既有管理审计，本次不做
自动改写；需要清理时继续使用现有删除操作。

## 兼容性与安全

- 不修改表结构、支付订单、充值日志、返佣金额、追回流程或邀请关系。
- 使用 GORM `IN` 条件，保持 SQLite、MySQL 和 PostgreSQL 兼容。
- 不删除 `user_ip_records` 原始数据，只限制返佣反欺诈读取范围。

## 验证

- 回归测试同时写入一个登录/注册共享 IP 和一个 `payment/topup` 共享 IP。
- 验证 `GetIPOverlap`、`GetIPOverlapBatch` 和最终
  `affiliate_fraud_alerts.shared_ips` 只保留登录/注册 IP。
- 执行 `go test ./model ./controller ./service -count=1 -timeout 60s`、vet、build 和
  `git diff --check`。
