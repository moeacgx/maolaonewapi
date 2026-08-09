package atlascloud

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
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
	require.NotContains(t, payload, "image_url")
}

func TestConvertOpenAIImageEditRequestUsesImageField(t *testing.T) {
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
	require.Equal(t, "https://example.com/source.png", payload["image"])
	require.Equal(t, "auto", payload["quality"])
	require.NotContains(t, payload, "image_urls")
	require.NotContains(t, payload, "image_url")
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
