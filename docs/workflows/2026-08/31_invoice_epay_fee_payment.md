# 发票中心易支付微信/支付宝手续费支付契约

## 目标

确认并固化发票中心使用易支付的微信和支付宝支付开票服务费的能力，避免后续支付设置、
Default 或 Classic 模板改动导致该流程回归。

## 当前实现结论

当前主线已经具备完整链路，不需要新增支付协议或单独的发票支付网关：

- `operation_setting.PayMethods` 默认包含 `alipay` 和 `wxpay`。
- `availableInvoicePayMethods` 在易支付配置和支付合规确认有效时，将管理员配置的方式
  返回到 `/api/user/invoice/config`，并标记 `provider=epay`。
- Default 的 `InvoicePaymentSelector` 和 Classic 的 `InvoiceBatchRequestModal` 都按
  `pay_methods` 展示选项，选择后提交 `payment_method` 到发票外部支付接口。
- 外部支付复用现有易支付 `Purchase`、异步回调验签、金额快照和幂等处理。

## 保护范围

- 只展示管理员实际配置且当前可用的支付方式，不在发票页面硬编码支付渠道。
- `alipay` 和 `wxpay` 作为易支付方式传递给网关；支付方式名称和颜色继续来自配置。
- 未完成的 `payment_pending` 申请不进入待开票通知；支付成功后才转为 `pending`。
- Default 与 Classic 是独立模板，二者都保留相同的后端请求契约。

## 验证

- `go test ./controller -run TestGetInvoiceConfigFiltersExternalPaymentMethodsByComplianceAndAvailability -count=1 -timeout 60s`
- `node --test web/classic/src/pages/Invoice/__tests__/epay-payment.test.mjs`
- `web/src/features/invoices/__tests__/payment-methods.test.ts` 已补充 Default 配置归一化契约；
  当前工作树缺少 Bun，无法运行该 Vitest 用例。
- `git diff --check`

未执行生产部署或支付网关真实扣款验证；真实支付仍需在测试网关或低风险环境中按现有回调
流程验收。
