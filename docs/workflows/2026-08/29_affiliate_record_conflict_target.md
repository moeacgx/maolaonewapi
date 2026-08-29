# 返佣记录冲突目标一致性修复

## 目标

修复 GitHub Issue #92：充值完成事务写入返佣记录时，PostgreSQL 的
`ON CONFLICT` 目标与线上唯一索引不一致，导致 `SQLSTATE 42P10` 并回滚整笔
充值事务。

## 根因

返佣记录的业务幂等键是：

```text
source_type + source_id + user_id + level
```

但原实现存在三处不一致：

- 已有记录查询使用四个字段；
- `AffiliateRecord` 的 GORM 唯一索引声明缺少 `user_id`；
- 插入语句的 `ON CONFLICT` 目标也缺少 `user_id`。

当 PostgreSQL 实际唯一索引为四字段时，三字段冲突目标无法匹配任何唯一约束；
即使数据库接受该语句，来自不同返佣用户的同一来源也可能被错误去重。

## 修改范围

- `AffiliateRecord` 的 `idx_affiliate_record_source` 改为四字段，顺序为
  `source_type`、`source_id`、`user_id`、`level`。
- `createAffiliateRewardRecordTx` 的 `ON CONFLICT` 目标与查询条件统一为四字段。
- 主迁移和快速迁移在 `AutoMigrate` 前调用索引收敛逻辑：
  - 已有旧索引或 PostgreSQL 唯一约束时先删除；
  - 再按当前模型创建四字段唯一索引；
  - 已经正确的索引直接跳过，迁移可重复执行。
- SQLite 使用 `PRAGMA index_list/index_info` 检查索引列，避免依赖该方言不支持的
  `GetIndexes`；MySQL/PostgreSQL 使用 GORM Migrator，PostgreSQL 额外处理同名唯一约束。

## 数据与兼容性边界

- 迁移只重建唯一索引，不修改返佣记录、余额、订单或支付数据。
- 迁移仅在 `affiliate_records` 表及 `user_id` 列已存在时执行；新库由
  `AutoMigrate` 按四字段模型创建。
- 创建唯一索引时若已有数据违反四字段唯一性，迁移会失败并阻止启动，避免静默合并
  或丢失返佣记录。
- SQLite、MySQL、PostgreSQL 均使用各自 GORM 方言生成索引语句；未执行生产数据库
  操作，生产索引和待补单订单需上线前另行核对。

## 回归验证

- `TestAffiliateRewardSourceTupleKeepsDifferentUsersIndependent`：相同来源、层级但
  不同返佣用户各生成一条记录，余额分别增加。
- `TestMigrateAffiliateRecordSourceIndexReplacesLegacySQLiteIndex`：将旧三字段
  SQLite 索引迁移为四字段，重复执行保持幂等，并验证不同用户可共存。
- 保留既有重复回调测试，确保相同四字段不会重复增加余额。

验证命令：

```text
go test ./model -run 'Affiliate|MigrateAffiliateRecordSourceIndex' -count=1
git diff --check
```

## 回滚与上线注意事项

代码回滚可恢复旧逻辑；数据库索引回滚必须基于升级前备份并确认应用版本，不能在
生产环境直接手工删除订单、余额或返佣记录。本工作项不包含生产补单或数据修复。
