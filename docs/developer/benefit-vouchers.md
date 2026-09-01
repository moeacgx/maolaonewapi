# 福利营销时效额度券

## 能力边界

福利券是绑定稳定分组的一次性时效额度。额度保存在独立的福利券表中，不会并入
用户钱包；只有 token 或请求明确选择券绑定分组时才参与计费。`auto` 和继承用户
默认分组不会触发福利券扣费。

本能力不自动复制渠道或分组，不限制模型，不支持多券叠加，也不覆盖图像/视频异步
任务。分组必须由管理员预先配置，`groups.single_user_concurrency_limit` 的 `0`
表示不限。

## 数据与迁移

后端模型位于 `model/benefit_voucher.go`：

- `benefit_activities` 保存活动配置、分组快照、状态和终止信息。
- `benefit_activity_shares` 保存发布时预拆的固定/随机份额。
- `benefit_user_vouchers` 保存领取用户、原始/剩余/已用额度、状态和失效时间。
- `benefit_voucher_ledger` 保存预扣、结算差额、结算补偿、追加预扣退款、退款、作废和过期流水。

主库初始化会通过 GORM `AutoMigrate` 创建新增表和 `groups.single_user_concurrency_limit`
列，兼容 SQLite、MySQL 5.7.8+ 和 PostgreSQL 9.6+。旧版本会忽略这些表和列；回滚
代码时保留数据即可，不要手工删除表。

## 活动状态与金额

活动状态为 `draft`、`published`、`paused`、`ended`、`terminated`。草稿可编辑全部
字段；发布后只允许修改名称/说明，暂停、恢复和提前结束使用独立接口。发布或恢复
前会拒绝同一分组时间区间重叠的进行中活动。

管理端所有金额均按人民币元填写，最多保留两位小数（例如 `7.50` 表示 7.50 元）。
服务端使用 decimal 严格校验金额精度，再将总金额按当前美元兑人民币汇率换算为内部
quota；管理端不再填写 `total_quota`。数据库中的 `*_cents` 字段仅是历史兼容的
内部小数位存储，不代表接口金额单位或美元/美分语义。固定模式仍要求单份金额乘
份数等于总预算。随机模式先验证 `count * min <= total <= count * max`，再按 0.01
元粒度预拆出确定份额，领取时从可用份额中随机抽取；份额 quota 总和严格等于自动
换算后的活动总 quota。

管理端创建/编辑请求使用以下金额字段：`total_amount`、`fixed_amount`、
`min_amount`、`max_amount` 和 `claim_paid_threshold`，值为人民币元数字；活动
响应同时返回按元展示的金额字段；`total_quota` 仍随响应提供以兼容现有余额/报表
读取，但它是服务端计算的内部计费结果，前端不提供编辑入口。

活动结束采用 `now >= ends_at` 的硬失效边界。个人券失效时间是
`min(claimed_at + personal_valid_seconds, activity.ends_at)`。访问活动、券、领取和
扣费入口会惰性处理过期记录并写入流水。

## API 与权限

用户接口使用 `UserAuth`：

- `GET /api/benefit/activities`
- `GET /api/benefit/vouchers`
- `POST /api/benefit/activities/:id/claim`

管理接口使用 `AdminAuth`：

- `GET/POST /api/benefit/admin/activities`
- `GET/PUT /api/benefit/admin/activities/:id`
- `POST /api/benefit/admin/activities/:id/publish|pause|resume|end|terminate`
- `GET /api/benefit/admin/activities/:id/report`
- `GET /api/benefit/admin/activities/:id/vouchers`
- `GET /api/benefit/admin/vouchers/:id/ledger`
- `POST /api/benefit/admin/vouchers/:id/void`

终止和单券作废必须提交 `confirm: true` 及非空原因。强制作废操作记录管理员、时间、
动作、活动/券 ID、模式和原因。管理接口的时间始终由服务端 `common.GetTimestamp()`
生成；请求体中的 `now` 字段会被忽略，不能用于伪造状态变更或审计时间。模型层测试
仍可直接注入 `now` 参数，但该参数不暴露给生产 HTTP 契约。用户领取资格在点击时实时计算：注册满 30 分钟且历史
成功实付金额达到门槛；不满足条件统一返回“不符合领取条件”，抢不到最后份额返回
“已领完”。

## 计费契约

`service/BillingSession` 的福利组合顺序固定为：

```text
福利券 -> 订阅 -> 钱包
```

初次预扣按顺序分配，任一后续来源失败会逆序退款。实际用量小于预扣时按逆序退回，
大于预扣时按顺序追加预扣；追加预扣使用同一 `request_id` 的幂等流水，令牌额度上限
仍按请求总价计算。实际用量等于预扣值时也会写入一次福利券结算流水，重复结算不会
重复增加券的已用额度。组合结算在后续来源失败时，为福利券写入独立的
`settle_rollback` 补偿流水，使用原 `request_id` 与流水类型的组合键作为幂等键；补偿后再执行
最终退款时仍会写入原请求的 `refund` 流水，保证余额、`used_quota` 和流水可对账。
预扣回滚遇到多个资金源错误时会聚合错误并保留已写入的流水，调用方不得静默忽略。

追加预扣复用原请求的 `pre_consume` 流水。活动采用 `terminate_mode=unused` 终止且仍在
`ends_at` 内时，已领取且未过 `expires_at` 的券仍允许追加预扣；`terminate_mode=all`
则立即禁止继续使用。

组合会话同步保留旧订阅日志字段：`subscription_id`、`subscription_pre_consumed`、
`subscription_post_delta`、计划 ID/名称。所有来源最终额度写入
`other.billing_breakdown`：`voucher_quota`、`subscription_quota`、`wallet_quota`、
`activity_id`、`voucher_id`。`activity_id` 与福利券流水及消费日志的 `request_id`/`log_id`
一起用于争议追溯。福利抵扣计入消费和渠道成本，但不计入现金收入。

标准 Relay JSON 请求的 `group` 会先经过用户可用分组和显式 token 绑定校验，再成为最终
`using_group`；显式稳定分组才打开福利券门禁，`group=auto` 和省略 `group` 的继承路径
不会误触发福利券。

## 并发与前端

请求在最终分组确定后按 `user_id + group_id` 占用进程内槽位，请求结束、错误和重试
都会释放。福利分组超限提示“福利分组限制，你请求太快啦！”，普通分组保持 HTTP 429
语义。本期不提供多实例全局租约。

Default 和 Classic 均提供：

- 后台营销福利中的时效额度券活动创建、编辑、发布、暂停、恢复、提前结束、终止；
- 管理表单完整配置固定/随机面额、总预算/份数、领取门槛、个人有效期及北京时间起止时间；
- Classic 后台使用与优惠码一致的右侧抽屉创建/编辑活动；活动列表保留标题和标签分隔线、
  表格列头、行分隔与独立操作列。表单字段必须展示单位、约束和示例，金额使用人民币
  元且最多两位小数，额度由服务端按汇率自动换算，个人有效期使用秒，活动起止时间
  固定按 `Asia/Shanghai` 解释；
- 报表、券列表、单券流水和单券作废；
- 用户活动福利页、领取、原始/已用/剩余额度、分组、并发上限和失效时间；
- 钱包福利摘要和使用日志中的三方扣费拆分。

## 软关闭、回滚和验证

不发布新活动即可软关闭；已有活动可暂停或终止。回滚代码不删除新表和新列，旧版本
会继续使用钱包/订阅计费。交付前应执行相关 Go 测试、Default typecheck/build、
Classic `node --test`/build、前端受影响文件 lint，以及 `git diff --check`。Classic 入口使用
Semi UI 对外导出的基础样式路径；品牌图标使用当前 `react-icons` 版本实际导出的组件名，避免
锁文件解析到较新依赖时出现生产构建错误。

本次实现只修改当前分支代码、测试和开发文档，不部署 `dev.nu11.me`，不修改生产环境。
合并通过仓库的 Pull Request 流程完成；合并前必须确认后端与两套前端检查均通过。
