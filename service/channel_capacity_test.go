package service

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func TestOfficialCapacityLimitNeverAutoDisablesChannel(t *testing.T) {
	originalEnabled := common.AutomaticDisableChannelEnabled
	originalRanges := append([]operation_setting.StatusCodeRange(nil), operation_setting.AutomaticDisableStatusCodeRanges...)
	common.AutomaticDisableChannelEnabled = true
	operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: http.StatusTooManyRequests, End: http.StatusTooManyRequests}}
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = originalEnabled
		operation_setting.AutomaticDisableStatusCodeRanges = originalRanges
	})

	capacityErr := types.WithOpenAIError(types.OpenAIError{
		Type:    "server_error",
		Code:    "server_error",
		Message: "Selected model is at capacity. Please try a different model.",
	}, http.StatusOK)

	require.False(t, ShouldDisableChannel(capacityErr))

	channelShapedCapacityErr := types.WithOpenAIError(types.OpenAIError{
		Type:    "server_error",
		Code:    "channel:model_at_capacity",
		Message: "Selected model is at capacity. Please try a different model.",
	}, http.StatusOK)
	require.True(t, types.IsChannelError(channelShapedCapacityErr))
	require.True(t, types.IsUpstreamCapacityError(channelShapedCapacityErr))
	require.False(t, ShouldDisableChannel(channelShapedCapacityErr))
}
