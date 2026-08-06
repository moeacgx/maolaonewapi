package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorProbeSnapshotRejectsNewerChannelState(t *testing.T) {
	enabled := &model.Channel{
		Status: common.ChannelStatusEnabled,
		Key:    "single-key",
	}
	enabledSnapshot := newChannelMonitorProbeSnapshot(enabled)

	require.True(t, enabledSnapshot.matchesCurrentChannel(&model.Channel{
		Status: common.ChannelStatusEnabled,
		Key:    "single-key",
	}, "single-key", 0, false))
	require.False(t, enabledSnapshot.matchesCurrentChannel(&model.Channel{
		Status: common.ChannelStatusAutoDisabled,
		Key:    "single-key",
	}, "single-key", 0, false))
	require.False(t, enabledSnapshot.matchesCurrentChannel(&model.Channel{
		Status: common.ChannelStatusEnabled,
		Key:    "replacement-key",
	}, "single-key", 0, false))

	autoDisabled := &model.Channel{
		Status: common.ChannelStatusAutoDisabled,
		Key:    "single-key",
	}
	autoDisabledSnapshot := newChannelMonitorProbeSnapshot(autoDisabled)
	require.False(t, autoDisabledSnapshot.matchesCurrentChannel(&model.Channel{
		Status: common.ChannelStatusEnabled,
		Key:    "single-key",
	}, "single-key", 0, false))
}

func TestChannelMonitorProbeSnapshotChecksExactMultiKeyTarget(t *testing.T) {
	probed := &model.Channel{
		Status: common.ChannelStatusEnabled,
		Key:    "duplicate-key\nduplicate-key",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:         true,
			MultiKeyStatusList: map[int]int{},
		},
	}
	snapshot := newChannelMonitorProbeSnapshot(probed)

	require.True(t, snapshot.matchesCurrentChannel(&model.Channel{
		Status: common.ChannelStatusEnabled,
		Key:    "duplicate-key\nduplicate-key",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:         true,
			MultiKeyStatusList: map[int]int{},
		},
	}, "duplicate-key", 1, true))
	require.False(t, snapshot.matchesCurrentChannel(&model.Channel{
		Status: common.ChannelStatusEnabled,
		Key:    "duplicate-key\nduplicate-key",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey: true,
			MultiKeyStatusList: map[int]int{
				1: common.ChannelStatusManuallyDisabled,
			},
		},
	}, "duplicate-key", 1, true))
	require.False(t, snapshot.matchesCurrentChannel(probed, "duplicate-key", 1, false))
}
