package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestNormalizeChannelTestEndpointDetectsImageModels(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeXai}

	for _, modelName := range []string{
		"grok-imagine-image",
		"grok-imagine-image-pro",
		"grok-2-image-1212",
		"gpt-image-1",
		"gpt-image-2",
	} {
		t.Run(modelName, func(t *testing.T) {
			endpointType := normalizeChannelTestEndpoint(channel, modelName, "")
			require.Equal(t, string(constant.EndpointTypeImageGeneration), endpointType)

			request, ok := buildTestRequest(modelName, endpointType, channel, false).(*dto.ImageRequest)
			require.True(t, ok)
			require.Equal(t, modelName, request.Model)
			require.NotEmpty(t, request.Prompt)
		})
	}
}

func TestNormalizeChannelTestEndpointKeepsExplicitOverride(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeXai}

	endpointType := normalizeChannelTestEndpoint(channel, "grok-imagine-image", string(constant.EndpointTypeOpenAI))

	require.Equal(t, string(constant.EndpointTypeOpenAI), endpointType)
}

func TestNormalizeChannelTestEndpointUsesMappedTargetForCompactAlias(t *testing.T) {
	mapping := `{"gpt-5.5-openai-compact":"gpt-5.5"}`
	channel := &model.Channel{
		Type:         constant.ChannelTypeCodex,
		ModelMapping: &mapping,
	}

	endpointType := normalizeChannelTestEndpoint(channel, "gpt-5.5-openai-compact", "")

	require.Equal(t, string(constant.EndpointTypeOpenAIResponse), endpointType)

	request, ok := buildTestRequest("gpt-5.5-openai-compact", endpointType, channel, false).(*dto.OpenAIResponsesRequest)
	require.True(t, ok)
	require.Equal(t, "gpt-5.5-openai-compact", request.Model)
}

func TestNormalizeChannelTestEndpointKeepsUnmappedCompactSuffixOnCompactEndpoint(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeCodex}

	endpointType := normalizeChannelTestEndpoint(channel, "gpt-5.5-openai-compact", "")

	require.Equal(t, string(constant.EndpointTypeOpenAIResponseCompact), endpointType)
}

func TestNormalizeChannelTestEndpointKeepsChatModelsOnDefaultPath(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeXai}

	endpointType := normalizeChannelTestEndpoint(channel, "grok-4-fast-reasoning", "")

	require.Empty(t, endpointType)
}
