# 开发反思报告：内容审计阻断统一为 403

**日期**: 2026-07-30
**提交类型**: bugfix
**修改范围**: 内容审计客户端错误契约、HTTP/SSE/Realtime 输出与测试

## 1. 概述

本次修复把本地屏蔽词阻断的普通 HTTP 状态从 400 调整为 403，并将客户端正文统一为
“内容审计命中风险规则，请调整输入后重试”，随后追加请求编号。服务内部继续保留
`sensitive_words_detected` 作为审计、重试和指标判断依据，但不再通过客户端
`error.code`、`metadata`、Realtime 错误事件或关闭原因暴露该内部分类。

## 2. 修改内容

### 修改的文件

- `service/request_filter.go`：集中定义客户端正文、403 状态和错误对象脱敏转换。
- `service/http.go`、`controller/relay.go`：统一请求与非流式响应的最终写出契约。
- `middleware/prompt_audit.go`、`middleware/prompt_audit_realtime.go`：统一前置 HTTP 与
  Realtime 阻断行为。
- `relay/channel/openai/relay-openai.go`：统一已建立 Realtime 连接后的错误事件与关闭码。
- 对应测试与安全审计专题文档：覆盖 HTTP、SSE、Realtime 和 Relay 后备响应路径。

### 主要变更

- 普通 HTTP 请求和非流式响应阻断返回 HTTP 403。
- SSE 保持 HTTP 200，并通过 `event: error` 返回固定正文。
- Realtime 返回固定正文并以 WebSocket 4403 关闭，关闭原因使用通用分类。
- 客户端 OpenAI 错误对象返回 `code=null`，不返回内部 `metadata`。
- 内部稳定错误码和不可重试标记不变，避免破坏模型广场过滤、渠道指标和安全审计。

## 3. 遇到的错误

### 错误 1：最终响应点仍写死 HTTP 400

**严重程度**: 重要
请求前置阻断改为 403 后，非流式上游响应过滤和 Relay 后备写出仍可能返回旧状态。

### 错误 2：客户端仍能读取内部分类

**严重程度**: 重要
正文虽然改为面向审核场景的提示，但 `error.code` 和 `metadata.error_code` 仍会暴露
`sensitive_words_detected`，不符合客户端契约。

### 错误 3：文档与实现互相矛盾

**严重程度**: 次要
专题文档一处声明传输状态不变，另一处又声明 400 调整为 403；旧测试名称也仍描述为
“中英文错误”。

## 4. 根本原因分析

内容审计阻断存在多个最终写出点：中间件前置阻断、普通 Relay 错误、非流式响应过滤、
SSE 和 Realtime。只修改公共错误构造函数不能保证所有调用点都采用同一客户端契约。
同时，内部错误对象既承担服务端控制流又直接用于客户端序列化，导致内部分类容易随
通用转换函数泄露。

## 5. 调试过程

1. 全局检索 `sensitive_words_detected`、`StatusBadRequest` 和各传输层错误写出点。
2. 区分内部控制错误与客户端序列化对象，新增集中式客户端转换函数。
3. 修正两个残留的最终 400 响应点，并统一 Realtime 关闭原因。
4. 更新 HTTP、SSE、Realtime 与 Relay 后备路径测试，断言请求编号只追加一次。
5. 独立审查文档与实现，消除状态码、元数据和测试命名不一致。

## 6. 经验总结

- 对外错误契约必须按所有传输和最终写出点逐项核对，不能只检查错误构造位置。
- 内部稳定错误码应继续服务于审计和控制流，但客户端响应应通过显式转换形成最小暴露面。
- SSE 已写出响应头后不能改写状态码，必须把 HTTP 状态与事件级错误分别测试。

## 7. 知识提炼

### 可复用模式

- 内部错误对象保留稳定分类，在边界层转换为客户端专用对象。
- 同一策略拒绝分别定义 HTTP、SSE 和 WebSocket 契约，并共享唯一正文生成函数。

### 类似任务检查清单

- [x] 检查请求前置、响应后置、流式和 Realtime 写出点。
- [x] 检查客户端正文、状态码、结构化字段和关闭原因。
- [x] 确认内部审计、重试、计费和指标判断不受影响。
- [x] 同步专题文档、工作记录和开发文档索引。

## 8. 测试与验证

- `go test ./service ./controller`
- `go test ./middleware`
- `go test ./relay/helper ./relay/channel/openai`
- 受影响 Go 包 `go vet`
- Default 与 Classic 生产构建
- 固定本地环境验证 `/api/status`、Classic 首页、默认账号登录和演示数据接口
- `git diff --check`

全量 `go test ./...` 在项目规定的 60 秒内未完成并被终止；受影响包专项测试均通过。

## 9. 参考资料

- `docs/developer/prompt-security-audit.md`
- `docs/workflows/2026-07/29_sensitive_word_client_error_status.md`

## 10. 指标

- 严重错误数: 0
- 重要问题数: 2，均已修复
- 覆盖传输类型: HTTP、SSE、Realtime
- 新增后备响应契约测试: 1 组

---

**技能**: commit-with-reflection v3.0
