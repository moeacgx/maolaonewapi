package atlascloud

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestApplyImageFormulaBillingInputsOnlyRunsWhenFormulaEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedRouteVariants := ratio_setting.ModelRoutePriceVariants2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRoutePriceVariantsByJSONString(savedRouteVariants))
	})
	require.NoError(t, ratio_setting.UpdateModelRoutePriceVariantsByJSONString(`{}`))

	dataURL := testPNGDataURL(t, 31, 23)
	rawImages, err := common.Marshal([]string{dataURL})
	require.NoError(t, err)
	request := &dto.ImageRequest{
		Model:  "gpt-image-2-enterprise",
		Prompt: "edit",
		Images: rawImages,
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-image-2-enterprise",
		RelayMode:       relayconstant.RelayModeImagesEdits,
	}
	meta := &types.TokenCountMeta{}
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())

	ApplyImageFormulaBillingInputs(c, info, meta, request, "openai/gpt-image-2/edit", true)
	require.Empty(t, meta.BillingParams)
	require.Empty(t, meta.BillingImages)
}

func TestApplyImageFormulaBillingInputsProbesDataURLDimensions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedRouteVariants := ratio_setting.ModelRoutePriceVariants2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRoutePriceVariantsByJSONString(savedRouteVariants))
	})
	require.NoError(t, ratio_setting.UpdateModelRoutePriceVariantsByJSONString(`{
		"gpt-image-2-enterprise":{
			"image.edit":{
				"resolution_enabled":false,
				"quality_enabled":false,
				"formula":{
					"enabled":true,
					"expression":"input_image_tokens(48)",
					"defaults":{"size":"1024x1024","quality":"medium"}
				}
			}
		}
	}`))

	dataURL := testPNGDataURL(t, 31, 23)
	rawImages, err := common.Marshal([]string{dataURL})
	require.NoError(t, err)
	request := &dto.ImageRequest{
		Model:  "gpt-image-2-enterprise",
		Prompt: "edit",
		Images: rawImages,
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-image-2-enterprise",
		RelayMode:       relayconstant.RelayModeImagesEdits,
	}
	meta := &types.TokenCountMeta{}
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())

	ApplyImageFormulaBillingInputs(c, info, meta, request, "openai/gpt-image-2/edit", true)
	require.Equal(t, float64(1), meta.BillingParams[ratio_setting.ModelPriceExtraParamInputImages])
	require.Len(t, meta.BillingImages, 1)
	require.Equal(t, 31, meta.BillingImages[0].Width)
	require.Equal(t, 23, meta.BillingImages[0].Height)
}

func testPNGDataURL(t *testing.T, width, height int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buffer bytes.Buffer
	require.NoError(t, png.Encode(&buffer, img))
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buffer.Bytes())
}
