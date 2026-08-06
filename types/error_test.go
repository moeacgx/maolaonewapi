package types

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestClientErrorSerializationDoesNotMutateInternalError(t *testing.T) {
	require.NoError(t, common.UpdateErrorMessageReplacementRules(`[
		{"status_code":500,"match":"private upstream detail","mode":"exact","replace_status_code":429,"replace":"public client message"}
	]`))
	t.Cleanup(func() { require.NoError(t, common.UpdateErrorMessageReplacementRules(`[]`)) })

	relayErr := NewErrorWithStatusCode(
		errors.New("private upstream detail"),
		ErrorCodeBadResponse,
		http.StatusInternalServerError,
	)

	clientError := relayErr.ToOpenAIErrorForClient()
	require.Equal(t, "public client message", clientError.Message)
	require.Equal(t, http.StatusTooManyRequests, relayErr.StatusCodeForClient())
	// 客户端视图计算不能污染后续自动禁用、重试和错误日志读取的内部原文。
	require.Equal(t, "private upstream detail", relayErr.Error())
	require.Contains(t, relayErr.ErrorWithStatusCode(), "private upstream detail")
	require.NotContains(t, relayErr.ErrorWithStatusCode(), "public client message")
}

func TestErrOptionWithHideErrMsgPreservesCause(t *testing.T) {
	original := fmt.Errorf("post upstream: %w", context.Canceled)
	relayErr := NewError(
		original,
		ErrorCodeDoRequestFailed,
		ErrOptionWithHideErrMsg("upstream error: do request failed"),
	)

	require.Equal(t, "upstream error: do request failed", relayErr.Error())
	require.ErrorIs(t, relayErr, context.Canceled)
}

func TestNewOpenAIErrorPreservesRequestContextCause(t *testing.T) {
	tests := []struct {
		name  string
		cause error
	}{
		{name: "canceled", cause: context.Canceled},
		{name: "deadline exceeded", cause: context.DeadlineExceeded},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := fmt.Errorf("downstream stream write failed: %w", tt.cause)
			relayErr := NewOpenAIError(
				original,
				ErrorCodeBadResponse,
				http.StatusInternalServerError,
				ErrOptionWithHideErrMsg("upstream stream closed"),
			)

			require.Equal(t, "upstream stream closed", relayErr.Error())
			require.ErrorIs(t, relayErr, tt.cause)
		})
	}
}

func TestReadableRelayErrorMessageAddsChineseHintForStreamDisconnect(t *testing.T) {
	relayErr := NewErrorWithStatusCode(
		errors.New("upstream stream disconnected: connection reset by peer"),
		ErrorCodeDoRequestFailed,
		http.StatusInternalServerError,
	)

	require.Equal(t, "upstream stream disconnected: connection reset by peer", relayErr.Error())
	require.Contains(t, relayErr.ErrorWithStatusCode(), "status_code=500")
	require.Contains(t, relayErr.ErrorWithStatusCode(), "upstream stream disconnected: connection reset by peer")
	require.Contains(t, relayErr.ErrorWithStatusCode(), "中文说明：上游流式响应中途断开")
	require.Contains(t, relayErr.MaskSensitiveErrorWithStatusCode(), "中文说明：上游流式响应中途断开")
	require.Contains(t, relayErr.ToOpenAIError().Message, "中文说明：上游流式响应中途断开")
	require.Contains(t, relayErr.ToClaudeError().Message, "中文说明：上游流式响应中途断开")
}

func TestReadableRelayErrorMessageDoesNotDuplicateChineseHint(t *testing.T) {
	message := "upstream stream disconnected: connection reset by peer（中文说明：上游流式响应中途断开）"

	require.Equal(t, message, readableRelayErrorMessage(message))
}

func TestUpstreamCapacityErrorClassificationAndStatusNormalization(t *testing.T) {
	tests := []struct {
		name    string
		code    any
		message string
		want    bool
	}{
		{name: "official message", code: "server_error", message: "Selected model is at capacity. Please try a different model.", want: true},
		{name: "structured model code", code: "model_at_capacity", message: "temporary failure", want: true},
		{name: "structured account pool code", code: "account_pool_capacity_exhausted", message: "temporary failure", want: true},
		{name: "unrelated capacity text", code: "invalid_request", message: "request context is at capacity limit", want: false},
		{name: "generic server error", code: "server_error", message: "upstream failed", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			relayErr := WithOpenAIError(OpenAIError{
				Type:    "server_error",
				Code:    tt.code,
				Message: tt.message,
			}, http.StatusOK)

			require.Equal(t, tt.want, IsUpstreamCapacityError(relayErr))
			if tt.want {
				require.Equal(t, http.StatusTooManyRequests, relayErr.StatusCode)
				require.Equal(t, http.StatusOK, relayErr.OriginalStatusCode)
				require.Equal(t, UpstreamCapacityClientMessage, relayErr.ToOpenAIError().Message)
				require.Equal(t, UpstreamCapacityClientMessage, relayErr.ToClaudeError().Message)
				require.Contains(t, relayErr.ErrorWithStatusCode(), UpstreamCapacityClientMessage)
				require.Contains(t, relayErr.MaskSensitiveErrorWithStatusCode(), UpstreamCapacityClientMessage)
			} else {
				require.Equal(t, http.StatusOK, relayErr.StatusCode)
				require.Zero(t, relayErr.OriginalStatusCode)
			}
		})
	}

	localErr := NewError(
		errors.New("Selected model is at capacity. Please try a different model."),
		ErrorCodeInvalidRequest,
		ErrOptionWithStatusCode(http.StatusOK),
	)
	require.False(t, IsUpstreamCapacityError(localErr))
	require.Equal(t, http.StatusOK, localErr.StatusCode)
}

func TestUpstreamCapacityStatusIsNormalizedFromExistingServerError(t *testing.T) {
	relayErr := WithOpenAIError(OpenAIError{
		Code:    "server_error",
		Message: "Selected model is at capacity. Please try a different model.",
	}, http.StatusServiceUnavailable)

	require.Equal(t, http.StatusTooManyRequests, relayErr.StatusCode)
	require.Equal(t, http.StatusServiceUnavailable, relayErr.OriginalStatusCode)
	require.Equal(t, UpstreamCapacityClientMessage, relayErr.ToOpenAIError().Message)
}

func TestSetMessagePreservesCapacitySourceForClassification(t *testing.T) {
	relayErr := WithOpenAIError(OpenAIError{
		Code:    "server_error",
		Message: "Selected model is at capacity. Please try a different model.",
	}, http.StatusOK)

	relayErr.SetMessage(UpstreamCapacityClientMessage + "（request-id）")

	require.True(t, IsUpstreamCapacityError(relayErr))
	require.Contains(t, relayErr.Error(), "Selected model is at capacity")
	require.Contains(t, relayErr.ToOpenAIError().Message, "request-id")
}
