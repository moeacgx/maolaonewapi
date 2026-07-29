package controller

import (
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
