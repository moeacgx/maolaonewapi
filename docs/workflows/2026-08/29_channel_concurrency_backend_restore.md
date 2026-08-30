# 渠道并发上限后端能力恢复

## 根因

`d5d696164` 在 2026-06-02 引入 `channels.concurrency_limit`、进程内槽位计数、渠道选择过滤和分发层释放逻辑。`e97018616` 又加入并发上限缓降。2026-08-18 的 `d182efadc`（Merge staged upstream integration）在分支整合时删除了 `model.Channel.ConcurrencyLimit`、`model/channel_concurrency.go`、中间件接入、控制器批量更新和相关测试/文档，导致前端字段仍发送但后端静默忽略，列表响应也不再返回该字段。

## 本次恢复

- 恢复 `Channel.ConcurrencyLimit *int`，GORM 标记为 `default:0;not null`。
- 恢复线程安全的本地回退 acquire/release；`nil` 或 `0` 表示不限，负值不能作为有限上限。Redis 开启时有限上限改由共享租约实现，详见 [三容器渠道并发统一计数](30_channel_concurrency_multi_instance.md)。
- 恢复普通 PUT、标签批量 PUT 的校验和持久化，并为旧调用保留可选参数兼容。
- 缓存和非缓存选择器均在保持 request path、优先级、权重和 selection exclusions 的前提下跳过已满渠道，并继续尝试较低优先级候选。
- 分发层在 `SetupContextForSelectedChannel` 获取槽位；设置上下文或密钥选择失败时释放槽位。普通选择请求在抢占失败后排除该渠道并重新选择。
- 全部候选已满时返回独立的 `channel_concurrency_limit` 错误码，不伪装成模型不存在。

## 数据与升级兼容

`AutoMigrate(&Channel{})` 会在已有 `channels` 表缺少该列时添加列，并保留已有非零值。当前代码没有删除该列的迁移。升级前应先备份数据库；生产数据是否仍存在只能通过只读检查确认，本工作项未连接生产数据库。

如果历史列曾被外部脚本删除，无法从当前数据库恢复原值。恢复代码后，列表/详情 API 会重新返回该字段，前端保存请求才会真正生效。

## 回滚注意事项

回滚到不含 `ConcurrencyLimit` 的版本不会自动删除列，但该版本不会读取或更新列，且会再次使并发限制失效。不要用删列或重建 `channels` 表作为回滚步骤。

## 多实例边界

本记录描述的早期恢复版本只有单进程计数。当前版本已由共享 Redis 租约统一有限渠道的多实例计数；三容器必须使用同一个 Redis 服务和同一个逻辑数据库，否则会形成多个独立计数域。Redis 未启用时仍保留本地计数，仅适用于单实例部署或测试。

## 验证

- `go test ./model ./middleware ./controller -count=1 -timeout 60s`
- `cd relaykit && GOWORK=off go build ./...`
- `gofmt`、`git diff --check`

回归覆盖 JSON 字段契约、AutoMigrate 补列与非零值保留、单渠道 PUT 保存/回读、标签批量更新、`0`/`nil` 不限、选择器跳过已满渠道、全满独立错误和 SetupContext 失败释放槽位。
