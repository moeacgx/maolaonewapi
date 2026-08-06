package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRunCanvasImageTaskRelayRetriesQuotaSyncThenSucceeds(t *testing.T) {
	setupCanvasImageTaskTestDB(t)
	task := &model.Task{
		TaskID:   "task_quota_sync_recovers",
		UserId:   1,
		Platform: constant.TaskPlatformImage,
		Status:   model.TaskStatusQueued,
		Progress: "0%",
	}
	require.NoError(t, task.Insert())

	relayReq := canvasImageTaskRelayRequest{
		TaskID: task.TaskID,
		Keys: map[string]any{
			string(constant.ContextKeyUserId):         task.UserId,
			string(constant.ContextKeyAsyncImageTask): true,
		},
	}
	attempts := 0
	runCanvasImageTaskRelayWithRetryPolicy(
		relayReq,
		time.Second,
		func(req canvasImageTaskRelayRequest) (*httptest.ResponseRecorder, int) {
			attempts++
			recorder := httptest.NewRecorder()
			isQuotaRetry, _ := req.Keys[string(constant.ContextKeyAsyncImageTaskQuotaSyncRetry)].(bool)
			require.Equal(t, attempts > 1, isQuotaRetry)
			if attempts == 1 {
				req.Keys[string(constant.ContextKeyAsyncImageTaskQuotaSync)] = true
				recorder.WriteHeader(http.StatusServiceUnavailable)
				_, err := recorder.WriteString(`{"error":{"message":"quota sync unavailable"}}`)
				require.NoError(t, err)
				return recorder, 0
			}
			recorder.WriteHeader(http.StatusOK)
			_, err := recorder.WriteString(`{"data":[{"url":"https://example.com/recovered.png"}]}`)
			require.NoError(t, err)
			return recorder, 44
		},
		2,
		func(int) time.Duration { return 0 },
	)

	reloaded, exists, err := model.GetByTaskId(task.UserId, task.TaskID)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, 2, attempts)
	require.EqualValues(t, model.TaskStatusSuccess, reloaded.Status)
	require.Equal(t, 44, reloaded.ChannelId)
}

func TestRunCanvasImageTaskRelayDoesNotRetryOtherServiceUnavailable(t *testing.T) {
	setupCanvasImageTaskTestDB(t)
	task := &model.Task{
		TaskID:   "task_other_503",
		UserId:   1,
		Platform: constant.TaskPlatformImage,
		Status:   model.TaskStatusQueued,
		Progress: "0%",
	}
	require.NoError(t, task.Insert())

	attempts := 0
	runCanvasImageTaskRelayWithRetryPolicy(
		canvasImageTaskRelayRequest{
			TaskID: task.TaskID,
			Keys: map[string]any{
				string(constant.ContextKeyUserId):         task.UserId,
				string(constant.ContextKeyAsyncImageTask): true,
			},
		},
		time.Second,
		func(req canvasImageTaskRelayRequest) (*httptest.ResponseRecorder, int) {
			attempts++
			recorder := httptest.NewRecorder()
			recorder.WriteHeader(http.StatusServiceUnavailable)
			_, err := recorder.WriteString(`{"error":{"message":"upstream unavailable"}}`)
			require.NoError(t, err)
			return recorder, 19
		},
		2,
		func(int) time.Duration { return 0 },
	)

	reloaded, exists, err := model.GetByTaskId(task.UserId, task.TaskID)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, 1, attempts)
	require.EqualValues(t, model.TaskStatusFailure, reloaded.Status)
}

func TestRunCanvasImageTaskRelayStopsAtQuotaSyncRetryLimit(t *testing.T) {
	for _, replacedStatus := range []int{http.StatusOK, http.StatusFound} {
		t.Run(http.StatusText(replacedStatus), func(t *testing.T) {
			setupCanvasImageTaskTestDB(t)
			task := &model.Task{
				TaskID:   fmt.Sprintf("task_quota_sync_retry_limit_%d", replacedStatus),
				UserId:   1,
				Platform: constant.TaskPlatformImage,
				Status:   model.TaskStatusQueued,
				Progress: "0%",
			}
			require.NoError(t, task.Insert())

			attempts := 0
			runCanvasImageTaskRelayWithRetryPolicy(
				canvasImageTaskRelayRequest{
					TaskID: task.TaskID,
					Keys: map[string]any{
						string(constant.ContextKeyUserId):         task.UserId,
						string(constant.ContextKeyAsyncImageTask): true,
					},
				},
				time.Second,
				func(req canvasImageTaskRelayRequest) (*httptest.ResponseRecorder, int) {
					attempts++
					req.Keys[string(constant.ContextKeyAsyncImageTaskQuotaSync)] = true
					recorder := httptest.NewRecorder()
					recorder.WriteHeader(replacedStatus)
					_, err := recorder.WriteString(`{"error":{"message":"quota sync unavailable"}}`)
					require.NoError(t, err)
					return recorder, 0
				},
				2,
				func(int) time.Duration { return 0 },
			)

			reloaded, exists, err := model.GetByTaskId(task.UserId, task.TaskID)
			require.NoError(t, err)
			require.True(t, exists)
			require.Equal(t, 3, attempts)
			require.EqualValues(t, model.TaskStatusFailure, reloaded.Status)
		})
	}
}

func TestPreUpstreamErrorMarksAsyncImageQuotaSync(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	common.SetContextKey(ctx, constant.ContextKeyAsyncImageTask, true)
	relayErr := types.NewErrorWithStatusCode(
		fmt.Errorf("%w: wait timeout", model.ErrUserQuotaCacheSync),
		types.ErrorCodeQueryDataError,
		http.StatusServiceUnavailable,
		types.ErrOptionWithSkipRetry(),
	)

	rememberAsyncImageTaskPreUpstreamError(ctx, relayErr)
	writeRelayErrorResponse(ctx, nil, types.RelayFormatOpenAIImage, relayErr, "")

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyAsyncImageTaskQuotaSync))
	require.Equal(t,
		string(types.ErrorCodeQueryDataError),
		common.GetContextKeyString(ctx, constant.ContextKeyAsyncImageTaskErrorCode),
	)
}

func TestPreUpstreamErrorDoesNotMarkNonQuotaSync503(t *testing.T) {
	tests := []struct {
		name string
		err  *types.NewAPIError
	}{
		{
			name: "unrelated service unavailable",
			err: types.NewErrorWithStatusCode(
				fmt.Errorf("database unavailable"),
				types.ErrorCodeQueryDataError,
				http.StatusServiceUnavailable,
			),
		},
		{
			name: "quota sync with wrong internal status",
			err: types.NewErrorWithStatusCode(
				fmt.Errorf("%w: unexpected mapping", model.ErrUserQuotaCacheSync),
				types.ErrorCodeQueryDataError,
				http.StatusInternalServerError,
			),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			common.SetContextKey(ctx, constant.ContextKeyAsyncImageTask, true)
			rememberAsyncImageTaskPreUpstreamError(ctx, tt.err)
			require.False(t, common.GetContextKeyBool(ctx, constant.ContextKeyAsyncImageTaskQuotaSync))
		})
	}
}

func TestCanvasImageTaskQuotaSyncBlockedUsesInternalMarker(t *testing.T) {
	keys := map[string]any{
		string(constant.ContextKeyAsyncImageTaskQuotaSync): true,
	}
	serviceUnavailable := httptest.NewRecorder()
	serviceUnavailable.WriteHeader(http.StatusServiceUnavailable)
	okResponse := httptest.NewRecorder()
	okResponse.WriteHeader(http.StatusOK)
	redirectResponse := httptest.NewRecorder()
	redirectResponse.WriteHeader(http.StatusFound)
	replacedBadRequest := httptest.NewRecorder()
	replacedBadRequest.WriteHeader(http.StatusBadRequest)

	require.True(t, canvasImageTaskQuotaSyncBlocked(serviceUnavailable, keys))
	require.True(t, canvasImageTaskQuotaSyncBlocked(replacedBadRequest, keys))
	require.True(t, canvasImageTaskQuotaSyncBlocked(okResponse, keys))
	require.True(t, canvasImageTaskQuotaSyncBlocked(redirectResponse, keys))
	require.False(t, canvasImageTaskQuotaSyncBlocked(serviceUnavailable, nil))
	require.Equal(t, http.StatusServiceUnavailable, canvasImageTaskRelayStatusCode(okResponse, keys))
	require.Equal(t, http.StatusServiceUnavailable, canvasImageTaskRelayStatusCode(redirectResponse, keys))
}
