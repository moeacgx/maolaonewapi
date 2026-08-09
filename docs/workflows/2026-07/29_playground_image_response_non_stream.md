# 操练场图片模型响应改为非流式显示

## 问题

图片模型从操练场提交到 `/pg/chat/completions` 时，后端会自动改写到图片生成
端点。图片生成端点始终返回标准 JSON（`data[].url` 或 `data[].b64_json`），
不返回 Chat Completions SSE。操练场默认开启流式请求，收到 JSON 后按 SSE 解析，
因此显示“解析响应数据时发生错误”。

## 修改范围

- Classic 和 Default 操练场识别内置图片模型后，自动关闭本次请求的流式模式。
- 非流式响应识别 `data[].url` 与 `data[].b64_json`，转换为操练场可显示的图片内容。
- 普通文本模型、自定义请求体的文本请求和标准 `/v1/images/generations` API 行为不变。

## 兼容性与安全边界

- 仅影响前端操练场请求；不会修改客户端提交的模型名、后端计费模型或渠道选择。
- 图片 Base64 只在浏览器当前消息中转换为 `data:image/png`，不写入服务端配置。
- 图片模型即使用户打开流式开关，也会按一次性 JSON 请求处理，避免错误解析。

## 验证

- 选择 `gpt-image-2`，保持“流式输出”开启，发送文本提示词，应显示生成图片而不是
  “解析响应数据时发生错误”。
- 选择 `gpt-4.1`，流式输出仍按 SSE 工作。
- 执行 `bun run build`（Classic）和 `bun run build:check`（Default），并执行
  `git diff --check`。
