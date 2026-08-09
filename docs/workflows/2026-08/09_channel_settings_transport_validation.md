# 渠道代理与 HTTP 传输配置保存校验

## 问题与目标

渠道 `settings` 支持代理地址和 HTTP 传输参数。旧保存校验只检查 JSON 可解析，非法代理 URL、带路径/查询片段的代理地址、非法 HTTP 协议值或 HTTP/1 与 HTTP/2 分片组合冲突可以被持久化，运行期才暴露错误。

本次目标是在渠道保存阶段拒绝无效配置，避免坏配置进入数据库。

## 实现契约

- `common.ParseProxyURLStrict` 用于保存期校验代理地址：允许空值；非空时只允许 `http`、`https`、`socks5`、`socks5h`，必须包含 host，端口必须在 `1..65535`，不得包含 path、query 或 fragment。
- `dto.ChannelSettings.ValidateHTTPTransport` 校验渠道 HTTP 传输字段：`http_protocol` 只允许空值、`auto`、`http1`；`http2_connection_shards` 范围为 `0..8`；当 `http_protocol=http1` 时分片数不得大于 1。
- `model.Channel.ValidateSettings` 在解析 `settings` 后执行代理与传输校验，返回带上下文的错误，阻止保存坏配置。

## 兼容性

不新增数据表或字段。已有空配置继续通过；运行期兼容清洗逻辑不变。本变更只收紧新保存/更新的渠道设置。

## 验证

- `go test ./common ./dto ./model -run 'Test(ParseProxyURLStrictRejectsUnsafeOrAmbiguousValues|ChannelSettingsValidateHTTPTransport|ChannelValidateSettingsRejectsInvalidProxyAndHTTPTransport)' -count=1`
