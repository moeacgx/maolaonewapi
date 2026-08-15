# 开票充值折扣策略

## 变更目标

支付设置的发票区域提供正向语义的“开票时享受充值折扣”开关。开关开启时，申请发票的充值沿用预设金额折扣、站内优惠码和 Stripe Checkout 促销码；开关关闭时，明确申请发票的充值不再使用这些折扣。未申请发票的充值与订阅购买保持原有行为。

## 配置与接口

- 底层配置键继续使用 `InvoiceDiscountDisabled`，布尔值默认 `false`，避免数据库迁移并保持现有站点行为。
- 界面采用反向映射：正向开关开启对应 `InvoiceDiscountDisabled=false`，关闭对应 `InvoiceDiscountDisabled=true`。
- `GET /api/user/topup` 与 `GET /api/user/invoice/config` 返回的 `invoice.discount_disabled` 表示底层禁用策略。
- Default 与 Classic 的支付设置均显示“开票时享受充值折扣”。
- 充值确认时若策略生效，两套前端均按后端重新计算的无折扣金额展示；Classic 同时禁用优惠码输入，Default 展示不可使用折扣的说明。

## 金额与订单契约

- Epay、Stripe、BEPUSDT、OKPay、Waffo、Waffo Pancake 的金额预览和正式下单共用同一判断。
- 策略生效时，预设金额折扣倍率固定为 `1`，站内优惠码不参与计算，也不写入订单优惠快照。
- Stripe 创建或重新创建 Checkout Session 时关闭促销码入口，防止支付平台再次修改金额。
- 订单通过 `invoice_discount_disabled` 固化创建时的策略；重新支付读取订单快照，不受管理员之后修改全局开关影响。
- 发票基础金额以无折扣的充值实付金额换算为 CNY，再按原有发票费用规则计算服务费；订单的 `OriginalMoney`、`ActualMoney`、`Money` 与发票金额快照保持一致。
- 充值分组倍率继续生效。它属于分组定价，不归入本开关控制的活动折扣。
- 历史已支付订单的合并开票仍按订单实际支付金额处理，不追溯修改历史订单。

## 安全与兼容性

- 后端依据请求中的 `invoice.required` 强制执行，旧客户端不能通过隐藏开关或伪造显示金额绕过。
- 配置继续存储在现有 `options` 表；`top_ups` 由 GORM AutoMigrate 新增布尔列 `invoice_discount_disabled`，默认 `false`，兼容 SQLite、MySQL 和 PostgreSQL。
- 正向界面开关开启时，所有充值金额、优惠码和 Stripe 促销码行为与变更前一致；关闭时仅禁止申请发票的充值折扣。
- 运行时刷新要求：`InvoiceDiscountDisabled` 虽以 `Disabled` 结尾，也必须按布尔 Option 刷新内存态；否则后台保存成功但本进程仍按旧策略计价。
- Creem 固定产品原本不支持充值开票或优惠码，不在本次变更范围内。

### Classic 设置页布尔值一致性

- Classic 支付设置从 `/api/option/` 读取的值均为字符串；`InvoiceDiscountDisabled` 必须与 `*Enabled` 配置一样先转为布尔值，再计算正向开关状态。否则字符串 `"false"` 在 React 中仍为真值，会反转真实策略并使保存差异比较漏掉变更。
- Classic 的通用布尔配置解析同时识别 `Enabled` 与 `Disabled` 后缀；正向表单字段 `InvoiceDiscountEnabled` 与底层 `InvoiceDiscountDisabled` 必须始终反向映射。
- 回归验证需覆盖底层 `"false"` 显示为开启、`"true"` 显示为关闭，并确认关闭正向开关会持久化 `InvoiceDiscountDisabled=true`，重新开启则持久化为 `false`。

## 验证计划

1. “开票时享受充值折扣”开启且申请发票时，原有金额折扣与优惠码继续生效。
2. 正向开关关闭且申请发票时，金额预览、正式订单和发票快照均不含折扣，优惠码快照为空。
3. 正向开关关闭但不申请发票时，折扣行为保持不变。
4. 验证六个可开票充值网关的预览和下单路径，并确认 Stripe Checkout 不允许促销码。
5. 验证 Default、Classic 设置开关、充值确认提示及多语言文案。
6. 执行相关 Go 测试、两个前端构建、i18n 同步和 `git diff --check`。
