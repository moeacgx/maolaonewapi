# 充值成功日志余额快照审计恢复

## 目标

恢复充值成功日志的管理员审计契约：`other.admin_info` 必须包含订单号
`trade_no`、实付金额 `paid_amount_cny`，以及同一充值事务中额度更新前后取得的
`balance_before`、`credited_quota`、`balance_after`。普通用户日志仍由既有
`formatUserLogs` 移除 `admin_info`，不扩大敏感字段可见范围。

## 根因

v244 合并后 `creditTopUpQuota` 只执行原子额度更新，不再返回更新前/后的余额；
充值完成路径改用只写基础支付信息的 `RecordTopupLog`，导致 Classic 仍读取的
余额快照字段缺失。管理员重复补单路径还会在已成功订单上继续写一条空订单日志。

## 修改范围

- `creditTopUpQuotaWithSnapshot` 在同一 GORM 事务内锁定用户、执行带 int64 钱包
  上限谓词的更新，并读取更新后的余额；原 `creditTopUpQuota` 保留给兑换码、
  邀请和返佣转余额等不需要订单审计的调用方。
- 充值订单对象保存事务内的临时快照标记，成功提交后统一通过
  `RecordTopupOrderLog` 写入订单号、实付金额和余额三字段。
- Epay、Stripe、支付尝试通用路径（Creem/Waffo/Bepusdt/OKPay）、免费优惠码、
  Waffo Pancake 以及管理员补单全部接入订单日志；重复回调、失败回调和钱包上限
  拒绝路径不写成功审计日志。
- 管理员补单仅在本次事务实际完成订单时同步缓存和写日志，已成功订单幂等返回。

## 数据库与安全边界

余额读取和写入使用 `lockForUpdate` 与 GORM 查询，未引入方言专用 SQL；SQLite、
MySQL、PostgreSQL 均沿用现有事务和 int64/BIGINT 钱包契约。额度上限谓词仍由
`common.MaxWalletQuota` 防护，快照值不参与新的额度计算。日志 JSON 通过
`common.MapToJsonStr` 生成，管理员字段继续嵌套在 `admin_info`。

## 验证

- 模型行为测试覆盖 Epay、Stripe、支付尝试、免费优惠码、Waffo Pancake、管理员
  补单的快照字段和重复回调幂等，以及钱包上限失败不写日志。
- 控制器测试覆盖管理员补单接口成功响应与审计字段。
- 服务测试覆盖 Waffo Pancake webhook 订单解析到充值审计的链路。
- 交付前执行相关 Go 测试、前端充值/日志相关测试（若存在）、`gofmt` 和
  `git diff --check`；未执行的外部支付网关回调和真实多数据库部署需在交付摘要中
  单独说明。

## 基线

本工作树在开始时已 `git fetch origin`，`HEAD`、`origin/custom-main` 和
merge-base 均为 `871db1f367a5c85e6aa5aaf8a3a19fd2f1d8a7bc`，ahead/behind 为
`0/0`。
