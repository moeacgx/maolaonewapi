# 发票待开票通知兼容合并订单

## 问题

2026-08-31 发现新建的两张发票没有进入通知中心投递。检查发现通知中心仍定义了
`invoice_pending` 事件，但发票业务创建记录后没有再调用事件入队函数。该回归不只影响
合并订单，也影响单笔充值/订阅发票和外部支付成功后的待开票状态。

## 根因

`invoice_pending` 的统一入队函数及其调用点在后续上游合并中丢失：

- 单笔充值、订阅发票记录创建后未入队。
- 余额支付的合并发票创建后未入队。
- 合并外部支付创建时若仍为 `payment_pending` 不应通知；支付成功转为
  `pending` 后原本应入队，但调用缺失。
- 零服务费外部支付创建时已直接为 `pending`，同样因调用缺失而漏通知。

## 修改范围

- 恢复 `enqueueInvoicePendingNotificationTx`，统一生成事件负载和幂等键
  `invoice:<invoice_id>`。
- 在单笔发票记录、合并余额发票创建、零费/已支付状态创建及外部支付完成转
  `pending` 的事务内调用。
- 保持通知存储未迁移时静默跳过，不改变发票主事务结果。

## 兼容性与安全边界

- 合并发票使用 `source_type=batch`，事件金额使用合并记录的 `total_amount`，不再按
  来源订单拆成多条通知。
- 外部服务费未确认支付时不发送待开票通知，避免把未付款申请误报为待开票。
- 支付回调保持幂等，重复回调不会生成重复事件或投递。

## 验证

- `go test ./model -run TestCreateCombinedInvoiceWithBalanceChargesFeeAndPreventsReuse -count=1 -timeout 60s`
- `go test ./model -run "Test(DirectInvoiceRecordCreationIsIdempotent|CreateCombinedInvoiceWithBalanceChargesFeeAndPreventsReuse|CreateCombinedInvoiceExternalPaymentDoesNotChargeBalanceAndFreezesOrders|CreateCombinedInvoiceExternalPaymentCompletesZeroFeeImmediately|CompleteInvoiceExternalPaymentTransitionsAndIsIdempotent)$" -count=1 -timeout 60s`

以上命令均通过。回归测试覆盖单笔来源、合并余额、外部支付待付款/零费/支付完成及
重复回调场景。
