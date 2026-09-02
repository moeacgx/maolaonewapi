# Responses WebSocket 支持设计

## 状态

- 设计状态：待审阅
- 设计日期：2026-09-02
- 第一阶段范围：NewAPI 下游 `GET /v1/responses` WebSocket
- 第二阶段范围：TokensPro 上游 WebSocket，单独实施

## 目标

让支持 WebSocket 的 Codex/CLIProxyAPI 客户端可以通过 NewAPI 的
`GET /v1/responses` 建立长连接，并在连接内发送多轮 Responses 请求；现有
HTTP 和 HTTP/SSE 接口继续可用。管理员可以通过开关启用或关闭该能力，使用日志
能够明确区分客户端使用的是 WebSocket 还是 HTTP/SSE。

## 非目标

- 第一阶段不修改 TokensPro，不把 WebSocket 转发到 TokensPro。
- 第一阶段不改变 `/v1/realtime` 的音频实时协议。
- 第一阶段不新增日志数据库列，不迁移已有日志数据。
- 第一阶段不改变普通 API Key 的鉴权语义、渠道选择规则或计费价格。
- 不在没有协议证据的情况下实现其他供应商自定义 WebSocket 帧格式。

## 现状与依据

当前路由已经提供 `GET /v1/realtime`，由 `controller/relay.go` 升级连接并由
`relay/websocket.go` 转发；普通 `POST /v1/responses` 仍走 HTTP/SSE。现有
`RelayInfo` 已保存客户端和上游 WebSocket 连接，但普通 Responses 没有连接级会话
状态。

CLIProxyAPI 的 Responses WebSocket 实现使用 JSON 文本帧：

- 首轮请求：`{"type":"response.create", ...}`
- 后续增量请求：`{"type":"response.append", "input":[...]}`，或继续使用
  带 `previous_response_id` 的 `response.create`
- 返回帧：Responses 事件对象，例如 `response.created`、
  `response.output_text.delta`、`response.completed`、`response.done` 和 `error`
- 握手：`GET /v1/responses`，鉴权和会话信息通过 HTTP Header 传递

该合同与上游仓库公开实现和本仓库上游 Issue #3027 的客户端请求样例一致。
第一阶段只保证下游合同；NewAPI 连接上游时继续复用现有 HTTP/SSE 适配器。

## 运行开关

新增全局布尔选项 `ResponsesWebsocketEnabled`，默认值为 `false`，并接入现有系统
设置读取、保存和缓存刷新流程。关闭时：

- `POST /v1/responses` 行为不变；
- `GET /v1/responses` 不升级连接，返回 404 或等价的“不支持此路由”响应，使
  Codex 客户端能够按其既有策略回退到 HTTP/SSE；
- 不影响 `GET /v1/realtime`。

第一阶段不强制所有租户或令牌使用 WS。该开关只控制服务端是否接受下游 WS 握手。

## 路由与中间件

在 `/v1` Bearer Relay 路由组中新增：

```text
GET /v1/responses        -> ResponsesWebsocket
POST /v1/responses       -> 现有 Relay(RelayFormatOpenAIResponses)
```

新 GET 路由必须复用现有 `RouteTag("relay")`、系统性能检查、`TokenAuth`、模型
限流、提示词审计和 `Distribute` 中间件。只有鉴权和开关检查通过后才调用 Gorilla
WebSocket Upgrade；Upgrade 失败时保留 HTTP 错误，不创建计费记录。

## 下游 WebSocket 会话

一次 WebSocket 连接对应一个下游会话，连接内的请求严格串行处理，禁止多个 turn
同时写入同一个连接。每个 turn 都拥有独立的内部 request ID；优先使用客户端
`X-Client-Request-ID`，缺失时生成 UUID，不能把整个连接的 ID 当作所有 turn 的
幂等键。

### 首轮 `response.create`

1. 读取 JSON 文本或二进制消息；二进制消息按 UTF-8 JSON 解析，其他消息类型忽略。
2. 要求 `model` 和 `input`（缺失 `input` 时按空数组兼容）。
3. 移除内部事件字段 `type`，强制上游请求 `stream=true`。
4. 通过现有 Responses 请求校验、模型映射、渠道选择和计费预扣流程。
5. 将上游 HTTP/SSE 的每个 JSON `data:` 事件转换为 WebSocket 文本 JSON 帧。

### 后续 `response.append` / 增量 `response.create`

服务端保存上一轮的规范化请求、已完成输出、响应 ID 和未完成工具调用状态。
后续输入按 Responses 事件合同合并为完整的 HTTP 请求；如果客户端带有
`previous_response_id`，必须与当前连接保存的响应 ID 匹配，否则发送：

```json
{"type":"error","error":{"code":"previous_response_not_found"}}
```

随后关闭本轮，不重复扣费。若客户端发送完整转录，服务端使用完整转录替换旧状态，
避免重复拼接工具调用。

### 返回与关闭

- 上游 SSE 的 JSON 数据帧原样作为 WebSocket 文本帧发送，保留事件 `type` 和字段。
- `[DONE]` 只在上游没有终止事件时转换成单个 `response.done`，不得重复发送终止事件。
- 上游在首个事件前失败时，复用现有可重试错误和跨渠道重试；已发送部分事件后
  不自动重放，改为发送 `error` 或带原因的 WebSocket Close 帧。
- 客户端正常关闭、网络断开和请求上下文取消都必须停止上游请求、释放计费会话和
  渠道并发槽位。
- 一个 turn 结束后保留连接和会话状态，连接关闭时统一释放全部状态。

## HTTP/SSE 上游桥接

第一阶段的上游请求继续调用现有 `ResponsesHelper` 所使用的适配器和 HTTP 请求
路径，不要求渠道实现 WebSocket。需要抽取一个可测试的事件输出边界，使 Responses
流事件可以写入 WebSocket，而不依赖 `gin.ResponseWriter` 的 SSE Header/Flush。

桥接层必须保持：

- 现有请求体转换、透传、参数覆盖和禁用字段策略；
- 现有模型价格、预扣、结算、退款、福利券/订阅/钱包组合计费；
- 现有 retry、channel metric、request archive 和 prompt audit 语义；
- 普通 POST 请求仍使用原来的 HTTP/SSE 写出路径。

如果上游适配器不能产生可解析的 Responses 事件，按现有错误合同返回错误，不把
Chat SSE 数据未经转换地伪装成 Responses WebSocket 事件。

## 鉴权、安全与资源限制

- WebSocket 握手前执行现有 Bearer Token 鉴权、令牌状态、IP 白名单、模型限流和
  提示词审计；不接受通过查询参数传递 API Token。
- 继续使用项目统一的 WebSocket Upgrade 配置和 Origin 策略，禁止把客户端的
  `Sec-WebSocket-*` 伪造 Header 透传给 HTTP 上游。
- 单连接只允许一个活动 turn；消息大小、输入数组和会话历史使用与 Responses
  HTTP 相同的请求上限，超限发送 `message_too_big`/400 等价错误并释放连接。
- 任何客户端断开路径都必须取消上游 context，避免 goroutine、读取体和并发槽位泄漏。

## 计费、幂等与日志

每个 turn 复用现有 Responses 的“预扣 -> 实际 usage 结算 -> 失败退款”链路。内部
request ID 按 turn 隔离，重复收到相同 request ID 时不得重复写入消费日志或重复扣费。
跨 turn 的响应状态只用于拼接输入，不用于重复计费。

在现有 `Log.Other` JSON 中增加稳定字段：

```json
{
  "transport": "websocket",
  "upstream_transport": "http"
}
```

普通 HTTP 和 HTTP/SSE 请求写入 `transport: "http"`；现有 Realtime 写入
`transport: "websocket"`；旧日志没有该字段时前端显示“未知”。第一阶段的
`upstream_transport` 固定为 `http`，为第二阶段 TokensPro 的真正上游 WS 留出区分
空间。字段写入必须经过统一的日志辅助函数，不能在各个适配器中复制字符串。

Default 和 Classic 使用日志详情与移动端卡片都显示传输标签，但不改变普通用户的
管理员字段过滤边界；缺失字段按旧日志兼容显示。

## CCS/Codex 导入合同

Default 和 Classic 的 CCS URL 构造器在 `app=codex` 时增加：

```text
supports_websockets=true
```

Claude、Gemini 导入不增加该参数。该参数是 CCS/Codex 客户端的能力声明，不是
NewAPI 令牌数据库字段，不改变令牌鉴权，也不绕过服务端 `ResponsesWebsocketEnabled`
开关。链接解析、API 地址、`/v1` 拼接和现有模型参数保持不变。

## 前端模板边界

本变更明确覆盖 Default (`web/src`) 与 Classic (`web/classic`) 两套模板：

- Default：`web/src/features/keys` 的 CCS 导入、`web/src/features/usage-logs` 的
  日志标签、系统设置开关和对应测试；
- Classic：`web/classic/src/helpers/ccSwitch.js`、令牌表 CCS 模态框、日志详情/卡片、
  系统设置开关和对应 `node:test` 契约测试；
- 两套模板共享后端 API 和 `other.transport` 合同，但构建、样式和测试分别维护。

## 第二阶段 TokensPro 接口边界

第一阶段完成并在 NewAPI 本地合同测试通过后，单独创建 TokensPro 任务：

- TokensPro 连接 NewAPI 的 `GET /v1/responses`，发送同一 `response.create`/
  `response.append` 帧；
- TokensPro 将 NewAPI 的 WS 会话桥接到其自身上游，成功时 `upstream_transport` 才
  变为 `websocket`；
- TokensPro 不得改变 NewAPI 的鉴权、usage、request ID 或日志字段语义；
- 第二阶段必须有独立的集成测试和逐项目发布，不与第一阶段混合部署。

## 测试与验收要求

后端：

- 开关关闭时 GET 路由回退、开启时 Upgrade 和 Bearer 鉴权；
- `response.create`、`response.append`、`previous_response_id` 和完整转录替换；
- SSE 事件到 JSON WS 帧、`[DONE]` 去重、错误帧和 Close 原因；
- 首事件前重试、部分输出后不重放、客户端断开取消和并发槽位释放；
- 每 turn 预扣/结算/退款幂等，以及 `transport`/`upstream_transport` 日志字段；
- SQLite 定向测试，并执行 MySQL/PostgreSQL 兼容性检查所能覆盖的测试集合。

Default：

- Codex CCS URL 只对 Codex 增加 `supports_websockets=true`；
- 使用日志显示 websocket/http/未知三种状态；
- 系统设置开关读写、刷新和默认关闭状态。

Classic：

- 同样的 CCS URL 参数条件；
- 桌面和移动日志显示传输标签；
- 系统设置开关读写和默认关闭状态；
- 使用项目原生 `node:test` 契约测试。

构建验收：后端 `go test`、Default Vitest/typecheck/build、Classic 定向
`node:test`/lint/build；真实 TokensPro 联调留到第二阶段，不能用本地假数据替代。

## 发布与回滚

第一阶段默认关闭开关发布，先在 zzapi 验证 GET 握手、单轮和多轮 Codex 请求、日志
标签以及 HTTP 回退，再按用户授权逐实例更新 maolaoapi。回滚只需关闭
`ResponsesWebsocketEnabled` 或回退镜像；HTTP/SSE 与 `/v1/realtime` 不随该开关关闭。
未完成 TokensPro 联调前，不宣称“全链路 WS”。
