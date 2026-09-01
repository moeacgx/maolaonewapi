# 本项目二次开发能力

本页登记可复用的二次开发能力及其稳定性边界。长期专题文档负责接口和行为契约，
`docs/workflows/` 负责单次问题的根因、变更和验证记录。

## 福利营销时效额度券

- 文档：[福利营销时效额度券](benefit-vouchers.md)
- 稳定性：已实现后端模型、接口、组合计费、流水和 Default/Classic 页面；两套模板共享
  API、权限、状态和金额语义，组件实现与构建链路分别维护。
- 金额边界：内部 quota 是计费真值；页面按当前 USD/CNY/CUSTOM/TOKENS 展示，货币精度
  为 0.01，Tokens 为整数。领取门槛始终是 CNY 实付快照，API/表单仅按当前展示单位回显。
- 删除边界：福利活动、兑换码、优惠码均使用管理员鉴权、关键操作限流和 GORM 软删除；
  单条/批量接口返回 `deleted_ids` 与逐项 `skipped`，保留券、流水、充值、支付和审计。
  优惠码已有 payment reservation 允许通过 `Unscoped` 回调结算，删除后禁止新 reservation。
- 关键契约：福利券独立余额；显式分组才抵扣；福利券 -> 订阅 -> 钱包；请求总价仍受
  token 上限约束；所有差额和回滚按 `request_id` 幂等。组合结算补偿使用独立
  `settle_rollback` 流水和原 `request_id`/类型组合键，日志 breakdown 关联
  `activity_id`/`voucher_id`/`request_id`/`log_id`。
- 已知限制：分组并发为单实例进程内限制；活动不自动复制渠道/分组；本期不覆盖图像和
  视频异步任务；Default 前端时间输入显式按 `Asia/Shanghai` 转换为 Unix 秒。管理 HTTP
  接口使用服务端时间，忽略请求体 `now`；个人券有效期对外按小时传输，数据库内部仍按
  秒保存并兼容旧秒字段；模型层时间参数仅供内部测试注入。
- 历史清理：福利活动批量删除仅归档可安全删除的 `draft`/`ended`/`terminated` 状态；
  `published`、`paused` 或仍有 active 券的活动会跳过。兑换码、优惠码批量删除和失效清理
  沿用软删除并保留账务关联。三类资源接口均限制最多 500 个 ID，仅管理员可调用，返回
  实际删除和跳过原因；重复请求幂等。
- 迁移/回滚：额度配置迁移仅填充空字段，按历史 share quota 或旧人民币语义转换一次，
  异常即停止且不补零；回滚代码保留新表、新列和历史审计，不物理删除数据。本次 Task 10
  未部署、未推送、未开 PR，发布前仍需重新确认目标实例与回滚方案。

## 扩展模块

扩展模块的宿主、权限和通知契约见 [扩展模块开发](extensions.md)。新增扩展页面时，
同时登记入口、所需角色、API 和回滚方式。

## 通知中心

通知事件、模板变量和投递边界见 [通知中心与模块事件](notifications.md)。新增通知事件
时必须补充事件类型、权限、失败重试和敏感字段处理。

## 发票中心

发票中心支持用户选择近 30 天内符合条件的充值或订阅订单申请发票。开票服务费必须
使用已配置的外部支付方式；零服务费申请不产生实际支付。

### 开票服务费支付

- 管理员在支付设置的 `PayMethods` 中配置易支付方式。默认配置包括 `alipay`（支付宝）
  和 `wxpay`（微信）；发票中心不提供账户余额支付。
- 易支付地址、商户 ID、商户密钥和支付合规确认均满足条件时，发票配置接口返回已配置
  的易支付方式。
- Default 与 Classic 发票中心读取 `/api/user/invoice/config` 的 `pay_methods`，按
  配置展示支付选项，并将所选类型提交到 `/api/user/invoice/payment`。
- 外部支付订单先进入 `payment_pending`；易支付回调验签、金额和商户快照校验通过后
  才转为 `pending` 待开票状态。
- 事件键、支付订单号和回调处理保持幂等。未完成支付的申请不会触发待开票通知。

### 相关接口

- `GET /api/user/invoice/config`：返回发票配置、可用支付方式和支付链信息。
- `POST /api/user/invoice/preview`：计算所选订单的开票服务费。
- `POST /api/user/invoice/request`：仅用于零服务费时提交申请；正服务费请求会被拒绝。
- `POST /api/user/invoice/payment`：创建外部支付申请并返回易支付收银台参数。
- `GET|POST /api/invoice/epay/notify`：易支付异步回调。
- `GET|POST /api/invoice/epay/return`：易支付同步回跳。

### 模板边界

Default 使用 `web/src/features/invoices`，Classic 使用
`web/classic/src/components/invoice`。两套模板分别读取相同的后端配置契约，修改一套
不会自动改变另一套。
