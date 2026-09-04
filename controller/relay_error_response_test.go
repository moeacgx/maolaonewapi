package controller

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	geminirelay "github.com/QuantumNous/new-api/relay/channel/gemini"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestWriteRelayErrorResponseSkipsCanceledRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	requestContext, cancel := context.WithCancel(context.Background())
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(requestContext)
	cancel()

	relayErr := types.NewError(context.Canceled, types.ErrorCodeDoRequestFailed)
	writeRelayErrorResponse(c, nil, types.RelayFormatOpenAI, relayErr, "canceled-1")

	require.Empty(t, recorder.Body.String())
}

func TestRequestContextErrorReasonKeepsIndependentUpstreamCancellation(t *testing.T) {
	requestContext, cancel := context.WithCancel(context.Background())
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(requestContext)
	cancel()

	requestError := types.NewError(context.Canceled, types.ErrorCodeDoRequestFailed)
	require.Equal(t, "request_context_canceled", requestContextErrorReason(c, requestError))

	upstreamError := types.NewError(errors.New("context canceled by upstream"), types.ErrorCodeDoRequestFailed)
	require.Empty(t, requestContextErrorReason(c, upstreamError))
}

func TestWriteRelayErrorResponseReplacesOnlyClientMessageAndStatus(t *testing.T) {
	require.NoError(t, common.UpdateErrorMessageReplacementRules(`[{"match":"Insufficient balance","mode":"contains","status_code":403,"replace":"请求过多，请稍后重试","replace_status_code":429}]`))
	t.Cleanup(func() { require.NoError(t, common.UpdateErrorMessageReplacementRules(`[]`)) })
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	relayErr := types.NewErrorWithStatusCode(errors.New("upstream: Insufficient balance"), types.ErrorCodeBadResponseStatusCode, http.StatusForbidden)
	writeRelayErrorResponse(c, nil, types.RelayFormatOpenAI, relayErr, "relay-status-1")
	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	var payload struct {
		Error types.OpenAIError `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, "请求过多，请稍后重试 (request id: relay-status-1)", payload.Error.Message)
	require.Equal(t, http.StatusForbidden, relayErr.StatusCode)
	require.Equal(t, "upstream: Insufficient balance", relayErr.Error())
}

func TestClientErrorReplacementKeepsMaskedClientCandidateForPartialModes(t *testing.T) {
	require.NoError(t, common.UpdateErrorMessageReplacementRules(`[{"match":"balance","mode":"exact","replace":"client"}]`))
	t.Cleanup(func() { require.NoError(t, common.UpdateErrorMessageReplacementRules(`[]`)) })

	relayErr := types.WithOpenAIError(types.OpenAIError{
		Type:    "server_error",
		Code:    "upstream_error",
		Message: "https://api.example.com/v1 balance",
	}, http.StatusBadGateway)

	clientErr, clientStatus := clientOpenAIError(relayErr, "masked-1")
	require.Equal(t, http.StatusBadGateway, clientStatus)
	require.Equal(t, "https://***.com/*** client (request id: masked-1)", clientErr.Message)
}

func TestGeminiEmptyCandidatesUsesCentralClientErrorReplacement(t *testing.T) {
	require.NoError(t, common.UpdateErrorMessageReplacementRules(`[{"match":"request blocked by Gemini API","mode":"contains","status_code":400,"replace":"client blocked","replace_status_code":429}]`))
	t.Cleanup(func() { require.NoError(t, common.UpdateErrorMessageReplacementRules(`[]`)) })

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatOpenAI,
		OriginModelName: "gemini-2.5-flash",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-2.5-flash",
		},
	}

	usage, relayErr := geminirelay.GeminiChatHandler(c, info, &http.Response{
		Body: io.NopCloser(bytes.NewBufferString(`{"promptFeedback":{"blockReason":"SAFETY"}}`)),
	})
	require.NotNil(t, usage)
	require.NotNil(t, relayErr)
	require.Empty(t, recorder.Body.String())

	writeRelayErrorResponse(c, nil, types.RelayFormatOpenAI, relayErr, "gemini-error-1")
	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	var payload struct {
		Error types.OpenAIError `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, "client blocked (request id: gemini-error-1)", payload.Error.Message)
}

func TestClientErrorReplacementIgnoresInternalQuotaErrors(t *testing.T) {
	require.NoError(t, common.UpdateErrorMessageReplacementRules(`[{"matches":["balance","额度"],"mode":"contains","status_code":403,"replace":"出现异常波动，请重试。","replace_status_code":403}]`))
	t.Cleanup(func() { require.NoError(t, common.UpdateErrorMessageReplacementRules(`[]`)) })
	relayErr := types.NewErrorWithStatusCode(
		errors.New("预扣费额度失败, 用户剩余额度: $5.000000, 需要预扣费额度: $10.000000"),
		types.ErrorCodeInsufficientUserQuota,
		http.StatusForbidden,
		types.ErrOptionWithSkipRetry(),
	)

	clientErr, clientStatus := clientOpenAIError(relayErr, "quota-1")

	require.Equal(t, http.StatusForbidden, clientStatus)
	require.Contains(t, clientErr.Message, "预扣费额度失败")
	require.Contains(t, clientErr.Message, "用户剩余额度")
	require.NotContains(t, clientErr.Message, "出现异常波动")
}

func TestWriteRelayErrorResponseKeepsCommittedStreamUntouched(t *testing.T) {
	require.NoError(t, common.UpdateErrorMessageReplacementRules(`[{"match":"upstream","mode":"contains","replace":"client"}]`))
	t.Cleanup(func() { require.NoError(t, common.UpdateErrorMessageReplacementRules(`[]`)) })
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Writer.WriteHeader(http.StatusOK)
	_, err := c.Writer.WriteString("data: committed\n\n")
	require.NoError(t, err)
	relayErr := types.NewErrorWithStatusCode(errors.New("upstream failure"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)
	writeRelayErrorResponse(c, nil, types.RelayFormatOpenAI, relayErr, "stream-1")
	require.Equal(t, "data: committed\n\n", recorder.Body.String())
	require.Equal(t, "upstream failure", relayErr.Error())
}

func TestRealtimeClientErrorViewChangesPayloadOnly(t *testing.T) {
	require.NoError(t, common.UpdateErrorMessageReplacementRules(`[{"match":"realtime upstream","mode":"exact","replace":"realtime client","replace_status_code":429}]`))
	t.Cleanup(func() { require.NoError(t, common.UpdateErrorMessageReplacementRules(`[]`)) })
	relayErr := types.NewErrorWithStatusCode(errors.New("realtime upstream"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)
	clientErr, clientStatus := clientOpenAIError(relayErr, "realtime-1")
	require.Equal(t, "realtime client (request id: realtime-1)", clientErr.Message)
	require.Equal(t, http.StatusTooManyRequests, clientStatus)
	require.Equal(t, "realtime upstream", relayErr.Error())
	require.Equal(t, http.StatusBadGateway, relayErr.StatusCode)
}

func TestWriteCapacityErrorClearsUncommittedEventStreamHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	helper.SetEventStreamHeaders(c)
	relayErr := types.WithOpenAIError(types.OpenAIError{
		Type:    "server_error",
		Code:    "server_error",
		Message: "Selected model is at capacity. Please try a different model.",
	}, http.StatusOK)

	writeRelayErrorResponse(c, nil, types.RelayFormatOpenAIResponses, relayErr, "capacity-1")

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))
	require.Empty(t, recorder.Header().Get("Transfer-Encoding"))
	require.Contains(t, recorder.Body.String(), types.UpstreamCapacityClientMessage)
}
