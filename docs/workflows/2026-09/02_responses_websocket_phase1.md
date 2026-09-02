# Responses WebSocket 第一阶段实施记录

## 目标与范围

本次实现 NewAPI 下游 `GET /v1/responses` WebSocket，保留现有
`POST /v1/responses` HTTP/SSE 和 `GET /v1/realtime`。范围包含后端开关、会话帧规范化、
HTTP/SSE 事件桥接、Default/Classic CCS 导入和使用日志标签。TokensPro 上游 WebSocket
属于第二阶段，本记录不宣称全链路 WebSocket。

## 稳定合同

- `ResponsesWebsocketEnabled` 默认 `false`，关闭时 GET 返回 404，不升级连接。
- GET 路由位于 `/v1` Bearer Relay 组，使用现有鉴权、系统性能检查、模型限流、提示词审计
  和分发中间件；空 GET 握手跳过需要请求体的解析，每个 turn 用 synthetic POST 执行完整链路。
- 客户端帧支持 `response.create`、`response.append` 和 `previous_response_id` 校验；返回
  Responses JSON 事件文本帧，`[DONE]` 不重复生成终止事件。
- 日志写入 `other.transport` 与 `other.upstream_transport`。第一阶段取值分别为
  `websocket` 与 `http`；普通 HTTP/SSE 为 `http`。
- Codex CCS 导入增加 `supports_websockets=true`，Claude/Gemini 不增加。

## 代码提交

- `d7fea41a3`：设计文档。
- `f6fc821eb`：实施计划。
- `2f5c13229`：后端开关与传输元数据。
- `8bad5f183`：Responses WebSocket 帧规范化与会话状态。
- `4a71a88a2`：HTTP/SSE 到 WebSocket sink。
- `550ecbc46`：WebSocket 控制器、隔离 turn、断连取消和计费链路。
- `6be8634cc`：GET 路由注册与路由合同测试。
- `ca337a70a`：Default/Classic CCS 导入、日志标签和系统开关。

## 本地验证

- Go：`go test ./router ./controller ./relay ./service ./model -count=1 -timeout 60s`，
  通过；Responses WebSocket 定向测试通过。
- Classic：CCS/transport 原生 Node 测试 11/11 通过；触及文件 Prettier 检查通过；
  Vite 生产构建通过（16792 modules transformed）。
- Default：代码和 locale 已提交；当前工作环境缺少 `web/node_modules` 中的 Vitest、
  tsgo、oxlint、oxfmt 和 Rsbuild，因此 Default 测试、类型检查、lint 与构建未能执行。
- 所有已执行检查：`git diff --check` 通过；Classic 工作流未访问真实后端数据。

## 发布与回滚边界

- 本记录未执行 pull、restart、up、deploy、push 或生产变更。
- 真实验收目标为已审计的 zzapi：CloudSSH `serverId=52`，目录 `/home/docker/zzapi`，
  三个应用服务逐个验证。`maolaoapi`/`serverId=38` 不在本任务范围。
- 发布前必须先补齐 Default 构建依赖并完成真实登录、单轮/多轮 Codex WS、关闭开关的
  HTTP/SSE 回退和 `other.transport` 日志验证。回滚优先关闭 `ResponsesWebsocketEnabled`；
  必要时回退镜像。

## 第二阶段交接

TokensPro Issue #20 保持开放。只有 TokensPro 实际连接 NewAPI GET WebSocket、完成上游
WS 桥接、重连/失败转移、usage 与 `upstream_transport=websocket` 集成测试后，才可宣称
全链路 WebSocket。
