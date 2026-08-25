package controller

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInjectResponsesImageFunctionToolUsesStrictCompatibleSchema(t *testing.T) {
	request := &dto.OpenAIResponsesRequest{}
	injected, err := injectResponsesImageFunctionTool(request)
	require.NoError(t, err)
	require.True(t, injected)

	var tools []struct {
		Name       string `json:"name"`
		Strict     bool   `json:"strict"`
		Parameters struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		} `json:"parameters"`
	}
	require.NoError(t, common.Unmarshal(request.Tools, &tools))
	require.Len(t, tools, 1)
	assert.Equal(t, responsesImageFunctionName, tools[0].Name)
	assert.True(t, tools[0].Strict)
	assert.ElementsMatch(t, []string{"prompt", "size", "quality", "output_format"}, tools[0].Parameters.Required)

	for _, name := range []string{"size", "quality", "output_format"} {
		var property struct {
			Type []string `json:"type"`
		}
		require.NoError(t, common.Unmarshal(tools[0].Parameters.Properties[name], &property))
		assert.Equal(t, []string{"string", "null"}, property.Type, name)
	}
}

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

func TestImageRequestFromResponsesFunctionArgumentsAcceptsNullOptionalArguments(t *testing.T) {
	request, err := imageRequestFromResponsesFunctionArguments(
		[]byte(`{"prompt":"draw a cat","size":null,"quality":null,"output_format":null}`),
		"gpt-image-2",
	)
	require.NoError(t, err)
	require.NotNil(t, request)
	assert.Equal(t, "draw a cat", request.Prompt)
	assert.Empty(t, request.Size)
	assert.Empty(t, request.Quality)
	assert.Empty(t, request.OutputFormat)
}
