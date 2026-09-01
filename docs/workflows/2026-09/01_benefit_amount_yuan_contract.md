# 福利活动金额改为人民币元并自动换算 quota

## 问题

福利活动表单此前把预算、面额和领取门槛展示为“分”，并要求管理员同时填写
`total_quota`。这会把产品的基础金额与内部计费单位混为一谈，也会让 `$`/美分看起来
像是业务金额语义。

## 变更

- 管理端创建/编辑请求改用 `total_amount`、`fixed_amount`、`min_amount`、
  `max_amount`、`claim_paid_threshold`，单位统一为人民币元。
- 服务端用 decimal 解析并拒绝超过两位小数的金额；固定/随机面额的等式和范围校验
  仍按 0.01 元粒度执行。
- 服务端根据 `USDExchangeRate` 与 `QuotaPerUnit` 自动计算活动总 quota，并按金额
  比例分配到券份额；前端不再提供 `total_quota` 输入。
- 数据库现有 `*_cents` 字段保留用于历史数据和精确的小数位存储，但不再作为对外金额
  单位，也不表示美元或美分。
- Classic 与 Default 的表单、列表和金额文案统一显示 `元`，金额控件步进为 `0.01`。

## 接口契约

示例请求：

```json
{
  "amount_mode": "fixed",
  "total_amount": 7.5,
  "fixed_amount": 1.25,
  "total_count": 6,
  "claim_paid_threshold": 0
}
```

该请求会按站点汇率换算 `total_quota`，并在发布时生成每份券的内部 quota。管理
响应提供元金额字段用于编辑回显；`total_quota` 可随响应返回以兼容现有读取方，但
它是服务端计算的内部计费结果，不能由管理表单覆盖。

## 安全与兼容性

- 金额转换使用 decimal，避免浮点舍入导致预算不相等。
- 汇率或 quota 单位配置无效、换算超出 BIGINT 范围或结果为零时拒绝保存。
- 已有旧活动仍可读取；编辑草稿时前端会把旧 `*_cents` 响应转换为元金额。
- 活动发布后的关键金额字段继续冻结，状态和计费链路不变。

## 验证

- `go test ./controller ./model`
- Default 福利表单测试与 typecheck/build
- Classic 福利契约测试与 build
- `git diff --check`
