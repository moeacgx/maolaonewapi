package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestEnableAutoDisabledSingleKeyChannelRefreshesSelectionCache(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))

	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	channelSyncLock.RLock()
	oldGroupCache, oldChannelCache := group2model2channels, channelsIDM
	channelSyncLock.RUnlock()
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		channelSyncLock.Lock()
		group2model2channels, channelsIDM = oldGroupCache, oldChannelCache
		channelSyncLock.Unlock()
		resetChannelConcurrencyForTest()
	})

	const (
		channelID = 991013
		modelName = "monitor-single-key-cache-model"
	)
	priority := int64(10)
	channel := &Channel{
		Id:       channelID,
		Name:     "monitor-single-key-cache-recovery",
		Status:   common.ChannelStatusAutoDisabled,
		Key:      "single-probed-key",
		Group:    "default",
		Models:   modelName,
		Priority: &priority,
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&Ability{
		Group:     "default",
		Model:     modelName,
		ChannelId: channelID,
		Enabled:   false,
		Priority:  &priority,
		Weight:    uint(channel.GetWeight()),
	}).Error)
	InitChannelCache()
	require.False(t, IsChannelEnabledForGroupModel("default", modelName, channelID))

	require.True(t, EnableAutoDisabledSingleKeyChannel(channelID, "single-probed-key"))
	require.True(t, IsChannelEnabledForGroupModel("default", modelName, channelID))

	var reloaded Channel
	require.NoError(t, db.First(&reloaded, channelID).Error)
	require.Equal(t, common.ChannelStatusEnabled, reloaded.Status)
	var ability Ability
	require.NoError(t, db.First(&ability, "channel_id = ?", channelID).Error)
	require.True(t, ability.Enabled)
}

func TestEnableAutoDisabledSingleKeyChannelRejectsChangedKey(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))

	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	const channelID = 991014
	channel := &Channel{
		Id:     channelID,
		Name:   "monitor-single-key-replaced",
		Status: common.ChannelStatusAutoDisabled,
		Key:    "replacement-key",
		Group:  "default",
		Models: "monitor-single-key-guard-model",
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&Ability{
		Group:     "default",
		Model:     "monitor-single-key-guard-model",
		ChannelId: channelID,
		Enabled:   false,
	}).Error)

	require.False(t, EnableAutoDisabledSingleKeyChannel(channelID, "single-probed-key"))

	var reloaded Channel
	require.NoError(t, db.First(&reloaded, channelID).Error)
	require.Equal(t, common.ChannelStatusAutoDisabled, reloaded.Status)
	var ability Ability
	require.NoError(t, db.First(&ability, "channel_id = ?", channelID).Error)
	require.False(t, ability.Enabled)
}
