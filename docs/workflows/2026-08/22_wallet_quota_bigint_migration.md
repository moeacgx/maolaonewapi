# 钱包额度 BIGINT 迁移

## 目标

解除充值额度仍受 32 位整数列影响的问题。钱包余额、充值入账、订阅返佣和
返佣账本统一使用有符号 64 位额度；单次请求计费的饱和边界仍由独立的
`MaxQuota` 保护，不因钱包扩容而取消。

## 回归根因

`.243` 正常、`.244` 出现 `top-up quota limit exceeded` 的差异已通过标签
对比确认。官方上游提交 `47ba9d2c6`（`fix: guard wallet quota during
recharge`）在 `.244` 的合并链路中引入了 `ValidateTopUpQuotaCapacity` 和
`creditTopUpQuota`，并直接使用 `common.MaxQuota`（`math.MaxInt32`）作为钱包
余额上限。默认 `QuotaPerUnit=500000` 时，约 4294.96 美元就会触发该旧边界，
即使尚未发起支付也会在金额预览阶段失败。

本次修复将持久化钱包/充值/返佣额度改为 `int64/BIGINT`，把守卫改为
`MaxWalletQuota` 的 int64 溢出保护；`MaxQuota` 仅继续约束单次 API 请求计费。

## 范围

- 迁移 `users`、`top_ups`、`affiliate_*`、`subscription_orders` 和
  `redemptions` 中保存钱包或返佣额度的列。
- 移除 Stripe 充值接口遗留的 10,000 金额上限；各支付入口统一只受
  `int64/BIGINT` 可表示范围和支付渠道自身规则约束。
- Classic 和 Default 充值输入不再设置额外的前端金额上限；金额校验以服务端
  `int64/BIGINT` 边界及支付渠道自身规则为准。
- `TOKENS` 展示模式下充值请求的 `amount` 已按额度单位处理，Stripe 只应用
  分组倍率，不会再次乘 `QuotaPerUnit`；货币展示模式仍按货币金额换算额度。
- 启动迁移在主迁移和快速迁移入口均执行，并按表/列存在性幂等跳过新旧结构
  的差异。
- SQLite 使用已有的 64 位 `INTEGER` 存储能力；MySQL 和 PostgreSQL 在
  `AutoMigrate` 前显式将旧整数列改为 `BIGINT`。

## 数据库兼容性

- MySQL 查询 `information_schema.columns`，保留可空性、默认值和列注释，再以
  有符号 `BIGINT` 重写列；旧的 `UNSIGNED` 定义不会被保留到 int64 契约中。
- PostgreSQL 使用 `ALTER COLUMN ... TYPE BIGINT USING ...::bigint`，保留原有
  默认值和约束。
- SQLite 不执行不支持的 `ALTER COLUMN`，其整数亲和性可直接保存 int64。
- 迁移失败会中止启动并保留已完成的列变更；下次启动会从剩余列继续，避免
  静默截断余额。

## 回滚与验证

升级前应保留数据库备份。BIGINT 到旧 int32 的回滚只在确认所有余额低于旧边界
后进行，不能直接降级列类型。验证至少包括 5,000,000,000 额度写入/读取、
重复迁移、SQLite 兼容性以及 MySQL/PostgreSQL 的元数据检查；相关 Go 测试
单次超时设为 60 秒，并执行 `git diff --check`。
