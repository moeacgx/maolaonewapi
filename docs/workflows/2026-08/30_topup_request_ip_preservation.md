# 充值日志仅保留订单请求 IP

日期：2026-08-30

## 问题

充值订单在面板创建时会保存 `TopUp.RequestIP`。EPay、Stripe、统一支付尝试和
Waffo Pancake 完成订单时，如果历史订单的 `RequestIP` 为空，会使用支付 webhook
或管理员请求的 `callerIp` 回填。后续 `RecordTopupOrderLog` 再把该值写入
`logs.ip` 和 `other.admin_info.request_ip`，导致回调链路地址被误认为用户下单地址。

## 修改

- 删除支付完成与结算函数中的 `callerIp` / `callerIPs` 参数，支付 webhook 和
  管理员补单请求地址不再进入模型结算层。
- 订单原本保存了请求 IP 时继续原样记录；历史订单缺失时保持为空。
- webhook 或管理员请求地址不写入充值订单、`logs.ip` 或
  `other.admin_info`；支付平台 IP 不属于充值业务日志契约，也不保留为审计字段。
- 管理员补单仍由独立的 `LogTypeManage` 记录管理员操作审计；该日志不属于充值成功
  日志，也不修改 `TopUp.RequestIP`。
- 不修改订阅订单和余额购买流程；这些路径传入的请求 IP 本来就是用户操作上下文，
  不是异步支付回调地址。

## 保留的主线契约

- 保留 `balance_before`、`credited_quota`、`balance_after` 和
  `paid_amount_cny` 充值审计快照。
- 保留免费优惠码充值成功日志。
- 保留管理员补单和 Waffo Pancake 重复回调不重复写成功日志的幂等行为。
- 不改变支付验签、订单状态、额度结算、返佣、发票或数据库结构。

## 验证

- 表驱动回归测试覆盖 EPay、Stripe、统一支付尝试、旧 BEpusdt、Waffo Pancake 和
  管理员补单。每条路径同时验证：历史订单的 `RequestIP` 为空时保持为空；存在面板
  下单 IP 时，订单、`logs.ip`、`other.admin_info.caller_ip` 和
  `other.admin_info.request_ip` 均原样保留该值。
- TDD RED 阶段确认旧函数签名仍强制接收回调 IP；GREEN 阶段删除参数并通过测试。
  变异检查临时清空日志请求 IP 时，新测试会在六条路径的非空场景全部失败。
- 同时执行现有充值余额审计、实付金额、重复回调和管理员补单幂等测试。
- 执行 `go test ./model ./controller ./service -count=1 -timeout 60s` 和
  `git diff --check`。

## 兼容性与回滚

该修改不迁移或回写历史订单、历史日志。代码回滚会重新允许异步回调地址覆盖空的
订单请求 IP；不会影响已有非空 `RequestIP` 数据。
