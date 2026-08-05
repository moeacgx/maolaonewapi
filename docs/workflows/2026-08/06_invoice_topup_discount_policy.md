# 开票充值不享受折扣

## 变更目标

支付设置的发票区域新增“开票时不享受充值折扣”开关。开关启用后，用户在充值支付时明确申请发票，该笔充值不再使用预设金额折扣、站内优惠码或 Stripe Checkout 促销码；未申请发票的充值与订阅购买保持原有行为。

## 配置与接口

- 配置键：`InvoiceDiscountDisabled`，布尔值，默认 `false`，升级后不改变现有站点行为。
- `GET /api/user/topup` 与 `GET /api/user/invoice/config` 返回的 `invoice.discount_disabled` 表示当前策略。
- Default 与 Classic 的支付设置均在发票配置区域提供开关。
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
- 开关关闭时，所有充值金额、优惠码和 Stripe 促销码行为与变更前一致。
- Creem 固定产品原本不支持充值开票或优惠码，不在本次变更范围内。

## 验证计划

1. 开关关闭时，申请发票的充值仍可使用原有金额折扣与优惠码。
2. 开关开启且申请发票时，金额预览、正式订单和发票快照均不含折扣，优惠码快照为空。
3. 开关开启但不申请发票时，折扣行为保持不变。
4. 验证六个可开票充值网关的预览和下单路径，并确认 Stripe Checkout 不允许促销码。
5. 验证 Default、Classic 设置开关、充值确认提示及多语言文案。
6. 执行相关 Go 测试、两个前端构建、i18n 同步和 `git diff --check`。
