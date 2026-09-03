# 上游错误内容状态码参与自动重试

日期：2026-09-03

## 问题

部分中继或上游会把真实 HTTP 状态码放进错误内容，并把外层 HTTP 状态
改成 200 或 400。例如：

```text
Upstream returned HTTP 403 Forbidden
```

网关原先只使用外层 `http.Response.StatusCode`。因此外层 400 会被当作
不可重试，外层 200 的错误事件也可能按成功状态处理，无法使用后台配置的
自动重试状态码。

## 处理契约

- `relaykit/types.WithOpenAIError` 与 `service.RelayErrorHandler` 仅窄匹配明确的
  `Upstream returned HTTP NNN` 句式，其中 `NNN` 必须是 100–599。
- 匹配成功时，将该状态码作为当前错误的 `StatusCode`；当它与外层状态
  不同，则同时保存到 `OriginalStatusCode`，供自动重试和状态码映射使用。
- JSON 错误对象和纯文本错误正文都使用同一窄匹配；JSON 解析失败时仍会
  保留可信的嵌套上游状态码。
- 普通 `status_code=500`、`StatusCode = 500` 或本地校验文案不会被推断为
  上游状态，避免把本地错误误判为可重试上游错误。
- 已标记为上游来源的 `rate limit exceeded` / `rate limited` 错误统一归一为
  HTTP 429；本地错误不会进入该分类。
- 既有 `AutomaticRetryStatusCodeRanges`、`skipRetry`、指定渠道、响应已提交
  和重试次数门禁保持不变。该改动只恢复可信的上游状态来源。
- 命中亲和绑定的渠道发生符合换渠条件的失败时，网关只 compare-delete 本次命中的
  旧绑定，并在本请求内排除该渠道后选择其他候选渠道；并发成功请求已写入的新绑定
  不会被旧失败删除。流已提交时当前请求仍不会重试，但同样淘汰失败绑定，防止下一
  次请求继续命中它。最终失败即使被中继写成外层 200，也不会被记录为新的亲和绑定。

## 兼容性与安全边界

- 不新增数据库字段、配置项或迁移；SQLite、MySQL、PostgreSQL 均不受影响。
- `OriginalStatusCode` 继续表示用于重试/映射的原始上游状态；已有真实 HTTP
  状态码路径行为不变。
- `skip_retry_on_failure=true` 仍表示未被自动重试状态码明确允许的失败不做
  本请求的全局换渠重试；当 403、429、500 等状态已配置为可重试时，既有优先级
  仍允许换渠。淘汰绑定不会修改该规则的后续失败策略。
- 只接受明确的上游短语，不扫描任意数字或错误码字段，降低本地错误、用户输入
  和参数覆盖造成误重试的风险。
- `relaykit` 的解析代码只依赖标准库，保持独立模块构建能力。

## 验证

```text
cd relaykit && GOWORK=off go test ./types -run 'TestWithOpenAIError' -count=1 -timeout=60s
go test ./service -run 'TestRelayErrorHandlerUsesEmbeddedUpstreamHTTPStatus' -count=1 -timeout=60s
go test ./controller -run 'TestShouldRetryUsesEmbeddedUpstreamStatusAfterOuterMapping' -count=1 -timeout=60s
go test ./pkg/cachex ./service ./controller -count=1 -timeout=60s
```

回归覆盖：

- 外层 400、错误内容上游 403 可按 403 重试；
- 纯文本错误正文中的上游 403 也可按 403 重试；
- 外层 200、错误内容上游 429 可按 429 重试；
- 普通本地 400 文案不会被误判为上游状态。
- 上游限流文案在外层 200 时归一为 429 并触发换渠；
- 亲和命中渠道的可重试失败会淘汰旧绑定，且不会删除并发成功写入的新绑定；
- 最终失败不会因中继外层状态异常被当成成功而刷新亲和缓存。
