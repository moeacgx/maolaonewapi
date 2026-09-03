# 对话归档容量控制与本地化

## 目标

对话归档是数据库持久化能力，不应无限增长。Root 需要能够设置全局保留的
最近会话数量，超出时自动删除最旧归档，并在不再需要排查数据时手动清空。
Classic 与 Default 原生页面必须显示当前语言的文案，不能因缺少翻译键回退为英文。

## 方案

- 配置增加全局 `max_archive_count`。默认保留最近 1000 条，允许设置为
  1 至 100000 条；单条清洗正文上限和按天过期清理保持原有语义。
- 每次归档写入、定时过期清理和手动清空均在同一数据库事务内锁定配置单例；
  写入后按 `created_at ASC, id ASC` 删除溢出的最旧记录。配置降低上限时，
  在同一配置 CAS 事务内立即裁剪已有记录。裁剪每批重新计数，删除零行会使
  事务回滚，避免与非预期外部删除竞争时循环不退出。
- Root 清空接口要求显式确认、关键操作限流，并与写入操作使用同一配置行锁；
  清空过程中不读取或记录归档正文。采集未关闭时，之后命中的请求仍会再次归档。
- 原生页面新增会话上限与清空操作；Default 语言包通过规定的同步脚本维护，
  Classic 语言包逐语言补齐与页面实际 `t()` 键一致的文案。

## 兼容性与验证计划

模型迁移只使用 GORM 标准字段和查询，兼容 SQLite、MySQL 5.7.8+ 与
PostgreSQL 9.6+；不使用 `DELETE ... LIMIT`、窗口函数或方言 SQL。

验证覆盖配置 CAS 后即时裁剪、写入后仅保留最新记录、过期清理的有界批次、
Root 确认清空与权限边界，以及 Default/Classic 所有归档页面翻译键。Classic
原生测试入口包含翻译回归测试。交付前执行对应 Go、Node 原生页面测试、格式化、
`git diff --check` 和可用的前端 i18n 同步检查。

## 验证结果

- `go test ./model -count=1 -timeout 60s`、`go test ./service -count=1 -timeout 60s`、
  `go test ./router -count=1 -timeout 60s` 与 `go test ./extension -count=1 -timeout 60s`
  均通过。
- `node --test scripts/conversation-archive-native.test.mjs
  scripts/conversation-archive-i18n.test.mjs scripts/native-sdk-live-api.test.mjs
  scripts/security-audit-load-data.test.mjs` 通过；本地化测试会拒绝中文键缺失或直接
  回退为英文原文。
- `node web/scripts/sync-i18n.mjs` 执行成功。完整 `go test ./...` 仅根包因工作树不含
  `web/classic/dist` 的嵌入目录而无法建立，未发现本次改动引入的编译错误。
