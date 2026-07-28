# 图片模型误用 Chat/Responses 端点的自动路由

## 问题

部分 OpenAI SDK 会把图片模型请求发送到 `/v1/chat/completions` 或
`/v1/responses`。网关按路径把这类请求当成文本请求，导致图片渠道进入错误的
转换链路，最终请求失败或被上游拒绝。

## 修改范围

- 仅处理 JSON 请求，且请求格式为 OpenAI Chat 或 OpenAI Responses。
- 模型名命中内置图片模型识别规则，或模型元数据声明了
  `image-generation` 端点时，才启用自动路由。
- `/v1/chat/completions`、`/v1/responses` 在网关内部改写为
  `/v1/images/generations`，不向客户端发送 HTTP 重定向。
- Responses 的 `input`、Chat 的 `messages`、以及 `instructions` 会提取文本写入
  图片请求的 `prompt`；已有非空 `prompt` 时保持原值。
- Canvas 的 `/canvas/v1/chat/completions` 同样改写为
  `/canvas/v1/images/generations`。Responses compact、普通文本模型和非 JSON
  请求保持原行为。

## 兼容性与安全边界

- 请求体仍由统一的 `common.BodyStorage` 管理，改写后会重建可重复读取的请求体，
  不影响重试和上游请求体透传。
- 不修改模型名、计费模型或渠道选择逻辑；自动路由发生在渠道选定后的 Relay
  请求校验前，图片计费与图片响应处理保持原链路。
- 无法解析的 JSON 不会被自动改写，继续由原有请求校验返回错误。
- 不对 `/v1/responses/compact`、Realtime、multipart 图片编辑或其他非图片模型做
  猜测性转换。

## 测试计划与结果

- 单元测试覆盖 Chat、Responses `input`、上下文模型回退、Canvas 路径、文本模型
  保持不变以及 Responses compact 排除。
- 执行 `go test ./controller -run 'TestAutoRouteImageRequest|TestImageGenerationPath'`。
- 交付前执行 `git diff --check`，并按项目固定入口检查本地服务状态；本次不涉及
  数据库迁移、配置新增或前端变更。

## 回滚

回滚 `controller/image_auto_route.go`、对应测试和 `controller/relay.go` 的调用点，
即可恢复原有按客户端路径选择 Relay 格式的行为；不涉及数据库回滚。
