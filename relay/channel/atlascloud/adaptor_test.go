package atlascloud

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestConvertImageRequestBuildsAtlasPayload(t *testing.T) {
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	n := uint(2)
	request := dto.ImageRequest{
		Model:       "seedream-3.0",
		Prompt:      "mountain",
		Size:        "1024x1024",
		N:           &n,
		ExtraFields: json.RawMessage(`{"aspect_ratio":"1:1"}`),
	}
	info := &relaycommon.RelayInfo{}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
	require.NoError(t, err)

	payload, ok := converted.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "/api/v1/model/generateImage", info.RequestURLPath)
	require.Equal(t, "seedream-3.0", payload["model"])
	require.Equal(t, "mountain", payload["prompt"])
	require.Equal(t, "1024x1024", payload["size"])
	require.Equal(t, 2, payload["num_images"])
	require.Equal(t, "1:1", payload["aspect_ratio"])
}

func TestConvertOpenAIImageRequestOnlyForwardsCountWhenSupported(t *testing.T) {
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	n := uint(2)

	supported, err := (&Adaptor{}).ConvertImageRequest(c, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "openai/gpt-image-1/text-to-image"},
	}, dto.ImageRequest{Model: "gpt-image-1-enterprise", Prompt: "mountain", N: &n})
	require.NoError(t, err)
	supportedPayload := supported.(map[string]any)
	require.Equal(t, 2, supportedPayload["n"])
	require.NotContains(t, supportedPayload, "num_images")

	unsupported, err := (&Adaptor{}).ConvertImageRequest(c, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "openai/gpt-image-2/text-to-image"},
	}, dto.ImageRequest{Model: "gpt-image-2-enterprise", Prompt: "mountain", N: &n})
	require.NoError(t, err)
	unsupportedPayload := unsupported.(map[string]any)
	require.NotContains(t, unsupportedPayload, "num_images")
	require.NotContains(t, unsupportedPayload, "n")
}

func TestConvertOpenAIImageRequestDropsUnsupportedExtraCountFields(t *testing.T) {
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	request := dto.ImageRequest{
		Model:       "gpt-image-2-enterprise",
		Prompt:      "mountain",
		ExtraFields: json.RawMessage(`{"num_images":2,"n":2}`),
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "openai/gpt-image-2/text-to-image"},
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
	require.NoError(t, err)

	payload, ok := converted.(map[string]any)
	require.True(t, ok)
	require.NotContains(t, payload, "num_images")
	require.NotContains(t, payload, "n")
}

func TestImageOutputCountParameterName(t *testing.T) {
	require.Equal(t, "n", ImageOutputCountParameterName("openai/gpt-image-1/text-to-image", false))
	require.Equal(t, "", ImageOutputCountParameterName("openai/gpt-image-1.5/text-to-image", false))
	require.Equal(t, "", ImageOutputCountParameterName("openai/gpt-image-2/edit", true))
	require.Equal(t, "num_images", ImageOutputCountParameterName("seedream-3.0", false))
}

func TestConvertImageRequestPreservesModelForChannelMapping(t *testing.T) {
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	request := dto.ImageRequest{
		Model:  ModelGPTImage1,
		Prompt: "mountain",
	}
	info := &relaycommon.RelayInfo{}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
	require.NoError(t, err)

	payload, ok := converted.(map[string]any)
	require.True(t, ok)
	require.Equal(t, ModelGPTImage1, payload["model"])
	require.Equal(t, ModelGPTImage1, info.UpstreamModelName)
}

func TestConvertImageRequestAppliesOpenAIMappedDefaults(t *testing.T) {
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	request := dto.ImageRequest{
		Model:  "gpt-image-2-enterprise",
		Prompt: "mountain",
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "openai/gpt-image-2/text-to-image"},
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
	require.NoError(t, err)

	payload, ok := converted.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "openai/gpt-image-2/text-to-image", payload["model"])
	require.Equal(t, "1024x1024", payload["size"])
	require.Equal(t, "medium", payload["quality"])
}

func TestConvertImageRequestAppliesGrokMappedDefaults(t *testing.T) {
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	request := dto.ImageRequest{
		Model:  "grok-imagine-image-enterprise",
		Prompt: "mountain",
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "xai/grok-imagine-image/text-to-image"},
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
	require.NoError(t, err)

	payload, ok := converted.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "xai/grok-imagine-image/text-to-image", payload["model"])
	require.Equal(t, "1k", payload["resolution"])
	require.Equal(t, "1:1", payload["aspect_ratio"])
	require.NotContains(t, payload, "quality")
	require.NotContains(t, payload, "size")
}

func TestConvertImageRequestExtraFieldsOverrideGrokDefaults(t *testing.T) {
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	request := dto.ImageRequest{
		Model:       "grok-imagine-image-enterprise",
		Prompt:      "mountain",
		ExtraFields: json.RawMessage(`{"resolution":"2k","aspect_ratio":"16:9"}`),
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "xai/grok-imagine-image/text-to-image"},
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
	require.NoError(t, err)

	payload, ok := converted.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "2k", payload["resolution"])
	require.Equal(t, "16:9", payload["aspect_ratio"])
}

func TestConvertImageRequestDropsCanvasGroupBeforeAtlasCloudPayload(t *testing.T) {
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	var request dto.ImageRequest
	err := common.Unmarshal([]byte(`{
		"model":"gpt-image-1.5-enterprise",
		"prompt":"mountain",
		"group":"vip",
		"extra_fields":{"group":"other","size":"1024x1536"}
	}`), &request)
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "openai/gpt-image-1.5/text-to-image"},
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
	require.NoError(t, err)

	payload, ok := converted.(map[string]any)
	require.True(t, ok)
	require.NotContains(t, payload, "group")
	require.Equal(t, "1024x1536", payload["size"])
}

func TestApplyImageBillingDefaultsUsesExplicitAndModelDefaults(t *testing.T) {
	openAIRequest := &dto.ImageRequest{Model: "gpt-image-2-enterprise", Prompt: "mountain"}
	openAIMeta := &types.TokenCountMeta{}
	ApplyImageRequestDefaults(nil, openAIRequest, "openai/gpt-image-2/text-to-image", false)
	ApplyImageBillingDefaults(openAIMeta, openAIRequest, "openai/gpt-image-2/text-to-image", false)
	require.Equal(t, "1024x1024", openAIMeta.BillingDimensions[ratio_setting.ModelPriceVariantResolution])
	require.Equal(t, "medium", openAIMeta.BillingDimensions[ratio_setting.ModelPriceVariantQuality])

	grokRequest := &dto.ImageRequest{
		Model:       "grok-imagine-image-enterprise",
		Prompt:      "mountain",
		ExtraFields: json.RawMessage(`{"resolution":"2k"}`),
	}
	grokMeta := &types.TokenCountMeta{}
	ApplyImageRequestDefaults(nil, grokRequest, "xai/grok-imagine-image/text-to-image", false)
	ApplyImageBillingDefaults(grokMeta, grokRequest, "xai/grok-imagine-image/text-to-image", false)
	require.Equal(t, "2k", grokMeta.BillingDimensions[ratio_setting.ModelPriceVariantResolution])
	require.NotContains(t, grokMeta.BillingDimensions, ratio_setting.ModelPriceVariantQuality)
}

func TestApplyImageBillingDefaultsDropsUnsupportedOutputCount(t *testing.T) {
	n := uint(3)
	request := &dto.ImageRequest{Model: "gpt-image-2-enterprise", Prompt: "mountain", N: &n}
	meta := request.GetTokenCountMeta()
	require.Equal(t, float64(3), meta.BillingRatios["n"])

	ApplyImageBillingDefaults(meta, request, "openai/gpt-image-2/text-to-image", false)

	require.NotNil(t, request.N)
	require.Equal(t, float64(3), meta.BillingRatios["n"])
}

func TestApplyImageBillingDefaultsDropsEditOutputCount(t *testing.T) {
	n := uint(3)
	request := &dto.ImageRequest{Model: "gpt-image-2-enterprise", Prompt: "edit", N: &n}
	meta := request.GetTokenCountMeta()
	require.Equal(t, float64(3), meta.BillingRatios["n"])

	ApplyImageBillingDefaults(meta, request, "openai/gpt-image-2/edit", true)

	require.Nil(t, request.N)
	require.NotContains(t, meta.BillingRatios, "n")
}

func TestConvertImageEditRequestUsesEditModelAndImageURLs(t *testing.T) {
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	request := dto.ImageRequest{
		Model:  ModelGrokImage,
		Prompt: "make it red",
		Image:  json.RawMessage(`"https://example.com/source.png"`),
	}
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "xai/grok-imagine-image/text-to-image"},
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
	require.NoError(t, err)

	payload, ok := converted.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "xai/grok-imagine-image/edit", payload["model"])
	require.Equal(t, "xai/grok-imagine-image/edit", info.UpstreamModelName)
	require.Equal(t, []string{"https://example.com/source.png"}, payload["image_urls"])
	require.Equal(t, "1k", payload["resolution"])
	require.Equal(t, "1:1", payload["aspect_ratio"])
	require.NotContains(t, payload, "quality")
	require.NotContains(t, payload, "image_url")
}

func TestConvertOpenAIImageEditRequestUsesImagesArray(t *testing.T) {
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	request := dto.ImageRequest{
		Model:   ModelGPTImage1,
		Prompt:  "make it red",
		Quality: "standard",
		Image:   json.RawMessage(`"https://example.com/source.png"`),
	}
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "openai/gpt-image-1/text-to-image"},
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
	require.NoError(t, err)

	payload, ok := converted.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "openai/gpt-image-1/edit", payload["model"])
	require.Equal(t, "openai/gpt-image-1/edit", info.UpstreamModelName)
	require.Equal(t, []string{"https://example.com/source.png"}, payload["images"])
	require.Equal(t, "auto", payload["quality"])
	require.NotContains(t, payload, "image")
	require.NotContains(t, payload, "image_urls")
	require.NotContains(t, payload, "image_url")
}

func TestConvertOpenAIImageEditRequestDropsOutputCount(t *testing.T) {
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	n := uint(2)
	request := dto.ImageRequest{
		Model:  ModelGPTImage1,
		Prompt: "make it red",
		N:      &n,
		Image:  json.RawMessage(`"https://example.com/source.png"`),
	}
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "openai/gpt-image-1/text-to-image"},
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
	require.NoError(t, err)

	payload, ok := converted.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "openai/gpt-image-1/edit", payload["model"])
	require.NotContains(t, payload, "num_images")
}

func TestConvertOpenAIImageEditRequestDropsExtraOutputCountFields(t *testing.T) {
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	request := dto.ImageRequest{
		Model:       ModelGPTImage1,
		Prompt:      "make it red",
		Image:       json.RawMessage(`"https://example.com/source.png"`),
		ExtraFields: json.RawMessage(`{"num_images":2,"n":2}`),
	}
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "openai/gpt-image-1/text-to-image"},
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
	require.NoError(t, err)

	payload, ok := converted.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "openai/gpt-image-1/edit", payload["model"])
	require.NotContains(t, payload, "num_images")
	require.NotContains(t, payload, "n")
}

func TestConvertOpenAIImageEditRequestSupportsMultipleImages(t *testing.T) {
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	request := dto.ImageRequest{
		Model:  ModelGPTImage2,
		Prompt: "make it red",
		Images: json.RawMessage(`["https://example.com/a.png","https://example.com/b.png"]`),
	}
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "openai/gpt-image-2/text-to-image"},
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
	require.NoError(t, err)

	payload, ok := converted.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "openai/gpt-image-2/edit", payload["model"])
	require.Equal(t, []string{"https://example.com/a.png", "https://example.com/b.png"}, payload["images"])
	require.NotContains(t, payload, "image")
	require.NotContains(t, payload, "image_urls")
}

func TestConvertOpenAIImageEditJSONRequestDoesNotParseMultipart(t *testing.T) {
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	request := dto.ImageRequest{
		Model:  ModelGPTImage1,
		Prompt: "make it red",
		Images: json.RawMessage(`[
			"https://example.com/a.png",
			"https://example.com/b.png",
			"https://example.com/c.png"
		]`),
	}
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "openai/gpt-image-1/text-to-image"},
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
	require.NoError(t, err)

	payload, ok := converted.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "openai/gpt-image-1/edit", payload["model"])
	require.Equal(t, []string{
		"https://example.com/a.png",
		"https://example.com/b.png",
		"https://example.com/c.png",
	}, payload["images"])
	require.NotContains(t, payload, "image_urls")
}

func TestConvertOpenAIImageEditRequestNormalizesExtraImageURLs(t *testing.T) {
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	request := dto.ImageRequest{
		Model:       ModelGPTImage2,
		Prompt:      "make it red",
		ExtraFields: json.RawMessage(`{"image_urls":["https://example.com/a.png","https://example.com/b.png"]}`),
	}
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "openai/gpt-image-2/text-to-image"},
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
	require.NoError(t, err)

	payload, ok := converted.(map[string]any)
	require.True(t, ok)
	require.Equal(t, []string{"https://example.com/a.png", "https://example.com/b.png"}, payload["images"])
	require.NotContains(t, payload, "image")
	require.NotContains(t, payload, "image_urls")
	require.NotContains(t, payload, "image_url")
}

func TestConvertGrokImageEditRequestNormalizesExtraImages(t *testing.T) {
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	request := dto.ImageRequest{
		Model:       ModelGrokImage,
		Prompt:      "make it red",
		ExtraFields: json.RawMessage(`{"images":["https://example.com/a.png","https://example.com/b.png"]}`),
	}
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "xai/grok-imagine-image/text-to-image"},
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
	require.NoError(t, err)

	payload, ok := converted.(map[string]any)
	require.True(t, ok)
	require.Equal(t, []string{"https://example.com/a.png", "https://example.com/b.png"}, payload["image_urls"])
	require.NotContains(t, payload, "image")
	require.NotContains(t, payload, "images")
	require.NotContains(t, payload, "image_url")
}

func TestConvertImageEditRequestRejectsTooManyImages(t *testing.T) {
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	request := dto.ImageRequest{
		Model:  ModelGPTImage2,
		Prompt: "make it red",
		Images: json.RawMessage(`[
			"https://example.com/1.png",
			"https://example.com/2.png",
			"https://example.com/3.png",
			"https://example.com/4.png",
			"https://example.com/5.png",
			"https://example.com/6.png",
			"https://example.com/7.png",
			"https://example.com/8.png",
			"https://example.com/9.png",
			"https://example.com/10.png",
			"https://example.com/11.png"
		]`),
	}
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "openai/gpt-image-2/text-to-image"},
	}

	_, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
	require.ErrorContains(t, err, "at most 10")
}

func TestConvertImageEditRequestRequiresImage(t *testing.T) {
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	request := dto.ImageRequest{
		Model:  ModelGrokImage,
		Prompt: "make it red",
	}
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}

	_, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
	require.ErrorContains(t, err, "image is required for edits")
}

func TestBuildOpenAIImageResponseUsesURLsAndCountsOutputs(t *testing.T) {
	info := &relaycommon.RelayInfo{}

	response, err := buildOpenAIImageResponse([]string{"", "https://example.com/a.png", "https://example.com/b.png"}, info)
	require.NoError(t, err)

	require.Len(t, response.Data, 2)
	require.Equal(t, "https://example.com/a.png", response.Data[0].Url)
	ratio, ok := info.PriceData.GetOtherRatio("n")
	require.True(t, ok)
	require.Equal(t, float64(2), ratio)
}

func TestBuildOpenAIImageResponseDoesNotCountAtlasCloudEditsAsOutputMultiplier(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "openai/gpt-image-1/edit"},
	}

	response, err := buildOpenAIImageResponse([]string{"https://example.com/a.png"}, info)
	require.NoError(t, err)

	require.Len(t, response.Data, 1)
	require.False(t, info.PriceData.HasOtherRatio("n"))
}

func TestAtlasCloudFanoutUnsupportedTextToImageOutputCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	n := uint(2)
	var submitted []map[string]any
	var submittedMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/model/generateImage", r.URL.Path)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var payload map[string]any
		require.NoError(t, common.Unmarshal(body, &payload))
		submittedMu.Lock()
		submitted = append(submitted, payload)
		outputIndex := len(submitted)
		submittedMu.Unlock()
		require.NotContains(t, payload, "num_images")
		require.NotContains(t, payload, "n")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"data":{"id":"pred-%d","status":"completed","outputs":["https://example.com/out-%d.png"]}}`, outputIndex, outputIndex)
	}))
	defer server.Close()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	request := dto.ImageRequest{Model: "gpt-image-2-enterprise", Prompt: "mountain", N: &n}
	info := &relaycommon.RelayInfo{
		Request: &request,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    server.URL,
			ApiKey:            "test-key",
			UpstreamModelName: "openai/gpt-image-2/text-to-image",
		},
	}
	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, info, request)
	require.NoError(t, err)
	body, err := common.Marshal(converted)
	require.NoError(t, err)

	resp, err := adaptor.DoRequest(c, info, bytes.NewReader(body))
	require.NoError(t, err)
	httpResp, ok := resp.(*http.Response)
	require.True(t, ok)
	_, apiErr := adaptor.DoResponse(c, httpResp, info)
	require.Nil(t, apiErr)

	require.Len(t, submitted, 2)
	var imageResponse dto.ImageResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &imageResponse))
	require.Len(t, imageResponse.Data, 2)
	require.ElementsMatch(t, []string{"https://example.com/out-1.png", "https://example.com/out-2.png"}, []string{
		imageResponse.Data[0].Url,
		imageResponse.Data[1].Url,
	})
	ratio, ok := info.PriceData.GetOtherRatio("n")
	require.True(t, ok)
	require.Equal(t, float64(2), ratio)
	require.Equal(t, "actual", info.PriceData.BillingMeta["image_count_settlement"])
	require.Equal(t, "2", info.PriceData.BillingMeta["image_count_request"])
	require.Equal(t, "2", info.PriceData.BillingMeta["image_count_delivered"])
}

func TestAtlasCloudMediaHeadersAuthorizeOnlyAtlasCloudURLs(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "secret-key"},
	}

	headers := atlasCloudMediaHeaders("https://static.atlascloud.ai/media/image.png", info)
	require.Equal(t, "Bearer secret-key", headers["Authorization"])
	require.Equal(t, "https://www.atlascloud.ai/", headers["Referer"])
	require.NotEmpty(t, headers["User-Agent"])

	headers = atlasCloudMediaHeaders("https://cdn.example.com/image.png", info)
	require.NotContains(t, headers, "Authorization")
	require.NotEmpty(t, headers["Accept"])
}

func TestBuildAPIURLAvoidsDuplicateAPIVersion(t *testing.T) {
	require.Equal(t,
		"https://api.atlascloud.ai/api/v1/model/generateImage",
		BuildAPIURL("https://api.atlascloud.ai/api/v1", "/api/v1/model/generateImage"),
	)
}

func TestUpstreamImageModelNameDerivesEditActionFromMappedModel(t *testing.T) {
	require.Equal(t, ModelGrokImage, UpstreamImageModelName(ModelGrokImage, true))
	require.Equal(t, ModelGPTImage1, UpstreamImageModelName(ModelGPTImage1, true))
	require.Equal(t, ModelGPTImage15, UpstreamImageModelName(ModelGPTImage15, true))
	require.Equal(t, ModelGPTImage2, UpstreamImageModelName(ModelGPTImage2, true))
	require.Equal(t, "xai/grok-imagine-image/edit", UpstreamImageModelName("xai/grok-imagine-image/text-to-image", true))
	require.Equal(t, "openai/gpt-image-1/edit", UpstreamImageModelName("openai/gpt-image-1/text-to-image", true))
	require.Equal(t, "openai/gpt-image-1.5/edit", UpstreamImageModelName("openai/gpt-image-1.5/text-to-image", true))
	require.Equal(t, "openai/gpt-image-2/edit", UpstreamImageModelName("openai/gpt-image-2/text-to-image", true))
	require.Equal(t, "xai/grok-imagine-image/text-to-image", UpstreamImageModelName("xai/grok-imagine-image/text-to-image", false))
	require.False(t, imageEditUsesImageField("xai/grok-imagine-image/edit"))
	require.True(t, imageEditUsesImageField("openai/gpt-image-1/edit"))
	require.True(t, imageEditUsesImageField("openai/gpt-image-1.5/edit"))
	require.True(t, imageEditUsesImageField("openai/gpt-image-2/edit"))
	require.Equal(t, "auto", normalizeImageQuality("openai/gpt-image-1/edit", true, "standard"))
	require.Equal(t, "auto", normalizeImageQuality("openai/gpt-image-2/edit", true, "standard"))
	require.Equal(t, "standard", normalizeImageQuality("openai/gpt-image-1/text-to-image", false, "standard"))
}

func TestImagePollTimeoutExtendsGPTImage2Only(t *testing.T) {
	require.Equal(t, 120*time.Second, imagePollTimeout(ModelGPTImage1))
	require.Equal(t, 120*time.Second, imagePollTimeout("openai/gpt-image-1.5/text-to-image"))
	require.Equal(t, 300*time.Second, imagePollTimeout(ModelGPTImage2))
	require.Equal(t, 300*time.Second, imagePollTimeout("openai/gpt-image-2/text-to-image"))
	require.Equal(t, 300*time.Second, imagePollTimeout("openai/gpt-image-2/edit"))
	require.Equal(t, 120*time.Second, imagePollTimeout("xai/grok-imagine-image/text-to-image"))
}

func TestPollPredictionRetriesTemporaryRateLimit(t *testing.T) {
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/model/prediction/pred-1", r.URL.Path)
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = fmt.Fprint(w, `{"message":"Please retry after 1 seconds."}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data":{"id":"pred-1","status":"completed","outputs":["https://example.com/out.png"]}}`)
	}))
	defer server.Close()
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: "test-key"},
	}

	result, err := pollPrediction(c, info, "pred-1", time.Millisecond, 5*time.Second)
	require.NoError(t, err)

	require.Equal(t, 2, attempts)
	require.Equal(t, []string{"https://example.com/out.png"}, result.Outputs)
}

func TestAtlasCloudPredictionRetryDelayParsesRetryAfter(t *testing.T) {
	headers := http.Header{}
	headers.Set("Retry-After", "7")
	require.Equal(t, 7*time.Second, atlasCloudPredictionRetryDelay(headers, ""))

	headers = http.Header{}
	require.Equal(t, 4*time.Second, atlasCloudPredictionRetryDelay(headers, "Please retry after 4 seconds."))
}

func TestModelListIncludesVerifiedAtlasCloudModels(t *testing.T) {
	require.ElementsMatch(t, []string{
		ModelGrokImage,
		ModelGrokVideo,
		ModelGrokVideo15,
		ModelGPTImage1,
		ModelGPTImage15,
		ModelGPTImage2,
	}, ModelList)
}

func TestUploadMediaURLSupportsDocumentedAndRuntimeShapes(t *testing.T) {
	require.Equal(t, "https://example.com/top.png", uploadMediaURL(uploadMediaResponse{
		URL: " https://example.com/top.png ",
	}))
	require.Equal(t, "https://example.com/data.png", uploadMediaURL(uploadMediaResponse{
		Data: uploadMediaData{URL: "https://example.com/data.png"},
	}))
	require.Equal(t, "https://example.com/download.png", uploadMediaURL(uploadMediaResponse{
		Data: uploadMediaData{DownloadURL: "https://example.com/download.png"},
	}))
	require.Equal(t, "https://example.com/file.png", uploadMediaURL(uploadMediaResponse{
		Data: uploadMediaData{FileURL: "https://example.com/file.png"},
	}))
	require.Empty(t, uploadMediaURL(uploadMediaResponse{}))
}
