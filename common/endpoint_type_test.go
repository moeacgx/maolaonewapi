package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

func TestGetEndpointTypesByChannelTypeDefaultOpenAIIncludesResponses(t *testing.T) {
	got := GetEndpointTypesByChannelType(constant.ChannelTypeOpenAI, "gpt-5.4")

	if len(got) != 2 {
		t.Fatalf("endpoint types len = %d, want 2: %v", len(got), got)
	}
	if got[0] != constant.EndpointTypeOpenAI {
		t.Fatalf("first endpoint type = %q, want %q", got[0], constant.EndpointTypeOpenAI)
	}
	if got[1] != constant.EndpointTypeOpenAIResponse {
		t.Fatalf("second endpoint type = %q, want %q", got[1], constant.EndpointTypeOpenAIResponse)
	}
}

func TestGetEndpointTypesByChannelTypeResponseOnlyModel(t *testing.T) {
	got := GetEndpointTypesByChannelType(constant.ChannelTypeOpenAI, "o3-pro")

	if len(got) != 1 {
		t.Fatalf("endpoint types len = %d, want 1: %v", len(got), got)
	}
	if got[0] != constant.EndpointTypeOpenAIResponse {
		t.Fatalf("endpoint type = %q, want %q", got[0], constant.EndpointTypeOpenAIResponse)
	}
}

func TestGetEndpointTypesByChannelTypeCodexUsesResponses(t *testing.T) {
	got := GetEndpointTypesByChannelType(constant.ChannelTypeCodex, "gpt-5.4")

	if len(got) != 2 {
		t.Fatalf("endpoint types len = %d, want 2: %v", len(got), got)
	}
	if got[0] != constant.EndpointTypeOpenAIResponse || got[1] != constant.EndpointTypeOpenAIAlphaSearch {
		t.Fatalf("endpoint types = %v, want Responses and Alpha Search", got)
	}
}

func TestGetEndpointTypesByChannelTypeCodexCompactUsesResponsesCompact(t *testing.T) {
	got := GetEndpointTypesByChannelType(constant.ChannelTypeCodex, "gpt-5.4-openai-compact")

	if len(got) != 2 {
		t.Fatalf("endpoint types len = %d, want 2: %v", len(got), got)
	}
	if got[0] != constant.EndpointTypeOpenAIResponseCompact || got[1] != constant.EndpointTypeOpenAIAlphaSearch {
		t.Fatalf("endpoint types = %v, want Responses Compact and Alpha Search", got)
	}
}

func TestGetEndpointTypesByChannelTypeXAIVideoIncludesVideoEndpoint(t *testing.T) {
	got := GetEndpointTypesByChannelType(constant.ChannelTypeXai, "grok-imagine-video-1.5")

	if len(got) != 3 {
		t.Fatalf("endpoint types len = %d, want 3: %v", len(got), got)
	}
	if got[0] != constant.EndpointTypeOpenAIVideo {
		t.Fatalf("first endpoint type = %q, want %q", got[0], constant.EndpointTypeOpenAIVideo)
	}
}

func TestGetEndpointTypesByChannelTypeXAITextRemainsTextOnly(t *testing.T) {
	got := GetEndpointTypesByChannelType(constant.ChannelTypeXai, "grok-4-1-fast-reasoning")

	if len(got) != 2 {
		t.Fatalf("endpoint types len = %d, want 2: %v", len(got), got)
	}
	if got[0] != constant.EndpointTypeOpenAI || got[1] != constant.EndpointTypeOpenAIResponse {
		t.Fatalf("endpoint types = %v, want OpenAI and Responses", got)
	}
}

func TestGetEndpointTypesByChannelTypeAtlasCloudImageOnly(t *testing.T) {
	for _, modelName := range []string{"seedream-3.0", "grok-imagine-image", "gpt-image-1.5", "gpt-image-2"} {
		t.Run(modelName, func(t *testing.T) {
			got := GetEndpointTypesByChannelType(constant.ChannelTypeAtlasCloud, modelName)

			if len(got) != 1 {
				t.Fatalf("endpoint types len = %d, want 1: %v", len(got), got)
			}
			if got[0] != constant.EndpointTypeImageGeneration {
				t.Fatalf("endpoint type = %q, want %q", got[0], constant.EndpointTypeImageGeneration)
			}
		})
	}
}

func TestGetEndpointTypesByChannelTypeAtlasCloudVideoOnly(t *testing.T) {
	got := GetEndpointTypesByChannelType(constant.ChannelTypeAtlasCloud, "kling-v2.0")

	if len(got) != 1 {
		t.Fatalf("endpoint types len = %d, want 1: %v", len(got), got)
	}
	if got[0] != constant.EndpointTypeOpenAIVideo {
		t.Fatalf("endpoint type = %q, want %q", got[0], constant.EndpointTypeOpenAIVideo)
	}
}
