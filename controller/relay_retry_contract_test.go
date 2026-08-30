package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"
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
