# 福利营销时效额度券设计

## 目标

在现有兑换码、订阅和钱包之外，新增绑定稳定分组的一次性时效额度券。
福利额度保持独立余额，仅在用户显式选择券绑定分组时参与计费，并支持固定或
随机面额活动、领取资格、混合扣费、强终止、完整流水和活动报表。

本设计以 GitHub Issue #77 的功能口径为准，不采用 Issue 中曾记录的开发进度。
当前交付只包含当前分支的代码、测试和开发文档，不部署、不修改生产环境、不创建
Pull Request。

## 范围

本期包含：

- 管理端活动创建、草稿编辑、发布、暂停、恢复、提前结束、两种强终止和报表。
- 用户活动列表、领取、券列表、钱包摘要入口和拆分扣费日志。
- 固定面额及按分预拆的随机面额，每个活动一人最多领取一张。
- 福利券、订阅、钱包三种资金源的组合预扣、结算和失败回滚。
- 通用的分组单用户并发字段及 `user_id + group_id` 请求生命周期计数。
- Default 和 Classic 两套前端、中文和英文基础文案。
- SQLite、MySQL 5.7.8+、PostgreSQL 9.6+ 兼容。

本期不包含自动复制渠道或分组、图像和视频异步任务、多实例全局分组并发、多券
叠加、按模型限制、退款后重新判定领取资格、并入钱包余额、部署和 PR。

## 数据模型

### 活动与份额

`benefit_activities` 保存活动配置、稳定 `group_id`、分组 code/name 快照、状态、
金额配置、领取门槛、个人券有效期和操作人快照。显示金额以整数分落库，内部额度以
有符号 64 位 quota 落库；请求级扣费仍遵守现有 int32 饱和边界。

活动状态为 `draft`、`published`、`paused`、`ended`、`terminated`。草稿允许修改
全部字段；发布后仅允许修改名称和说明，其他变化通过专用状态接口完成。发布和恢复
时拒绝同一分组内时间区间相交的其他 `published` 或 `paused` 活动。

`benefit_activity_shares` 保存发布时一次性生成的所有份额。固定面额要求单份金额乘
份数等于总预算。随机面额先用整数分验证
`count * min <= total <= count * max`，再生成 N 个确定份额且总和严格等于预算。
领取时在数据库事务内锁定活动及候选份额，并用条件更新完成认领。

### 用户券与流水

`benefit_user_vouchers` 保存用户、活动、份额、原始额度、剩余额度、已使用额度、
领取和失效时间。唯一索引保证一个用户在一个活动中最多一张券。券失效时间为
`min(领取时间 + 个人有效期, 活动结束时间)`。

`benefit_voucher_ledger` 保存 `pre_consume`、`settle_delta`、`settle_rollback`、
`refund_additional`、`refund`、`void`、`expire` 流水，并关联 activity、voucher、user、
request_id、log_id、额度变化和变化后余额。请求内的重复退款、追加退款和结算补偿通过
`request_id + type` 及模型事务保证幂等；结算补偿使用原 request_id 的独立流水类型，
不复用 `settle_delta`。

过期采用访问时惰性归档：活动/券查询、领取和扣费入口先把已过期记录更新为终态并
写流水，不新增定时任务。时间以 Unix 秒落库；管理端日期输入和展示统一解释为
`Asia/Shanghai`，边界规则为 `now >= ends_at` 即失效。

### 分组并发

`groups.single_user_concurrency_limit` 为非负整数，`0` 明确表示不限，不回退其他
用户组并发设置。字段进入 `Group`、`GroupConfig` 和 `/api/group/details`，并由
Default/Classic 分组编辑器读写。

## API 与权限

管理端统一使用 `AdminAuth`：

- `GET/POST /api/benefit/admin/activities`
- `GET/PUT /api/benefit/admin/activities/:id`
- `POST /api/benefit/admin/activities/:id/publish`
- `POST /api/benefit/admin/activities/:id/pause`
- `POST /api/benefit/admin/activities/:id/resume`
- `POST /api/benefit/admin/activities/:id/end`
- `POST /api/benefit/admin/activities/:id/terminate`
- `GET /api/benefit/admin/activities/:id/report`
- `GET /api/benefit/admin/activities/:id/vouchers`
- `GET /api/benefit/admin/vouchers/:id/ledger`
- `POST /api/benefit/admin/vouchers/:id/void`

强终止和单券作废要求请求体携带二次确认布尔值及非空原因。操作通过现有管理审计
日志记录操作者、时间、动作、activity/voucher ID、模式和原因；管理 HTTP 请求体中的
`now` 不参与状态变更，时间统一由服务端时钟生成，模型层时间参数仅用于内部测试注入。

用户端统一使用 `UserAuth`：

- `GET /api/benefit/activities`
- `GET /api/benefit/vouchers`
- `POST /api/benefit/activities/:id/claim`

活动列表返回可领取、已领取和往期活动。不符合资格的活动仍返回，但只提供泛化的
不可领取状态；领取接口对注册不足 30 分钟或历史成功实付不足统一返回
“不符合领取条件”。历史实付使用成功充值记录中的真实实付快照；订阅购买已映射到
同 trade number 的充值记录，因此不重复累计。并发抢不到最后一份返回“已领完”。

## 状态与终止语义

- 暂停：禁止新领取，已领取券仍可使用到原失效时间。
- 提前结束：把活动结束时间收口到当前时刻，所有券随活动硬失效。
- 终止并作废未用券：作废未领取份额；已领取券继续使用到原失效时间。
- 终止并作废所有券：作废未领取份额，并把所有已领取券剩余额度清零。
- 作废保留原领取额度和已使用额度，用户页展示“已作废，剩余额度 0”。

活动行的 `terminated` 只禁止新领取。已领取券能否继续使用由终止模式、券状态和原
`ends_at` 共同决定，从而区分两种终止模式。

## 计费架构

现有单一 `FundingSource` 扩展为可组合的资金计划。`BillingSession` 仍负责一次请求
的令牌额度预扣，并维护按优先级排列的资金份额：福利券、订阅、钱包。

只有请求通过 token 或请求参数显式选择稳定分组，且该分组存在可用券时，才进入
组合资金计划。非活动分组不查询福利券，沿用现有用户计费偏好。命中福利券后固定按
“福利券 -> 订阅 -> 钱包”分配剩余额度；这一路径覆盖用户原计费偏好，以满足 Issue
规定的固定抵扣顺序。

标准 Relay JSON 的显式 `group` 先经过用户可用分组和 token 绑定校验，再写入最终
`using_group`；省略字段继续继承 token/用户分组，`group=auto` 保持自动门禁。

预扣按正序执行；任一资金源失败时按逆序回滚已经成功的资金源。实际结算优先保留
高优先级资金源的消耗，多余预扣按逆序退回。实际费用高于预扣时继续按正序补扣。
令牌周期额度始终按请求总价预扣和结算，不因福利券承担部分费用而降低。

福利券余额耗尽后继续请求同一分组时，按订阅再钱包结算。没有可用订阅或钱包额度时
整单失败，所有已经预扣的福利额度必须回滚。客户端断流、上游失败、跨渠道重试和
零用量重试继续服从现有 `BillingSession` 生命周期。

消费日志的 `other.billing_breakdown` 保存 voucher、subscription、wallet 三项最终
额度以及 activity/voucher ID。用户和管理员均可查看资金拆分；只有管理员可见的
审计字段继续放在 `other.admin_info`。福利抵扣仍计入消费 quota 和渠道成本，但收入
聚合只读取真实支付订单，不把福利额度计为现金收入。

## 请求并发

在最终显式分组确定后，按 `user_id + group_id` 尝试占用进程内槽位，并把所有权放入
Gin Context。请求结束统一按 Context 精确释放，覆盖流式、非流式、错误和重试路径。
本期不复用渠道级 Redis 租约，因为 Issue 明确限定为单实例实现。

超限统一返回 HTTP 429。若当前分组存在活动或用户可用福利券，文案为
“福利分组限制，你请求太快啦！”；其他分组使用现有 Too Many Requests 语义。

## 前端

Default 与 Classic 都在现有“营销福利”后台入口增加“时效额度券”标签页，提供活动
列表、草稿表单、状态操作、终止二次确认、报表、券列表和流水查看。管理界面保持
控制台现有的紧凑工作台风格，不引入新的视觉体系。

两套前端都新增 `/console/benefits` 用户页，分区展示已领取、可领取和往期活动；钱包
页只展示总剩余额度、最近失效时间和进入活动页的摘要。使用日志显示三种资金源拆分。
所有用户可见文案进入现有 i18n；Default 更新全部现有 locale，Classic 至少保证中文
源文案及英文翻译完整。

## 测试与回滚

测试按真实边界分层：

- 模型：金额精度、随机拆分、状态机、跨库迁移、并发领取、唯一券、资格统计、作废、
  过期、报表和流水幂等。
- 服务：券全额、券+订阅、券+钱包、券+订阅+钱包、补扣、逆序退款、后续资金失败和
  token 总价限制。
- 中间件/控制器：分组并发 key 与释放、权限、接口状态转换、领取错误语义。
- 前端：活动表单关键校验、危险确认、用户列表状态、钱包摘要和日志拆分。
- 构建：相关 Go 包、Default test/typecheck/build、Classic test/build、`git diff --check`。

回滚代码时保留新表和新列；旧版本忽略它们。上线后的软关闭方式是不发布新活动，或
暂停/终止已有活动。本次交付不执行任何部署或生产数据操作。
