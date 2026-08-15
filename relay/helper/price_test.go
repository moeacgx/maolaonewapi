package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelPriceHelperTieredUsesPreloadedRequestInput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"tiered-test-model":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"tiered-test-model":"param(\"stream\") == true ? tier(\"stream\", p * 3) : tier(\"base\", p * 2)"}`,
	}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/channel/test/1", nil)
	req.Body = nil
	req.ContentLength = 0
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	ctx.Set("group", "default")

	info := &relaycommon.RelayInfo{
		OriginModelName: "tiered-test-model",
		UserGroup:       "default",
		UsingGroup:      "default",
		RequestHeaders:  map[string]string{"Content-Type": "application/json"},
		BillingRequestInput: &billingexpr.RequestInput{
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    []byte(`{"stream":true}`),
		},
	}

	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.Equal(t, 1500, priceData.QuotaToPreConsume)
	require.NotNil(t, info.TieredBillingSnapshot)
	require.Equal(t, "stream", info.TieredBillingSnapshot.EstimatedTier)
	require.Equal(t, billing_setting.BillingModeTieredExpr, info.TieredBillingSnapshot.BillingMode)
	require.Equal(t, common.QuotaPerUnit, info.TieredBillingSnapshot.QuotaPerUnit)
}

func TestModelPriceHelperTieredUsesCompletionFallbackAndRejectsOverflow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{
			"tiered-fallback-model":"tiered_expr",
			"tiered-overflow-model":"tiered_expr"
		}`,
		"billing_setting.billing_expr": `{
			"tiered-fallback-model":"tier(\"base\", p * 3 + c * 15)",
			"tiered-overflow-model":"tier(\"overflow\", p * 1000000000)"
		}`,
		"group_ratio_setting.group_ratio": `{"default":1,"free":0}`,
	}))

	newInfo := func(model, group string) (*gin.Context, *relaycommon.RelayInfo) {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		ctx.Set("group", group)
		return ctx, &relaycommon.RelayInfo{
			OriginModelName: model,
			UserGroup:       group,
			UsingGroup:      group,
			BillingRequestInput: &billingexpr.RequestInput{
				Body: []byte(`{}`),
			},
		}
	}

	ctx, info := newInfo("tiered-fallback-model", "default")
	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	// (1000*3 + 8192*15) / 1e6 * 500000 = 62940
	require.Equal(t, 62940, priceData.QuotaToPreConsume)

	ctx, info = newInfo("tiered-fallback-model", "free")
	priceData, err = ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.Zero(t, priceData.QuotaToPreConsume)

	ctx, info = newInfo("tiered-overflow-model", "default")
	_, err = ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	var clamp *common.QuotaClamp
	require.ErrorAs(t, err, &clamp)
	require.Equal(t, "QuotaRound", clamp.Op)
	require.Equal(t, common.QuotaClampOverflow, clamp.Kind)
}

func TestModelPriceHelperAppliesRequestBillingRatiosOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedModelPrices := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedModelPrices))
	})

	modelPrices, err := common.Marshal(map[string]float64{"fixed-image-price": 0.04})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(modelPrices)))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("group", "default")
	info := &relaycommon.RelayInfo{
		OriginModelName: "fixed-image-price",
		UserGroup:       "default",
		UsingGroup:      "default",
	}
	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{
		ImagePriceRatio: 3,
		BillingRatios:   map[string]float64{"n": 3},
	})

	require.NoError(t, err)
	require.Equal(t, 180000, priceData.QuotaToPreConsume)
	require.Equal(t, float64(3), priceData.OtherRatios()["n"])
}

func TestModelPriceHelperAppliesModelPriceVariantDimensions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedModelPrices := ratio_setting.ModelPrice2JSONString()
	savedVariants := ratio_setting.ModelPriceVariants2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedModelPrices))
		require.NoError(t, ratio_setting.UpdateModelPriceVariantsByJSONString(savedVariants))
	})

	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"fixed-image-price":0.04}`))
	require.NoError(t, ratio_setting.UpdateModelPriceVariantsByJSONString(`{
		"fixed-image-price":{
			"resolution_enabled":true,
			"quality_enabled":true,
			"rules":[
				{"resolution":"1024x1024","quality":"high","price":0.2}
			]
		}
	}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("group", "default")
	info := &relaycommon.RelayInfo{
		OriginModelName: "fixed-image-price",
		UserGroup:       "default",
		UsingGroup:      "default",
	}
	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{
		ImagePriceRatio: 10,
		BillingRatios:   map[string]float64{"n": 2},
		BillingDimensions: map[string]string{
			ratio_setting.ModelPriceVariantResolution: "1024x1024",
			ratio_setting.ModelPriceVariantQuality:    "HIGH",
		},
	})

	require.NoError(t, err)
	require.Equal(t, 0.2, priceData.ModelPrice)
	require.Equal(t, 200000, priceData.QuotaToPreConsume)
	require.Equal(t, "matched", priceData.BillingMeta["variant_price_status"])
	require.Equal(t, "1024x1024", priceData.BillingMeta["resolution"])
	require.Equal(t, "high", priceData.BillingMeta["quality"])
	ratio, ok := priceData.GetOtherRatio("n")
	require.True(t, ok)
	require.Equal(t, float64(2), ratio)
}

func TestModelPriceHelperPrefersImageEditRoutePriceVariant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedModelPrices := ratio_setting.ModelPrice2JSONString()
	savedVariants := ratio_setting.ModelPriceVariants2JSONString()
	savedRouteVariants := ratio_setting.ModelRoutePriceVariants2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedModelPrices))
		require.NoError(t, ratio_setting.UpdateModelPriceVariantsByJSONString(savedVariants))
		require.NoError(t, ratio_setting.UpdateModelRoutePriceVariantsByJSONString(savedRouteVariants))
	})

	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"fixed-image-price":0.04}`))
	require.NoError(t, ratio_setting.UpdateModelPriceVariantsByJSONString(`{
		"fixed-image-price":{
			"resolution_enabled":true,
			"quality_enabled":true,
			"rules":[{"resolution":"1024x1024","quality":"medium","price":0.2}]
		}
	}`))
	require.NoError(t, ratio_setting.UpdateModelRoutePriceVariantsByJSONString(`{
		"fixed-image-price":{
			"image.edit":{
				"resolution_enabled":true,
				"quality_enabled":true,
				"rules":[{"resolution":"1024x1024","quality":"medium","price":0.32}]
			}
		}
	}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("group", "default")
	info := &relaycommon.RelayInfo{
		OriginModelName: "fixed-image-price",
		RelayMode:       relayconstant.RelayModeImagesEdits,
		UserGroup:       "default",
		UsingGroup:      "default",
	}
	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{
		BillingRatios: map[string]float64{"n": 2},
		BillingDimensions: map[string]string{
			ratio_setting.ModelPriceVariantResolution: "1024x1024",
			ratio_setting.ModelPriceVariantQuality:    "medium",
		},
	})

	require.NoError(t, err)
	require.Equal(t, 0.32, priceData.ModelPrice)
	require.Equal(t, 320000, priceData.QuotaToPreConsume)
	require.Equal(t, "image.edit", priceData.BillingMeta["price_route"])
	require.Equal(t, "matched", priceData.BillingMeta["route_price_status"])
	require.Empty(t, priceData.BillingMeta["variant_price_status"])
}

func TestModelPriceHelperAddsRouteExtraParamSurcharge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedModelPrices := ratio_setting.ModelPrice2JSONString()
	savedVariants := ratio_setting.ModelPriceVariants2JSONString()
	savedRouteVariants := ratio_setting.ModelRoutePriceVariants2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedModelPrices))
		require.NoError(t, ratio_setting.UpdateModelPriceVariantsByJSONString(savedVariants))
		require.NoError(t, ratio_setting.UpdateModelRoutePriceVariantsByJSONString(savedRouteVariants))
	})

	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"fixed-image-price":0.04}`))
	require.NoError(t, ratio_setting.UpdateModelRoutePriceVariantsByJSONString(`{
		"fixed-image-price":{
			"image.edit":{
				"resolution_enabled":true,
				"quality_enabled":true,
				"rules":[{"resolution":"1024x1024","quality":"medium","price":0.32}],
				"extra_params":[{"key":"input_images","base":1,"unit_price":0.01}]
			}
		}
	}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("group", "default")
	info := &relaycommon.RelayInfo{
		OriginModelName: "fixed-image-price",
		RelayMode:       relayconstant.RelayModeImagesEdits,
		UserGroup:       "default",
		UsingGroup:      "default",
	}
	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{
		BillingRatios: map[string]float64{"n": 2},
		BillingDimensions: map[string]string{
			ratio_setting.ModelPriceVariantResolution: "1024x1024",
			ratio_setting.ModelPriceVariantQuality:    "medium",
		},
		BillingParams: map[string]float64{
			ratio_setting.ModelPriceExtraParamInputImages: 3,
		},
	})

	require.NoError(t, err)
	require.InDelta(t, 0.34, priceData.ModelPrice, 1e-12)
	require.Equal(t, 340000, priceData.QuotaToPreConsume)
	require.Equal(t, "matched", priceData.BillingMeta["route_price_status"])
	require.Equal(t, "0.02", priceData.BillingMeta["extra_price"])
	require.Equal(t, "2", priceData.BillingMeta["extra_param_input_images_extra_units"])
}

func TestModelPriceHelperAddsRouteOnlyExtraParamSurcharge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedModelPrices := ratio_setting.ModelPrice2JSONString()
	savedVariants := ratio_setting.ModelPriceVariants2JSONString()
	savedRouteVariants := ratio_setting.ModelRoutePriceVariants2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedModelPrices))
		require.NoError(t, ratio_setting.UpdateModelPriceVariantsByJSONString(savedVariants))
		require.NoError(t, ratio_setting.UpdateModelRoutePriceVariantsByJSONString(savedRouteVariants))
	})

	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"fixed-image-price":0.04}`))
	require.NoError(t, ratio_setting.UpdateModelRoutePriceVariantsByJSONString(`{
		"fixed-image-price":{
			"image.edit":{
				"resolution_enabled":false,
				"quality_enabled":false,
				"extra_params":[{"key":"input_images","base":1,"unit_price":0.01}]
			}
		}
	}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("group", "default")
	info := &relaycommon.RelayInfo{
		OriginModelName: "fixed-image-price",
		RelayMode:       relayconstant.RelayModeImagesEdits,
		UserGroup:       "default",
		UsingGroup:      "default",
	}
	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{
		BillingParams: map[string]float64{
			ratio_setting.ModelPriceExtraParamInputImages: 3,
		},
	})

	require.NoError(t, err)
	require.InDelta(t, 0.06, priceData.ModelPrice, 1e-12)
	require.Equal(t, 30000, priceData.QuotaToPreConsume)
	require.Equal(t, "disabled", priceData.BillingMeta["route_price_status"])
	require.Equal(t, "0.02", priceData.BillingMeta["extra_price"])
}

func TestModelPriceHelperAppliesRouteFormulaWithoutBaseModelPrice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedModelPrices := ratio_setting.ModelPrice2JSONString()
	savedRouteVariants := ratio_setting.ModelRoutePriceVariants2JSONString()
	savedFreePreConsume := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedModelPrices))
		require.NoError(t, ratio_setting.UpdateModelRoutePriceVariantsByJSONString(savedRouteVariants))
		operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = savedFreePreConsume
	})

	operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = false
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateModelRoutePriceVariantsByJSONString(`{
		"formula-only-image":{
			"image.edit":{
				"resolution_enabled":false,
				"quality_enabled":false,
				"formula":{
					"enabled":true,
					"expression":"input_images * 0.5",
					"defaults":{"size":"1024x1024","quality":"medium"}
				}
			}
		}
	}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("group", "default")
	info := &relaycommon.RelayInfo{
		OriginModelName: "formula-only-image",
		RelayMode:       relayconstant.RelayModeImagesEdits,
		UserGroup:       "default",
		UsingGroup:      "default",
	}
	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{
		BillingRatios: map[string]float64{"n": 3},
		BillingParams: map[string]float64{
			ratio_setting.ModelPriceExtraParamInputImages: 2,
		},
	})

	require.NoError(t, err)
	require.Equal(t, 1.0, priceData.ModelPrice)
	require.Equal(t, 1500000, priceData.QuotaToPreConsume)
	require.True(t, priceData.UsePrice)
	require.False(t, priceData.FreeModel)
	require.Equal(t, "formula", priceData.BillingMeta["route_price_status"])
	require.Equal(t, "1", priceData.BillingMeta["formula_price"])
}

func TestModelPriceHelperRouteFormulaRecordsCalculationBreakdown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedModelPrices := ratio_setting.ModelPrice2JSONString()
	savedRouteVariants := ratio_setting.ModelRoutePriceVariants2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedModelPrices))
		require.NoError(t, ratio_setting.UpdateModelRoutePriceVariantsByJSONString(savedRouteVariants))
	})

	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"atlas-formula-image":0.04}`))
	require.NoError(t, ratio_setting.UpdateModelRoutePriceVariantsByJSONString(`{
		"atlas-formula-image":{
			"image.edit":{
				"formula":{
					"enabled":true,
					"expression":"(input_image_tokens(input_base) * input_image_token_price + output_tokens(medium_output_base) * output_token_price + text_input_price) * currency_rate",
					"variables":{
						"input_base":48,
						"medium_output_base":48,
						"input_image_token_price":0.000008,
						"output_token_price":0.00003,
						"text_input_price":0.005,
						"currency_rate":1
					},
					"defaults":{"size":"1024x1024","quality":"medium"}
				}
			}
		}
	}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("group", "default")
	info := &relaycommon.RelayInfo{
		OriginModelName: "atlas-formula-image",
		RelayMode:       relayconstant.RelayModeImagesEdits,
		UserGroup:       "default",
		UsingGroup:      "default",
	}
	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{
		BillingDimensions: map[string]string{
			ratio_setting.ModelPriceVariantResolution: "1024x1024",
			ratio_setting.ModelPriceVariantQuality:    "medium",
		},
		BillingParams: map[string]float64{
			ratio_setting.ModelPriceExtraParamInputImages: 1,
		},
		BillingImages: []types.BillingImageMeta{{Width: 1024, Height: 1024}},
		CombineText:   "test prompt",
	})

	require.NoError(t, err)
	require.InDelta(t, 0.071728, priceData.ModelPrice, 1e-12)
	require.Equal(t, "formula", priceData.BillingMeta["route_price_status"])
	require.Equal(t, "0.071728", priceData.BillingMeta["formula_price"])
	require.Equal(t, "1756", priceData.BillingMeta["formula_calc_input_image_tokens"])
	require.Equal(t, "0.014048", priceData.BillingMeta["formula_calc_input_image_cost"])
	require.Equal(t, "1756", priceData.BillingMeta["formula_calc_output_tokens"])
	require.Equal(t, "0.05268", priceData.BillingMeta["formula_calc_output_cost"])
	require.Equal(t, "0.005", priceData.BillingMeta["formula_calc_text_input_cost"])
	require.Equal(t, "0.071728", priceData.BillingMeta["formula_calc_subtotal"])
	require.Equal(t, "0.071728", priceData.BillingMeta["formula_calc_converted_total"])
	require.Contains(t, priceData.BillingMeta["formula_detail"], "公式计费：按照上游公式计算")
	require.NotContains(t, priceData.BillingMeta["formula_detail"], "currency_rate")
	require.NotContains(t, priceData.BillingMeta["formula_detail"], "公式变量")
	require.NotContains(t, priceData.BillingMeta["formula_detail"], "公式分项")
}

func TestModelPriceHelperRouteFormulaOverridesSpecsAndExtraParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedModelPrices := ratio_setting.ModelPrice2JSONString()
	savedVariants := ratio_setting.ModelPriceVariants2JSONString()
	savedRouteVariants := ratio_setting.ModelRoutePriceVariants2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedModelPrices))
		require.NoError(t, ratio_setting.UpdateModelPriceVariantsByJSONString(savedVariants))
		require.NoError(t, ratio_setting.UpdateModelRoutePriceVariantsByJSONString(savedRouteVariants))
	})

	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"fixed-image-price":0.04}`))
	require.NoError(t, ratio_setting.UpdateModelPriceVariantsByJSONString(`{
		"fixed-image-price":{
			"resolution_enabled":true,
			"quality_enabled":true,
			"rules":[{"resolution":"1024x1024","quality":"medium","price":0.2}]
		}
	}`))
	require.NoError(t, ratio_setting.UpdateModelRoutePriceVariantsByJSONString(`{
		"fixed-image-price":{
			"image.edit":{
				"resolution_enabled":true,
				"quality_enabled":true,
				"rules":[{"resolution":"1024x1024","quality":"medium","price":0.32}],
				"extra_params":[{"key":"input_images","base":1,"unit_price":0.1}],
				"formula":{
					"enabled":true,
					"expression":"0.7",
					"defaults":{"size":"1024x1024","quality":"medium"}
				}
			}
		}
	}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("group", "default")
	info := &relaycommon.RelayInfo{
		OriginModelName: "fixed-image-price",
		RelayMode:       relayconstant.RelayModeImagesEdits,
		UserGroup:       "default",
		UsingGroup:      "default",
	}
	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{
		BillingDimensions: map[string]string{
			ratio_setting.ModelPriceVariantResolution: "1024x1024",
			ratio_setting.ModelPriceVariantQuality:    "medium",
		},
		BillingParams: map[string]float64{
			ratio_setting.ModelPriceExtraParamInputImages: 10,
		},
	})

	require.NoError(t, err)
	require.Equal(t, 0.7, priceData.ModelPrice)
	require.Equal(t, 350000, priceData.QuotaToPreConsume)
	require.Equal(t, "formula", priceData.BillingMeta["route_price_status"])
	require.Empty(t, priceData.BillingMeta["variant_price_status"])
	require.Empty(t, priceData.BillingMeta["extra_price"])
}

func TestModelPriceHelperFallsBackWhenImageEditRouteVariantMisses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedModelPrices := ratio_setting.ModelPrice2JSONString()
	savedVariants := ratio_setting.ModelPriceVariants2JSONString()
	savedRouteVariants := ratio_setting.ModelRoutePriceVariants2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedModelPrices))
		require.NoError(t, ratio_setting.UpdateModelPriceVariantsByJSONString(savedVariants))
		require.NoError(t, ratio_setting.UpdateModelRoutePriceVariantsByJSONString(savedRouteVariants))
	})

	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"fixed-image-price":0.04}`))
	require.NoError(t, ratio_setting.UpdateModelPriceVariantsByJSONString(`{
		"fixed-image-price":{
			"resolution_enabled":true,
			"quality_enabled":true,
			"rules":[{"resolution":"1024x1024","quality":"medium","price":0.2}]
		}
	}`))
	require.NoError(t, ratio_setting.UpdateModelRoutePriceVariantsByJSONString(`{
		"fixed-image-price":{
			"image.edit":{
				"resolution_enabled":true,
				"quality_enabled":true,
				"rules":[{"resolution":"1536x1024","quality":"medium","price":0.32}]
			}
		}
	}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("group", "default")
	info := &relaycommon.RelayInfo{
		OriginModelName: "fixed-image-price",
		RelayMode:       relayconstant.RelayModeImagesEdits,
		UserGroup:       "default",
		UsingGroup:      "default",
	}
	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{
		BillingDimensions: map[string]string{
			ratio_setting.ModelPriceVariantResolution: "1024x1024",
			ratio_setting.ModelPriceVariantQuality:    "medium",
		},
	})

	require.NoError(t, err)
	require.Equal(t, 0.2, priceData.ModelPrice)
	require.Equal(t, 100000, priceData.QuotaToPreConsume)
	require.Equal(t, "legacy", priceData.BillingMeta["route_price_status"])
	require.Equal(t, "matched", priceData.BillingMeta["variant_price_status"])
}

func TestModelPriceHelperPerCallCarriesConfiguredPriceUnit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedPrices := ratio_setting.ModelPrice2JSONString()
	savedUnits := ratio_setting.ModelPriceUnit2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices))
		require.NoError(t, ratio_setting.UpdateModelPriceUnitByJSONString(savedUnits))
	})

	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"video-unit-test":0.05}`))
	require.NoError(t, ratio_setting.UpdateModelPriceUnitByJSONString(`{"video-unit-test":"second"}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("group", "default")
	info := &relaycommon.RelayInfo{
		OriginModelName: "video-unit-test",
		UserGroup:       "default",
		UsingGroup:      "default",
	}
	priceData, err := ModelPriceHelperPerCall(ctx, info)

	require.NoError(t, err)
	require.Equal(t, types.ModelPriceUnitSecond, priceData.ModelPriceUnit)
}
