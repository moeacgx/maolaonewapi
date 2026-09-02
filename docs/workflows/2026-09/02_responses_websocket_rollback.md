# Responses WebSocket 回滚记录

日期：2026-09-02

## 问题

Codex 使用自定义 Responses 提供方时，后台已启用 Responses WebSocket，
但生产使用日志持续显示 `transport: "http"`。真实请求未进入稳定的
`GET /v1/responses` WebSocket 握手流程。

## 回滚范围

- 移除 Responses WebSocket 下游路由、会话规范化、SSE 桥接和相关测试。
- 移除 `ResponsesWebsocketEnabled` 后端选项及 Default/Classic 设置入口。
- 移除 CCS/Codex WebSocket 能力声明和使用日志 WS 徽标。
- 恢复传输元数据、RelayInfo 和中间件到 WebSocket 功能前的行为。
- 删除对应设计、实施计划、发布记录，保留本回滚记录。
- 保留同一批次中无关的操练场分组权限修复。

## 兼容性与部署边界

回滚后仅保留原有 HTTP/SSE Responses 接口和既有 Realtime WebSocket 接口；
`GET /v1/responses` 不再注册，Codex 会按客户端策略回退到 HTTP/SSE。
本次只提交代码回滚，未修改生产容器；线上 `.297` 需要单独执行已确认的
发布回滚流程。

## 验证

- `go test ./model ./service ./relay/... ./controller ./router ./middleware -count=1 -timeout 60s`
- `go test ./model ./service ./relay/... ./controller ./router ./middleware -run '^$' -count=1 -timeout 60s`
- `git diff --check`

上述检查均通过，回滚提交为 `7237644bb9ece571c03fab4cd39a09d1fd840d48`。

后续已通过 `v1.0.0-rc.10.1.10.298` 发布记录完成 zzapi 线上部署；
`maolaoapi` 未在本次范围内更新。
