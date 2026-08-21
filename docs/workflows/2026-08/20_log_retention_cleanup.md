# 数据库业务日志自动保留清理

## 目标

PostgreSQL `logs` 表长期无人手动清理时会持续增长。本变更为数据库业务日志增加可配置保留天数，避免依赖管理员偶发手动点击“清除历史日志”。

## 范围

- 后端新增 `LogRetentionDays` 选项；`0` 表示关闭自动清理，正整数表示只保留最近 N 天数据库业务日志。
- 主节点每小时执行一次清理任务；每轮最多删除固定批次数量，避免单次长事务。
- Classic 与 Default 日志设置页补齐保留天数输入项；手动按时间清理统一创建系统任务。
- 仅清理数据库 `logs` 表，不处理 `/app/logs` 文件日志、请求归档、任务结果或 PostgreSQL VACUUM。

## 安全与兼容性

- 默认值为 `0`，升级后不会自动删除既有业务日志；需要管理员显式设置保留天数。
- 自动任务只在 `common.IsMasterNode` 为 true 的实例运行，避免多实例重复清理。
- 删除按 `created_at < now - retention_days` 执行，仍使用现有 `DeleteOldLog` 查询条件与 GORM 删除路径。
- 删除行后 PostgreSQL 物理体积仍需 autovacuum、`VACUUM FULL`、`pg_repack` 或 dump/restore 才可能回收磁盘/备份体积。
- SQLite 旧库若已有 `logs` 数据，先以可空普通列补齐 `idempotency_key`，再由 GORM 建唯一索引，避免 `ALTER TABLE ... ADD ... UNIQUE` 在 SQLite 上失败。

## Classic 前端契约

- Classic 的“清除历史日志”调用 `POST /api/system-task/log-cleanup`，查询参数
  `target_timestamp` 使用 Unix 秒时间戳；不再调用已下线的同步
  `DELETE /api/log/`。
- 接口先返回异步系统任务，Classic 轮询 `GET /api/system-task/:task_id` 直到成功或
  失败，再显示删除数量或错误信息。
- 创建和轮询请求均使用 `skipErrorHandler`，由页面统一显示错误，避免 Axios 全局拦截器
  与页面捕获逻辑重复弹出同一条错误。

## 验证结果

- `go test ./model ./service -run 'TestUpdateLogRetentionDaysOptionUpdatesRuntimeValue|TestRunLogRetentionCleanupOnceHonorsRetentionSetting' -count=1 -timeout 60s`：通过。
- `go test ./model -run '^TestEnsureSQLiteLogIdempotencyKeyAllowsExistingLogsMigration$' -count=1 -timeout 60s`：通过，覆盖已有 `logs` 行的 SQLite 升级路径。
- `npm run typecheck`（`web`）：通过，覆盖 Default 日志设置页新增 `LogRetentionDays` 表单字段的 TypeScript 类型。
- `npx --no-install eslint src/pages/Setting/Operation/SettingsLog.jsx src/pages/Setting/Operation/settingsLogOptions.js src/components/settings/OperationSetting.jsx`（`web/classic`）：通过，覆盖 Classic 日志设置页新增输入项和选项解析。
- 15 个前端 locale JSON 文件解析通过。

## 生产现状核验

- 手动清理历史日志后，`maolaoapi` 已先在 `zzapi-postgres` 演练 `pg_repack`，再备份并重写 `maolaoapi-postgres.public.logs`。
- 备份文件：`/home/docker/maolaoapi/ops-backups/pre-pg-repack-20260820-161349/new-api-pre-pg-repack.dump`，大小 `1,833,999,875` bytes，sha256 `04474747770ca8c9dca189c74fd3b04d72d8a16835c5d139ccde0e87f980b501`。
- `new-api` PostgreSQL 数据库从约 `51 GB` 降到约 `30 GB`；PostgreSQL 数据卷从约 `53 GB` 降到约 `32 GB`。
- `logs` 表从约 `32 GB` 降到约 `12 GB`，其中 heap 约 `9750 MB`、索引约 `2172 MB`；`n_dead_tup = 0`。
- 当前最大表为 `tasks` 约 `14 GB`，其次为 `logs` 约 `12 GB`；后续磁盘增长需通过 `LogRetentionDays` 自动保留策略控制。
