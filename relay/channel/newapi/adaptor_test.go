package newapi

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdaptorPassesThroughSupportedRequestFormats(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	a := &Adaptor{}
	info := &relaycommon.RelayInfo{}

	openAIReq := &dto.GeneralOpenAIRequest{Model: "gpt-4o-mini"}
	convertedOpenAI, err := a.ConvertOpenAIRequest(c, info, openAIReq)
	require.NoError(t, err)
	require.Same(t, openAIReq, convertedOpenAI)

	responsesReq := dto.OpenAIResponsesRequest{Model: "gpt-4o-mini"}
	convertedResponses, err := a.ConvertOpenAIResponsesRequest(c, info, responsesReq)
	require.NoError(t, err)
	require.Equal(t, responsesReq, convertedResponses)

	embeddingReq := dto.EmbeddingRequest{Model: "text-embedding-3-small", Input: []any{"hello"}}
	convertedEmbedding, err := a.ConvertEmbeddingRequest(c, info, embeddingReq)
	require.NoError(t, err)
	require.Equal(t, embeddingReq, convertedEmbedding)

	claudeReq := &dto.ClaudeRequest{Model: "claude-sonnet-4-5"}
	convertedClaude, err := a.ConvertClaudeRequest(c, info, claudeReq)
	require.NoError(t, err)
	require.Same(t, claudeReq, convertedClaude)

	geminiReq := &dto.GeminiChatRequest{}
	convertedGemini, err := a.ConvertGeminiRequest(c, info, geminiReq)
	require.NoError(t, err)
	require.Same(t, geminiReq, convertedGemini)
}

func TestAdaptorSetupRequestHeaderForFormats(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("{}"))
	c.Request.Header.Set("Content-Type", "application/json")
	a := &Adaptor{}

	headers := http.Header{}
	err := a.SetupRequestHeader(c, &headers, &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "test-key"},
	})
	require.NoError(t, err)
	require.Equal(t, "Bearer test-key", headers.Get("Authorization"))
	require.Equal(t, "test-key", headers.Get("x-api-key"))
	require.Equal(t, "2023-06-01", headers.Get("anthropic-version"))
	require.Equal(t, "application/json", headers.Get("Content-Type"))

	headers = http.Header{}
	err = a.SetupRequestHeader(c, &headers, &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatGemini,
		ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "gemini-key"},
	})
	require.NoError(t, err)
	require.Equal(t, "Bearer gemini-key", headers.Get("Authorization"))
	require.Equal(t, "gemini-key", headers.Get("x-goog-api-key"))
}

func TestAdaptorConvertsMultipartImageEditsWithAllFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-1"))
	require.NoError(t, writer.WriteField("prompt", "edit prompt"))
	require.NoError(t, writer.WriteField("size", "1024x1024"))
	image, err := writer.CreateFormFile("image", "input.png")
	require.NoError(t, err)
	_, err = image.Write([]byte("png-bytes"))
	require.NoError(t, err)
	mask, err := writer.CreateFormFile("mask", "mask.png")
	require.NoError(t, err)
	_, err = mask.Write([]byte("mask-bytes"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	a := &Adaptor{}
	converted, err := a.ConvertImageRequest(c, &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeNewAPI}}, dto.ImageRequest{Model: "gpt-image-1"})
	require.NoError(t, err)
	convertedBody, ok := converted.(*bytes.Buffer)
	require.True(t, ok)
	require.Contains(t, c.Request.Header.Get("Content-Type"), "multipart/form-data; boundary=")
	require.Contains(t, convertedBody.String(), `name="prompt"`)
	require.Contains(t, convertedBody.String(), "edit prompt")
	require.Contains(t, convertedBody.String(), `name="size"`)
	require.Contains(t, convertedBody.String(), `name="image"; filename="input.png"`)
	require.Contains(t, convertedBody.String(), `name="mask"; filename="mask.png"`)
}

func TestAdaptorGetRequestURLUsesIncomingPath(t *testing.T) {
	a := &Adaptor{}
	url, err := a.GetRequestURL(&relaycommon.RelayInfo{
		RequestURLPath: "/v1/responses",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://newapi.example.com",
			ChannelType:    constant.ChannelTypeNewAPI,
		},
	})
	require.NoError(t, err)
	require.Equal(t, "https://newapi.example.com/v1/responses", url)
}
