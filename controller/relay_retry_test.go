package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	openairelay "github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func buildRelayRetryTestContext() *gin.Context {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	return ctx
}

func TestWriteRelayErrorResponseSkipsCommittedStreamAndCancellation(t *testing.T) {
	t.Run("committed stream is not followed by json", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		_, err := ctx.Writer.Write([]byte("data: {\"type\":\"response.output_text.delta\"}\n\n"))
		require.NoError(t, err)
		bodyBefore := recorder.Body.String()
		relayErr := types.NewErrorWithStatusCode(errors.New("upstream stream reset"), types.ErrorCodeBadResponse, http.StatusInternalServerError)

		writeRelayErrorResponse(ctx, nil, types.RelayFormatOpenAI, relayErr, "request-1")

		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, bodyBefore, recorder.Body.String())
		require.Equal(t, "upstream stream reset", relayErr.Error())
	})

	t.Run("uncommitted stream headers are replaced by json error", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		common.SetContextKey(ctx, constant.ContextKeyIsStream, true)
		ctx.Writer.Header().Set("Content-Type", "text/event-stream")
		ctx.Writer.Header().Set("Cache-Control", "no-cache")
		ctx.Writer.Header().Set("Transfer-Encoding", "chunked")
		relayErr := types.WithOpenAIError(types.OpenAIError{
			Code:    "server_error",
			Message: "Selected model is at capacity. Please try a different model.",
		}, http.StatusOK)

		writeRelayErrorResponse(ctx, nil, types.RelayFormatOpenAI, relayErr, "request-capacity")

		require.Equal(t, http.StatusTooManyRequests, recorder.Code)
		require.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))
		require.Empty(t, recorder.Header().Get("Cache-Control"))
		require.Empty(t, recorder.Header().Get("Transfer-Encoding"))
		require.Contains(t, recorder.Body.String(), types.UpstreamCapacityClientMessage)
		require.Contains(t, recorder.Body.String(), "request-capacity")
	})

	t.Run("matching cancellation does not create a 500 response", func(t *testing.T) {
		requestContext, cancel := context.WithCancel(context.Background())
		cancel()
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(requestContext)
		relayErr := types.NewErrorWithStatusCode(context.Canceled, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)

		writeRelayErrorResponse(ctx, nil, types.RelayFormatOpenAI, relayErr, "request-2")

		require.False(t, ctx.Writer.Written())
		require.Empty(t, recorder.Body.String())
		require.Equal(t, context.Canceled.Error(), relayErr.Error())
	})

	t.Run("real transport error remains visible when cancellation races", func(t *testing.T) {
		requestContext, cancel := context.WithCancel(context.Background())
		cancel()
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(requestContext)
		relayErr := types.NewErrorWithStatusCode(errors.New("connection reset by peer"), types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)

		writeRelayErrorResponse(ctx, nil, types.RelayFormatOpenAI, relayErr, "request-3")

		require.Equal(t, http.StatusInternalServerError, recorder.Code)
		require.Contains(t, recorder.Body.String(), "connection reset by peer")
	})
}

func TestShouldRetryWithReasonRetriesConfiguredBadRequest(t *testing.T) {
	originalRanges := operation_setting.AutomaticRetryStatusCodeRanges
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{
		{Start: 400, End: 400},
	}
	t.Cleanup(func() {
		operation_setting.AutomaticRetryStatusCodeRanges = originalRanges
	})

	err := types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, http.StatusBadRequest)
	decision := shouldRetryWithReason(buildRelayRetryTestContext(), err, 2)

	require.True(t, decision.Retry)
	require.Equal(t, "status_code_retry", decision.Reason)
}

func TestShouldRetryWithReasonUsesStatusBeforeChannelMapping(t *testing.T) {
	originalRanges := operation_setting.AutomaticRetryStatusCodeRanges
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{
		{Start: http.StatusForbidden, End: http.StatusForbidden},
	}
	t.Cleanup(func() {
		operation_setting.AutomaticRetryStatusCodeRanges = originalRanges
	})

	ctx := buildRelayRetryTestContext()
	ctx.Set("channel_affinity_skip_retry_on_failure", true)
	relayErr := types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, http.StatusOK)
	relayErr.OriginalStatusCode = http.StatusForbidden

	decision := shouldRetryWithReason(ctx, relayErr, 2)

	require.True(t, decision.Retry)
	require.Equal(t, "status_code_retry", decision.Reason)
}

func TestShouldRetryWithReasonReportsBlockingReason(t *testing.T) {
	err := types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, http.StatusBadRequest)

	t.Run("no remaining retries", func(t *testing.T) {
		decision := shouldRetryWithReason(buildRelayRetryTestContext(), err, 0)
		require.False(t, decision.Retry)
		require.Equal(t, "retry_exhausted", decision.Reason)
	})

	t.Run("channel error with no remaining retries", func(t *testing.T) {
		channelErr := types.NewError(errors.New("channel unavailable"), types.ErrorCode("channel:unavailable"))
		decision := shouldRetryWithReason(buildRelayRetryTestContext(), channelErr, 0)
		require.False(t, decision.Retry)
		require.Equal(t, "retry_exhausted", decision.Reason)
	})

	t.Run("specific channel", func(t *testing.T) {
		ctx := buildRelayRetryTestContext()
		ctx.Set("specific_channel_id", "13")
		decision := shouldRetryWithReason(ctx, err, 2)
		require.False(t, decision.Retry)
		require.Equal(t, "specific_channel", decision.Reason)
	})

	t.Run("specific channel with channel error", func(t *testing.T) {
		ctx := buildRelayRetryTestContext()
		ctx.Set("specific_channel_id", "13")
		channelErr := types.NewError(errors.New("channel unavailable"), types.ErrorCode("channel:unavailable"))
		decision := shouldRetryWithReason(ctx, channelErr, 2)
		require.False(t, decision.Retry)
		require.Equal(t, "specific_channel", decision.Reason)
	})

	t.Run("channel affinity skip", func(t *testing.T) {
		ctx := buildRelayRetryTestContext()
		ctx.Set("channel_affinity_skip_retry_on_failure", true)
		decision := shouldRetryWithReason(ctx, err, 2)
		require.False(t, decision.Retry)
		require.Equal(t, "channel_affinity_skip", decision.Reason)
	})

	t.Run("configured 403 overrides channel affinity skip", func(t *testing.T) {
		ctx := buildRelayRetryTestContext()
		ctx.Set("channel_affinity_skip_retry_on_failure", true)
		forbiddenErr := types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, http.StatusForbidden)
		decision := shouldRetryWithReason(ctx, forbiddenErr, 2)
		require.True(t, decision.Retry)
		require.Equal(t, "status_code_retry", decision.Reason)
	})

	t.Run("stream already written", func(t *testing.T) {
		ctx := buildRelayRetryTestContext()
		_, writeErr := ctx.Writer.Write([]byte("data: {\"delta\":\"hello\"}\n\n"))
		require.NoError(t, writeErr)
		decision := shouldRetryWithReason(ctx, err, 2)
		require.False(t, decision.Retry)
		require.Equal(t, "no_retry_after_stream_started", decision.Reason)
	})
}

func TestShouldRetryWithReasonUsesActualDownstreamCommitBoundary(t *testing.T) {
	originalRanges := operation_setting.AutomaticRetryStatusCodeRanges
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{
		{Start: http.StatusInternalServerError, End: http.StatusInternalServerError},
	}
	t.Cleanup(func() {
		operation_setting.AutomaticRetryStatusCodeRanges = originalRanges
	})
	relayErr := types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, http.StatusInternalServerError)

	t.Run("first SSE error frame has not started downstream", func(t *testing.T) {
		ctx := buildRelayRetryTestContext()
		info := &relaycommon.RelayInfo{ReceivedResponseCount: 1}
		info.ResetFirstResponseTiming(time.Now())
		info.SetFirstResponseTime()
		common.SetContextKey(ctx, constant.ContextKeyRelayInfo, info)
		helper.SetEventStreamHeaders(ctx)

		decision := shouldRetryWithReason(ctx, relayErr, 2)

		require.False(t, ctx.Writer.Written())
		require.True(t, decision.Retry)
		require.Equal(t, "status_code_retry", decision.Reason)
	})

	t.Run("received non-stream response headers do not block retry", func(t *testing.T) {
		ctx := buildRelayRetryTestContext()
		info := &relaycommon.RelayInfo{}
		info.ResetFirstResponseTiming(time.Now())
		info.SetFirstResponseTime()
		common.SetContextKey(ctx, constant.ContextKeyRelayInfo, info)

		decision := shouldRetryWithReason(ctx, relayErr, 2)

		require.True(t, decision.Retry)
		require.Equal(t, "status_code_retry", decision.Reason)
	})

	t.Run("buffered send count does not override available writer", func(t *testing.T) {
		ctx := buildRelayRetryTestContext()
		common.SetContextKey(ctx, constant.ContextKeyRelayInfo, &relaycommon.RelayInfo{
			ReceivedResponseCount: 2,
			SendResponseCount:     1,
		})

		decision := shouldRetryWithReason(ctx, relayErr, 2)

		require.False(t, ctx.Writer.Written())
		require.True(t, decision.Retry)
		require.Equal(t, "status_code_retry", decision.Reason)
	})

	t.Run("missing writer falls back to send count", func(t *testing.T) {
		ctx := buildRelayRetryTestContext()
		ctx.Writer = nil
		common.SetContextKey(ctx, constant.ContextKeyRelayInfo, &relaycommon.RelayInfo{SendResponseCount: 1})

		decision := shouldRetryWithReason(ctx, relayErr, 2)

		require.False(t, decision.Retry)
		require.Equal(t, "no_retry_after_stream_started", decision.Reason)
	})
}

func TestOpenAIStreamFirstErrorFrameReachesStatusCodeRetry(t *testing.T) {
	originalRanges := operation_setting.AutomaticRetryStatusCodeRanges
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{
		{Start: http.StatusInternalServerError, End: http.StatusInternalServerError},
	}
	t.Cleanup(func() {
		operation_setting.AutomaticRetryStatusCodeRanges = originalRanges
	})

	ctx := buildRelayRetryTestContext()
	info := &relaycommon.RelayInfo{
		IsStream:        true,
		RelayFormat:     types.RelayFormatOpenAI,
		OriginModelName: "gpt-5.6-sol",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5.6-sol",
		},
	}
	info.ResetFirstResponseTiming(time.Now())
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(
			"data: {\"error\":{\"message\":\"upstream failed\",\"type\":\"server_error\",\"code\":\"server_error\"}}\n\n",
		)),
	}

	usage, relayErr := openairelay.OaiStreamHandler(ctx, info, resp)
	require.Nil(t, usage)
	require.NotNil(t, relayErr)
	require.Equal(t, http.StatusInternalServerError, relayErr.StatusCode)
	require.Equal(t, 1, info.ReceivedResponseCount)
	require.False(t, ctx.Writer.Written())
	common.SetContextKey(ctx, constant.ContextKeyRelayInfo, info)

	decision := shouldRetryWithReason(ctx, relayErr, 2)

	require.True(t, decision.Retry)
	require.Equal(t, "status_code_retry", decision.Reason)
}

func TestShouldRetryWithReasonCapacityErrorBypassesAffinityBeforeOutput(t *testing.T) {
	capacityErr := types.WithOpenAIError(types.OpenAIError{
		Type:    "server_error",
		Code:    "server_error",
		Message: "Selected model is at capacity. Please try a different model.",
	}, http.StatusOK)

	t.Run("retry despite received SSE and affinity skip", func(t *testing.T) {
		ctx := buildRelayRetryTestContext()
		ctx.Set("channel_affinity_skip_retry_on_failure", true)
		info := &relaycommon.RelayInfo{ReceivedResponseCount: 1}
		common.SetContextKey(ctx, constant.ContextKeyRelayInfo, info)

		decision := shouldRetryWithReason(ctx, capacityErr, 2)

		require.True(t, decision.Retry)
		require.Equal(t, "upstream_capacity", decision.Reason)
	})

	t.Run("retry when attempted stream output is still buffered", func(t *testing.T) {
		ctx := buildRelayRetryTestContext()
		info := &relaycommon.RelayInfo{
			ReceivedResponseCount: 2,
			SendResponseCount:     1,
		}
		common.SetContextKey(ctx, constant.ContextKeyRelayInfo, info)

		decision := shouldRetryWithReason(ctx, capacityErr, 2)

		require.False(t, ctx.Writer.Written())
		require.True(t, decision.Retry)
		require.Equal(t, "upstream_capacity", decision.Reason)
	})

	t.Run("do not retry after downstream output", func(t *testing.T) {
		ctx := buildRelayRetryTestContext()
		info := &relaycommon.RelayInfo{ReceivedResponseCount: 2}
		common.SetContextKey(ctx, constant.ContextKeyRelayInfo, info)
		_, writeErr := ctx.Writer.Write([]byte("event: response.created\n\n"))
		require.NoError(t, writeErr)

		decision := shouldRetryWithReason(ctx, capacityErr, 2)

		require.False(t, decision.Retry)
		require.Equal(t, "no_retry_after_stream_started", decision.Reason)
	})

	t.Run("respect retry budget", func(t *testing.T) {
		decision := shouldRetryWithReason(buildRelayRetryTestContext(), capacityErr, 0)
		require.False(t, decision.Retry)
		require.Equal(t, "retry_exhausted", decision.Reason)
	})

	t.Run("respect specific channel", func(t *testing.T) {
		ctx := buildRelayRetryTestContext()
		ctx.Set("specific_channel_id", "13")
		decision := shouldRetryWithReason(ctx, capacityErr, 2)
		require.False(t, decision.Retry)
		require.Equal(t, "specific_channel", decision.Reason)
	})
}

func TestShouldRetryWithReasonStopsWhenRequestContextEnds(t *testing.T) {
	relayErr := types.NewError(errors.New("transport failed"), types.ErrorCodeDoRequestFailed)

	t.Run("client canceled", func(t *testing.T) {
		requestContext, cancel := context.WithCancel(context.Background())
		cancel()
		ctx := buildRelayRetryTestContext()
		ctx.Request = ctx.Request.WithContext(requestContext)

		decision := shouldRetryWithReason(ctx, relayErr, 2)

		require.False(t, decision.Retry)
		require.Equal(t, "request_context_canceled", decision.Reason)
	})

	t.Run("request deadline exceeded", func(t *testing.T) {
		requestContext, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		ctx := buildRelayRetryTestContext()
		ctx.Request = ctx.Request.WithContext(requestContext)

		decision := shouldRetryWithReason(ctx, relayErr, 2)

		require.False(t, decision.Retry)
		require.Equal(t, "request_context_deadline_exceeded", decision.Reason)
	})

	t.Run("received frame without downstream write still reports cancellation", func(t *testing.T) {
		requestContext, cancel := context.WithCancel(context.Background())
		cancel()
		ctx := buildRelayRetryTestContext()
		ctx.Request = ctx.Request.WithContext(requestContext)
		info := &relaycommon.RelayInfo{ReceivedResponseCount: 1}
		common.SetContextKey(ctx, constant.ContextKeyRelayInfo, info)

		decision := shouldRetryWithReason(ctx, relayErr, 2)

		require.False(t, decision.Retry)
		require.Equal(t, "request_context_canceled", decision.Reason)
	})

	t.Run("committed stream remains the primary reason", func(t *testing.T) {
		requestContext, cancel := context.WithCancel(context.Background())
		cancel()
		ctx := buildRelayRetryTestContext()
		ctx.Request = ctx.Request.WithContext(requestContext)
		_, writeErr := ctx.Writer.Write([]byte("data: started\n\n"))
		require.NoError(t, writeErr)

		decision := shouldRetryWithReason(ctx, relayErr, 2)

		require.False(t, decision.Retry)
		require.Equal(t, "no_retry_after_stream_started", decision.Reason)
	})
}

func TestResetUncommittedStreamAttemptForRetry(t *testing.T) {
	ctx := buildRelayRetryTestContext()
	info := &relaycommon.RelayInfo{IsStream: true}
	helper.SetEventStreamHeaders(ctx)
	require.Equal(t, "text/event-stream", ctx.Writer.Header().Get("Content-Type"))
	require.True(t, ctx.GetBool("event_stream_headers_set"))

	resetUncommittedStreamAttemptForRetry(ctx, info)

	require.Empty(t, ctx.Writer.Header().Get("Content-Type"))
	require.False(t, ctx.GetBool("event_stream_headers_set"))
}

func TestShouldEvictChannelAffinityAfterRetryableFailureBoundaries(t *testing.T) {
	originalRanges := operation_setting.AutomaticRetryStatusCodeRanges
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{
		{Start: http.StatusInternalServerError, End: http.StatusInternalServerError},
	}
	t.Cleanup(func() {
		operation_setting.AutomaticRetryStatusCodeRanges = originalRanges
	})
	retryableErr := types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, http.StatusInternalServerError)

	t.Run("committed retryable failure is evicted for the next client reconnect", func(t *testing.T) {
		ctx := buildRelayRetryTestContext()
		_, writeErr := ctx.Writer.Write([]byte("data: started\n\n"))
		require.NoError(t, writeErr)
		require.False(t, shouldRetryWithReason(ctx, retryableErr, 2).Retry)
		require.True(t, shouldEvictChannelAffinityAfterFailure(ctx, retryableErr, 2))
	})

	t.Run("request cancellation is not evicted", func(t *testing.T) {
		requestContext, cancel := context.WithCancel(context.Background())
		cancel()
		ctx := buildRelayRetryTestContext()
		ctx.Request = ctx.Request.WithContext(requestContext)
		require.False(t, shouldEvictChannelAffinityAfterFailure(ctx, retryableErr, 2))
	})

	t.Run("specific channel is not evicted", func(t *testing.T) {
		ctx := buildRelayRetryTestContext()
		ctx.Set("specific_channel_id", "462")
		require.False(t, shouldEvictChannelAffinityAfterFailure(ctx, retryableErr, 2))
	})

	t.Run("affinity skip rule does not retain configured retry failure", func(t *testing.T) {
		ctx := buildRelayRetryTestContext()
		ctx.Set("channel_affinity_skip_retry_on_failure", true)
		require.True(t, shouldEvictChannelAffinityAfterFailure(ctx, retryableErr, 2))
	})

	t.Run("skip retry error is not evicted", func(t *testing.T) {
		skipErr := types.NewErrorWithStatusCode(
			errors.New("local validation failed"),
			types.ErrorCodeBadResponse,
			http.StatusInternalServerError,
			types.ErrOptionWithSkipRetry(),
		)
		require.False(t, shouldEvictChannelAffinityAfterFailure(buildRelayRetryTestContext(), skipErr, 2))
	})

	t.Run("unconfigured status is not evicted", func(t *testing.T) {
		notRetryableErr := types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, http.StatusUnprocessableEntity)
		require.False(t, shouldEvictChannelAffinityAfterFailure(buildRelayRetryTestContext(), notRetryableErr, 2))
	})

	t.Run("exhausted retry budget is not evicted", func(t *testing.T) {
		require.False(t, shouldEvictChannelAffinityAfterFailure(buildRelayRetryTestContext(), retryableErr, 0))
	})
}
func TestRemainingRelayRetriesUsesGlobalAttemptIndex(t *testing.T) {
	require.Equal(t, []int{3, 2, 1, 0}, []int{
		remainingRelayRetries(3, 0),
		remainingRelayRetries(3, 1),
		remainingRelayRetries(3, 2),
		remainingRelayRetries(3, 3),
	})
	require.Zero(t, remainingRelayRetries(3, 4))
}

func TestExcludeChannelFromRetryPreservesControlledReuse(t *testing.T) {
	t.Run("exclude ordinary transport failure", func(t *testing.T) {
		param := &service.RetryParam{}
		channel := &model.Channel{Id: 326}
		excludeChannelFromRetry(buildRelayRetryTestContext(), param, channel, types.NewError(errors.New("transport failed"), types.ErrorCodeDoRequestFailed))

		_, excluded := param.ExcludedChannelIDs[channel.Id]
		require.True(t, excluded)
		_, fallback := param.RetryFallbackChannelIDs[channel.Id]
		require.True(t, fallback)
	})

	t.Run("exclude single key rate limited channel", func(t *testing.T) {
		param := &service.RetryParam{}
		channel := &model.Channel{Id: 326}
		excludeChannelFromRetry(buildRelayRetryTestContext(), param, channel, types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests))

		_, excluded := param.ExcludedChannelIDs[channel.Id]
		require.True(t, excluded)
		_, fallback := param.RetryFallbackChannelIDs[channel.Id]
		require.False(t, fallback)
	})

	t.Run("keep multi key rate limited channel reusable", func(t *testing.T) {
		param := &service.RetryParam{}
		channel := &model.Channel{Id: 326, ChannelInfo: model.ChannelInfo{IsMultiKey: true}}
		excludeChannelFromRetry(buildRelayRetryTestContext(), param, channel, types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests))

		require.Empty(t, param.ExcludedChannelIDs)
	})

	t.Run("keep multi key channel reusable", func(t *testing.T) {
		param := &service.RetryParam{}
		channel := &model.Channel{Id: 326, ChannelInfo: model.ChannelInfo{IsMultiKey: true}}
		excludeChannelFromRetry(buildRelayRetryTestContext(), param, channel, types.NewError(errors.New("key failed"), types.ErrorCodeDoRequestFailed))

		require.Empty(t, param.ExcludedChannelIDs)
	})

	t.Run("exclude multi key channel on forbidden response", func(t *testing.T) {
		param := &service.RetryParam{}
		channel := &model.Channel{Id: 326, ChannelInfo: model.ChannelInfo{IsMultiKey: true}}
		excludeChannelFromRetry(buildRelayRetryTestContext(), param, channel, types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, http.StatusForbidden))

		_, excluded := param.ExcludedChannelIDs[channel.Id]
		require.True(t, excluded)
		_, fallback := param.RetryFallbackChannelIDs[channel.Id]
		require.False(t, fallback)
	})

	t.Run("exclude multi key channel on forbidden response before status mapping", func(t *testing.T) {
		param := &service.RetryParam{}
		channel := &model.Channel{Id: 327, ChannelInfo: model.ChannelInfo{IsMultiKey: true}}
		relayErr := types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, http.StatusOK)
		relayErr.OriginalStatusCode = http.StatusForbidden
		excludeChannelFromRetry(buildRelayRetryTestContext(), param, channel, relayErr)

		_, excluded := param.ExcludedChannelIDs[channel.Id]
		require.True(t, excluded)
		_, fallback := param.RetryFallbackChannelIDs[channel.Id]
		require.False(t, fallback)
	})

	t.Run("exclude multi key channel on capacity error", func(t *testing.T) {
		param := &service.RetryParam{}
		channel := &model.Channel{Id: 326, ChannelInfo: model.ChannelInfo{IsMultiKey: true}}
		capacityErr := types.WithOpenAIError(types.OpenAIError{
			Code:    "server_error",
			Message: "Selected model is at capacity. Please try a different model.",
		}, http.StatusOK)
		excludeChannelFromRetry(buildRelayRetryTestContext(), param, channel, capacityErr)

		_, excluded := param.ExcludedChannelIDs[channel.Id]
		require.True(t, excluded)
		_, fallback := param.RetryFallbackChannelIDs[channel.Id]
		require.False(t, fallback)
	})

	t.Run("prefer cross group failover over rate limit reuse", func(t *testing.T) {
		ctx := buildRelayRetryTestContext()
		common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)
		param := &service.RetryParam{TokenGroup: "auto"}
		channel := &model.Channel{Id: 326}
		excludeChannelFromRetry(ctx, param, channel, types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests))

		_, excluded := param.ExcludedChannelIDs[channel.Id]
		require.True(t, excluded)
		_, fallback := param.RetryFallbackChannelIDs[channel.Id]
		require.False(t, fallback)
	})

	t.Run("prefer explicit multi group failover over key reuse", func(t *testing.T) {
		param := &service.RetryParam{TokenGroup: "group-a,group-b"}
		channel := &model.Channel{Id: 326, ChannelInfo: model.ChannelInfo{IsMultiKey: true}}
		excludeChannelFromRetry(buildRelayRetryTestContext(), param, channel, types.NewError(errors.New("key failed"), types.ErrorCodeDoRequestFailed))

		_, excluded := param.ExcludedChannelIDs[channel.Id]
		require.True(t, excluded)
		_, fallback := param.RetryFallbackChannelIDs[channel.Id]
		require.False(t, fallback)
	})
}

func TestRepeatedChannelRetryDelay(t *testing.T) {
	t.Run("skip different channel", func(t *testing.T) {
		err := &types.NewAPIError{StatusCode: http.StatusTooManyRequests}
		require.Zero(t, repeatedChannelRetryDelay(err, 1, false))
	})

	t.Run("skip non rate limit error", func(t *testing.T) {
		err := &types.NewAPIError{StatusCode: http.StatusBadGateway}
		require.Zero(t, repeatedChannelRetryDelay(err, 1, true))
	})

	t.Run("use exponential fallback", func(t *testing.T) {
		err := &types.NewAPIError{StatusCode: http.StatusTooManyRequests}
		require.Equal(t, 500*time.Millisecond, repeatedChannelRetryDelay(err, 1, true))
		require.Equal(t, time.Second, repeatedChannelRetryDelay(err, 2, true))
		require.Equal(t, 2*time.Second, repeatedChannelRetryDelay(err, 3, true))
	})

	t.Run("prefer bounded retry after", func(t *testing.T) {
		err := &types.NewAPIError{StatusCode: http.StatusTooManyRequests, RetryAfter: 30 * time.Second}
		require.Equal(t, 10*time.Second, repeatedChannelRetryDelay(err, 1, true))
	})

	t.Run("recognize rate limit before status mapping", func(t *testing.T) {
		err := &types.NewAPIError{
			StatusCode:         http.StatusServiceUnavailable,
			OriginalStatusCode: http.StatusTooManyRequests,
		}
		require.Equal(t, repeatedChannelRetryBaseDelay, repeatedChannelRetryDelay(err, 1, true))
	})
}

func TestChannelRetryStateIsIsolatedByChannel(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	states := make(map[int]channelRetryState)

	recordChannelRetryState(states, 342, &types.NewAPIError{
		StatusCode: http.StatusTooManyRequests,
		RetryAfter: 10 * time.Second,
	}, now)
	recordChannelRetryState(states, 351, &types.NewAPIError{
		StatusCode: http.StatusTooManyRequests,
		RetryAfter: time.Second,
	}, now)

	require.Equal(t, 8*time.Second, channelRetryDelay(states, 342, now.Add(2*time.Second)))
	require.Zero(t, channelRetryDelay(states, 351, now.Add(2*time.Second)))
}

func TestChannelRetryStateSurvivesOtherChannelFailure(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	states := make(map[int]channelRetryState)

	recordChannelRetryState(states, 342, &types.NewAPIError{
		StatusCode: http.StatusTooManyRequests,
		RetryAfter: 10 * time.Second,
	}, now)
	recordChannelRetryState(states, 351, &types.NewAPIError{StatusCode: http.StatusBadGateway}, now)

	require.Equal(t, 9*time.Second, channelRetryDelay(states, 342, now.Add(time.Second)))
	require.Zero(t, channelRetryDelay(states, 351, now.Add(time.Second)))
}

func TestChannelRetryStateUsesPerChannelBackoff(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	states := make(map[int]channelRetryState)
	rateLimitErr := &types.NewAPIError{StatusCode: http.StatusTooManyRequests}

	recordChannelRetryState(states, 342, rateLimitErr, now)
	require.Equal(t, 500*time.Millisecond, channelRetryDelay(states, 342, now))

	recordChannelRetryState(states, 342, rateLimitErr, now.Add(time.Second))
	require.Equal(t, time.Second, channelRetryDelay(states, 342, now.Add(time.Second)))
}

func TestWaitForRelayRetryStopsOnCanceledRequest(t *testing.T) {
	requestContext, cancel := context.WithCancel(context.Background())
	cancel()
	ctx := buildRelayRetryTestContext()
	ctx.Request = ctx.Request.WithContext(requestContext)

	startedAt := time.Now()
	require.False(t, waitForRelayRetry(ctx, time.Second))
	require.Less(t, time.Since(startedAt), 500*time.Millisecond)
}

func TestAppendErrorLogRequestConversion(t *testing.T) {
	other := map[string]interface{}{}
	appendErrorLogRequestConversion(&relaycommon.RelayInfo{
		RequestConversionChain: []types.RelayFormat{
			types.RelayFormatClaude,
			types.RelayFormatOpenAI,
		},
	}, other)

	require.Equal(t, []string{"Claude Messages", "OpenAI Compatible"}, other["request_conversion"])
}

func TestProcessChannelErrorRecordsOnlyFinalAttempt(t *testing.T) {
	db := setupRelayErrorLogTestDB(t)
	ctx := buildRelayRetryTestContext()
	ctx.Set("id", 1001)
	ctx.Set("username", "relay-log-user")
	ctx.Set("token_name", "relay-log-token")
	ctx.Set("token_id", 2001)
	ctx.Set("original_model", "gpt-test")
	ctx.Set("group", "default")
	ctx.Set("use_channel", []string{"326"})
	common.SetContextKey(ctx, constant.ContextKeyRequestStartTime, time.Now().Add(-time.Second))

	relayErr := types.NewError(errors.New("transport failed"), types.ErrorCodeDoRequestFailed)
	firstChannel := *types.NewChannelError(326, 1, "first", false, "", false)
	processChannelError(ctx, nil, firstChannel, relayErr, false)

	var count int64
	require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeError).Count(&count).Error)
	require.Zero(t, count)

	ctx.Set("use_channel", []string{"326", "322"})
	finalChannel := *types.NewChannelError(322, 1, "final", false, "", false)
	processChannelError(ctx, nil, finalChannel, relayErr, false)
	restoredErr := finalizePendingChannelFailure(ctx, nil, &channelFailureSnapshot{channel: finalChannel, err: relayErr})

	require.Same(t, relayErr, restoredErr)
	require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeError).Count(&count).Error)
	require.EqualValues(t, 1, count)
	var recorded model.Log
	require.NoError(t, db.Where("type = ?", model.LogTypeError).First(&recorded).Error)
	require.Equal(t, 322, recorded.ChannelId)

	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(recorded.Other, &other))
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	require.EqualValues(t, 2, adminInfo["attempt_count"])

	requestContext, cancel := context.WithCancel(context.Background())
	cancel()
	ctx.Request = ctx.Request.WithContext(requestContext)
	canceledRelayErr := types.NewError(
		context.Canceled,
		types.ErrorCodeDoRequestFailed,
		types.ErrOptionWithHideErrMsg("upstream error: do request failed"),
	)
	require.ErrorIs(t, canceledRelayErr, context.Canceled)
	require.Equal(t, "upstream error: do request failed", canceledRelayErr.Error())
	processChannelError(ctx, nil, finalChannel, canceledRelayErr, true)
	require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeError).Count(&count).Error)
	require.EqualValues(t, 1, count)

	processChannelError(ctx, nil, finalChannel, relayErr, true)
	require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeError).Count(&count).Error)
	require.EqualValues(t, 2, count)
}

func TestRecordChannelErrorLogShowsReplacementAndKeepsUpstreamDetail(t *testing.T) {
	db := setupRelayErrorLogTestDB(t)
	ctx := buildRelayRetryTestContext()
	ctx.Set("id", 1002)
	ctx.Set("username", "relay-log-user")
	ctx.Set("token_name", "relay-log-token")
	ctx.Set("token_id", 2002)
	ctx.Set("original_model", "gpt-test")
	ctx.Set("group", "default")
	ctx.Set("use_channel", []string{"403"})
	common.SetContextKey(ctx, constant.ContextKeyRequestStartTime, time.Now().Add(-time.Second))

	require.NoError(t, common.UpdateErrorMessageReplacementRules(
		`[{"status_code":403,"match":"Insufficient balance","mode":"contains","replace":"渠道余额不足，请稍后重试","replace_status_code":429}]`,
	))
	t.Cleanup(func() { require.NoError(t, common.UpdateErrorMessageReplacementRules(`[]`)) })

	relayErr := types.NewErrorWithStatusCode(
		errors.New("upstream: Insufficient balance"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusForbidden,
	)
	recordChannelErrorLog(ctx, nil, types.ChannelError{ChannelId: 403, ChannelName: "balance", ChannelType: 1}, relayErr)

	var recorded model.Log
	require.NoError(t, db.Where("type = ?", model.LogTypeError).First(&recorded).Error)
	require.Contains(t, recorded.Content, "status_code=429")
	require.Contains(t, recorded.Content, "渠道余额不足，请稍后重试")
	require.NotContains(t, recorded.Content, "upstream: Insufficient balance")
	require.Equal(t, http.StatusForbidden, relayErr.StatusCode)
	require.Contains(t, relayErr.Error(), "upstream: Insufficient balance")

	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(recorded.Other, &other))
	require.EqualValues(t, http.StatusForbidden, other["status_code"])
	require.Equal(t, "status_code=403, upstream: Insufficient balance", other["upstream_error"])

	userLogs, _, err := model.GetUserLogs(1002, model.LogTypeError, 0, 0, "", "", 0, 10, "", "", "")
	require.NoError(t, err)
	require.Len(t, userLogs, 1)
	var userOther map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(userLogs[0].Other, &userOther))
	require.NotContains(t, userOther, "upstream_error")
}

func setupRelayErrorLogTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	originalDB, originalLogDB := model.DB, model.LOG_DB
	originalRedis := common.RedisEnabled
	originalErrorLogEnabled := constant.ErrorLogEnabled
	common.RedisEnabled = false
	constant.ErrorLogEnabled = true

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}))
	model.DB, model.LOG_DB = db, db

	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
		model.DB, model.LOG_DB = originalDB, originalLogDB
		common.RedisEnabled = originalRedis
		constant.ErrorLogEnabled = originalErrorLogEnabled
	})
	return db
}

func TestTaskErrorToChannelMetricErrorPreservesLocalClassification(t *testing.T) {
	upstream := taskErrorToChannelMetricError(&dto.TaskError{
		Error: errors.New("upstream failed"), StatusCode: http.StatusBadGateway,
	})
	require.Equal(t, types.ErrorCodeBadResponseStatusCode, upstream.GetErrorCode())
	require.Equal(t, http.StatusBadGateway, upstream.StatusCode)

	local := taskErrorToChannelMetricError(&dto.TaskError{
		Error: errors.New("convert failed"), StatusCode: http.StatusBadRequest, LocalError: true,
	})
	require.Equal(t, types.ErrorCodeConvertRequestFailed, local.GetErrorCode())
	require.Equal(t, http.StatusBadRequest, local.StatusCode)
}
