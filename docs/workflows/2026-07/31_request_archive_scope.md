# 请求归档范围：全部请求或仅审计事件

## 目标与范围

完整请求归档新增 `archive_scope`，允许 Root 在安全审计页面选择：

- `all_requests`：归档所有符合条件的已鉴权 Relay 请求正文与 Realtime 客户端帧；
- `audit_events`：只有对应审计事件成功写入后才归档原始请求或帧。

默认值为 `all_requests`，旧配置空值也按该范围读取，保证升级不改变已有部署行为。

## 方案与数据契约

`request_archive_configs.archive_scope` 使用 `VARCHAR(32)`，配置 API 的读取和更新请求
同步返回或接收 `archive_scope`，只允许上述两个稳定值。请求归档任务增加可空的唯一
去重键和 `audit_event_id`；全量任务不使用去重键，事件模式以一次原始候选的 UUID
去重，使同一请求产生多个审计事件时仍只归档一次，并保留首个成功触发事件的 ID。

HTTP 事件模式在协议转换和屏蔽词改写前保留原始候选。内置屏蔽词与上游
`cyber_policy` 在事件落库成功后触发归档；Blocking Guard 只在最终事件确认保留后
触发，不能因临时 `pending` 事件归档 Safe 请求。Async Guard 把原始候选放入已有的
受保护任务载荷，不写入可查询的明文 `snapshot`；最终没有事件时随任务完成清空。

Realtime 采用帧级语义：客户端帧自身触发事件时归档该帧；上游风控归档最近的相关
客户端帧，不回溯此前音频和控制帧。需要风险连接完整回放时必须使用全量范围。

## 安全与兼容性

- Authorization、Cookie、URL query/fragment 和除 Content-Type 外的请求头仍不进入候选；
- 事件写入失败时不归档，归档入队失败仍只增加 `dropped`，不影响客户端请求；
- 没有 `CRYPTO_SECRET` 的本地目标继续使用既有 Root-only 明文兼容边界；
- 数据库迁移只新增列和索引，兼容 SQLite、MySQL 5.7.8+ 与 PostgreSQL 9.6+；
- Default 与 Classic 都提供相同范围设置，并在旧响应缺字段时回退为全量。

## 测试计划与回滚

验证配置默认值、非法值、CAS、全量旧行为、Safe 不归档、各事件来源触发、异步 Guard、
去重、正文超限和队列错误；执行 Go 定向测试、两套前端测试、i18n 同步及
`git diff --check`。回滚应用版本前先把范围切回 `all_requests`；新增列和索引可以保留，
不需要破坏性数据库操作。
