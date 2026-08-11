package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
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
	request := &dto.ImageRequest{
		Model:  "gpt-image-2-enterprise",
		Prompt: "mountain",
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-image-2-enterprise",
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		Request:         request,
	}

	err := applyAtlasCloudImageDefaultsForPricing(c, info, types.RelayFormatOpenAIImage, request)
	require.NoError(t, err)
	meta := request.GetTokenCountMeta()
	applyAtlasCloudImageBillingDefaultsForPricing(c, info, types.RelayFormatOpenAIImage, request, meta)

	require.Equal(t, "gpt-image-2-enterprise", info.OriginModelName)
	require.Equal(t, "openai/gpt-image-2/text-to-image", info.UpstreamModelName)
	require.Equal(t, "1024x1024", request.Size)
	require.Equal(t, "medium", request.Quality)
	require.Equal(t, "1024x1024", meta.BillingDimensions[ratio_setting.ModelPriceVariantResolution])
	require.Equal(t, "medium", meta.BillingDimensions[ratio_setting.ModelPriceVariantQuality])
}

func TestAtlasCloudGrokDefaultsForPricingDoNotInjectQuality(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeAtlasCloud)
	common.SetContextKey(c, constant.ContextKeyChannelModelMapping, `{
		"grok-imagine-image-enterprise":"xai/grok-imagine-image/text-to-image"
	}`)
	request := &dto.ImageRequest{
		Model:  "grok-imagine-image-enterprise",
		Prompt: "mountain",
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "grok-imagine-image-enterprise",
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		Request:         request,
	}

	err := applyAtlasCloudImageDefaultsForPricing(c, info, types.RelayFormatOpenAIImage, request)
	require.NoError(t, err)
	meta := request.GetTokenCountMeta()
	applyAtlasCloudImageBillingDefaultsForPricing(c, info, types.RelayFormatOpenAIImage, request, meta)

	require.Empty(t, request.Size)
	require.Empty(t, request.Quality)
	require.Equal(t, "xai/grok-imagine-image/text-to-image", info.UpstreamModelName)
	require.Equal(t, "1k", meta.BillingDimensions[ratio_setting.ModelPriceVariantResolution])
	require.NotContains(t, meta.BillingDimensions, ratio_setting.ModelPriceVariantQuality)
}
