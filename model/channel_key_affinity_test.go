package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestChannelGetKeyByIndexValidatesBoundsAndStatus(t *testing.T) {
	channel := &Channel{
		Key: "first\nsecond",
		ChannelInfo: ChannelInfo{
			IsMultiKey:         true,
			MultiKeyStatusList: map[int]int{1: common.ChannelStatusManuallyDisabled},
		},
	}
	key, index, apiErr := channel.GetKeyByIndex(0)
	require.Nil(t, apiErr)
	require.Equal(t, "first", key)
	require.Equal(t, 0, index)
	_, _, apiErr = channel.GetKeyByIndex(1)
	require.NotNil(t, apiErr)
	_, _, apiErr = channel.GetKeyByIndex(2)
	require.NotNil(t, apiErr)

	single := &Channel{Key: "single", ChannelInfo: ChannelInfo{IsMultiKey: false, MultiKeyMode: constant.MultiKeyModeRandom}}
	key, index, apiErr = single.GetKeyByIndex(99)
	require.Nil(t, apiErr)
	require.Equal(t, "single", key)
	require.Equal(t, 0, index)
}
