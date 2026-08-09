# 单分组普通上游失败重试兜底

日期：2026-08-09

## 问题

管理员已配置 `RetryTimes > 0` 且 `AutomaticRetryStatusCodes` 包含 500 时，单分组、单候选渠道收到上游 5xx（例如 `status_code=500, Upstream service temporarily unavailable`）后仍可能直接把首个错误返回给客户端。

根因是 relay 在判定 5xx 可重试后会优先把失败渠道加入本次请求排除列表，用于切换其它候选渠道；但单分组只有一个可用渠道时，下一次选渠会因为排除列表为空集命中而提前结束，未真正消耗 `RetryTimes` 预算。

## 变更

- 普通可重试失败仍优先排除当前渠道，以便有其它候选时继续切换渠道。
- 当单分组请求已经进入重试、且排除后没有任何未尝试候选渠道时，可以放宽“普通失败”产生的渠道排除，复用已失败渠道继续消耗剩余重试次数。
- 429、403、上游容量错误、多分组和 `auto` 跨组重试仍保持强制切换/不回退语义，避免限流放大、权限错误重复撞同渠道或破坏分组顺序。
- 自引用渠道、渠道类型排除等硬排除不参与放宽，仍必须保持阻断。

## 兼容性

不新增配置、接口、数据表或迁移。现有 `RetryTimes` 和 `AutomaticRetryStatusCodes` 继续控制是否进入重试；本次只改变“单分组没有其它候选渠道时”的普通失败兜底选择。

## 验证计划

- 覆盖普通失败会记录可放宽排除；429、403、容量错误、跨组重试不会记录可放宽排除。
- 覆盖单分组没有未尝试候选时，可以复用被普通失败排除的渠道；硬排除渠道不被复用。
- 执行 controller/service 相关单元测试。
- 执行 `git diff --check`。

## 验证结果

- `go test ./controller -run "TestExcludeChannelFromRetryPreservesControlledReuse|TestShouldRetryWithReason|TestRemainingRelayRetriesUsesGlobalAttemptIndex" -count=1 -timeout 60s`：通过。
- `go test ./service -run "TestSingleGroupRetryFallsBackToRetryExcludedChannelWhenNoAlternative|TestChannelTypeExclusion|TestMultiGroupFailureAdvancesImmediately|TestAutoCrossGroupRetryAdvancesImmediately" -count=1 -timeout 60s`：通过。
- `go test ./controller ./service -count=1 -timeout 120s`：通过。
- `git diff --check`：通过。