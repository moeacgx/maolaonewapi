package helper

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

func TestImageEditPriceVariantDefaultsFillMissingDimensions(t *testing.T) {
	savedRouteVariants := ratio_setting.ModelRoutePriceVariants2JSONString()
	t.Cleanup(func() { _ = ratio_setting.UpdateModelRoutePriceVariantsByJSONString(savedRouteVariants) })

	requireNoError(t, ratio_setting.UpdateModelRoutePriceVariantsByJSONString(`{
		"gpt-image-2":{
			"image.edit":{
				"resolution_enabled":true,
				"quality_enabled":true,
				"rules":[{"resolution":"1024x1024","quality":"medium","price":0.072}]
			}
		}
	}`))

	request := &dto.ImageRequest{Model: "gpt-image-2"}
	applyImageEditPriceVariantDefaults(relayconstant.RelayModeImagesEdits, request)

	if request.Size != "1024x1024" {
		t.Fatalf("Size = %q, want 1024x1024", request.Size)
	}
	if request.Quality != "medium" {
		t.Fatalf("Quality = %q, want medium", request.Quality)
	}
}

func TestImageEditPriceVariantDefaultsPreserveExplicitDimensions(t *testing.T) {
	savedRouteVariants := ratio_setting.ModelRoutePriceVariants2JSONString()
	t.Cleanup(func() { _ = ratio_setting.UpdateModelRoutePriceVariantsByJSONString(savedRouteVariants) })

	requireNoError(t, ratio_setting.UpdateModelRoutePriceVariantsByJSONString(`{
		"gpt-image-2":{
			"image.edit":{
				"resolution_enabled":true,
				"quality_enabled":true,
				"rules":[{"resolution":"1024x1024","quality":"medium","price":0.072}]
			}
		}
	}`))

	request := &dto.ImageRequest{
		Model:   "gpt-image-2",
		Size:    "1536x1024",
		Quality: "high",
	}
	applyImageEditPriceVariantDefaults(relayconstant.RelayModeImagesEdits, request)

	if request.Size != "1536x1024" {
		t.Fatalf("Size = %q, want 1536x1024", request.Size)
	}
	if request.Quality != "high" {
		t.Fatalf("Quality = %q, want high", request.Quality)
	}
}

func TestImageEditPriceVariantDefaultsSkipGenerationRoute(t *testing.T) {
	savedRouteVariants := ratio_setting.ModelRoutePriceVariants2JSONString()
	t.Cleanup(func() { _ = ratio_setting.UpdateModelRoutePriceVariantsByJSONString(savedRouteVariants) })

	requireNoError(t, ratio_setting.UpdateModelRoutePriceVariantsByJSONString(`{
		"gpt-image-2":{
			"image.edit":{
				"resolution_enabled":true,
				"quality_enabled":true,
				"rules":[{"resolution":"1024x1024","quality":"medium","price":0.072}]
			}
		}
	}`))

	request := &dto.ImageRequest{Model: "gpt-image-2"}
	applyImageEditPriceVariantDefaults(relayconstant.RelayModeImagesGenerations, request)

	if request.Size != "" || request.Quality != "" {
		t.Fatalf("generation defaults = %q/%q, want empty", request.Size, request.Quality)
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
