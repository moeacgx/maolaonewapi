# 返佣提现实际打款与收款信息展示

## 目标

返佣提现审核和用户提现记录不再展示难以浏览的原始收款 JSON。USDT 收款地址直接显示、标注区块链并支持复制；支付宝和微信显示账号及姓名。

## 金额契约

- 内部 `quota` 仍用于返佣余额冻结、驳回返还和已打款结算。
- 支付宝、微信的实际打款金额为 `quota / QuotaPerUnit * USDExchangeRate`，币种为人民币。
- USDT 先计算人民币基准金额，再除以 OKPay 当前配置的 USDT/CNY 汇率，币种为 USDT，保留 8 位小数。
- USDT 汇率优先使用已启用的 OKX 支付宝汇率内置模块；失败时回退 `OkpayUsdtCnyRate` 或 `OkpayExchangeRate`，并标记回退来源。两项配置都无效时拒绝提现，避免按错误汇率打款。
- 提交提现时锁定实际金额、汇率、来源和时间，历史申请不会随行情变化。

## 接口

- `POST /api/affiliate/withdraw/preview` 返回提交前预估金额，前端标注“预计实际打款（按提款时 OKX 汇率）”。
- `POST /api/affiliate/withdraw` 在服务端重新计算并锁定金额，不能信任预览结果。
- 用户和管理员提现列表返回 `display_amount`、`display_currency`、人民币基准金额、汇率及 `payout_details`。
- `payout_snapshot` 保留用于历史兼容和审计，但前端不再直接渲染原始 JSON。

## 兼容性与安全

- 旧提现记录仍可按原有 USD 字段显示；缺少结构化快照时前端降级为额度显示。
- 汇率、额度和 QuotaPerUnit 必须是正数且为有限值，异常配置直接拒绝提现预估或提交。
- 本次变更不修改公共美元汇率、返佣账务转移规则或生产环境部署。

## 验证

- `go test ./model ./controller -run 'Affiliate|OkpayRate' -count=1`
- `git diff --check`
- 前端应在具备 Bun 依赖的环境执行 `bun run typecheck`、受影响测试和构建。
