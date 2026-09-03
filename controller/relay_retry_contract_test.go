package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func retryContractError() *types.NewAPIError {
	return types.NewErrorWithStatusCode(errors.New("upstream unavailable"), types.ErrorCodeBadResponseStatusCode, http.StatusServiceUnavailable)
}

func TestShouldRetryStopsForCanceledRequest(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	reqCtx, cancel := context.WithCancel(context.Background())
	cancel()
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(reqCtx)
	require.False(t, shouldRetry(ctx, retryContractError(), 1))
}

func TestShouldRetryStopsAfterResponseCommitted(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	_, err := ctx.Writer.Write([]byte("committed"))
	require.NoError(t, err)
	require.False(t, shouldRetry(ctx, retryContractError(), 1))
}

func TestShouldRetryStopsForSpecificChannel(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Set("specific_channel_id", "7")
	require.False(t, shouldRetry(ctx, retryContractError(), 1))
}

func TestShouldRetryAllowsEmptyUsageResponse(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	emptyUsageErr := types.NewOpenAIError(errors.New("upstream returned no billable usage"), types.ErrorCodeEmptyResponse, http.StatusBadGateway)

	require.True(t, shouldRetry(ctx, emptyUsageErr, 1))
}

func TestShouldRetryOfficialCapacityErrorDespiteAffinitySkip(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Set("channel_affinity_skip_retry_on_failure", true)
	capacityErr := types.WithOpenAIError(types.OpenAIError{
		Type:    "server_error",
		Code:    "server_error",
		Message: "Selected model is at capacity. Please try a different model.",
	}, http.StatusOK)
	// 模拟渠道状态码映射把已识别的容量错误再次改回上游原始 200。
	capacityErr.StatusCode = http.StatusOK

	require.True(t, shouldRetry(ctx, capacityErr, 1))
}

func TestShouldRetryAllowsConfigured502AfterCodexAffinityFailure(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	storage, err := common.CreateBodyStorage([]byte(`{"prompt_cache_key":"retry-affinity-502"}`))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })
	ctx.Set(common.KeyBodyStorage, storage)

	service.GetPreferredChannelByAffinity(ctx, "gpt-5.6-sol", "default")
	service.MarkChannelAffinityUsed(ctx, "default", 601)
	require.True(t, service.ShouldSkipRetryAfterChannelAffinityFailure(ctx))

	upstream502 := types.NewErrorWithStatusCode(
		errors.New("upstream bad gateway"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadGateway,
	)
	require.True(t, shouldRetry(ctx, upstream502, 1))
}

func TestShouldRetryUsesOriginalStatusAfterMapping(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Set("channel_affinity_skip_retry_on_failure", true)

	mappedError := types.NewErrorWithStatusCode(
		errors.New("upstream bad gateway"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusOK,
	)
	mappedError.OriginalStatusCode = http.StatusBadGateway

	require.True(t, shouldRetry(ctx, mappedError, 1))
}

func TestShouldRetryCapacityOverridesBadResponseBodySkipCode(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	capacityErr := types.WithOpenAIError(types.OpenAIError{
		Type:    "server_error",
		Code:    types.ErrorCodeBadResponseBody,
		Message: "Selected model is at capacity. Please try a different model.",
	}, http.StatusOK)

	require.True(t, shouldRetry(ctx, capacityErr, 1))
}

func TestShouldRetryKeepsAffinityForInvalidStatusCode(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	storage, err := common.CreateBodyStorage([]byte(`{"prompt_cache_key":"retry-affinity-invalid-status"}`))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })
	ctx.Set(common.KeyBodyStorage, storage)

	service.GetPreferredChannelByAffinity(ctx, "gpt-5.6-sol", "default")
	service.MarkChannelAffinityUsed(ctx, "default", 601)
	require.True(t, service.ShouldSkipRetryAfterChannelAffinityFailure(ctx))

	invalidStatus := types.NewError(
		errors.New("upstream transport failed"),
		types.ErrorCodeDoRequestFailed,
		types.ErrOptionWithStatusCode(0),
	)
	require.False(t, shouldRetry(ctx, invalidStatus, 1))
}

func TestSettledEmptyUsageRecordsOnlyOneErrorLog(t *testing.T) {
	channel := setupLegacyChannelStatusUpdateTestDB(t)
	previousErrorLogEnabled := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = true
	t.Cleanup(func() { constant.ErrorLogEnabled = previousErrorLogEnabled })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Set(common.RequestIdKey, "req-empty-usage-once")
	ctx.Set("id", 1)
	ctx.Set("username", "admin")
	ctx.Set("token_name", "test-token")
	ctx.Set("original_model", "gpt-test")
	ctx.Set("group", "default")
	ctx.Set("channel_id", channel.Id)
	ctx.Set("channel_name", channel.Name)
	ctx.Set("channel_type", channel.Type)

	info := &relaycommon.RelayInfo{
		UserId:          1,
		OriginModelName: "gpt-test",
		UsingGroup:      "default",
		StartTime:       time.Now(),
		IsStream:        true,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: channel.Id,
		},
		PriceData: hosttypes.PriceData{
			ModelRatio:     1,
			GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1},
		},
	}

	usageErr := service.PostTextConsumeQuota(ctx, info, &dto.Usage{}, nil)
	require.Error(t, usageErr)
	require.False(t, types.IsRecordErrorLog(usageErr))
	processChannelError(ctx, types.ChannelError{
		ChannelId:   channel.Id,
		ChannelType: channel.Type,
		ChannelName: channel.Name,
	}, usageErr)

	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", "req-empty-usage-once", model.LogTypeError).Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Contains(t, logs[0].Content, "上游没有返回计费信息")
}
