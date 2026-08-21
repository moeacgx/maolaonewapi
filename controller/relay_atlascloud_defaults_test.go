package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAtlasCloudImageDefaultsForPricingUseMappedUpstreamModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeAtlasCloud)
	common.SetContextKey(c, constant.ContextKeyChannelModelMapping, `{
		"gpt-image-2-enterprise":"openai/gpt-image-2/text-to-image"
	}`)
	c.Set("model_mapping", `{
		"gpt-image-2-enterprise":"openai/gpt-image-2/text-to-image"
	}`)

	request := &dto.ImageRequest{Model: "gpt-image-2-enterprise", Prompt: "cat"}
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-image-2-enterprise",
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-image-2-enterprise",
		},
	}

	err := applyAtlasCloudImageDefaultsForPricing(c, info, types.RelayFormatOpenAIImage, request)
	require.NoError(t, err)
	meta := request.GetTokenCountMeta()
	applyAtlasCloudImageBillingDefaultsForPricing(c, info, types.RelayFormatOpenAIImage, request, meta)

	require.Equal(t, "gpt-image-2-enterprise", info.OriginModelName)
	require.Equal(t, "openai/gpt-image-2/text-to-image", info.UpstreamModelName)
	require.Equal(t, "openai/gpt-image-2/text-to-image", request.Model)
	require.Equal(t, "1024x1024", request.Size)
	require.Equal(t, "medium", request.Quality)
}

func TestAtlasCloudGrokDefaultsForPricingDoNotInjectQuality(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeAtlasCloud)
	common.SetContextKey(c, constant.ContextKeyChannelModelMapping, `{
		"grok-imagine-image-enterprise":"xai/grok-imagine-image/text-to-image"
	}`)
	c.Set("model_mapping", `{
		"grok-imagine-image-enterprise":"xai/grok-imagine-image/text-to-image"
	}`)

	request := &dto.ImageRequest{Model: "grok-imagine-image-enterprise", Prompt: "cat"}
	info := &relaycommon.RelayInfo{
		OriginModelName: "grok-imagine-image-enterprise",
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "grok-imagine-image-enterprise",
		},
	}

	err := applyAtlasCloudImageDefaultsForPricing(c, info, types.RelayFormatOpenAIImage, request)
	require.NoError(t, err)
	meta := request.GetTokenCountMeta()
	applyAtlasCloudImageBillingDefaultsForPricing(c, info, types.RelayFormatOpenAIImage, request, meta)

	require.Empty(t, request.Quality)
	require.Equal(t, "1k", request.Size)
}
