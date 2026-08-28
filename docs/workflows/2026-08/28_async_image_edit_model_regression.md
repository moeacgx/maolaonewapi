# 异步图片编辑模型解析回归修复

日期: 2026-08-28

## 问题

回归从 `v1.0.0-rc.10.1.10.244`（合并提交 `d182efadc`，紧接在 `v1.0.0-rc.10.1.10.243` 之后）开始。异步图片任务的编辑请求全部进入失败状态。任务日志显示统一的 `image generation request was rejected`，但同步图片生成仍可正常工作。

## 根因

异步图片编辑使用 `multipart/form-data` 请求。Distributor 对 multipart 请求不会走通用 JSON 模型解析，而是依赖图片编辑路径分支从表单读取 `model`。`.244` 的合并结果删除了原有的 `isImageEditPath` 和 `isImageGenerationPath`，并把判断收窄为仅匹配 `/v1/images/...`，因此内部异步重放路径 `/canvas/v1/images/edits` 无法识别模型，渠道分配在本地以 400 终止，任务层再将该错误掩码为统一失败原因。合并前已有修复提交 `16ccc2f19`，但未保留在 `.244` 的结果中。

同一迁移还曾在 `.244` 至 `.245` 删除 AtlasCloud 图片适配器；该部分已由 `.246` 的 `015ec2fc8` 恢复。当前问题与 AtlasCloud 适配器是否存在相互独立，当前分支的失败发生在渠道选择之前。

## 修复

- 恢复图片生成和编辑路径辅助判断，同时覆盖 `/v1/images/...` 与 `/canvas/v1/images/...`。
- 保持普通令牌 API、Canvas 同步请求和上游适配器行为不变。
- 增加 multipart Canvas 图片编辑模型解析回归测试，保护渠道选择前的模型提取契约。

## 兼容性与安全边界

- 仅扩大 Distributor 对已注册 Canvas 图片路径的识别范围，不改变任意其他路径的路由行为。
- 模型仍来自请求表单并继续经过现有分组、令牌模型权限和渠道能力校验；不会绕过鉴权或权限检查。
- 上游请求、计费、任务状态和图片结果存储协议不变。

## 验证

- `go test ./middleware -run TestGetModelRequestReadsCanvasMultipartImageEditModel -count=1 -timeout 60s`
- `go test ./controller -run '^TestCanvasAsyncImageEditTaskReplaysThroughPromptAuditDistributeAndRelay$' -count=1 -timeout 60s`
- `go test ./controller -run 'TestCanvasRouteAsyncSubmissionReplaysAndSettlesWithoutToken|TestCanvasAsyncImageTaskSubmitReAuditsReplayPromptAudit' -count=1 -timeout 60s`
- `go test ./middleware ./controller -count=1 -timeout 60s`
- `git diff --check`
