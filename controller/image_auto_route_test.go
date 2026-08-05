package controller

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestImageGenerationPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/v1/chat/completions", "/v1/images/generations"},
		{"/v1/responses", "/v1/images/generations"},
		{"/v1/responses/compact", ""},
		{"/pg/chat/completions", "/pg/images/generations"},
		{"/canvas/v1/chat/completions", "/canvas/v1/images/generations"},
		{"/v1/embeddings", ""},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			require.Equal(t, tt.want, imageGenerationPath(tt.path))
		})
	}
}

func TestShouldAutoRouteImageModel(t *testing.T) {
	tests := []struct {
		name          string
		modelName     string
		channelType   int
		endpointTypes []constant.EndpointType
		want          bool
	}{
		{
			name:          "custom image-only model",
			modelName:     "custom-image-model",
			channelType:   constant.ChannelTypeOpenAI,
			endpointTypes: []constant.EndpointType{constant.EndpointTypeImageGeneration},
			want:          true,
		},
		{
			name:        "custom multi-endpoint model",
			modelName:   "custom-multimodal-model",
			channelType: constant.ChannelTypeOpenAI,
			endpointTypes: []constant.EndpointType{
				constant.EndpointTypeImageGeneration,
				constant.EndpointTypeOpenAI,
			},
			want: false,
		},
		{
			name:          "compact model protected from metadata",
			modelName:     "gpt-5.5-openai-compact",
			channelType:   constant.ChannelTypeOpenAI,
			endpointTypes: []constant.EndpointType{constant.EndpointTypeImageGeneration},
			want:          false,
		},
		{
			name:          "codex channel protected from metadata",
			modelName:     "custom-image-model",
			channelType:   constant.ChannelTypeCodex,
			endpointTypes: []constant.EndpointType{constant.EndpointTypeImageGeneration},
			want:          false,
		},
		{
			name:        "built-in image model",
			modelName:   "gpt-image-2",
			channelType: constant.ChannelTypeOpenAI,
			endpointTypes: []constant.EndpointType{
				constant.EndpointTypeImageGeneration,
				constant.EndpointTypeOpenAI,
			},
			want: true,
		},
		{
			name:          "text model without metadata",
			modelName:     "gpt-5.6",
			channelType:   constant.ChannelTypeOpenAI,
			endpointTypes: nil,
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shouldAutoRouteImageModel(tt.modelName, tt.channelType, tt.endpointTypes))
		})
	}
}

func TestImageAutoRouteExcludedChannelTypes(t *testing.T) {
	require.Nil(t, imageAutoRouteExcludedChannelTypes(false))

	excludedChannelTypes := imageAutoRouteExcludedChannelTypes(true)
	_, excludesCodex := excludedChannelTypes[constant.ChannelTypeCodex]
	_, excludesOpenAI := excludedChannelTypes[constant.ChannelTypeOpenAI]
	require.True(t, excludesCodex)
	require.False(t, excludesOpenAI)
}

func TestAutoRouteImageRequestFromResponsesInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", bytes.NewBufferString(`{"model":"gpt-image-1","input":"一只戴墨镜的猫"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	defer common.CleanupBodyStorage(c)

	format, err := autoRouteImageRequest(c, types.RelayFormatOpenAIResponses)
	require.NoError(t, err)
	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIImage), format)
	require.Equal(t, "/v1/images/generations", c.Request.URL.Path)

	var body map[string]any
	require.NoError(t, common.UnmarshalBodyReusable(c, &body))
	require.Equal(t, "一只戴墨镜的猫", body["prompt"])
}

func TestAutoRouteImageRequestFromChatMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString(`{"model":"grok-imagine-image-pro","messages":[{"role":"user","content":"画一座雪山"}]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	defer common.CleanupBodyStorage(c)

	format, err := autoRouteImageRequest(c, types.RelayFormatOpenAI)
	require.NoError(t, err)
	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIImage), format)
	require.Equal(t, "/v1/images/generations", c.Request.URL.Path)

	var body map[string]any
	require.NoError(t, common.UnmarshalBodyReusable(c, &body))
	require.Equal(t, "画一座雪山", body["prompt"])
}

func TestAutoRouteImageRequestFromPlaygroundChat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/pg/chat/completions", bytes.NewBufferString(`{"model":"gpt-image-2","messages":[{"role":"user","content":"画一只小狗"}]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	defer common.CleanupBodyStorage(c)

	format, err := autoRouteImageRequest(c, types.RelayFormatOpenAI)
	require.NoError(t, err)
	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIImage), format)
	require.Equal(t, "/pg/images/generations", c.Request.URL.Path)
}

func TestAutoRouteImageRequestLeavesTextModelUntouched(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4.1","messages":[{"role":"user","content":"你好"}]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	defer common.CleanupBodyStorage(c)

	format, err := autoRouteImageRequest(c, types.RelayFormatOpenAI)
	require.NoError(t, err)
	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAI), format)
	require.Equal(t, "/v1/chat/completions", c.Request.URL.Path)
}

func TestAutoRouteImageRequestUsesOriginalModelContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", bytes.NewBufferString(`{"input":"画一只鸟"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-image-1")
	defer common.CleanupBodyStorage(c)

	format, err := autoRouteImageRequest(c, types.RelayFormatOpenAIResponses)
	require.NoError(t, err)
	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIImage), format)
	require.Equal(t, "/v1/images/generations", c.Request.URL.Path)
}

func TestAutoRouteImageRequestLeavesImageEndpointUntouched(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/generations", bytes.NewBufferString(`{"model":"gpt-image-2","prompt":"画一只猫"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	defer common.CleanupBodyStorage(c)

	format, err := autoRouteImageRequest(c, types.RelayFormatOpenAIImage)
	require.NoError(t, err)
	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIImage), format)
	require.Equal(t, "/v1/images/generations", c.Request.URL.Path)
}

func TestAutoRouteImageRequestLeavesCodexChannelUntouched(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-image-2","messages":[{"role":"user","content":"画一只猫"}]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeCodex)
	defer common.CleanupBodyStorage(c)

	format, err := autoRouteImageRequest(c, types.RelayFormatOpenAI)
	require.NoError(t, err)
	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAI), format)
	require.Equal(t, "/v1/chat/completions", c.Request.URL.Path)
}
