# 福利营销批量删除契约

## 背景

福利活动、兑换码和优惠码页面需要批量清理历史数据，但这些记录可能关联用户余额、订单或
福利流水，不能直接物理删除。

## 变更

- `DELETE /api/benefit/admin/activities/:id` 与 `/batch`（批量请求体 `{ "ids": [1, 2] }`）：
  仅软删除草稿（尚未产生领取数据）、已结束且归一化后无 active 券、或已终止且无可用券的
  活动。`published`、`paused`、带可用券的 `terminated/unused` 和不存在的 ID 返回逐项
  `skipped`。活动的 shares、user vouchers、ledger 保持不变，管理员仍可通过历史关联审计。
- `DELETE /api/redemption/:id`、`/batch` 和 `/invalid`：兑换码使用 GORM 软删除，三类删除路由
  均使用管理员鉴权与关键操作限流，保留充值日志、
  用户到账结果和返佣来源。
- `DELETE /api/promo_code/:id`、`/batch` 和 `/invalid`：优惠码逐条写入 `deleted_id` 后软删除，
  三类删除路由均使用管理员鉴权与关键操作限流，释放代码唯一键。`/invalid` 只归档 `disabled`、
  `used` 或 `enabled` 且已到期的优惠码；仍有效的优惠码不会被清理。

三个资源接口都要求管理员认证，批量 ID 必须是正整数、最多 500 个，重复 ID 在服务端去重。
批量成功响应使用统一 API 包装：

```json
{
  "deleted_ids": [12],
  "skipped": [{"id": 13, "reason": "not_deletable"}]
}
```

优惠码失效清理也返回 `deleted_ids`，批量删除会将请求中未找到或已软删除的 ID 放入
`skipped: [{"id": 13, "reason": "not_found"}]`，不记录兑换码/优惠码完整明文。福利券管理列表支持
`keyword`/`status` 筛选，批量作废接口会为每张成功作废的 active 券写一条带操作者和原因的
流水；普通用户只能读取自己的券流水，响应剥离管理员元数据。

活动固定面额请求先按展示单位验证 `fixed_amount * total_count == total_amount`，内部总额度
使用单份 `fixed_quota * total_count` 的安全乘积，避免分别换算造成舍入差异。管理活动响应的
`total_amount`、面额和领取门槛按当前展示设置回显，`amount_display_type` 始终是当前类型；
创建时单位/汇率/`QuotaPerUnit` 仅保存在快照字段中用于历史解释。

## 安全与回滚

批量操作在服务端事务中执行，数据库错误会回滚整批。活动删除在行锁下重新读取状态并归一化
过期记录，进行中活动不可删除。兑换码和优惠码沿用 GORM `DeletedAt` 软删除；优惠码删除维护
`deleted_id`，已有支付 reservation 通过 `Unscoped` 继续回调结算，新 reservation 不能使用已删
优惠码。福利活动新增 `DeletedAt`、内部 quota 配置和展示设置快照，通过 AutoMigrate 兼容
SQLite、MySQL 和 PostgreSQL。误操作时只能由数据库管理员在确认审计记录后恢复对应
`deleted_at`，不要清理关联账务数据。活动迁移快照中的展示类型、展示汇率和 `QuotaPerUnit`
记录旧活动被迁移时采用的 CNY 解释环境，后续回显使用当前展示上下文，不用快照重新计费。

## 验证

- `go test ./model -run 'TestDeleteInvalidPromoCodesArchivesOnlyInvalidCodes|TestDeletePromoCodesByIDsArchivesSelectedCodes' -count=1 -timeout 60s`
- `go test ./controller ./router -run 'PromoCodeAdminRoutes|MarketingDelete' -count=1 -timeout 60s`
- `git diff --check`
