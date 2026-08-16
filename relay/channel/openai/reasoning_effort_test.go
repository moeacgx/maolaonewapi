package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestConvertOpenAIRequestNormalizesReasoningEffort(t *testing.T) {
	info := newOpenAIReasoningTestInfo("gpt-5.5")
	request := &dto.GeneralOpenAIRequest{
		Model:           "gpt-5.5",
		ReasoningEffort: "extra high",
	}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)

	require.NoError(t, err)
	convertedRequest := converted.(*dto.GeneralOpenAIRequest)
	require.Equal(t, "xhigh", convertedRequest.ReasoningEffort)
	require.Equal(t, "xhigh", info.ReasoningEffort)
}

func TestConvertOpenAIRequestNormalizesRawReasoningEffort(t *testing.T) {
	info := newOpenAIReasoningTestInfo("gpt-5.5")
	request := &dto.GeneralOpenAIRequest{
		Model:     "gpt-5.5",
		Reasoning: mustMarshalOpenAIReasoningRaw(t, map[string]any{"effort": "extra_high", "summary": "auto"}),
	}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)

	require.NoError(t, err)
	convertedRequest := converted.(*dto.GeneralOpenAIRequest)
	require.Equal(t, "xhigh", gjson.GetBytes(convertedRequest.Reasoning, "effort").String())
	require.Equal(t, "auto", gjson.GetBytes(convertedRequest.Reasoning, "summary").String())
	require.Equal(t, "xhigh", info.ReasoningEffort)
}

func TestConvertOpenAIResponsesRequestNormalizesReasoningEffort(t *testing.T) {
	info := &relaycommon.RelayInfo{}
	request := dto.OpenAIResponsesRequest{
		Model:     "gpt-5.5",
		Reasoning: &dto.Reasoning{Effort: "extra-high"},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, request)

	require.NoError(t, err)
	convertedRequest := converted.(dto.OpenAIResponsesRequest)
	require.NotNil(t, convertedRequest.Reasoning)
	require.Equal(t, "xhigh", convertedRequest.Reasoning.Effort)
	require.Equal(t, "xhigh", info.ReasoningEffort)
}

func TestConvertOpenAIResponsesRequestPreservesMaxReasoningEffort(t *testing.T) {
	info := &relaycommon.RelayInfo{}
	request := dto.OpenAIResponsesRequest{
		Model:     "gpt-5.6-sol",
		Reasoning: &dto.Reasoning{Effort: "maximum"},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, request)

	require.NoError(t, err)
	convertedRequest := converted.(dto.OpenAIResponsesRequest)
	require.NotNil(t, convertedRequest.Reasoning)
	require.Equal(t, "max", convertedRequest.Reasoning.Effort)
	require.Equal(t, "max", info.ReasoningEffort)
}

func TestConvertOpenAIResponsesRequestParsesUltraReasoningSuffix(t *testing.T) {
	info := &relaycommon.RelayInfo{}
	request := dto.OpenAIResponsesRequest{Model: "gpt-5.6-sol-ultra"}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, request)

	require.NoError(t, err)
	convertedRequest := converted.(dto.OpenAIResponsesRequest)
	require.Equal(t, "gpt-5.6-sol", convertedRequest.Model)
	require.NotNil(t, convertedRequest.Reasoning)
	require.Equal(t, "ultra", convertedRequest.Reasoning.Effort)
	require.Equal(t, "ultra", info.ReasoningEffort)
}

func newOpenAIReasoningTestInfo(model string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			UpstreamModelName: model,
		},
	}
}

func mustMarshalOpenAIReasoningRaw(t *testing.T, value any) []byte {
	t.Helper()
	data, err := common.Marshal(value)
	require.NoError(t, err)
	return data
}
