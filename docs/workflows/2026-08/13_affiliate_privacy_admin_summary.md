# 返佣动态隐私与用户邀请汇总

## 问题与目标

普通用户侧的返佣记录与邀请动态接口只需要展示下级的匿名身份，但旧响应把下级 `username`、`display_name` 直接放在 `/api/affiliate/records` 与 `/api/affiliate/invitations` 的 `invitee` 对象中。管理员侧需要在“用户邀请”页面直接看到某个邀请人产生的总充值、产生额度、累计返佣与可提现余额，不能只到返佣黑名单 / 风控处置里搜索余额快照。

## 接口与数据契约

- 普通用户接口：
  - `GET /api/affiliate/invitations`
  - `GET /api/affiliate/records`
- 普通用户响应中的 `invitee.username` 与 `invitee.display_name` 保留为空字符串，仅返回 `invitee.masked_name`、`id`、`status`、`created_at`。前端只能展示 `masked_name` 或 ID 兜底。
- 管理员接口 `GET /api/affiliate/admin/invitations` 继续返回完整邀请人和下级身份，同时在分页对象上新增：
  - `items[].recharge_amount`：该下级成功充值实付金额；优惠码订单按 `actual_money`，无优惠码旧数据按 `actual_money` 非零值否则回退 `money`。
  - `summary.matched_inviter_count`：当前搜索命中的邀请人数。
  - `summary.matched_invitee_count`：当前搜索命中的下级人数。
  - `summary.topup_count`：当前搜索范围内下级成功充值次数。
  - `summary.topup_quota`：当前搜索范围内下级产生额度，优先 `top_ups.affiliate_source_quota`，旧数据回退返佣记录 `source_quota`，再回退 `top_ups.amount`。
  - `summary.recharge_amount`：当前搜索范围内下级成功充值实付金额。
  - `summary.balance`：当前搜索范围内唯一邀请人的返佣余额汇总，含 `available_quota`、`pending_quota`、`total_quota` 等。
- 风控预览与风控用户列表新增 `generated_topup` 汇总，表示该用户作为邀请人时直属下级产生的充值次数、额度与实付金额。

## 页面变更

- Default 与 Classic 普通返佣页不再显示普通用户可见的下级完整用户名；Classic 改为优先显示 `masked_name`。
- Default “返佣方式设置 / 用户邀请”新增搜索范围汇总卡片：匹配邀请人、匹配下级、充值次数、产生额度、产生充值、可提现。
- Default “用户邀请”表新增“产生充值”列。
- Default 风控预览与风控用户列表新增“产生充值 / 产生额度”。
- Classic 管理员返佣设置同步展示用户邀请汇总、充值实付列、风控产生充值与产生额度。

## 安全与兼容性

- 管理员 `/api/affiliate/admin/*` 路由仍走 `AdminAuth`，保留完整用户名、显示名、邮箱和邀请码，便于审计。
- 普通用户接口只做响应脱敏，不修改邀请关系、返佣结算、余额、提现或风控处置逻辑。
- `username` / `display_name` 字段没有删除，仅置空，降低旧前端或第三方客户端因字段缺失崩溃的风险。
- 新增字段均为只读派生统计，不新增数据库列，不触发迁移。

## 验证计划

1. Go 模型测试覆盖普通邀请/返佣记录脱敏、管理员邀请汇总、充值实付金额、风控产生充值汇总。
2. Default 执行 i18n 同步、类型检查和生产构建。
3. Classic 执行生产构建。
4. 执行 `git diff --check`。
