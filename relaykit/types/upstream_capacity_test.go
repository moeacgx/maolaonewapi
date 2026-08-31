package types

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOfficialCapacityErrorFromSuccessfulHTTPStreamBecomesRetryableRateLimit(t *testing.T) {
	relayErr := WithOpenAIError(OpenAIError{
		Type:    "server_error",
		Code:    "server_error",
		Message: "Selected model is at capacity. Please try a different model.",
	}, http.StatusOK)

	require.Equal(t, http.StatusTooManyRequests, relayErr.StatusCode)
	assert.Equal(t, http.StatusOK, relayErr.OriginalStatusCode)
	assert.Equal(t, "已触发 OpenAI 官方限流，请重试", relayErr.ToOpenAIError().Message)
}

func TestStableUpstreamCapacityCodesBecomeRetryableRateLimits(t *testing.T) {
	codes := []string{
		"account_pool_capacity_exhausted",
		"capacity_exhausted",
		"model_at_capacity",
		"model_capacity_exhausted",
		"model_overloaded",
		"upstream_capacity_exhausted",
	}

	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			relayErr := WithOpenAIError(OpenAIError{
				Type:    "server_error",
				Code:    code,
				Message: "temporary upstream failure",
			}, http.StatusOK)

			require.True(t, IsUpstreamCapacityError(relayErr))
			assert.Equal(t, http.StatusTooManyRequests, relayErr.StatusCode)
			assert.Equal(t, http.StatusOK, relayErr.OriginalStatusCode)
			assert.Equal(t, UpstreamCapacityClientMessage, relayErr.ToOpenAIError().Message)
		})
	}
}

func TestLocalCapacityTextIsNotReportedAsOfficialRateLimit(t *testing.T) {
	relayErr := NewError(
		errors.New("Selected model is at capacity. Please try a different model."),
		ErrorCodeBadResponse,
		ErrOptionWithStatusCode(http.StatusOK),
	)

	require.False(t, IsUpstreamCapacityError(relayErr))
	assert.Equal(t, http.StatusOK, relayErr.StatusCode)
}

func TestLocalOpenAIShapedCapacityTextIsNotReportedAsOfficialRateLimit(t *testing.T) {
	relayErr := NewOpenAIError(
		errors.New("advanced custom model is at capacity route resolution failed"),
		ErrorCodeInvalidRequest,
		http.StatusBadRequest,
	)

	require.False(t, IsUpstreamCapacityError(relayErr))
	assert.Equal(t, http.StatusBadRequest, relayErr.StatusCode)
	assert.Contains(t, relayErr.ToOpenAIError().Message, "model is at capacity")
}
