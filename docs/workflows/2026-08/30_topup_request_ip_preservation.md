# 充值日志保留订单请求 IP

日期：2026-08-30

## 问题

充值订单在面板创建时会保存 `TopUp.RequestIP`。EPay、Stripe、统一支付尝试和
Waffo Pancake 完成订单时，如果历史订单的 `RequestIP` 为空，会使用支付 webhook
或管理员请求的 `callerIp` 回填。后续 `RecordTopupOrderLog` 再把该值写入
`logs.ip` 和 `other.admin_info.request_ip`，导致回调链路地址被误认为用户下单地址。

## 修改

- 删除四条支付完成路径对空 `TopUp.RequestIP` 的回填。
- 订单原本保存了请求 IP 时继续原样记录；历史订单缺失时保持为空。
- webhook 或管理员请求地址仍通过 `RecordTopupOrderLog` 的独立参数写入
  `other.admin_info.callback_ip`，不丢失管理员审计信息。
- 不修改订阅订单和余额购买流程；这些路径传入的请求 IP 本来就是用户操作上下文，
  不是异步支付回调地址。

## 保留的主线契约

- 保留 `balance_before`、`credited_quota`、`balance_after` 和
  `paid_amount_cny` 充值审计快照。
- 保留免费优惠码充值成功日志。
- 保留管理员补单和 Waffo Pancake 重复回调不重复写成功日志的幂等行为。
- 不改变支付验签、订单状态、额度结算、返佣、发票或数据库结构。

## 验证

- 新增表驱动回归测试，覆盖 EPay、Stripe、统一支付尝试、旧 BEpusdt 回调和
  Waffo Pancake：
  历史订单的 `RequestIP` 为空时，完成后订单与 `logs.ip` 仍为空，回调地址只存在于
  `other.admin_info.callback_ip`。
- 同时执行现有充值余额审计、实付金额、重复回调和管理员补单幂等测试。
- 执行 `go test ./model ./controller ./service -count=1 -timeout 60s` 和
  `git diff --check`。

## 兼容性与回滚

该修改不迁移或回写历史订单、历史日志。代码回滚会重新允许异步回调地址覆盖空的
订单请求 IP；不会影响已有非空 `RequestIP` 数据。
