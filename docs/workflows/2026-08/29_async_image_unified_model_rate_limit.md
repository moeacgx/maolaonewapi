# 异步生图复用模型请求限流

## 目标

异步生图提交与普通模型请求使用同一套“模型请求速率限制”配置。用户、令牌、
用户分组、请求分组、全局范围以及“请求次数/成功次数”的统计口径保持一致，
不再额外叠加异步专用的固定每分钟窗口。

## 根因

异步任务入口曾在 `ImageTaskAdmissionGuard` 中维护用户每分钟 12 次、令牌每分钟
8 次的进程内窗口。该窗口独立于控制台的模型请求限流，导致同一令牌在普通模型
请求和异步生图之间出现不一致的 429 行为。此前的路由整合还曾移除异步提交入口
上的 `ModelRequestRateLimit`，使问题更加明显。

## 行为契约

- `POST /v1/images/tasks` 和 `POST /canvas/v1/images/tasks` 继续挂载
  `middleware.ModelRequestRateLimit()`。
- 异步提交会按照当前控制台配置选择全局、请求分组或用户分组规则，并使用相同的
  总请求次数和成功请求次数计数。
- 异步提交返回 `202 Accepted`，因此在“成功次数”规则下按已接受的提交计数；任务
  最终是否成功不改变该请求限流的统计。
- `ImageTaskAdmissionGuard` 只负责活动任务并发上限：用户最多 8 个、单令牌最多
  4 个活动任务。它不再维护独立 RPM 窗口。
- 任务查询和内容下载不计入模型请求限流；只有创建任务的 POST 请求计入。

## 多实例与兼容性

模型请求限流启用 Redis 时由共享 Redis 统一计数；未启用 Redis 时保持现有的进程内
限流语义。活动任务并发通过数据库事务锁定用户行，兼容 SQLite、MySQL 和
PostgreSQL。移除固定 RPM 窗口不会删除任务数据，也不会改变已有活动任务。

本记录 supersede 了曾将异步提交与模型请求限流隔离的
`2026-08/29_async_image_rate_limit_isolation.md` 方案。

## 验证

- `go test ./controller -run 'TestImageTaskAdmission' -count=1 -timeout 60s`
- `go test ./router -run 'TestCanvasImageTaskRouteHidesCrossUserTasks|TestFeatureRoutesRegisterExactlyOnce' -count=1 -timeout 60s`
- `gofmt` 和 `git diff --check`
