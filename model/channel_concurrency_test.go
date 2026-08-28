package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelJSONIncludesConcurrencyLimit(t *testing.T) {
	limit := 12
	data, err := common.Marshal(&Channel{Id: 7, ConcurrencyLimit: &limit})
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(data, &payload))
	assert.Equal(t, float64(12), payload["concurrency_limit"])
}

func TestChannelConcurrencyZeroAndNilAreUnlimited(t *testing.T) {
	resetChannelConcurrencyForTest()
	zero := 0
	for _, channel := range []*Channel{
		{Id: 1, ConcurrencyLimit: &zero},
		{Id: 2},
	} {
		require.True(t, TryAcquireChannelConcurrency(channel))
		require.True(t, TryAcquireChannelConcurrency(channel))
	}
}

func TestCachedSelectorSkipsFullHigherPriorityChannel(t *testing.T) {
	resetChannelConcurrencyForTest()
	oldMemory := common.MemoryCacheEnabled
	oldGroups, oldChannels := group2model2channels, channelsIDM
	t.Cleanup(func() {
		common.MemoryCacheEnabled = oldMemory
		group2model2channels, channelsIDM = oldGroups, oldChannels
		resetChannelConcurrencyForTest()
	})

	common.MemoryCacheEnabled = true
	limit := 1
	high := &Channel{Id: 10, Priority: common.GetPointer[int64](10), Weight: common.GetPointer[uint](0), ConcurrencyLimit: &limit}
	low := &Channel{Id: 11, Priority: common.GetPointer[int64](1), Weight: common.GetPointer[uint](0), ConcurrencyLimit: &limit}
	channelsIDM = map[int]*Channel{10: high, 11: low}
	group2model2channels = map[string]map[string][]int{"default": {"gpt-test": {10, 11}}}
	require.True(t, TryAcquireChannelConcurrency(high))

	selected, err := GetRandomSatisfiedChannelWithSelectionExclusions("default", "gpt-test", 0, "", ChannelSelectionExclusions{})
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, 11, selected.Id)
}

func TestCachedSelectorReturnsDistinctErrorWhenAllChannelsFull(t *testing.T) {
	resetChannelConcurrencyForTest()
	oldMemory := common.MemoryCacheEnabled
	oldGroups, oldChannels := group2model2channels, channelsIDM
	t.Cleanup(func() {
		common.MemoryCacheEnabled = oldMemory
		group2model2channels, channelsIDM = oldGroups, oldChannels
		resetChannelConcurrencyForTest()
	})

	common.MemoryCacheEnabled = true
	limit := 1
	channelsIDM = map[int]*Channel{
		20: {Id: 20, Priority: common.GetPointer[int64](1), Weight: common.GetPointer[uint](0), ConcurrencyLimit: &limit},
		21: {Id: 21, Priority: common.GetPointer[int64](0), Weight: common.GetPointer[uint](0), ConcurrencyLimit: &limit},
	}
	group2model2channels = map[string]map[string][]int{"default": {"gpt-test": {20, 21}}}
	require.True(t, TryAcquireChannelConcurrency(channelsIDM[20]))
	require.True(t, TryAcquireChannelConcurrency(channelsIDM[21]))

	selected, err := GetRandomSatisfiedChannel("default", "gpt-test", 0, "")
	require.Nil(t, selected)
	assert.ErrorIs(t, err, ErrChannelConcurrencyLimitReached)
}

func TestCachedSelectorKeepsNilWhenAllCandidatesAreExcluded(t *testing.T) {
	resetChannelConcurrencyForTest()
	oldMemory := common.MemoryCacheEnabled
	oldGroups, oldChannels := group2model2channels, channelsIDM
	t.Cleanup(func() {
		common.MemoryCacheEnabled = oldMemory
		group2model2channels, channelsIDM = oldGroups, oldChannels
		resetChannelConcurrencyForTest()
	})

	common.MemoryCacheEnabled = true
	limit := 1
	channelsIDM = map[int]*Channel{30: {Id: 30, Priority: common.GetPointer[int64](1), ConcurrencyLimit: &limit}}
	group2model2channels = map[string]map[string][]int{"default": {"gpt-test": {30}}}

	selected, err := GetRandomSatisfiedChannelWithSelectionExclusions(
		"default", "gpt-test", 0, "", ChannelSelectionExclusions{ChannelIDs: map[int]struct{}{30: {}}},
	)
	assert.Nil(t, selected)
	assert.NoError(t, err)
}
