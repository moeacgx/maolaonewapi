# v244 上游容量限流重试回归修复

日期：2026-08-31

## 问题与版本边界

OpenAI/Codex 的 Responses 接口可能先以 HTTP 200 建立 SSE，再通过 `error`、
`response.error` 或 `response.failed` 事件返回模型容量不足。例如：

```text
Selected model is at capacity. Please try a different model.
```

v243 已通过提交 `cd36373f1` 将这类事件归一为临时 429，并允许外层切换渠道重试。
v244 的 staged upstream 合并提交 `d182efadc` 删除了容量分类器、Responses 流错误
处理和对应回归测试。当前代码因此可能把 HTTP 200 流内错误当作零用量成功，或在已
写出前导事件后失去重试机会，客户端最终只看到流中断。

## 恢复后的契约

- 稳定错误码 `model_at_capacity`、`model_capacity_exhausted`、
  `model_overloaded`、`capacity_exhausted`、`account_pool_capacity_exhausted`
  和 `upstream_capacity_exhausted` 归类为官方容量限流。
- 对官方仍使用 `server_error` 的情况，仅在上游错误类型中窄匹配已知容量文案。
  本地错误即使包含相同文字，也不归类为官方限流；参数覆盖 `return_error` 会显式
  禁用上游容量分类，避免 OpenAI 形状的本地错误被误报。
- HTTP 200 的容量错误保留 `OriginalStatusCode=200`，内部状态归一为 429，客户端
  文案统一为“已触发 OpenAI 官方限流，请重试”。
- `/v1/responses` 暂存不含实际内容的 `response.created`、
  `response.in_progress`、`response.queued` 等前导事件。容量错误在客户端响应尚未
  提交时丢弃这些事件，由外层按重试预算切换渠道。
- Responses 上游转换为 Chat/Claude/Gemini 的实时流，以及缓冲后返回非流 Chat 的
  路径复用同一错误解析；`response.created` 不会先于后续容量错误提交响应。
- `response.content_part.added/done` 只在已知 `output_text` 类型且文本为空时暂存；
  `response.reasoning_summary_part.added/done` 只在已知 `summary_text` 类型且文本为空时
  暂存。`refusal`、缺少 part 和未知类型均立即视为实际输出，不能在后续容量错误时
  被丢弃或透明换渠。
- 如果实际文本、工具调用或其他内容已经下发，不再透明换渠，避免把两个上游响应
  拼成一条流；当前流会补发明确的官方限流错误事件。
- 重试前清理未提交的 SSE 头、敏感词缓冲，以及首个失败渠道复制的
  `X-Reasoning-Included` / `X-Codex-Turn-State`；第二渠道成功时只返回第二渠道的
  尝试级响应头。最终 JSON 429 同样不会残留 `text/event-stream`。
- 渠道亲和的“失败后不重试”不能拦截官方容量限流，但请求取消、指定渠道、响应已
  提交和重试预算耗尽仍保持原有门禁。
- 状态码映射保留并使用真实上游 `OriginalStatusCode` 做重试判断；映射后的 200/400
  不会掩盖原始 502，HTTP 200 流内容量错误也不会被合成状态再次覆盖。
- 官方容量限流是临时上游状态，即使管理员把 429 配入自动禁用范围，也不自动封禁
  渠道。

## 兼容性与安全边界

- 不新增配置、数据库字段或迁移，SQLite、MySQL 和 PostgreSQL 均不受影响。
- `relaykit` 仅使用自身类型和标准库，继续保持独立构建能力。
- 普通 Nginx 502、网络解码失败或无明确容量标记的流断开仍保留真实错误归因，不会
  被伪装为 OpenAI 官方限流。
- 前导事件最多暂存 16 个、累计 1 MiB；超过任一上限即按普通流转发，避免无界内存
  占用。

## 验证

聚焦测试覆盖：

- HTTP 200 官方容量错误归一为 429，并保留原始 200；
- 六个稳定容量错误码均归一为 429；
- 本地同文案错误不误判；
- 参数覆盖生成的 OpenAI 形状本地错误不误判；
- Responses 前导事件后的容量错误保持未提交并可重试；
- Responses→Chat 的实时/缓冲转换能识别顶层 `type:error`，实时 created 前导不会阻断重试；
- 已输出内容后在当前流内返回统一限流文案；
- 无 `type` 的非流式错误对象仍可识别；
- `response.failed.response.error` 嵌套容量错误仍可识别；
- `refusal` content part 会立即提交，并阻止后续容量错误透明换渠；
- 容量错误绕过亲和不重试门禁，但不自动禁用渠道；
- 重试后可重新设置 SSE 头，且不会叠加首个失败渠道的 Codex 尝试级响应头；
- 第 17 个前导事件或累计超过 1 MiB 时立即退化为普通流式提交；
- `relaykit` 在 `GOWORK=off` 下独立测试和构建。

所有 Go 测试命令使用不超过 60 秒的超时；最终结果以交付说明记录的本次实际执行为准。
