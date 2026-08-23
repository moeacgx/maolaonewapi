package deepseek

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
)

func TestConvertOpenAIRequestPreservesDynamicRoutingTargetModel(t *testing.T) {
	const targetModel = "deepseek-v4-pro-max"
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
	require.Equal(t, targetModel, convertedRequest.Model)
	require.Equal(t, targetModel, info.UpstreamModelName)
	require.Empty(t, convertedRequest.THINKING)
	require.Empty(t, convertedRequest.ReasoningEffort)
}
