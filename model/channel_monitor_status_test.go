package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestHandlerMultiKeyUpdateRestoresOnlyProbedKey(t *testing.T) {
	channel := &Channel{
		Id:     991004,
		Status: common.ChannelStatusAutoDisabled,
		Key:    "auto-disabled-key-0\nauto-disabled-key-1",
		ChannelInfo: ChannelInfo{
			IsMultiKey: true,
			MultiKeyStatusList: map[int]int{
				0: common.ChannelStatusAutoDisabled,
				1: common.ChannelStatusAutoDisabled,
			},
		},
	}

	handlerMultiKeyUpdate(channel, "auto-disabled-key-0", common.ChannelStatusEnabled, "")

	if channel.Status != common.ChannelStatusEnabled {
		t.Fatalf("expected channel to become enabled after restoring one key, got status %d", channel.Status)
	}
	if _, exists := channel.ChannelInfo.MultiKeyStatusList[0]; exists {
		t.Fatal("expected the probed key to be removed from the disabled status list")
	}
	if status := channel.ChannelInfo.MultiKeyStatusList[1]; status != common.ChannelStatusAutoDisabled {
		t.Fatalf("expected the unprobed key to remain auto-disabled, got status %d", status)
	}
}

func TestHandlerMultiKeyUpdateAtIndexRestoresExactDuplicateKey(t *testing.T) {
	channel := &Channel{
		Id:     991005,
		Status: common.ChannelStatusAutoDisabled,
		Key:    "duplicate-key\nduplicate-key",
		ChannelInfo: ChannelInfo{
			IsMultiKey: true,
			MultiKeyStatusList: map[int]int{
				0: common.ChannelStatusAutoDisabled,
				1: common.ChannelStatusAutoDisabled,
			},
		},
	}

	updated := handlerMultiKeyUpdateAtIndex(channel, 1, common.ChannelStatusEnabled, "")

	if !updated {
		t.Fatal("expected the probed key index to be restored")
	}
	if status := channel.ChannelInfo.MultiKeyStatusList[0]; status != common.ChannelStatusAutoDisabled {
		t.Fatalf("expected duplicate key at index 0 to remain auto-disabled, got status %d", status)
	}
	if _, exists := channel.ChannelInfo.MultiKeyStatusList[1]; exists {
		t.Fatal("expected the actually probed duplicate key at index 1 to be restored")
	}
}

func TestEnableAutoDisabledChannelKeyRestoresExpectedDuplicateIndex(t *testing.T) {
	const channelID = 991006
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	require.NoError(t, DB.Unscoped().Delete(&Channel{}, channelID).Error)
	t.Cleanup(func() {
		_ = DB.Unscoped().Delete(&Channel{}, channelID).Error
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		if previousMemoryCacheEnabled {
			InitChannelCache()
		}
	})

	channel := &Channel{
		Id:     channelID,
		Name:   "monitor-duplicate-recovery",
		Status: common.ChannelStatusAutoDisabled,
		Key:    "duplicate-key\nduplicate-key",
		ChannelInfo: ChannelInfo{
			IsMultiKey: true,
			MultiKeyStatusList: map[int]int{
				0: common.ChannelStatusAutoDisabled,
				1: common.ChannelStatusAutoDisabled,
			},
		},
	}
	require.NoError(t, DB.Create(channel).Error)

	require.True(t, EnableAutoDisabledChannelKey(channelID, 1, "duplicate-key"))

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channelID).Error)
	require.Equal(t, common.ChannelStatusEnabled, reloaded.Status)
	require.Equal(t, common.ChannelStatusAutoDisabled, reloaded.ChannelInfo.MultiKeyStatusList[0])
	_, exists := reloaded.ChannelInfo.MultiKeyStatusList[1]
	require.False(t, exists)
}

func TestEnableAutoDisabledChannelKeyRejectsChangedProbeTarget(t *testing.T) {
	const channelID = 991007
	require.NoError(t, DB.Unscoped().Delete(&Channel{}, channelID).Error)
	t.Cleanup(func() {
		_ = DB.Unscoped().Delete(&Channel{}, channelID).Error
	})

	channel := &Channel{
		Id:     channelID,
		Name:   "monitor-key-replaced",
		Status: common.ChannelStatusAutoDisabled,
		Key:    "probed-key\nother-key",
		ChannelInfo: ChannelInfo{
			IsMultiKey: true,
			MultiKeyStatusList: map[int]int{
				0: common.ChannelStatusAutoDisabled,
				1: common.ChannelStatusAutoDisabled,
			},
		},
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channelID).Update("key", "replacement-key\nother-key").Error)

	require.False(t, EnableAutoDisabledChannelKey(channelID, 0, "probed-key"))

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channelID).Error)
	require.Equal(t, common.ChannelStatusAutoDisabled, reloaded.Status)
	require.Equal(t, common.ChannelStatusAutoDisabled, reloaded.ChannelInfo.MultiKeyStatusList[0])
}

func TestEnableAutoDisabledChannelKeyPreservesManualDisable(t *testing.T) {
	const channelID = 991008
	require.NoError(t, DB.Unscoped().Delete(&Channel{}, channelID).Error)
	t.Cleanup(func() {
		_ = DB.Unscoped().Delete(&Channel{}, channelID).Error
	})

	channel := &Channel{
		Id:     channelID,
		Name:   "monitor-manual-disable",
		Status: common.ChannelStatusAutoDisabled,
		Key:    "probed-key\nother-key",
		ChannelInfo: ChannelInfo{
			IsMultiKey: true,
			MultiKeyStatusList: map[int]int{
				0: common.ChannelStatusAutoDisabled,
				1: common.ChannelStatusAutoDisabled,
			},
		},
	}
	require.NoError(t, DB.Create(channel).Error)
	channel.ChannelInfo.MultiKeyStatusList[0] = common.ChannelStatusManuallyDisabled
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channelID).Update("channel_info", channel.ChannelInfo).Error)

	require.False(t, EnableAutoDisabledChannelKey(channelID, 0, "probed-key"))

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channelID).Error)
	require.Equal(t, common.ChannelStatusAutoDisabled, reloaded.Status)
	require.Equal(t, common.ChannelStatusManuallyDisabled, reloaded.ChannelInfo.MultiKeyStatusList[0])
}
func TestEnableAutoDisabledChannelKeyUsesRawSQLiteChannelInfoForCAS(t *testing.T) {
	if !common.UsingSQLite {
		t.Skip("SQLite-specific CAS test")
	}
	const channelID = 991011
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	require.NoError(t, DB.Unscoped().Delete(&Channel{}, channelID).Error)
	t.Cleanup(func() {
		_ = DB.Unscoped().Delete(&Channel{}, channelID).Error
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		if previousMemoryCacheEnabled {
			InitChannelCache()
		}
	})

	channel := &Channel{
		Id: channelID, Name: "monitor-raw-channel-info", Status: common.ChannelStatusAutoDisabled,
		Key: "probed-key\nother-key",
		ChannelInfo: ChannelInfo{
			IsMultiKey: true,
			MultiKeyStatusList: map[int]int{
				0: common.ChannelStatusAutoDisabled,
				1: common.ChannelStatusAutoDisabled,
			},
		},
	}
	require.NoError(t, DB.Create(channel).Error)
	var rawChannelInfo []byte
	require.NoError(t, DB.Model(&Channel{}).
		Select("channel_info").
		Where("id = ?", channelID).
		Row().
		Scan(&rawChannelInfo))
	paddedChannelInfo := append([]byte(" \n"), rawChannelInfo...)
	paddedChannelInfo = append(paddedChannelInfo, []byte("\n ")...)
	require.NoError(t, DB.Exec(
		"UPDATE channels SET channel_info = ? WHERE id = ?",
		paddedChannelInfo, channelID,
	).Error)

	require.True(t, EnableAutoDisabledChannelKey(channelID, 0, "probed-key"))

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channelID).Error)
	require.Equal(t, common.ChannelStatusEnabled, reloaded.Status)
	_, exists := reloaded.ChannelInfo.MultiKeyStatusList[0]
	require.False(t, exists)
}

func TestEnableAutoDisabledChannelKeyRefreshesSelectionCache(t *testing.T) {
	const (
		channelID = 991009
		modelName = "monitor-cache-model"
	)
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	_ = DB.Where("channel_id = ?", channelID).Delete(&Ability{}).Error
	require.NoError(t, DB.Unscoped().Delete(&Channel{}, channelID).Error)
	t.Cleanup(func() {
		_ = DB.Where("channel_id = ?", channelID).Delete(&Ability{}).Error
		_ = DB.Unscoped().Delete(&Channel{}, channelID).Error
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		if previousMemoryCacheEnabled {
			InitChannelCache()
		}
	})

	channel := &Channel{
		Id: channelID, Name: "monitor-cache-recovery", Status: common.ChannelStatusAutoDisabled,
		Key: "probed-key\nother-key", Group: "default", Models: modelName,
		ChannelInfo: ChannelInfo{
			IsMultiKey: true,
			MultiKeyStatusList: map[int]int{
				0: common.ChannelStatusAutoDisabled,
				1: common.ChannelStatusAutoDisabled,
			},
		},
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create(&Ability{
		Group: "default", Model: modelName, ChannelId: channelID, Enabled: false,
	}).Error)
	InitChannelCache()
	require.False(t, IsChannelEnabledForGroupModel("default", modelName, channelID))

	require.True(t, EnableAutoDisabledChannelKey(channelID, 0, "probed-key"))
	require.True(t, IsChannelEnabledForGroupModel("default", modelName, channelID))
	cachedInfo, err := CacheGetChannelInfo(channelID)
	require.NoError(t, err)
	_, exists := cachedInfo.MultiKeyStatusList[0]
	require.False(t, exists)

	var ability Ability
	require.NoError(t, DB.First(&ability, "channel_id = ?", channelID).Error)
	require.True(t, ability.Enabled)
}

func TestEnableAutoDisabledChannelKeyReloadsMissingCacheEntry(t *testing.T) {
	const (
		channelID = 991012
		modelName = "monitor-cache-reload-model"
	)
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	_ = DB.Where("channel_id = ?", channelID).Delete(&Ability{}).Error
	require.NoError(t, DB.Unscoped().Delete(&Channel{}, channelID).Error)
	t.Cleanup(func() {
		_ = DB.Where("channel_id = ?", channelID).Delete(&Ability{}).Error
		_ = DB.Unscoped().Delete(&Channel{}, channelID).Error
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		if previousMemoryCacheEnabled {
			InitChannelCache()
		}
	})

	channel := &Channel{
		Id: channelID, Name: "monitor-cache-reload", Status: common.ChannelStatusAutoDisabled,
		Key: "probed-key\nother-key", Group: "default", Models: modelName,
		ChannelInfo: ChannelInfo{
			IsMultiKey: true,
			MultiKeyStatusList: map[int]int{
				0: common.ChannelStatusAutoDisabled,
				1: common.ChannelStatusAutoDisabled,
			},
		},
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create(&Ability{
		Group: "default", Model: modelName, ChannelId: channelID, Enabled: false,
	}).Error)
	InitChannelCache()
	channelSyncLock.Lock()
	delete(channelsIDM, channelID)
	channelSyncLock.Unlock()

	require.True(t, EnableAutoDisabledChannelKey(channelID, 0, "probed-key"))
	require.True(t, IsChannelEnabledForGroupModel("default", modelName, channelID))
}

func TestEnableAutoDisabledChannelKeyRollsBackWhenAbilityUpdateFails(t *testing.T) {
	const channelID = 991010
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	_ = DB.Where("channel_id = ?", channelID).Delete(&Ability{}).Error
	require.NoError(t, DB.Unscoped().Delete(&Channel{}, channelID).Error)
	t.Cleanup(func() {
		_ = DB.Where("channel_id = ?", channelID).Delete(&Ability{}).Error
		_ = DB.Unscoped().Delete(&Channel{}, channelID).Error
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		if previousMemoryCacheEnabled {
			InitChannelCache()
		}
	})

	channel := &Channel{
		Id: channelID, Name: "monitor-ability-rollback", Status: common.ChannelStatusAutoDisabled,
		Key: "probed-key\nother-key", Group: "default", Models: "monitor-rollback-model",
		ChannelInfo: ChannelInfo{
			IsMultiKey: true,
			MultiKeyStatusList: map[int]int{
				0: common.ChannelStatusAutoDisabled,
				1: common.ChannelStatusAutoDisabled,
			},
		},
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create(&Ability{
		Group: "default", Model: "monitor-rollback-model", ChannelId: channelID, Enabled: false,
	}).Error)

	injectedErr := errors.New("forced ability update failure")
	callbackName := "test:reject_monitor_ability_update"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "abilities" {
			tx.AddError(injectedErr)
		}
	}))
	t.Cleanup(func() {
		_ = DB.Callback().Update().Remove(callbackName)
	})

	require.False(t, EnableAutoDisabledChannelKey(channelID, 0, "probed-key"))

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channelID).Error)
	require.Equal(t, common.ChannelStatusAutoDisabled, reloaded.Status)
	require.Equal(t, common.ChannelStatusAutoDisabled, reloaded.ChannelInfo.MultiKeyStatusList[0])
	var ability Ability
	require.NoError(t, DB.First(&ability, "channel_id = ?", channelID).Error)
	require.False(t, ability.Enabled)
}
