package helper

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
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
	applyImageEditPriceVariantDefaults(nil, relayconstant.RelayModeImagesEdits, request)

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
	applyImageEditPriceVariantDefaults(nil, relayconstant.RelayModeImagesEdits, request)

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
	applyImageEditPriceVariantDefaults(nil, relayconstant.RelayModeImagesGenerations, request)

	if request.Size != "" || request.Quality != "" {
		t.Fatalf("generation defaults = %q/%q, want empty", request.Size, request.Quality)
	}
}

func TestImageEditPriceVariantDefaultsSyncMultipartForm(t *testing.T) {
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

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	requireNoError(t, writer.WriteField("model", "gpt-image-2"))
	requireNoError(t, writer.WriteField("prompt", "edit this"))
	requireNoError(t, writer.Close())
	req := httptest.NewRequest("POST", "/v1/images/edits", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	c.Request = req

	request, err := GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
	requireNoError(t, err)

	if request.Size != "1024x1024" || request.Quality != "medium" {
		t.Fatalf("request defaults = %q/%q, want 1024x1024/medium", request.Size, request.Quality)
	}
	if got := c.Request.MultipartForm.Value["size"]; len(got) != 1 || got[0] != "1024x1024" {
		t.Fatalf("multipart size = %#v, want 1024x1024", got)
	}
	if got := c.Request.MultipartForm.Value["quality"]; len(got) != 1 || got[0] != "medium" {
		t.Fatalf("multipart quality = %#v, want medium", got)
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
