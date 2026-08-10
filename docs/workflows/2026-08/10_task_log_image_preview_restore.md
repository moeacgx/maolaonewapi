# 任务日志图片预览链接恢复

日期: 2026-08-10

## 问题

Classic 任务日志中,部分成功的 Canvas/Image API 图片任务在结果尚未过期时,`详情` 列显示 `无`,而不是 `查看图片`。

`无` 的含义是当前行没有可展示的失败原因、视频结果地址或图片预览入口。对成功图片任务而言,这不是预期展示。

## 根因

任务日志列表查询为了避免直接返回大体积 Base64 图片,主查询会省略 `tasks.data`。后续只为非图片任务回填 `data`,导致图片任务 DTO 生成阶段拿不到 `data.data[]` 中的 `b64_json`/`url` 条目,无法生成轻量的 `/api/task/{task_id}/content/{index}` 图片预览 URL。

已超过图片数据保留时间的旧图片任务应继续显示 `已过期`;未过期且数据库仍保留图片结果的成功任务应显示 `查看图片`。

## 修复方案

- 日志列表主查询继续省略 `data`,保持首段查询轻量。
- 对需要生成预览入口的成功且未过期图片任务,二次按 ID 回填 `data`。
- DTO 阶段继续把实际 `data` 置空,仅返回轻量预览 URL。
- 若成功图片任务结果已过期或数据已被清理,前端展示 `已过期`,避免误导为普通 `无`。

## 验证结果

- `go test ./model -run 'TestTaskLogListQueriesHydrateUnexpiredImageDataForPreview|TestTaskLogSelectOmitsImageDataColumn' -count=1` 通过。
- `go test ./controller -run 'TestPrepareImageTaskLogBuildsLightweightPreviewURLs|TestPrepareImageTaskLogMarksExpiredResult|TestPrepareImageTaskLogMarksMissingSuccessDataExpired' -count=1` 通过。
- `npx --yes bun run build` 在 `web/classic` 通过;仅保留既有 Browserslist/lottie warning。
- `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/local-test.ps1 -Action verify` 通过 `/api/status`、Classic 首页、root/demo 登录和演示数据接口验证。

## 回滚

回滚 `model/task.go` 与 `controller/task.go` 的图片日志数据回填/过期语义即可恢复旧行为;回滚后未过期图片任务可能再次因为列表查询缺少 `data` 而显示 `无`。
