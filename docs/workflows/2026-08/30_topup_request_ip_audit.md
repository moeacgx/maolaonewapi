# 充值日志使用面板订单 IP

## 目标

恢复 v243 的充值日志语义：充值成功日志的 `logs.ip` 使用用户在面板创建
充值订单时保存的 `TopUp.RequestIP`，不使用支付系统 webhook 到达时的回调 IP。

## 根因

v243 的支付回调先完成订单，再通过 `RecordTopupOrderLog` 传入订单对象；
该函数从订单读取 `RequestIP`。后续支付完成逻辑改为直接调用
`RecordTopupLog`，将 webhook 的 `callerIp` 写入 `logs.ip`，因此管理员看到的
充值 IP 变成支付系统或反代回调链路的地址。

## 修改范围

- 充值成功日志新增订单级记录入口，优先读取 `TopUp.RequestIP`。
- EPay、Stripe、支付尝试统一使用订单 IP 写入 `logs.ip`。
- 回调 IP 仅写入 `Other.admin_info.callback_ip`，用于审计区分，不作为用户
  充值 IP。
- 管理员补单和 Waffo Pancake 充值也恢复订单级 IP 语义。
- 订单创建时已有的 `RequestIP` 采集逻辑不在本工作项内调整；反代可信代理
  配置属于独立问题。

## 数据契约与兼容性

- 新订单的 `RequestIP` 来自面板创建充值订单的请求上下文。
- 历史订单若没有 `RequestIP`，日志 IP 保持为空，不用 webhook IP 伪造用户
  地址。
- webhook 运行日志仍可记录回调 IP；只改变充值业务日志的 `logs.ip` 字段。
- 不修改数据库结构、订单状态、额度结算或支付回调验签逻辑。

## 回归验证

- `TestRecordTopupOrderLogUsesRequestIP`：订单 IP 与 webhook IP 不同时，日志
  `Ip` 使用订单 IP，管理员审计字段同时保留回调 IP。
- `TestRecordTopupLogDoesNotUseCallbackIP`：基础日志入口不会把独立回调 IP
  写入日志 IP。
- `TestCompleteTopUpPaymentAttemptLogsOrderRequestIP`：统一支付尝试完成路径记录
  订单 IP。
- `TestCompleteTopUpPaymentAttemptDoesNotBackfillRequestIPFromCallback`：历史订单缺失
  `RequestIP` 时不再用 webhook IP 回填。
- `TestManualCompleteTopUpLogsOrderRequestIPAndAdminIPSeparately`：管理员补单日志区分
  订单 IP 与管理员请求 IP。

验证命令：

```text
gofmt -w model/log.go model/topup.go model/topup_payment_attempt.go model/topup_request_ip_test.go
go test ./model -run 'Topup.*IP|TopupLog|ManualCompleteTopUpLogsOrderRequestIP|CompleteTopUpPaymentAttempt.*RequestIP' -count=1 -timeout 60s
go test ./model -count=1 -timeout 60s
go test ./controller -run '^$' -count=1 -timeout 60s
git diff --check
```

## 回滚与上线注意事项

代码回滚可恢复旧日志入口，但会重新引入 webhook IP 作为充值 IP 的风险。
本工作项不包含生产部署、历史日志回写或订单数据修复；上线前应在测试环境
确认订单 IP 与回调 IP 的分栏展示。
