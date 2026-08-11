package dto

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestImageRequestTokenMetaKeepsCountAsIndependentBillingRatio(t *testing.T) {
	n := uint(3)
	request := ImageRequest{
		Model:   "dall-e-3",
		Prompt:  "draw",
		N:       &n,
		Size:    "1024x1792",
		Quality: "hd",
	}

	meta := request.GetTokenCountMeta()

	require.Equal(t, float64(3), meta.ImagePriceRatio)
	require.Equal(t, float64(3), meta.BillingRatios["n"])
}

func TestImageRequestTokenMetaCountsImagesArrayForBillingParams(t *testing.T) {
	var request ImageRequest
	require.NoError(t, common.Unmarshal([]byte(`{
		"model":"gpt-image-2",
		"prompt":"edit",
		"images":["https://example.com/a.png","https://example.com/b.png"]
	}`), &request))

	meta := request.GetTokenCountMeta()

	require.Equal(t, float64(2), meta.BillingParams["input_images"])
}

func TestImageRequestTokenMetaCountsLegacyImageForBillingParams(t *testing.T) {
	request := ImageRequest{
		Model:  "gpt-image-2",
		Prompt: "edit",
		Image:  []byte(`"https://example.com/a.png"`),
	}

	meta := request.GetTokenCountMeta()

	require.Equal(t, float64(1), meta.BillingParams["input_images"])
}

func TestImageRequestTokenMetaCountsExtraImageURLsForBillingParams(t *testing.T) {
	var request ImageRequest
	require.NoError(t, common.Unmarshal([]byte(`{
		"model":"grok-imagine-image",
		"prompt":"edit",
		"extra_fields":{"image_urls":["https://example.com/a.png","https://example.com/b.png","https://example.com/c.png"]}
	}`), &request))

	meta := request.GetTokenCountMeta()

	require.Equal(t, float64(3), meta.BillingParams["input_images"])
}
