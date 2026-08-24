package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageRequestFromResponsesFunctionArgumentsUsesImagesPayloadForBilling(t *testing.T) {
	request, err := imageRequestFromResponsesFunctionArguments(
		[]byte(`{"prompt":"draw a cat","size":"1024x1024","quality":"high","output_format":"png"}`),
		"gpt-image-2",
	)
	require.NoError(t, err)
	require.NotNil(t, request)
	assert.Equal(t, "gpt-image-2", request.Model)
	assert.Equal(t, "draw a cat", request.Prompt)
	assert.Equal(t, "1024x1024", request.Size)
	assert.Equal(t, "high", request.Quality)

	billingInput, err := helper.BuildBillingExprRequestInputFromRequest(
		request,
		map[string]string{"Content-Type": "application/json"},
	)
	require.NoError(t, err)

	var body map[string]any
	require.NoError(t, common.Unmarshal(billingInput.Body, &body))
	assert.Equal(t, "gpt-image-2", body["model"])
	assert.Equal(t, "1024x1024", body["size"])
	assert.Equal(t, "high", body["quality"])
	assert.Equal(t, "png", body["output_format"])
}
