package xai

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertOpenAIRequestDynamicRoutePreservesReasoningSuffixModel(t *testing.T) {
	const targetModel = "grok-3-mini-high"
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName:    targetModel,
			IsDynamicModelRouted: true,
		},
	}
	request := &dto.GeneralOpenAIRequest{Model: targetModel}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)

	require.NoError(t, err)
	convertedRequest := converted.(*dto.GeneralOpenAIRequest)
	assert.Equal(t, targetModel, convertedRequest.Model)
	assert.Equal(t, targetModel, info.UpstreamModelName)
	assert.Empty(t, convertedRequest.ReasoningEffort)
}
