package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertOpenAIRequestDynamicRoutePreservesReasoningSuffixModel(t *testing.T) {
	info := newOpenAIReasoningTestInfo("gpt-5.6-sol-xhigh")
	info.IsDynamicModelRouted = true
	request := &dto.GeneralOpenAIRequest{Model: "gpt-5.6-sol-xhigh"}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)

	require.NoError(t, err)
	convertedRequest := converted.(*dto.GeneralOpenAIRequest)
	assert.Equal(t, "gpt-5.6-sol-xhigh", convertedRequest.Model)
	assert.Equal(t, "gpt-5.6-sol-xhigh", info.UpstreamModelName)
	assert.Empty(t, convertedRequest.ReasoningEffort)
}
