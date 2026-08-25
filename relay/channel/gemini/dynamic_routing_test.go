package gemini

import (
	"encoding/json"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDynamicRouteToGeminiThinkingSuffixUsesOfficialBaseModelURL(t *testing.T) {
	settings := model_setting.GetGeminiSettings()
	originalEnabled := settings.ThinkingAdapterEnabled
	settings.ThinkingAdapterEnabled = false
	t.Cleanup(func() {
		settings.ThinkingAdapterEnabled = originalEnabled
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3.7-flash",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:       "https://generativelanguage.googleapis.com",
			ApiVersion:           "v1beta",
			ApiKey:               "test-key",
			UpstreamModelName:    "gemini-3.7-flash-high",
			IsDynamicModelRouted: true,
		},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{
		Model:     "gemini-3.7-flash-high",
		Input:     json.RawMessage(`"hello"`),
		Reasoning: &dto.Reasoning{Effort: "high"},
	})
	require.NoError(t, err)
	geminiRequest, ok := converted.(*dto.GeminiChatRequest)
	require.True(t, ok)
	require.NotNil(t, geminiRequest.GenerationConfig.ThinkingConfig)
	assert.Equal(t, "high", geminiRequest.GenerationConfig.ThinkingConfig.ThinkingLevel)

	url, err := (&Adaptor{}).GetRequestURL(info)

	require.NoError(t, err)
	assert.Contains(t, url, "/models/gemini-3.7-flash:")
	assert.NotContains(t, url, "gemini-3.7-flash-high")
	assert.Equal(t, "gemini-3.7-flash", info.UpstreamModelName)
}

func TestStaticMappingToGeminiThinkingSuffixUsesOfficialBaseModelURL(t *testing.T) {
	settings := model_setting.GetGeminiSettings()
	originalEnabled := settings.ThinkingAdapterEnabled
	settings.ThinkingAdapterEnabled = false
	t.Cleanup(func() {
		settings.ThinkingAdapterEnabled = originalEnabled
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3.7-flash",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://generativelanguage.googleapis.com",
			ApiVersion:        "v1beta",
			ApiKey:            "test-key",
			UpstreamModelName: "gemini-3.7-flash-medium",
			IsModelMapped:     true,
		},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{
		Model: "gemini-3.7-flash-medium",
		Input: json.RawMessage(`"hello"`),
	})
	require.NoError(t, err)
	geminiRequest, ok := converted.(*dto.GeminiChatRequest)
	require.True(t, ok)
	require.NotNil(t, geminiRequest.GenerationConfig.ThinkingConfig)
	assert.Equal(t, "medium", geminiRequest.GenerationConfig.ThinkingConfig.ThinkingLevel)

	url, err := (&Adaptor{}).GetRequestURL(info)

	require.NoError(t, err)
	assert.Contains(t, url, "/models/gemini-3.7-flash:")
	assert.NotContains(t, url, "gemini-3.7-flash-medium")
	assert.Equal(t, "gemini-3.7-flash", info.UpstreamModelName)
}
