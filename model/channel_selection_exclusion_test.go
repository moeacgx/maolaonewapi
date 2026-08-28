package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestGetChannelWithSelectionExclusionsKeepsUntriedHighPriorityCandidate(t *testing.T) {
	const (
		groupName = "selection-exclusion-group"
		modelName = "selection-exclusion-model"
	)
	require.NoError(t, DB.Where("channel_id IN ?", []int{9901, 9902, 9903}).Delete(&Ability{}).Error)
	require.NoError(t, DB.Where("id IN ?", []int{9901, 9902, 9903}).Delete(&Channel{}).Error)
	t.Cleanup(func() {
		_ = DB.Where("channel_id IN ?", []int{9901, 9902, 9903}).Delete(&Ability{}).Error
		_ = DB.Where("id IN ?", []int{9901, 9902, 9903}).Delete(&Channel{}).Error
	})

	for _, candidate := range []struct {
		id       int
		priority int64
	}{
		{id: 9901, priority: 100},
		{id: 9902, priority: 100},
		{id: 9903, priority: 50},
	} {
		require.NoError(t, DB.Create(&Channel{
			Id: candidate.id, Type: constant.ChannelTypeOpenAI, Key: fmt.Sprintf("key-%d", candidate.id),
			Status: common.ChannelStatusEnabled, Name: fmt.Sprintf("channel-%d", candidate.id),
		}).Error)
		require.NoError(t, DB.Create(&Ability{
			Group: groupName, Model: modelName, ChannelId: candidate.id, Enabled: true,
			Priority: &candidate.priority, Weight: 100,
		}).Error)
	}

	channel, err := GetChannelWithSelectionExclusions(groupName, modelName, 0, "", ChannelSelectionExclusions{
		ChannelIDs: map[int]struct{}{9901: {}},
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 9902, channel.Id)

	channel, err = GetChannelWithSelectionExclusions(groupName, modelName, 0, "", ChannelSelectionExclusions{
		ChannelIDs: map[int]struct{}{9901: {}, 9902: {}},
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 9903, channel.Id)

	channel, err = GetChannelWithSelectionExclusions(groupName, modelName, 0, "", ChannelSelectionExclusions{
		ChannelIDs: map[int]struct{}{9901: {}, 9902: {}, 9903: {}},
	})
	require.NoError(t, err)
	require.Nil(t, channel)
}
