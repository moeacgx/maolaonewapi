package gemini

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestConvertImageRequestNativeImagineModelUsesGenerateContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	info := newGeminiImageRelayInfo("gemini-3-pro-image-preview")
	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "gemini-3-pro-image-preview",
		Prompt: "a red apple on a white table",
		N:      lo.ToPtr(uint(1)),
		Size:   "auto",
	})
	require.NoError(t, err)

	geminiRequest, ok := converted.(*dto.GeminiChatRequest)
	require.True(t, ok, "native imagine models must convert to generateContent, got %T", converted)
	require.Len(t, geminiRequest.Contents, 1)
	require.Equal(t, "user", geminiRequest.Contents[0].Role)
	require.Len(t, geminiRequest.Contents[0].Parts, 1)
	require.Equal(t, "a red apple on a white table", geminiRequest.Contents[0].Parts[0].Text)
	require.Equal(t, []string{"TEXT", "IMAGE"}, geminiRequest.GenerationConfig.ResponseModalities)
	require.Empty(t, geminiRequest.GenerationConfig.ImageConfig)
}

func TestConvertImageRequestNativeImagineMapsSizeQualityAndReferenceImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	info := newGeminiImageRelayInfo("gemini-2.5-flash-image")
	imageJSON, err := json.Marshal("data:image/png;base64,aW1hZ2U=")
	require.NoError(t, err)

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, dto.ImageRequest{
		Model:   "gemini-2.5-flash-image",
		Prompt:  "make the apple green",
		N:       lo.ToPtr(uint(2)),
		Size:    "1792x1024",
		Quality: "hd",
		Image:   imageJSON,
	})
	require.NoError(t, err)

	encoded, err := common.Marshal(converted)
	require.NoError(t, err)
	require.Equal(t, "user", gjson.GetBytes(encoded, "contents.0.role").String())
	require.Equal(t, "make the apple green", gjson.GetBytes(encoded, "contents.0.parts.0.text").String())
	require.Equal(t, "image/png", gjson.GetBytes(encoded, "contents.0.parts.1.inlineData.mimeType").String())
	require.Equal(t, "aW1hZ2U=", gjson.GetBytes(encoded, "contents.0.parts.1.inlineData.data").String())
	require.False(t, gjson.GetBytes(encoded, "generationConfig.candidateCount").Exists())
	require.Equal(t, []string{"TEXT", "IMAGE"}, stringSlice(gjson.GetBytes(encoded, "generationConfig.responseModalities").Array()))
	require.Equal(t, "16:9", gjson.GetBytes(encoded, "generationConfig.imageConfig.aspectRatio").String())
	require.False(t, gjson.GetBytes(encoded, "generationConfig.imageConfig.imageSize").Exists())
}

func TestConvertImageRequestGemini3QualityMapsImageSize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	info := newGeminiImageRelayInfo("gemini-3-pro-image-preview")
	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, dto.ImageRequest{
		Model:   "gemini-3-pro-image-preview",
		Prompt:  "a red apple on a white table",
		Quality: "hd",
		Size:    "1024x1024",
	})
	require.NoError(t, err)

	encoded, err := common.Marshal(converted)
	require.NoError(t, err)
	require.Equal(t, "1:1", gjson.GetBytes(encoded, "generationConfig.imageConfig.aspectRatio").String())
	require.Equal(t, "2K", gjson.GetBytes(encoded, "generationConfig.imageConfig.imageSize").String())
}

func TestConvertImageRequestGeminiFileURIUsesFileData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	info := newGeminiImageRelayInfo("gemini-3-pro-image-preview")
	imageJSON, err := json.Marshal("gs://bucket/apple.png")
	require.NoError(t, err)

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "gemini-3-pro-image-preview",
		Prompt: "edit the apple",
		Image:  imageJSON,
	})
	require.NoError(t, err)

	encoded, err := common.Marshal(converted)
	require.NoError(t, err)
	require.Equal(t, "gs://bucket/apple.png", gjson.GetBytes(encoded, "contents.0.parts.1.fileData.fileUri").String())
	require.Equal(t, "image/png", gjson.GetBytes(encoded, "contents.0.parts.1.fileData.mimeType").String())
	require.False(t, gjson.GetBytes(encoded, "contents.0.parts.1.inlineData").Exists())
}

func TestConvertImageRequestEmptyPrompt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := newGeminiImageRelayInfo("gemini-3-pro-image-preview")

	_, err := (&Adaptor{}).ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "gemini-3-pro-image-preview",
		Prompt: "   ",
	})
	require.EqualError(t, err, "prompt is required")
}

func TestConvertImageRequestImagenStillUsesPredictInstances(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	info := newGeminiImageRelayInfo("imagen-4.0-generate-001")
	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "imagen-4.0-generate-001",
		Prompt: "a red apple on a white table",
		N:      lo.ToPtr(uint(1)),
		Size:   "1024x1024",
	})
	require.NoError(t, err)
	require.IsType(t, dto.GeminiImageRequest{}, converted)
}

func TestConvertImageRequestRejectsNonImagineGeminiChatModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := newGeminiImageRelayInfo("gemini-2.5-pro")

	_, err := (&Adaptor{}).ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "gemini-2.5-pro",
		Prompt: "a red apple on a white table",
	})
	require.EqualError(t, err, "not supported model for image generation, only imagen models are supported")
}

func TestGetRequestURLNativeImagineUsesGenerateContent(t *testing.T) {
	info := newGeminiImageRelayInfo("gemini-3-pro-image-preview")
	url, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	require.Contains(t, url, "/models/gemini-3-pro-image-preview:generateContent")
	require.NotContains(t, url, ":predict")
}

func TestDoResponseNativeImagineReturnsOpenAIImageFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	info := newGeminiImageRelayInfo("gemini-3-pro-image-preview")
	info.RelayFormat = types.RelayFormatOpenAIImage

	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{{
			Content: dto.GeminiChatContent{
				Role: "model",
				Parts: []dto.GeminiPart{
					{Text: "here is the apple"},
					{InlineData: &dto.GeminiInlineData{MimeType: "image/png", Data: "aW1hZ2U="}},
				},
			},
		}},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:     12,
			CandidatesTokenCount: 40,
			TotalTokenCount:      52,
		},
	}
	body, err := common.Marshal(payload)
	require.NoError(t, err)

	usage, newAPIError := (&Adaptor{}).DoResponse(c, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
	}, info)
	require.Nil(t, newAPIError)

	var imageResp dto.ImageResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &imageResp))
	require.Len(t, imageResp.Data, 1)
	require.Equal(t, "aW1hZ2U=", imageResp.Data[0].B64Json)
	require.Empty(t, imageResp.Data[0].Url)

	usageValue, ok := usage.(*dto.Usage)
	require.True(t, ok)
	require.Equal(t, 12, usageValue.PromptTokens)
	require.Equal(t, 40, usageValue.CompletionTokens)
	require.Equal(t, 52, usageValue.TotalTokens)
}

func TestIsGeminiModelSupportImagineIncludesCanvasModels(t *testing.T) {
	for _, model := range []string{
		"gemini-3-pro-image-preview",
		"gemini-3-pro-image",
		"gemini-2.5-flash-image",
		"gemini-3.1-flash-image",
		"gemini-3.1-flash-image-preview",
	} {
		require.True(t, model_setting.IsGeminiModelSupportImagine(model), model)
	}
	require.False(t, model_setting.IsGeminiModelSupportImagine("gemini-2.5-pro"))
}

func newGeminiImageRelayInfo(model string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		OriginModelName: model,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://generativelanguage.googleapis.com",
			ApiVersion:        "v1beta",
			UpstreamModelName: model,
		},
	}
}

func stringSlice(results []gjson.Result) []string {
	out := make([]string, 0, len(results))
	for _, result := range results {
		out = append(out, result.String())
	}
	return out
}
