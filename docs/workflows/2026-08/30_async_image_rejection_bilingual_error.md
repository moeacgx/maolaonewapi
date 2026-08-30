# 异步图片任务拒绝错误双语提示

日期: 2026-08-30

## 问题

普通异步图片任务和 Canvas 异步图片任务共用任务重放与完成链路。发生 HTTP
4xx 时，任务会把安全脱敏后的 `image generation request was rejected`
写入 `fail_reason`，任务查询和 Default/Classic 任务日志因此只显示英文。

## 根因

`controller/canvas_image_task.go` 的 `maskImageTaskFailure` 按 HTTP 状态码生成
公开失败原因。异步任务在后台执行，不能可靠地依赖提交请求的语言上下文；同时
任务日志直接展示持久化的 `fail_reason`，不会再经过前端 i18n。该字符串是 4xx
分类脱敏提示，不代表唯一的上游根因，具体原因仍需查看管理员可见的渠道或请求日志。

## 修改

- 将异步图片任务的 4xx 拒绝提示改为中英文双语：
  `image generation request was rejected / 图片生成请求被拒绝`。
- 普通 `/v1/images/tasks` 与 Canvas `/canvas/v1/images/tasks` 共用该提示，
  不改变认证、审计、分组、渠道选择、计费或敏感信息脱敏边界。
- 保留其他状态码分类和历史任务字段结构不变。

## 兼容性与安全边界

- 仍然不把上游响应正文、密钥、主机名或内部诊断信息返回给普通用户。
- `fail_reason` 仍是字符串字段，旧任务数据无需迁移。
- 双语文本同时适用于不经过前端渲染的 API 客户端、Default 和 Classic 日志界面。

## 验证

- `go test ./controller -run TestMaskImageTaskFailureUsesBilingualReasons -count=1 -timeout 60s`
- `go test ./controller -run TestImageTaskCompletionCASAndErrorRedaction -count=1 -timeout 60s`
- `git diff --check`
