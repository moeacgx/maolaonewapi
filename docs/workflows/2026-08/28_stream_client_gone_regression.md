# 流式客户端断开归因回归修复

## 变更目标

处理 `.267` 线上大量流式请求被记录为 `client_gone` / `context canceled`，而
`v243` 行为正常的问题。目标是恢复已验证的取消语义和上游请求生命周期，保留
真实上游错误，不以吞掉错误或伪造成功掩盖断流。

## 对比结论

- `v243`：`2b538c8e98a14004b8948b6e5fb7482cfeb32941`。
- `.267`：`c885fd965d5e06df430764d705c967d03f99ddbf`。
- 回归合并：`d182efadc57279807eeb8807bbc66b886e2458d5`。

`d182efadc` 合并时丢失了 `relay/helper/stream_scanner.go` 中的客户端断开识别、
ping 写入竞态保护、上游取消与客户端取消区分及正常断开日志降级；同时
`relay/channel/api_request.go` 的三个 HTTP 请求入口从
`http.NewRequestWithContext` 退回到 `http.NewRequest`，导致入口请求取消后上游请求
不能及时停止。控制器也缺少 `v243` 的请求上下文错误日志抑制，进一步放大了
`context canceled` 的错误日志和错误日志记录。

## 线上只读诊断

诊断通过 CloudSSH 在服务器 38（项目“API中转站”）执行，未读取凭据、令牌、Cookie、
请求体或完整请求 ID，未重启容器、未写入业务数据、未修改数据库。

最近 6 小时三实例的 `client_gone` 聚合如下，数量随采样时间增长，比例用于判断实例
分布：

| 实例 | stream reason=client_gone | 其中 context canceled | stream reason=scanner_error |
| --- | ---: | ---: | ---: |
| maolaoapi | 约 442 | 约 442 | 8 |
| slave-1 | 约 409 | 约 409 | 8 |
| slave-2 | 约 455 | 约 455 | 5 |

异常关联日志中的路由主要是 `/v1/responses`（主实例采样约 784 条），
`/v1/messages` 约 15 条，`/v1/chat` 约 1 条，三实例分布一致。流结束记录中的错误文本为
`context canceled`，而不是上游明确返回的业务错误。

主机 Nginx 的 `maolaoapi_backend` 使用 `least_conn`，后端为本机三个固定端口；流式
location 配置为 `proxy_buffering off`、`proxy_http_version 1.1`、
`proxy_read_timeout 600s` 和 `proxy_send_timeout 600s`。负载均衡只在建立请求时选后端，
不会在同一 HTTP 响应流中切换实例。应用流结束日志没有持续时长字段，当前只读采样
因此不臆造请求持续时间；发布后应从 Nginx `take_time`/`upstream_response_time`
指标按路由和状态码聚合。

## 修复范围

- 恢复 `newRelayHTTPRequest`，让 OpenAI、表单和任务 HTTP 转发继承 Gin 入口上下文。
- 恢复流扫描器对 `context.Canceled`、关闭响应体、断管和连接重置的客户端归因，
  并在客户端取消时先关闭上游响应体、停止 ping 和扫描器。
- 在 ping 写入前检查内部停止信号和入口上下文，避免终态后继续写入。
- 将客户端正常断开降为信息日志；扫描器、ping、超时和 panic 等真实异常仍保留错误
  归因和日志。
- 控制器仅对与当前入口上下文一致的取消错误跳过错误响应、渠道自动禁用和错误日志
  记录；独立的上游 `context.Canceled` 仍按真实上游错误处理。

## 行为契约

- 客户端正常断开：`client_gone`，关闭上游 Body，不写第二个错误响应，不伪造成功。
- 上游主动返回 `context.Canceled` 且客户端仍在线：`scanner_error`，保留错误。
- 收到 `[DONE]` 或 EOF：分别为 `done` / `eof`，记录正常完成。
- 流式空闲超过配置：`timeout`；ping 写入失败：`ping_fail`，除非错误明确来自
  客户端连接关闭。

## 验证与发布

已执行：

- `go test -timeout 60s ./relay/helper`
- `go test -timeout 60s ./relay/channel`
- `go test -timeout 60s ./controller`
- `git diff --check`

回归测试覆盖客户端取消、上游取消、正常 EOF/`[DONE]`、流式超时、ping 终止和
Flush 前的入口上下文取消。未修改前端流消费逻辑，因此无需前端构建。

发布采用三实例逐一滚动方式：先观察一个实例，再扩展到其余实例；无需数据库迁移，
可通过回滚单个提交恢复。验收指标：`client_gone` 仅在客户端连接确实关闭时出现，
`scanner_error` 不再把入口取消重复计入；三实例的流错误率、上游未完成请求数、
Nginx `499`、`upstream_response_time` p95/p99 和日志错误量均需与 v243 基线对比，
并确认正常 `[DONE]` 完成率不下降。
