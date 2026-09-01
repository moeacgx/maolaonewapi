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

内部 `quota` 是活动预算、券余额和流水的唯一计费真值：`total_quota`、
`distributed_quota`、`used_quota`、`original_quota`、`remaining_quota`、
`quota_delta` 和 `balance_after` 直接参与预扣、结算或退款。页面不得输出裸 quota，
也不得通过旧金额比例反推使用量。

页面按系统当前 `quota_display_type` 展示：`USD`、`CNY`、`CUSTOM` 使用当前符号和汇率，
货币输入/显示最多两位小数（步进 `0.01`）；`TOKENS` 显示 Tokens 整数（步进 `1`）。
切换展示类型只改变页面值，不改变历史 quota。活动快照字段只解释创建时的单位/汇率，
不参与当前页面展示。

管理端提交当前 `amount_display_type` 及 `total_amount`、`fixed_amount`、`min_amount`、
`max_amount`、`claim_paid_threshold`。服务端用 decimal 校验当前设置后换算内部 quota；
固定模式安全计算“固定额度 × 份数”，并要求乘积严格等于 `total_quota`；随机模式要求
`count * min <= total <= count * max`，拆分后的 share quota 总和必须严格等于总 quota。

领取门槛是 CNY 实付金额快照，不是 quota。表单/API 按当前 USD/CNY/CUSTOM/TOKENS 单位
回显输入，服务端另行换算为 CNY cents 后与历史充值实付比较；展示类型变化不会改写已保存
门槛。

管理端创建/编辑请求使用以下金额字段：`total_amount`、`fixed_amount`、
`min_amount`、`max_amount` 和 `claim_paid_threshold`。金额字段按系统当前
`USD/CNY/CUSTOM/TOKENS` 展示单位传输和回显：货币精确到 0.01，Tokens 必须为整数；
服务端再将其换算为内部 quota。活动响应同样按当前展示单位返回金额字段；`total_quota`
仍随响应提供以兼容现有余额/报表读取，但它是服务端计算的内部计费结果，前端不提供编辑入口。

活动结束采用 `now >= ends_at` 的硬失效边界。个人券失效时间是
`min(claimed_at + personal_valid_hours * 3600, activity.ends_at)`。管理端创建/编辑请求
使用 `personal_valid_hours`，小时值可带小数但换算结果必须是完整秒；活动响应也返回
按小时展示的 `personal_valid_hours`。数据库内部仍以 `personal_valid_seconds` 保存，旧
客户端提交该字段时服务端继续兼容读取。访问活动、券、领取和扣费入口会惰性处理过期
记录并写入流水。

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
- `DELETE /api/benefit/admin/activities/:id`
- `DELETE /api/benefit/admin/activities/batch`，请求体 `{ "ids": [1, 2] }`
- `POST /api/benefit/admin/vouchers/batch-void`，请求体含 `ids`、`reason`、`confirm`

终止和单券作废必须提交 `confirm: true` 及非空原因。强制作废操作记录管理员、时间、
动作、活动/券 ID、模式和原因。管理接口的时间始终由服务端 `common.GetTimestamp()`
生成；请求体中的 `now` 字段会被忽略，不能用于伪造状态变更或审计时间。模型层测试
仍可直接注入 `now` 参数，但该参数不暴露给生产 HTTP 契约。用户领取资格在点击时实时计算：注册满 30 分钟且历史
成功实付金额达到门槛；不满足条件统一返回“不符合领取条件”，抢不到最后份额返回
“已领完”。

用户流水路由必须按 `voucher_id + user_id` 校验所有权；普通用户只返回业务类型、额度变更、
余额和 request/log 关联，不返回管理员身份、作废原因等 `admin_info`。管理员流水可见操作人
和原因。券列表、流水、活动报表均保留活动/分组快照，活动软删除后通过 `Unscoped` 仍可对账。

### 删除状态矩阵

| 资源 | 可删除/清理 | 跳过 | 保留内容 |
| --- | --- | --- | --- |
| 福利活动 | `draft`（无领取数据）、`ended`、无可用券的 `terminated` | `published`、`paused`、仍有 active 券或未完成额度事务 | shares、用户券、流水和审计 |
| 兑换码 | 单条/批量；`invalid` 清理已用、禁用、过期 | 不存在 ID、非法状态 | 充值日志、到账结果、返佣来源 |
| 优惠码 | 单条/批量；`invalid` 清理禁用、用尽、过期 | 不存在 ID、非法状态 | 优惠使用、支付和审计记录 |

三类批量接口都去重 ID、限制最多 500 个正整数、使用管理员鉴权和关键操作限流，返回
`deleted_ids` 与 `skipped: [{id, reason}]`；重复请求幂等。所有删除都是 GORM 软删除，
不物理清除业务历史。优惠码删除前已建立的支付 reservation 仍可经 `Unscoped` 回调结算，
删除后不得创建新的 reservation 或订单。

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
- 活动表单按面额模式收敛字段：固定模式只填写每份金额和总份数，总预算自动计算；随机
  模式只填写总预算、总份数、最低面额和最高面额，并实时显示可行总预算范围。分组选择
  以分组名称为主，重复名称才追加 code，不展示长描述或内部 ID；提交仍使用稳定分组 ID。
- 管理表单完整配置固定/随机面额、总预算/份数、领取门槛、个人有效期（小时）及北京时间起止时间；
- Classic 后台使用与优惠码一致的右侧抽屉创建/编辑活动；活动列表保留标题和标签分隔线、
  表格列头、行分隔与独立操作列。表单字段按当前 USD/CNY/CUSTOM/TOKENS 展示类型显示
  单位和约束：货币最多两位小数，Tokens 为整数；额度由服务端按汇率自动换算，个人有效期
  使用小时，活动起止时间固定按 `Asia/Shanghai` 解释；
- 报表、券列表、单券流水和单券作废；
- 福利活动历史归档：管理员可调用 `DELETE /api/benefit/admin/activities/batch`，提交
  `{ "ids": [1, 2] }` 批量软删除可安全归档的活动。无领取数据的 `draft`、`ended` 和无可用券的
  `terminated` 活动可删除；`published`、`paused`、仍有 active 券或未完成额度事务的活动会跳过。
  活动关联的份额、用户券和流水始终保留，响应 `data` 返回实际 `deleted_ids` 与逐项 `skipped` 原因。
- 兑换码批量删除：`DELETE /api/redemption/batch`，失效清理：`DELETE /api/redemption/invalid`；
  优惠码批量删除：`DELETE /api/promo_code/batch`，失效清理：`DELETE /api/promo_code/invalid`。
  两者均提交 `{ "ids": [1, 2] }`（失效清理无需 body），最多 500 个正整数 ID，服务端去重并
  沿用软删除，响应返回实际 `deleted_ids` 与 `skipped`。批量操作需要管理员权限并写入管理审计。
- Classic 管理报表直接格式化真实 quota，并按系统当前 `USD/CNY/CUSTOM/TOKENS` 展示类型显示，
  同时展示发放状态、总预算、使用进度、金额去向及活动设置；
  活动行内操作统一收进“操作”菜单，减少表格按钮堆叠；
- 用户活动福利页、领取、原始/已用/剩余额度、分组、并发上限和失效时间；
- 钱包福利摘要和使用日志中的三方扣费拆分。

Classic 福利页面遵循统一后台容器规范：页面外壳可以全宽平铺，但“我的福利券”“可领取活动”
以及营销福利的标签模块必须放在有明确边框、背景和圆角的业务面板中，面板标题与内容区使用
稳定分隔。券卡片仍作为可重复操作项保留独立边界；报表、券列表和流水在面板内部继续使用
分组边界，避免把表格或空状态直接贴在页面背景上。

## 迁移、软关闭、回滚和验证

迁移函数只在新额度配置字段为空时写入：已发布活动优先从既有 share quota 推导，草稿按
旧人民币语义转换一次；无法无损推导的记录返回错误并停止，不静默补零。重复启动不会再次
换算，兼容 SQLite、MySQL 5.7.8+ 和 PostgreSQL 9.6+。迁移失败时保留原记录，不进入发布
或更新活动流程。

不发布新活动即可软关闭；已有活动可暂停或终止。回滚代码不删除新表和新列，旧版本
会继续使用钱包/订阅计费。交付前应执行相关 Go 测试、Default typecheck/build、
Classic `node --test`/build、前端受影响文件 lint，以及 `git diff --check`。Classic 入口使用
Semi UI 对外导出的基础样式路径；品牌图标使用当前 `react-icons` 版本实际导出的组件名，避免
锁文件解析到较新依赖时出现生产构建错误。

之前的表单收敛版本曾在 `zzapi` 测试环境验收；本 Task 10 仅做本地构建、测试和页面验收，
不部署、不推送、不创建 PR，也不修改受保护的 `maolaoapi`。生产发布需另行确认目标、镜像
和回滚方案。
