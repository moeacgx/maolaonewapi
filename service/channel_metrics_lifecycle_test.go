package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	channelmetrics "github.com/QuantumNous/new-api/pkg/channel_metrics"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/channel_metrics_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func installChannelMetricTestRuntime(t *testing.T) (*channelmetrics.Collector, <-chan []model.ChannelFailureEvent) {
	t.Helper()

	config := channelmetrics.DefaultConfig()
	config.ShardCount = 1
	collector := channelmetrics.NewCollector(config, nil)
	failureCh := make(chan []model.ChannelFailureEvent, 8)
	runtime := &channelMetricsRuntime{
		collector: collector,
		setting:   channel_metrics_setting.DefaultSetting(),
		failureCh: failureCh,
	}

	channelMetricsRuntimeMu.Lock()
	previous := channelMetricsCurrent
	channelMetricsCurrent = runtime
	channelMetricsRuntimeMu.Unlock()

	t.Cleanup(func() {
		channelMetricsRuntimeMu.Lock()
		if channelMetricsCurrent == runtime {
			channelMetricsCurrent = previous
		}
		channelMetricsRuntimeMu.Unlock()
	})

	return collector, failureCh
}

func newChannelMetricTestContext(t *testing.T, requestID string) *gin.Context {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(common.RequestIdKey, requestID)
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now().Add(-50*time.Millisecond))
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "requested-model")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	return c
}

func newChannelMetricTestRelayInfo() *relaycommon.RelayInfo {
	now := time.Now()
	return &relaycommon.RelayInfo{
		OriginModelName:   "requested-model",
		UsingGroup:        "default",
		StartTime:         now.Add(-30 * time.Millisecond),
		FirstResponseTime: now.Add(-10 * time.Millisecond),
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "upstream-model",
		},
	}
}

func drainChannelMetricTestBatch(t *testing.T, collector *channelmetrics.Collector) channelmetrics.MetricBatch {
	t.Helper()
	batch := collector.Drain()
	require.False(t, batch.Empty())
	require.Zero(t, batch.Quality.InvalidSampleCount)
	return batch
}

func channelMetricTestBuckets(batch channelmetrics.MetricBatch, scope channelmetrics.Scope, outcome channelmetrics.Outcome) []channelmetrics.Bucket {
	result := make([]channelmetrics.Bucket, 0)
	for _, bucket := range batch.Buckets {
		if bucket.Dimension.Scope == scope && bucket.Dimension.Outcome == outcome {
			result = append(result, bucket)
		}
	}
	return result
}

func channelMetricTestEventCount(buckets []channelmetrics.Bucket) int64 {
	var count int64
	for _, bucket := range buckets {
		count += bucket.Counters.EventCount
	}
	return count
}

func receiveChannelMetricFailureEvents(t *testing.T, failureCh <-chan []model.ChannelFailureEvent) []model.ChannelFailureEvent {
	t.Helper()
	select {
	case events := <-failureCh:
		return events
	default:
		require.FailNow(t, "未收到渠道失败事件")
		return nil
	}
}

func TestChannelMetricDisabledDoesNotEnqueueFailureEvents(t *testing.T) {
	setting := channel_metrics_setting.DefaultSetting()
	setting.Enabled = false
	failureCh := make(chan []model.ChannelFailureEvent, 1)
	runtime := &channelMetricsRuntime{setting: setting, failureCh: failureCh}

	channelMetricsRuntimeMu.Lock()
	previous := channelMetricsCurrent
	channelMetricsCurrent = runtime
	channelMetricsRuntimeMu.Unlock()
	t.Cleanup(func() {
		channelMetricsRuntimeMu.Lock()
		if channelMetricsCurrent == runtime {
			channelMetricsCurrent = previous
		}
		channelMetricsRuntimeMu.Unlock()
	})

	enqueueChannelFailureEvents([]model.ChannelFailureEvent{{EventId: "disabled-event", CreatedAt: 1}})
	select {
	case events := <-failureCh:
		require.Failf(t, "关闭渠道指标后不应入队失败事件", "events=%+v", events)
	default:
	}
}

func TestChannelMetricLifecycleRecordsSuccessfulRequestUsageAndStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	collector, failureCh := installChannelMetricTestRuntime(t)
	c := newChannelMetricTestContext(t, "request-success")
	info := newChannelMetricTestRelayInfo()

	BeginChannelMetricRequest(c)
	BindChannelMetricRelayInfo(c, info)
	BeginChannelMetricAttempt(c, info, 17, "主渠道", 1)
	callIndex := BeginChannelMetricUpstreamCall(c)
	require.Equal(t, 1, callIndex)
	CompleteChannelMetricUpstreamHeader(c, callIndex, http.StatusOK, nil)
	AttachChannelMetricUsage(c, ChannelMetricUsage{
		InputTokensTotal: 100,
		OutputTokens:     40,
		CacheReadTokens:  30,
		CacheWriteTokens: 10,
		ChargedQuota:     500,
	})
	FinishChannelMetricAttempt(c, info, nil, false, "")
	c.Writer.WriteHeader(http.StatusOK)
	FinishChannelMetricRequest(c, info, nil)

	batch := drainChannelMetricTestBatch(t, collector)
	require.Len(t, batch.Buckets, 3)

	attempts := channelMetricTestBuckets(batch, channelmetrics.ScopeChannelAttempt, channelmetrics.OutcomeSuccess)
	require.Len(t, attempts, 1)
	attempt := attempts[0]
	require.Equal(t, 17, attempt.Dimension.ChannelID)
	require.Equal(t, "主渠道", attempt.Dimension.ChannelNameSnapshot)
	require.Equal(t, "requested-model", attempt.Dimension.RequestedModel)
	require.Equal(t, "upstream-model", attempt.Dimension.UpstreamModel)
	require.True(t, attempt.Dimension.QualityEligible)
	require.EqualValues(t, 1, attempt.Counters.UsageSampleCount)
	require.EqualValues(t, 100, attempt.Counters.InputTokensTotal)
	require.EqualValues(t, 60, attempt.Counters.UncachedInputTokens)
	require.EqualValues(t, 40, attempt.Counters.OutputTokens)
	require.EqualValues(t, 30, attempt.Counters.CacheReadTokens)
	require.EqualValues(t, 10, attempt.Counters.CacheWriteTokens)
	require.EqualValues(t, 500, attempt.Counters.ChargedQuota)

	calls := channelMetricTestBuckets(batch, channelmetrics.ScopeUpstreamCall, channelmetrics.OutcomeSuccess)
	require.Len(t, calls, 1)
	require.Equal(t, channelmetrics.PresentStatus(http.StatusOK), calls[0].Dimension.UpstreamStatus)

	requests := channelMetricTestBuckets(batch, channelmetrics.ScopeFinalRequest, channelmetrics.OutcomeSuccess)
	require.Len(t, requests, 1)
	require.Equal(t, channelmetrics.PresentStatus(http.StatusOK), requests[0].Dimension.ClientStatus)

	select {
	case events := <-failureCh:
		require.Failf(t, "成功请求不应产生失败事件", "events=%+v", events)
	default:
	}
}

func TestAttachChannelMetricUsageAfterSettlementKeepsUsageWhenSettlementFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	installChannelMetricTestRuntime(t)
	c := newChannelMetricTestContext(t, "request-settlement-failed")
	info := newChannelMetricTestRelayInfo()

	BeginChannelMetricRequest(c)
	BindChannelMetricRelayInfo(c, info)
	BeginChannelMetricAttempt(c, info, 17, "主渠道", 1)
	AttachChannelMetricUsageAfterSettlement(c, ChannelMetricUsage{
		InputTokensTotal: 100,
		OutputTokens:     40,
		CacheReadTokens:  30,
		CacheWriteTokens: 10,
	}, 500, errors.New("settlement failed"))

	state := getChannelMetricRequestState(c)
	require.NotNil(t, state)
	state.mu.Lock()
	require.NotNil(t, state.current)
	require.NotNil(t, state.current.usage)
	usage := *state.current.usage
	state.mu.Unlock()

	require.EqualValues(t, 100, usage.InputTokensTotal)
	require.EqualValues(t, 60, usage.UncachedInputTokens)
	require.EqualValues(t, 40, usage.OutputTokens)
	require.EqualValues(t, 30, usage.CacheReadTokens)
	require.EqualValues(t, 10, usage.CacheWriteTokens)
	require.Zero(t, usage.ChargedQuota)
}

func TestAttachChannelMetricUsageAfterSettlementRecordsChargedQuotaOnSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	installChannelMetricTestRuntime(t)
	c := newChannelMetricTestContext(t, "request-settlement-succeeded")
	info := newChannelMetricTestRelayInfo()

	BeginChannelMetricRequest(c)
	BindChannelMetricRelayInfo(c, info)
	BeginChannelMetricAttempt(c, info, 17, "主渠道", 1)
	AttachChannelMetricUsageAfterSettlement(c, ChannelMetricUsage{
		InputTokensTotal: 100,
	}, 500, nil)

	state := getChannelMetricRequestState(c)
	require.NotNil(t, state)
	state.mu.Lock()
	require.NotNil(t, state.current)
	require.NotNil(t, state.current.usage)
	usage := *state.current.usage
	state.mu.Unlock()

	require.EqualValues(t, 100, usage.InputTokensTotal)
	require.EqualValues(t, 500, usage.ChargedQuota)
}

func TestChannelMetricLifecycleKeepsFailedAttemptWhenRetrySucceeds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	collector, failureCh := installChannelMetricTestRuntime(t)
	c := newChannelMetricTestContext(t, "request-retry")
	info := newChannelMetricTestRelayInfo()

	BeginChannelMetricRequest(c)
	BindChannelMetricRelayInfo(c, info)

	BeginChannelMetricAttempt(c, info, 11, "限流渠道", 1)
	firstCall := BeginChannelMetricUpstreamCall(c)
	CompleteChannelMetricUpstreamHeader(c, firstCall, http.StatusTooManyRequests, nil)
	firstErr := types.NewOpenAIError(errors.New("rate limited"), types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests)
	FinishChannelMetricAttempt(c, info, firstErr, true, "http_429")

	info.FirstResponseTime = time.Time{}
	info.SendResponseCount = 0
	info.ReceivedResponseCount = 0
	info.UpstreamModelName = "retry-upstream-model"
	BeginChannelMetricAttempt(c, info, 22, "备用渠道", 2)
	secondCall := BeginChannelMetricUpstreamCall(c)
	CompleteChannelMetricUpstreamHeader(c, secondCall, http.StatusOK, nil)
	FinishChannelMetricAttempt(c, info, nil, false, "")
	c.Writer.WriteHeader(http.StatusOK)
	FinishChannelMetricRequest(c, info, nil)

	batch := drainChannelMetricTestBatch(t, collector)
	require.EqualValues(t, 1, channelMetricTestEventCount(channelMetricTestBuckets(batch, channelmetrics.ScopeChannelAttempt, channelmetrics.OutcomeHTTPError)))
	require.EqualValues(t, 1, channelMetricTestEventCount(channelMetricTestBuckets(batch, channelmetrics.ScopeChannelAttempt, channelmetrics.OutcomeSuccess)))
	require.EqualValues(t, 1, channelMetricTestEventCount(channelMetricTestBuckets(batch, channelmetrics.ScopeFinalRequest, channelmetrics.OutcomeSuccess)))

	var retryAttempt *channelmetrics.Bucket
	for index := range batch.Buckets {
		bucket := &batch.Buckets[index]
		if bucket.Dimension.Scope == channelmetrics.ScopeChannelAttempt && bucket.Dimension.ChannelID == 22 {
			retryAttempt = bucket
			break
		}
	}
	require.NotNil(t, retryAttempt)
	require.Equal(t, "retry-upstream-model", retryAttempt.Dimension.UpstreamModel)
	require.EqualValues(t, 1, retryAttempt.Counters.NonFirstAttemptCount)

	events := receiveChannelMetricFailureEvents(t, failureCh)
	require.Len(t, events, 1)
	event := events[0]
	require.Equal(t, "request-retry", event.RequestId)
	require.Equal(t, 1, event.AttemptSeq)
	require.True(t, event.RetryPlanned)
	require.Equal(t, "http_429", event.RetryReason)
	require.False(t, event.IsLastStartedAttempt)
	require.True(t, event.CausalCallPresent)
	require.Equal(t, 1, event.CausalCallIndex)
	require.True(t, event.UpstreamStatusPresent)
	require.Equal(t, http.StatusTooManyRequests, event.UpstreamStatusCode)
	require.True(t, event.NormalizedStatusPresent)
	require.Equal(t, http.StatusTooManyRequests, event.NormalizedStatusCode)
	require.False(t, event.ClientStatusPresent, "前一次失败不能附加重试成功后的客户端状态码")
}

func TestChannelMetricLifecycleClassifiesStreamFailureAfterHTTP200(t *testing.T) {
	gin.SetMode(gin.TestMode)
	collector, failureCh := installChannelMetricTestRuntime(t)
	c := newChannelMetricTestContext(t, "request-stream-error")
	info := newChannelMetricTestRelayInfo()
	info.IsStream = true
	info.SendResponseCount = 1
	info.StreamStatus = relaycommon.NewStreamStatus()
	info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonTimeout, errors.New("stream timeout"))

	BeginChannelMetricRequest(c)
	BindChannelMetricRelayInfo(c, info)
	BeginChannelMetricAttempt(c, info, 33, "流式渠道", 3)
	callIndex := BeginChannelMetricUpstreamCall(c)
	CompleteChannelMetricUpstreamHeader(c, callIndex, http.StatusOK, nil)
	FinishChannelMetricAttempt(c, info, nil, false, "")
	c.Writer.WriteHeader(http.StatusOK)
	FinishChannelMetricRequest(c, info, nil)

	batch := drainChannelMetricTestBatch(t, collector)
	for _, scope := range []channelmetrics.Scope{
		channelmetrics.ScopeFinalRequest,
		channelmetrics.ScopeChannelAttempt,
		channelmetrics.ScopeUpstreamCall,
	} {
		buckets := channelMetricTestBuckets(batch, scope, channelmetrics.OutcomeStreamError)
		require.EqualValues(t, 1, channelMetricTestEventCount(buckets), "scope=%s", scope)
		require.True(t, buckets[0].Dimension.PartialResponse)
		require.Equal(t, channelmetrics.ErrorStageStream, buckets[0].Dimension.ErrorStage)
	}

	calls := channelMetricTestBuckets(batch, channelmetrics.ScopeUpstreamCall, channelmetrics.OutcomeStreamError)
	require.Equal(t, channelmetrics.PresentStatus(http.StatusOK), calls[0].Dimension.UpstreamStatus)

	events := receiveChannelMetricFailureEvents(t, failureCh)
	require.Len(t, events, 1)
	require.True(t, events[0].IsLastStartedAttempt)
	require.True(t, events[0].ClientStatusPresent)
	require.Equal(t, http.StatusOK, events[0].ClientStatusCode)
	require.True(t, events[0].UpstreamStatusPresent)
	require.Equal(t, http.StatusOK, events[0].UpstreamStatusCode)
	require.Equal(t, string(relaycommon.StreamEndReasonTimeout), events[0].StreamEndReason)
}

func TestChannelMetricLifecycleExcludesClientCancellationFromQuality(t *testing.T) {
	gin.SetMode(gin.TestMode)
	collector, failureCh := installChannelMetricTestRuntime(t)
	c := newChannelMetricTestContext(t, "request-cancelled")
	requestContext, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(requestContext)
	info := newChannelMetricTestRelayInfo()

	BeginChannelMetricRequest(c)
	BindChannelMetricRelayInfo(c, info)
	BeginChannelMetricAttempt(c, info, 44, "取消渠道", 4)
	callIndex := BeginChannelMetricUpstreamCall(c)
	CompleteChannelMetricUpstreamHeader(c, callIndex, http.StatusOK, nil)
	cancel()
	FinishChannelMetricAttempt(c, info, nil, false, "")
	c.Writer.WriteHeader(499)
	FinishChannelMetricRequest(c, info, nil)

	batch := drainChannelMetricTestBatch(t, collector)
	attempts := channelMetricTestBuckets(batch, channelmetrics.ScopeChannelAttempt, channelmetrics.OutcomeClientCancelled)
	require.Len(t, attempts, 1)
	require.Equal(t, channelmetrics.FailureOwnerClient, attempts[0].Dimension.FailureOwner)
	require.False(t, attempts[0].Dimension.QualityEligible)
	require.Zero(t, attempts[0].Counters.QualityEligibleCount)

	requests := channelMetricTestBuckets(batch, channelmetrics.ScopeFinalRequest, channelmetrics.OutcomeClientCancelled)
	require.Len(t, requests, 1)
	require.Equal(t, channelmetrics.PresentStatus(499), requests[0].Dimension.ClientStatus)

	events := receiveChannelMetricFailureEvents(t, failureCh)
	require.Len(t, events, 1)
	require.Equal(t, string(channelmetrics.OutcomeClientCancelled), events[0].Outcome)
	require.False(t, events[0].QualityEligible)
	require.Equal(t, string(channelmetrics.FailureOwnerClient), events[0].FailureOwner)
}

func TestClassifyChannelMetricAttemptPreservesCausalFailureDuringCancellation(t *testing.T) {
	c := newChannelMetricTestContext(t, "request-causal-cancel")
	requestContext, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(requestContext)
	cancel()

	t.Run("matching context error is client cancellation", func(t *testing.T) {
		relayErr := types.NewError(fmt.Errorf("request stopped: %w", context.Canceled), types.ErrorCodeDoRequestFailed)
		outcome, owner, stage, qualityEligible := classifyChannelMetricAttempt(c, newChannelMetricTestRelayInfo(), relayErr, false)
		require.Equal(t, channelmetrics.OutcomeClientCancelled, outcome)
		require.Equal(t, channelmetrics.FailureOwnerClient, owner)
		require.Equal(t, channelmetrics.ErrorStageConnect, stage)
		require.False(t, qualityEligible)
	})

	t.Run("real transport error is not hidden by later cancellation", func(t *testing.T) {
		relayErr := types.NewError(errors.New("connection reset by peer"), types.ErrorCodeDoRequestFailed)
		outcome, owner, stage, qualityEligible := classifyChannelMetricAttempt(c, newChannelMetricTestRelayInfo(), relayErr, false)
		require.Equal(t, channelmetrics.OutcomeTransportError, outcome)
		require.Equal(t, channelmetrics.FailureOwnerChannel, owner)
		require.Equal(t, channelmetrics.ErrorStageConnect, stage)
		require.False(t, qualityEligible)
	})

	t.Run("real scanner error wins over later cancellation", func(t *testing.T) {
		info := newChannelMetricTestRelayInfo()
		info.IsStream = true
		info.StreamStatus = relaycommon.NewStreamStatus()
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, errors.New("connection reset by peer"))
		outcome, owner, stage, qualityEligible := classifyChannelMetricAttempt(c, info, nil, true)
		require.Equal(t, channelmetrics.OutcomeStreamError, outcome)
		require.Equal(t, channelmetrics.FailureOwnerChannel, owner)
		require.Equal(t, channelmetrics.ErrorStageStream, stage)
		require.True(t, qualityEligible)
	})
}

func TestClassifyChannelMetricAttemptExcludesContentPolicyFromQuality(t *testing.T) {
	c := newChannelMetricTestContext(t, "request-content-policy")
	info := newChannelMetricTestRelayInfo()
	info.IsStream = true
	info.StreamStatus = relaycommon.NewStreamStatus()
	info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, errors.New("blocked"))

	t.Run("upstream cyber policy wins over stream error", func(t *testing.T) {
		relayErr := types.WithOpenAIError(types.OpenAIError{
			Message: "blocked", Type: "invalid_request_error", Code: "cyber_policy",
		}, http.StatusForbidden)
		outcome, owner, stage, qualityEligible := classifyChannelMetricAttempt(c, info, relayErr, true)
		require.Equal(t, channelmetrics.OutcomeHTTPError, outcome)
		require.Equal(t, channelmetrics.FailureOwnerClient, owner)
		require.Equal(t, channelmetrics.ErrorStageUpstream, stage)
		require.False(t, qualityEligible)
	})

	t.Run("local sensitive filter is not a connection failure", func(t *testing.T) {
		responseContext := newChannelMetricTestContext(t, "request-sensitive-response")
		MarkChannelMetricContentPolicyRejected(responseContext)
		outcome, owner, stage, qualityEligible := classifyChannelMetricAttempt(responseContext, info, nil, true)
		require.Equal(t, channelmetrics.OutcomeLocalError, outcome)
		require.Equal(t, channelmetrics.FailureOwnerClient, owner)
		require.Equal(t, channelmetrics.ErrorStagePreUpstream, stage)
		require.False(t, qualityEligible)
	})
}

func TestChannelMetricLifecycleCountsMultipleUpstreamCallsInsideOneAttempt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	collector, _ := installChannelMetricTestRuntime(t)
	c := newChannelMetricTestContext(t, "request-multiple-calls")
	info := newChannelMetricTestRelayInfo()

	BeginChannelMetricRequest(c)
	BindChannelMetricRelayInfo(c, info)
	BeginChannelMetricAttempt(c, info, 55, "多调用渠道", 5)
	for expectedIndex := 1; expectedIndex <= 2; expectedIndex++ {
		callIndex := BeginChannelMetricUpstreamCall(c)
		require.Equal(t, expectedIndex, callIndex)
		CompleteChannelMetricUpstreamHeader(c, callIndex, http.StatusOK, nil)
	}
	FinishChannelMetricAttempt(c, info, nil, false, "")
	c.Writer.WriteHeader(http.StatusOK)
	FinishChannelMetricRequest(c, info, nil)

	batch := drainChannelMetricTestBatch(t, collector)
	require.EqualValues(t, 2, channelMetricTestEventCount(channelMetricTestBuckets(batch, channelmetrics.ScopeUpstreamCall, channelmetrics.OutcomeSuccess)))
	require.EqualValues(t, 1, channelMetricTestEventCount(channelMetricTestBuckets(batch, channelmetrics.ScopeChannelAttempt, channelmetrics.OutcomeSuccess)))
	require.EqualValues(t, 1, channelMetricTestEventCount(channelMetricTestBuckets(batch, channelmetrics.ScopeFinalRequest, channelmetrics.OutcomeSuccess)))
}

func TestChannelMetricLifecycleKeepsTransportFailureWhenResponseAlsoExists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	collector, _ := installChannelMetricTestRuntime(t)
	c := newChannelMetricTestContext(t, "request-response-and-error")
	info := newChannelMetricTestRelayInfo()

	BeginChannelMetricRequest(c)
	BindChannelMetricRelayInfo(c, info)
	BeginChannelMetricAttempt(c, info, 66, "异常传输渠道", 6)
	callIndex := BeginChannelMetricUpstreamCall(c)
	CompleteChannelMetricUpstreamHeader(c, callIndex, http.StatusBadGateway, errors.New("redirect failed"))
	relayErr := types.NewError(errors.New("redirect failed"), types.ErrorCodeDoRequestFailed)
	FinishChannelMetricAttempt(c, info, relayErr, false, "transport_error")
	c.Writer.WriteHeader(http.StatusBadGateway)
	FinishChannelMetricRequest(c, info, relayErr)

	batch := drainChannelMetricTestBatch(t, collector)
	calls := channelMetricTestBuckets(batch, channelmetrics.ScopeUpstreamCall, channelmetrics.OutcomeTransportError)
	require.Len(t, calls, 1)
	require.Equal(t, channelmetrics.PresentStatus(http.StatusBadGateway), calls[0].Dimension.UpstreamStatus)
}

func TestChannelMetricLifecycleRecordsTransportFailureWithoutResponseHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	collector, _ := installChannelMetricTestRuntime(t)
	c := newChannelMetricTestContext(t, "request-transport-no-response")
	info := newChannelMetricTestRelayInfo()

	BeginChannelMetricRequest(c)
	BindChannelMetricRelayInfo(c, info)
	BeginChannelMetricAttempt(c, info, 67, "无响应渠道", 6)
	callIndex := BeginChannelMetricUpstreamCall(c)
	transportErr := errors.New("dial tcp: connection refused")
	CompleteChannelMetricUpstreamHeader(c, callIndex, 0, transportErr)

	state := getChannelMetricRequestState(c)
	require.NotNil(t, state)
	state.mu.Lock()
	require.NotNil(t, state.current)
	require.Len(t, state.current.calls, 1)
	call := state.current.calls[0]
	state.mu.Unlock()
	require.Equal(t, channelmetrics.PresentStatus(0), call.status)
	require.True(t, call.transportFail)
	require.True(t, call.headerAt.IsZero(), "无 HTTP 响应必须保持 ResponseHeaderPresent=false")

	relayErr := types.NewError(transportErr, types.ErrorCodeDoRequestFailed)
	FinishChannelMetricAttempt(c, info, relayErr, false, "transport_error")
	c.Writer.WriteHeader(http.StatusBadGateway)
	FinishChannelMetricRequest(c, info, relayErr)

	batch := drainChannelMetricTestBatch(t, collector)
	calls := channelMetricTestBuckets(batch, channelmetrics.ScopeUpstreamCall, channelmetrics.OutcomeTransportError)
	require.Len(t, calls, 1)
	require.Equal(t, channelmetrics.PresentStatus(0), calls[0].Dimension.UpstreamStatus)
}

func TestChannelMetricLifecycleClassifiesWebSocketHandshakeStatusesAsHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	collector, _ := installChannelMetricTestRuntime(t)
	for _, statusCode := range []int{http.StatusUnauthorized, http.StatusTooManyRequests, http.StatusInternalServerError} {
		c := newChannelMetricTestContext(t, fmt.Sprintf("websocket-handshake-%d", statusCode))
		info := newChannelMetricTestRelayInfo()
		BeginChannelMetricRequest(c)
		BindChannelMetricRelayInfo(c, info)
		BeginChannelMetricAttempt(c, info, statusCode, "WebSocket 渠道", 6)
		callIndex := BeginChannelMetricUpstreamCall(c)
		CompleteChannelMetricUpstreamHeader(c, callIndex, statusCode, nil)
		relayErr := types.NewOpenAIError(errors.New("websocket handshake rejected"), types.ErrorCodeBadResponseStatusCode, statusCode)
		FinishChannelMetricAttempt(c, info, relayErr, false, "http_handshake")
		c.Writer.WriteHeader(statusCode)
		FinishChannelMetricRequest(c, info, relayErr)
	}

	batch := drainChannelMetricTestBatch(t, collector)
	calls := channelMetricTestBuckets(batch, channelmetrics.ScopeUpstreamCall, channelmetrics.OutcomeHTTPError)
	require.Len(t, calls, 3)
	statuses := make(map[int]bool, len(calls))
	for _, call := range calls {
		require.True(t, call.Dimension.UpstreamStatus.Present)
		statuses[call.Dimension.UpstreamStatus.Code] = true
	}
	for _, statusCode := range []int{http.StatusUnauthorized, http.StatusTooManyRequests, http.StatusInternalServerError} {
		require.True(t, statuses[statusCode], "missing WebSocket handshake status %d", statusCode)
	}
	require.Empty(t, channelMetricTestBuckets(batch, channelmetrics.ScopeUpstreamCall, channelmetrics.OutcomeTransportError))
}

func TestChannelMetricLifecycleDoesNotGuessCausalCallForAmbiguousProtocolFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	collector, failureCh := installChannelMetricTestRuntime(t)
	c := newChannelMetricTestContext(t, "request-ambiguous-cause")
	info := newChannelMetricTestRelayInfo()

	BeginChannelMetricRequest(c)
	BindChannelMetricRelayInfo(c, info)
	BeginChannelMetricAttempt(c, info, 77, "多调用失败渠道", 7)
	for range 2 {
		callIndex := BeginChannelMetricUpstreamCall(c)
		CompleteChannelMetricUpstreamHeader(c, callIndex, http.StatusOK, nil)
	}
	relayErr := types.NewOpenAIError(errors.New("invalid upstream payload"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	FinishChannelMetricAttempt(c, info, relayErr, false, "protocol_error")
	c.Writer.WriteHeader(http.StatusInternalServerError)
	FinishChannelMetricRequest(c, info, relayErr)

	batch := drainChannelMetricTestBatch(t, collector)
	require.EqualValues(t, 1, channelMetricTestEventCount(channelMetricTestBuckets(batch, channelmetrics.ScopeUpstreamCall, channelmetrics.OutcomeSuccess)))
	require.EqualValues(t, 1, channelMetricTestEventCount(channelMetricTestBuckets(batch, channelmetrics.ScopeUpstreamCall, channelmetrics.OutcomeProtocolError)))

	events := receiveChannelMetricFailureEvents(t, failureCh)
	require.Len(t, events, 1)
	require.False(t, events[0].CausalCallPresent)
}

func TestChannelMetricLifecycleDoesNotPropagateLocalFailureToCompletedCall(t *testing.T) {
	gin.SetMode(gin.TestMode)
	collector, _ := installChannelMetricTestRuntime(t)
	c := newChannelMetricTestContext(t, "request-local-failure")
	info := newChannelMetricTestRelayInfo()

	BeginChannelMetricRequest(c)
	BindChannelMetricRelayInfo(c, info)
	BeginChannelMetricAttempt(c, info, 88, "本地转换失败渠道", 8)
	callIndex := BeginChannelMetricUpstreamCall(c)
	CompleteChannelMetricUpstreamHeader(c, callIndex, http.StatusOK, nil)
	relayErr := types.NewError(errors.New("local response conversion failed"), types.ErrorCodeConvertRequestFailed)
	FinishChannelMetricAttempt(c, info, relayErr, false, "local_error")
	c.Writer.WriteHeader(http.StatusInternalServerError)
	FinishChannelMetricRequest(c, info, relayErr)

	batch := drainChannelMetricTestBatch(t, collector)
	require.EqualValues(t, 1, channelMetricTestEventCount(channelMetricTestBuckets(batch, channelmetrics.ScopeChannelAttempt, channelmetrics.OutcomeLocalError)))
	require.EqualValues(t, 1, channelMetricTestEventCount(channelMetricTestBuckets(batch, channelmetrics.ScopeUpstreamCall, channelmetrics.OutcomeSuccess)))
}

func TestChannelMetricLifecycleRecordsEarlyTaskFailuresAsErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	collector, _ := installChannelMetricTestRuntime(t)
	cases := []struct {
		name       string
		info       *relaycommon.RelayInfo
		relayErr   *types.NewAPIError
		statusCode int
	}{
		{
			name:       "generate relay info",
			relayErr:   types.NewError(errors.New("relay info unavailable"), types.ErrorCodeGenRelayInfoFailed),
			statusCode: http.StatusInternalServerError,
		},
		{
			name: "resolve origin task",
			info: &relaycommon.RelayInfo{
				TaskRelayInfo: &relaycommon.TaskRelayInfo{},
			},
			relayErr:   types.NewOpenAIError(errors.New("task_origin_not_exist"), types.ErrorCodeBadResponseStatusCode, http.StatusBadRequest),
			statusCode: http.StatusBadRequest,
		},
	}

	for _, testCase := range cases {
		c := newChannelMetricTestContext(t, "early-task-"+strings.ReplaceAll(testCase.name, " ", "-"))
		BeginChannelMetricRequest(c)
		MarkChannelMetricTaskRequest(c)
		BindChannelMetricRelayInfo(c, testCase.info)
		c.Writer.WriteHeader(testCase.statusCode)
		FinishChannelMetricRequest(c, testCase.info, testCase.relayErr)
	}

	batch := drainChannelMetricTestBatch(t, collector)
	errors := channelMetricTestBuckets(batch, channelmetrics.ScopeFinalRequest, channelmetrics.OutcomeLocalError)
	require.EqualValues(t, len(cases), channelMetricTestEventCount(errors))
	for _, bucket := range errors {
		require.Equal(t, channelmetrics.TrafficSourceTask, bucket.Dimension.TrafficSource)
	}
	require.Empty(t, channelMetricTestBuckets(batch, channelmetrics.ScopeFinalRequest, channelmetrics.OutcomeSuccess))
}

func TestChannelMetricLifecycleMarksTrafficSourceBeforeRelayInfoExists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	collector, _ := installChannelMetricTestRuntime(t)
	cases := []struct {
		name   string
		source channelmetrics.TrafficSource
		mark   func(*gin.Context)
	}{
		{name: "probe", source: channelmetrics.TrafficSourceProbe, mark: MarkChannelMetricProbeRequest},
		{name: "task", source: channelmetrics.TrafficSourceTask, mark: MarkChannelMetricTaskRequest},
		{name: "playground", source: channelmetrics.TrafficSourcePlayground, mark: MarkChannelMetricPlaygroundRequest},
	}

	for _, testCase := range cases {
		c := newChannelMetricTestContext(t, "request-source-"+testCase.name)
		BeginChannelMetricRequest(c)
		testCase.mark(c)
		relayErr := types.NewError(errors.New("relay info unavailable"), types.ErrorCodeGenRelayInfoFailed)
		c.Writer.WriteHeader(http.StatusInternalServerError)
		FinishChannelMetricRequest(c, nil, relayErr)
	}

	batch := drainChannelMetricTestBatch(t, collector)
	recorded := make(map[channelmetrics.TrafficSource]bool, len(cases))
	for _, bucket := range batch.Buckets {
		require.Equal(t, channelmetrics.ScopeFinalRequest, bucket.Dimension.Scope)
		recorded[bucket.Dimension.TrafficSource] = true
	}
	for _, testCase := range cases {
		require.True(t, recorded[testCase.source], "缺少 %s 来源的最终请求样本", testCase.source)
	}
}

func TestNormalizeChannelMetricUsageClampsAndRecomputesUncachedTokens(t *testing.T) {
	usage := normalizeChannelMetricUsage(ChannelMetricUsage{
		InputTokensTotal:    10,
		UncachedInputTokens: 999,
		OutputTokens:        -1,
		CacheReadTokens:     8,
		CacheWriteTokens:    7,
		ChargedQuota:        -2,
	})

	require.EqualValues(t, 15, usage.InputTokensTotal)
	require.Zero(t, usage.UncachedInputTokens)
	require.Zero(t, usage.OutputTokens)
	require.EqualValues(t, 8, usage.CacheReadTokens)
	require.EqualValues(t, 7, usage.CacheWriteTokens)
	require.Zero(t, usage.ChargedQuota)
}

func TestBuildChannelFailureEventsTruncatesDatabaseSnapshotsAndKeepsModelHashes(t *testing.T) {
	longValue := strings.Repeat("长字段", 300)
	drafts := []channelMetricFailureDraft{{
		attemptSeq: 1, createdAt: time.Now(), channelID: 7, channelName: longValue,
		requestedModel: longValue + "-requested", upstreamModel: longValue + "-upstream", group: longValue,
		trafficSource: channelmetrics.TrafficSourceRelay, outcome: channelmetrics.OutcomeHTTPError,
		failureOwner: channelmetrics.FailureOwnerChannel, errorStage: channelmetrics.ErrorStageUpstream,
		streamEndReason: longValue, retryReason: longValue, maskedError: longValue,
	}}

	events := buildChannelFailureEvents(drafts, longValue, 1, http.StatusTooManyRequests)
	require.Len(t, events, 1)
	event := events[0]
	require.LessOrEqual(t, len(event.RequestId), 128)
	require.LessOrEqual(t, len(event.ChannelNameSnapshot), 191)
	require.LessOrEqual(t, len(event.RequestedModel), 191)
	require.LessOrEqual(t, len(event.UpstreamModel), 191)
	require.LessOrEqual(t, len(event.Group), 64)
	require.LessOrEqual(t, len(event.ErrorStage), 32)
	require.LessOrEqual(t, len(event.StreamEndReason), 64)
	require.LessOrEqual(t, len(event.RetryReason), 128)
	require.LessOrEqual(t, len(event.MaskedErrorSummary), 512)
	require.Equal(t, channelmetrics.SHA256String(drafts[0].requestedModel), event.RequestedModelHash)
	require.Equal(t, channelmetrics.SHA256String(drafts[0].upstreamModel), event.UpstreamModelHash)
}

func TestChannelMetricLifecycleMasksFailureSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, failureCh := installChannelMetricTestRuntime(t)
	c := newChannelMetricTestContext(t, "request-secret")
	info := newChannelMetricTestRelayInfo()

	BeginChannelMetricRequest(c)
	BindChannelMetricRelayInfo(c, info)
	BeginChannelMetricAttempt(c, info, 17, "secret-channel", 1)
	callIndex := BeginChannelMetricUpstreamCall(c)
	CompleteChannelMetricUpstreamHeader(c, callIndex, http.StatusUnauthorized, nil)
	relayErr := types.NewOpenAIError(
		errors.New("upstream rejected sk-abcdefghijklmnopqrstuvwxyz123456"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusUnauthorized,
	)
	FinishChannelMetricAttempt(c, info, relayErr, false, "")
	c.Writer.WriteHeader(http.StatusBadGateway)
	FinishChannelMetricRequest(c, info, relayErr)

	events := receiveChannelMetricFailureEvents(t, failureCh)
	require.Len(t, events, 1)
	require.NotContains(t, events[0].MaskedErrorSummary, "sk-abcdefghijklmnopqrstuvwxyz123456")
	require.LessOrEqual(t, len(events[0].MaskedErrorSummary), 512)
}
