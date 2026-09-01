# 福利营销批量删除契约

## 背景

福利活动、兑换码和优惠码页面需要批量清理历史数据，但这些记录可能关联用户余额、订单或
福利流水，不能直接物理删除。

## 变更

- `DELETE /api/benefit/admin/activities/batch`：请求体 `{ "ids": [1, 2] }`。
  仅软删除已结束（`ended`）或已终止（`terminated`）活动；草稿、已发布、已暂停和不存在
  的 ID 返回为 `skipped`。活动的 shares、user vouchers、ledger 保持不变，管理员仍可
  通过历史关联进行审计。
- `DELETE /api/redemption/batch`：批量软删除兑换码。
- `DELETE /api/promo_code/batch`：批量软删除优惠码，并逐条维护 `deleted_id`，保证历史
  唯一键和同码重建能力。

三个接口都要求管理员认证，ID 必须是正整数，最多 500 个，重复 ID 会在服务端去重。成功
响应使用统一 API 包装，`data.deleted` 表示实际归档数量；福利活动接口额外返回
`data.skipped`。

## 安全与回滚

批量操作在服务端事务中执行。兑换码和优惠码沿用 GORM `DeletedAt` 软删除；福利活动新增
`DeletedAt` 列并通过 AutoMigrate 兼容 SQLite、MySQL 和 PostgreSQL。误操作时只能由数据库
管理员在确认审计记录后恢复对应 `deleted_at`（不要清理关联账务数据）。

## 验证

- `go test ./model -run TestDeleteBenefitActivitiesByIDsOnlyArchivesHistoricalActivities -count=1 -timeout 60s`
- `go test ./controller ./router -run '^$' -count=0 -timeout 60s`
- `git diff --check`
