package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/constant"
)

func TestNormalizeChannelTestRequestForMappedCompactAlias(t *testing.T) {
	compact := &dto.OpenAIResponsesCompactionRequest{
		Model:              "gpt-5.5-openai-compact",
		Input:              []byte(`"hello"`),
		Instructions:       []byte(`"system"`),
		PreviousResponseID: "resp_previous",
		Tools:              []byte(`[{"type":"function"}]`),
	}

	normalized := normalizeChannelTestRequestForRelayMode(compact, constant.RelayModeResponses)
	responses, ok := normalized.(*dto.OpenAIResponsesRequest)
	if !ok {
		t.Fatalf("请求类型 = %T，期望 *dto.OpenAIResponsesRequest", normalized)
	}
	if string(responses.Input) != string(compact.Input) || responses.PreviousResponseID != compact.PreviousResponseID {
		t.Fatalf("转换丢失 Responses 字段: %#v", responses)
	}
}

func TestNormalizeChannelTestRequestKeepsCompactRequestForCompactRelay(t *testing.T) {
	compact := &dto.OpenAIResponsesCompactionRequest{Model: "gpt-5.5-openai-compact"}
	if got := normalizeChannelTestRequestForRelayMode(compact, constant.RelayModeResponsesCompact); got != compact {
		t.Fatalf("Compact RelayMode 不应转换请求类型")
	}
}
