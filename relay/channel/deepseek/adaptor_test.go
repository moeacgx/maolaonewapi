package deepseek

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/stretchr/testify/require"
)

func TestGetRequestURLResponses(t *testing.T) {
	adaptor := &Adaptor{}
	url, err := adaptor.GetRequestURL(&relaycommon.RelayInfo{
		RelayMode: constant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://api.deepseek.com",
		},
	})

	require.NoError(t, err)
	require.Equal(t, "https://api.deepseek.com/responses", url)
}

func TestConvertOpenAIResponsesRequestDeepSeekV4Suffix(t *testing.T) {
	tests := []struct {
		name       string
		model      string
		wantModel  string
		wantEffort string
	}{
		{name: "maximum reasoning", model: "deepseek-v4-preview-max", wantModel: "deepseek-v4-preview", wantEffort: "max"},
		{name: "reasoning disabled", model: "deepseek-v4-preview-none", wantModel: "deepseek-v4-preview", wantEffort: "none"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: tt.model}}
			converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{Model: tt.model})

			require.NoError(t, err)
			request, ok := converted.(dto.OpenAIResponsesRequest)
			require.True(t, ok)
			require.Equal(t, tt.wantModel, request.Model)
			require.NotNil(t, request.Reasoning)
			require.Equal(t, tt.wantEffort, request.Reasoning.Effort)
			require.Equal(t, tt.wantModel, info.UpstreamModelName)
			require.Equal(t, tt.wantEffort, info.ReasoningEffort)
		})
	}
}

func TestConvertOpenAIResponsesRequestPreservesExplicitReasoning(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "deepseek-chat"}}
	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{
		Model:     "deepseek-chat",
		Reasoning: &dto.Reasoning{Effort: "high"},
	})

	require.NoError(t, err)
	request := converted.(dto.OpenAIResponsesRequest)
	require.Equal(t, "deepseek-chat", request.Model)
	require.Equal(t, "high", request.Reasoning.Effort)
	require.Equal(t, "high", info.ReasoningEffort)
}
