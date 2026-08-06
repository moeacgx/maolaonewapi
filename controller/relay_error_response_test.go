package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestWriteRelayErrorResponseHidesSensitiveFilterInternalCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	relayErr := service.NewSensitiveFilterAPIError(nil)
	writeRelayErrorResponse(c, nil, types.RelayFormatOpenAI, relayErr, "relay-sensitive-1")

	require.Equal(t, service.SensitiveFilterHTTPStatus, recorder.Code)
	var payload struct {
		Error types.OpenAIError `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t,
		"内容审计命中风险规则，请调整输入后重试 (request id: relay-sensitive-1)",
		payload.Error.Message,
	)
	require.Nil(t, payload.Error.Code)
	require.Empty(t, payload.Error.Metadata)
	require.NotContains(t, recorder.Body.String(), string(types.ErrorCodeSensitiveWordsDetected))
}

func TestWriteRelayErrorResponseReplacesOnlyClientMessage(t *testing.T) {
	require.NoError(t, common.UpdateErrorMessageReplacementRules(
		`[{"match":"Insufficient balance","mode":"exact","replace":"渠道余额不足，请稍后重试"}]`,
	))
	t.Cleanup(func() {
		require.NoError(t, common.UpdateErrorMessageReplacementRules(`[]`))
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	relayErr := types.NewErrorWithStatusCode(
		errors.New("Insufficient balance"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusForbidden,
	)
	rawError := relayErr.ToOpenAIError()
	require.Equal(t, "Insufficient balance", rawError.Message)

	writeRelayErrorResponse(c, nil, types.RelayFormatOpenAI, relayErr, "relay-replace-1")

	var payload struct {
		Error types.OpenAIError `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t,
		"渠道余额不足，请稍后重试 (request id: relay-replace-1)",
		payload.Error.Message,
	)
	require.Equal(t, "Insufficient balance", relayErr.Error())
	require.Equal(t, http.StatusForbidden, relayErr.StatusCode)
}

func TestWriteRelayErrorResponseReplacesClientStatusAndMessageTogether(t *testing.T) {
	require.NoError(t, common.UpdateErrorMessageReplacementRules(
		`[{"match":"Insufficient balance","mode":"contains","status_code":403,"replace":"请求过多，请稍后重试","replace_status_code":429}]`,
	))
	t.Cleanup(func() {
		require.NoError(t, common.UpdateErrorMessageReplacementRules(`[]`))
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	relayErr := types.NewErrorWithStatusCode(
		errors.New("upstream: Insufficient balance"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusForbidden,
	)

	writeRelayErrorResponse(c, nil, types.RelayFormatOpenAI, relayErr, "relay-status-replace-1")

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	var payload struct {
		Error types.OpenAIError `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, "请求过多，请稍后重试 (request id: relay-status-replace-1)", payload.Error.Message)
	// 内部原始状态和错误正文必须继续用于重试、禁用、审计与日志。
	require.Equal(t, http.StatusForbidden, relayErr.StatusCode)
	require.Equal(t, "upstream: Insufficient balance", relayErr.Error())
}

func TestWriteRelayErrorResponseRequiresConfiguredStatusAndMessage(t *testing.T) {
	require.NoError(t, common.UpdateErrorMessageReplacementRules(
		`[{"match":"Insufficient balance","mode":"contains","status_code":403,"replace":"请求过多，请稍后重试","replace_status_code":429}]`,
	))
	t.Cleanup(func() {
		require.NoError(t, common.UpdateErrorMessageReplacementRules(`[]`))
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	relayErr := types.NewErrorWithStatusCode(
		errors.New("upstream: Insufficient balance"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadRequest,
	)

	writeRelayErrorResponse(c, nil, types.RelayFormatOpenAI, relayErr, "relay-status-miss-1")

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "upstream: Insufficient balance")
	require.NotContains(t, recorder.Body.String(), "请求过多，请稍后重试")
}

func TestWriteRelayErrorResponseCustomRuleOverridesCapacityMessage(t *testing.T) {
	require.NoError(t, common.UpdateErrorMessageReplacementRules(
		`[{"match":"Selected model is at capacity","mode":"contains","replace":"自定义容量提示"}]`,
	))
	t.Cleanup(func() {
		require.NoError(t, common.UpdateErrorMessageReplacementRules(`[]`))
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	relayErr := types.WithOpenAIError(types.OpenAIError{
		Code:    "server_error",
		Message: "Selected model is at capacity. Please try a different model.",
	}, http.StatusOK)

	writeRelayErrorResponse(c, nil, types.RelayFormatOpenAI, relayErr, "capacity-replace-1")

	require.Contains(t, recorder.Body.String(), "自定义容量提示")
	require.NotContains(t, recorder.Body.String(), types.UpstreamCapacityClientMessage)
	require.Contains(t, relayErr.Error(), "Selected model is at capacity")
}
