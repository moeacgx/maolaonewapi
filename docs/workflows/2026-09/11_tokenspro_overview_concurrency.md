# TokensPro overview 渠道并发对齐

## 问题

[Issue #195](https://github.com/moeacgx/maolaonewapi/issues/195) 同步了 TokensPro 下游 overview 合同。号池并发会随账号数、分组份额和健康降级变化；渠道并发如果一直是人手填的静态值，会打满上游或把份额关死。

NewAPI 的 `concurrency_limit = 0` 表示不限制，所以不能只把 `allowed = 0` 写成渠道并发。

## 方案

- 渠道 `settings` 增加显式开关，默认关闭，避免对非 TokensPro 的 OpenAI 兼容渠道误打 overview
- 每分钟系统任务轮询已开启的渠道，成功后按渠道间隔跳过
- 只写 `concurrency.allowed`；非 200 不改并发和状态
- `allowed = 0` 自动禁用，并只恢复因此被禁用的渠道
- Classic 与 Default 编辑页提供开关、间隔和上次同步信息

## 范围

- 后端：`relaykit/dto.ChannelOtherSettings`、`service/tokenspro_overview.go`、系统任务 `tokenspro_overview`、监控自动恢复跳过
- 前端：Classic `EditChannelModal`、Default 渠道编辑保留字段
- 文档：`docs/developer/tokenspro-overview-concurrency.md`

## 验证

- `go test ./service -run TokensProOverview -count=1`
- `cd relaykit && GOWORK=off go build ./...`
- `gofmt`、`git diff --check`
- `cd web && bun test src/features/channels/lib/__tests__/retained-channel-contracts.test.ts`

未连接生产 TokensPro，也未部署。
