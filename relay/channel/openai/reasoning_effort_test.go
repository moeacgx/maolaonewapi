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

func TestConvertOpenAIRequestOnlyParsesSupportedReasoningModelSuffixes(t *testing.T) {
	tests := []struct {
		model      string
		wantModel  string
		wantEffort string
	}{
		{model: "o3-mini-high", wantModel: "o3-mini", wantEffort: "high"},
		{model: "o3-medium", wantModel: "o3", wantEffort: "medium"},
		{model: "o4-mini-low", wantModel: "o4-mini", wantEffort: "low"},
		{model: "gpt-5.1-minimal", wantModel: "gpt-5.1", wantEffort: "minimal"},
		{model: "gpt-5.2-none", wantModel: "gpt-5.2", wantEffort: "none"},
		{model: "gpt-5.6-sol-xhigh", wantModel: "gpt-5.6-sol", wantEffort: "xhigh"},
		{model: "gpt-5.1-codex-max", wantModel: "gpt-5.1-codex-max"},
		{model: "qwen3-max", wantModel: "qwen3-max"},
		{model: "gpt-5.6-sol-ultra", wantModel: "gpt-5.6-sol-ultra"},
		{model: "custom-vision-ultra", wantModel: "custom-vision-ultra"},
		{model: "o3custom-high", wantModel: "o3custom-high"},
		{model: "gpt-50-high", wantModel: "gpt-50-high"},
	}

	for _, test := range tests {
		t.Run(test.model, func(t *testing.T) {
			info := newOpenAIReasoningTestInfo(test.model)
			request := &dto.GeneralOpenAIRequest{Model: test.model}

			converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)

			require.NoError(t, err)
			convertedRequest := converted.(*dto.GeneralOpenAIRequest)
			require.Equal(t, test.wantModel, convertedRequest.Model)
			require.Equal(t, test.wantModel, info.UpstreamModelName)
			require.Equal(t, test.wantEffort, convertedRequest.ReasoningEffort)
			require.Equal(t, test.wantEffort, info.ReasoningEffort)
			wire, err := common.Marshal(convertedRequest)
			require.NoError(t, err)
			require.Equal(t, test.wantModel, gjson.GetBytes(wire, "model").String())
			if test.wantEffort == "" {
				require.False(t, gjson.GetBytes(wire, "reasoning_effort").Exists())
			} else {
				require.Equal(t, test.wantEffort, gjson.GetBytes(wire, "reasoning_effort").String())
			}
		})
	}
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

func TestConvertOpenAIResponsesRequestPreservesUltraModelID(t *testing.T) {
	info := &relaycommon.RelayInfo{}
	request := dto.OpenAIResponsesRequest{Model: "gpt-5.6-sol-ultra"}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, request)

	require.NoError(t, err)
	convertedRequest := converted.(dto.OpenAIResponsesRequest)
	require.Equal(t, request.Model, convertedRequest.Model)
	require.Nil(t, convertedRequest.Reasoning)
	require.Empty(t, info.ReasoningEffort)
	wire, err := common.Marshal(convertedRequest)
	require.NoError(t, err)
	require.Equal(t, request.Model, gjson.GetBytes(wire, "model").String())
	require.False(t, gjson.GetBytes(wire, "reasoning").Exists())
}

func TestConvertOpenAIResponsesRequestOnlyParsesSupportedReasoningModelSuffixes(t *testing.T) {
	tests := []struct {
		model      string
		wantModel  string
		wantEffort string
	}{
		{model: "o3-mini-high", wantModel: "o3-mini", wantEffort: "high"},
		{model: "o3-medium", wantModel: "o3", wantEffort: "medium"},
		{model: "o4-mini-low", wantModel: "o4-mini", wantEffort: "low"},
		{model: "gpt-5.1-minimal", wantModel: "gpt-5.1", wantEffort: "minimal"},
		{model: "gpt-5.2-none", wantModel: "gpt-5.2", wantEffort: "none"},
		{model: "gpt-5.6-sol-xhigh", wantModel: "gpt-5.6-sol", wantEffort: "xhigh"},
		{model: "o3-max", wantModel: "o3-max"},
		{model: "gpt-5.1-codex-max", wantModel: "gpt-5.1-codex-max"},
		{model: "qwen3-max", wantModel: "qwen3-max"},
		{model: "qwen-max", wantModel: "qwen-max"},
		{model: "qwen-vl-max", wantModel: "qwen-vl-max"},
		{model: "gpt-5.6-sol-ultra", wantModel: "gpt-5.6-sol-ultra"},
		{model: "custom-vision-ultra", wantModel: "custom-vision-ultra"},
		{model: "o3custom-high", wantModel: "o3custom-high"},
		{model: "gpt-50-high", wantModel: "gpt-50-high"},
	}

	for _, test := range tests {
		t.Run(test.model, func(t *testing.T) {
			info := &relaycommon.RelayInfo{}
			converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{Model: test.model})

			require.NoError(t, err)
			convertedRequest := converted.(dto.OpenAIResponsesRequest)
			require.Equal(t, test.wantModel, convertedRequest.Model)
			wire, err := common.Marshal(convertedRequest)
			require.NoError(t, err)
			require.Equal(t, test.wantModel, gjson.GetBytes(wire, "model").String())
			if test.wantEffort == "" {
				require.Nil(t, convertedRequest.Reasoning)
				require.Empty(t, info.ReasoningEffort)
				require.False(t, gjson.GetBytes(wire, "reasoning").Exists())
				return
			}
			require.NotNil(t, convertedRequest.Reasoning)
			require.Equal(t, test.wantEffort, convertedRequest.Reasoning.Effort)
			require.Equal(t, test.wantEffort, info.ReasoningEffort)
			require.Equal(t, test.wantEffort, gjson.GetBytes(wire, "reasoning.effort").String())
		})
	}
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
