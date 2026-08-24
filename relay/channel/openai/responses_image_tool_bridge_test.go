package openai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesImageToolBridgeUsesV1ImagesGenerationPath(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:      relayconstant.RelayModeImagesGenerations,
		RelayFormat:    types.RelayFormatOpenAIResponses,
		RequestURLPath: dto.DynamicRoutingImageGenerationPath,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ChannelBaseUrl: "https://upstream.example",
		},
	}

	requestURL, err := (&Adaptor{}).GetRequestURL(info)

	require.NoError(t, err)
	assert.Equal(t, "https://upstream.example"+dto.DynamicRoutingImageGenerationPath, requestURL)
}

func TestWriteResponsesImageToolBridgeResponseReturnsResponsesJSON(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	info := &relaycommon.RelayInfo{
		RequestId: "req_json",
		ResponsesImageToolBridge: &relaycommon.ResponsesImageToolBridge{
			SourceModel: "gpt-5.6-sol",
		},
	}
	responseBody := []byte(`{"created":123,"data":[{"b64_json":"base64-image"}]}`)

	apiErr := writeResponsesImageToolBridgeResponse(context, info, responseBody, &dto.Usage{})

	require.Nil(t, apiErr)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"model":"gpt-5.6-sol"`)
	assert.Contains(t, recorder.Body.String(), `"type":"image_generation_call"`)
	assert.Contains(t, recorder.Body.String(), `"result":"base64-image"`)
}

func TestWriteResponsesImageToolBridgeResponseReturnsResponsesSSE(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	info := &relaycommon.RelayInfo{
		RequestId: "req_stream",
		ResponsesImageToolBridge: &relaycommon.ResponsesImageToolBridge{
			SourceModel:      "gpt-5.6-sol",
			DownstreamStream: true,
		},
	}
	responseBody := []byte(`{"created":123,"data":[{"b64_json":"base64-image"}]}`)

	apiErr := writeResponsesImageToolBridgeResponse(context, info, responseBody, &dto.Usage{})

	require.Nil(t, apiErr)
	assert.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.True(t, strings.HasPrefix(body, "event: response.created"))
	assert.Contains(t, body, "event: response.output_item.added")
	assert.Contains(t, body, `"type":"image_generation_call"`)
	assert.Contains(t, body, `"result":"base64-image"`)
	assert.Contains(t, body, "event: response.completed")
	assert.Contains(t, body, "data: [DONE]")
}

func TestResponsesImageToolBridgeResponseRejectsMissingBase64(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	info := &relaycommon.RelayInfo{
		RequestId: "req_missing_b64",
		ResponsesImageToolBridge: &relaycommon.ResponsesImageToolBridge{
			SourceModel: "gpt-5.6-sol",
		},
	}
	responseBody := []byte(`{"created":123,"data":[{}]}`)

	apiErr := writeResponsesImageToolBridgeResponse(context, info, responseBody, &dto.Usage{})

	require.Error(t, apiErr)
	assert.Equal(t, types.ErrorCodeBadResponseBody, apiErr.GetErrorCode())
}
