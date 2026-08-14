# 异步图片使用日志时间对齐

日期: 2026-08-13

## 问题

异步图片任务的使用日志原来按提交时间落库，导致 `logs.created_at` 与任务真正完成时间不一致。用户在任务列表里看到的完成时刻、后台终态写入的失败时刻、以及使用日志的时间轴会分离，排障时很难对应同一次任务。

另一个容易误读的点是：图片任务的 `completion_tokens` 可能为 `0`，这不等于失败。图片任务是否成功，应该看任务终态和 `image_output_count` / `task_status`，而不是把 `0` 直接理解成“没生成”。

## 根因

- `service.PostTextConsumeQuota` 之前把异步图片任务的消费日志时间固定成 `async_image_task_submit_time`。
- `service.RecordImageTaskFailureLog` 之前也沿用提交时间，失败日志和任务终态不在同一个时间点。
- 图片任务的输出数量已经通过 `image_output_count` 单独记录，但旧表格主要看 `completion_tokens`，容易把 `0` 误读成失败。

## 方案

1. 异步图片成功使用日志改为优先使用 `async_image_task_finish_time` 作为 `created_at`，回退到提交时间。
2. 异步图片失败使用日志改为优先使用任务 `finish_time` 作为 `created_at`，回退到提交时间。
3. Relay 在图片结算时同步写入 `async_image_task_finish_time`，保证成功和失败路径都拿得到同一个终态时间。
4. Default 与 Classic 任务日志页在 tokens 外单独展示图片交付数量，避免把图片任务的 `0` 误解为失败。
5. 详情弹窗继续展示 `Generated Images`、`Billing Marker`、`Task Submit Time`、`Task Start Time`、`Task Finish Time`，让排障者能按同一条任务串起提交、运行、完成和日志落点。

## 语义约定

- `completion_tokens = 0` 对图片任务不是失败信号。
- 图片任务是否成功，以任务终态和失败日志为准。
- `image_output_count` 是交付数量的单独展示，不是 prompt/completion token 的替代值。
- 成功消费日志和失败错误日志都应尽量落在任务完成时间附近，方便和任务表、前端详情、和使用统计对齐。

## 兼容性

- 不新增数据库字段，不改迁移。
- 不改变普通文本、语音、嵌入、排序、重排等非图片任务的日志时间。
- 失败日志仍受已有隐私和错误落库规则约束。

## 验证

- `go test ./service -run 'TestPostTextConsumeQuotaPersistsAsyncImageTaskMetadata|TestCalculateTextQuotaSummaryBillsNativeCacheWriteAndClampsRemainingTokens' -count=1`
- `go test ./controller -run 'TestFinishCanvasImageTaskStoresSuccessfulRelayResponse|TestFinishCanvasImageTaskRecordsFailureUsageLog|TestRunCanvasImageTaskRelayMarksBlockedTaskFailedAfterTimeout' -count=1`
- `go test ./relay -run 'TestResolveImageSettlementCount|TestNormalizeImageUsageInfoUsesOutputTokensForSyntheticImage' -count=1`
- `bun run build` 或 `bun run typecheck` 通过前端受影响页面的构建检查
