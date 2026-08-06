package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"

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

	require.True(t, enableAutoDisabledChannelKey(channelID, 1, "duplicate-key", false, false))

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

	require.False(t, enableAutoDisabledChannelKey(channelID, 0, "probed-key", false, false))

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

	require.False(t, enableAutoDisabledChannelKey(channelID, 0, "probed-key", false, false))

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

	require.True(t, enableAutoDisabledChannelKey(channelID, 0, "probed-key", false, false))

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
		Key: "probed-key\nother-key", Group: "default", Models: modelName, AutoBan: common.GetPointer(1),
		ChannelInfo: ChannelInfo{
			IsMultiKey: true,
			MultiKeyStatusList: map[int]int{
				0: common.ChannelStatusAutoDisabled,
				1: common.ChannelStatusAutoDisabled,
			},
		},
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		MonitorEnabled:              common.GetPointer(true),
		MonitorAutoEnableEnabled:    common.GetPointer(true),
		MonitorEnableThreshold:      common.GetPointer(1),
		MonitorConsecutiveSuccesses: 1,
	})
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create(&Ability{
		Group: "default", Model: modelName, ChannelId: channelID, Enabled: false,
	}).Error)
	InitChannelCache()
	require.False(t, IsChannelEnabledForGroupModel("default", modelName, channelID))

	require.True(t, EnableAutoDisabledChannelKey(channelID, 0, "probed-key", true))
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

	require.True(t, enableAutoDisabledChannelKey(channelID, 0, "probed-key", false, false))
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

	require.False(t, enableAutoDisabledChannelKey(channelID, 0, "probed-key", false, false))

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channelID).Error)
	require.Equal(t, common.ChannelStatusAutoDisabled, reloaded.Status)
	require.Equal(t, common.ChannelStatusAutoDisabled, reloaded.ChannelInfo.MultiKeyStatusList[0])
	var ability Ability
	require.NoError(t, DB.First(&ability, "channel_id = ?", channelID).Error)
	require.False(t, ability.Enabled)
}

func TestEnableAutoDisabledSingleKeyChannelRefreshesSelectionCache(t *testing.T) {
	const (
		channelID = 991013
		modelName = "monitor-single-key-cache-model"
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
		Id:      channelID,
		Name:    "monitor-single-key-cache-recovery",
		Status:  common.ChannelStatusAutoDisabled,
		Key:     "single-probed-key",
		Group:   "default",
		Models:  modelName,
		AutoBan: common.GetPointer(1),
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		MonitorEnabled:              common.GetPointer(true),
		MonitorAutoEnableEnabled:    common.GetPointer(true),
		MonitorEnableThreshold:      common.GetPointer(1),
		MonitorConsecutiveSuccesses: 1,
	})
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create(&Ability{
		Group: "default", Model: modelName, ChannelId: channelID, Enabled: false,
	}).Error)
	InitChannelCache()
	require.False(t, IsChannelEnabledForGroupModel("default", modelName, channelID))

	require.True(t, EnableAutoDisabledSingleKeyChannel(channelID, "single-probed-key", true))
	require.True(t, IsChannelEnabledForGroupModel("default", modelName, channelID))

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channelID).Error)
	require.Equal(t, common.ChannelStatusEnabled, reloaded.Status)
	var ability Ability
	require.NoError(t, DB.First(&ability, "channel_id = ?", channelID).Error)
	require.True(t, ability.Enabled)
}

func TestEnableAutoDisabledSingleKeyChannelRejectsChangedConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		channelID   int
		status      int
		key         string
		channelInfo ChannelInfo
	}{
		{
			name:      "key replaced after probe",
			channelID: 991014,
			status:    common.ChannelStatusAutoDisabled,
			key:       "replacement-key",
		},
		{
			name:      "channel manually disabled after probe",
			channelID: 991015,
			status:    common.ChannelStatusManuallyDisabled,
			key:       "single-probed-key",
		},
		{
			name:      "channel changed to multi key",
			channelID: 991016,
			status:    common.ChannelStatusAutoDisabled,
			key:       "single-probed-key\nother-key",
			channelInfo: ChannelInfo{
				IsMultiKey: true,
				MultiKeyStatusList: map[int]int{
					0: common.ChannelStatusAutoDisabled,
					1: common.ChannelStatusAutoDisabled,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, DB.Unscoped().Delete(&Channel{}, test.channelID).Error)
			t.Cleanup(func() {
				_ = DB.Unscoped().Delete(&Channel{}, test.channelID).Error
			})
			channel := &Channel{
				Id: test.channelID, Name: test.name, Status: test.status,
				Key: test.key, Group: "default", Models: "monitor-single-key-guard-model",
				ChannelInfo: test.channelInfo,
			}
			require.NoError(t, DB.Create(channel).Error)

			require.False(t, enableAutoDisabledSingleKeyChannel(test.channelID, "single-probed-key", false, false))

			var reloaded Channel
			require.NoError(t, DB.First(&reloaded, test.channelID).Error)
			require.Equal(t, test.status, reloaded.Status)
			require.Equal(t, test.key, reloaded.Key)
		})
	}
}

func TestEnableAutoDisabledSingleKeyChannelRollsBackWhenAbilityUpdateFails(t *testing.T) {
	const channelID = 991017
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
		Id: channelID, Name: "monitor-single-key-ability-rollback",
		Status: common.ChannelStatusAutoDisabled, Key: "single-probed-key",
		Group: "default", Models: "monitor-single-key-rollback-model",
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create(&Ability{
		Group: "default", Model: "monitor-single-key-rollback-model", ChannelId: channelID, Enabled: false,
	}).Error)

	injectedErr := errors.New("forced single-key ability update failure")
	callbackName := "test:reject_single_key_monitor_ability_update"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "abilities" {
			tx.AddError(injectedErr)
		}
	}))
	t.Cleanup(func() {
		_ = DB.Callback().Update().Remove(callbackName)
	})

	require.False(t, enableAutoDisabledSingleKeyChannel(channelID, "single-probed-key", false, false))

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channelID).Error)
	require.Equal(t, common.ChannelStatusAutoDisabled, reloaded.Status)
	var ability Ability
	require.NoError(t, DB.First(&ability, "channel_id = ?", channelID).Error)
	require.False(t, ability.Enabled)
}

func TestEnableAutoDisabledSingleKeyChannelReloadsMissingCacheEntry(t *testing.T) {
	const (
		channelID = 991018
		modelName = "monitor-single-key-cache-reload-model"
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
		Id: channelID, Name: "monitor-single-key-cache-reload",
		Status: common.ChannelStatusAutoDisabled, Key: "single-probed-key",
		Group: "default", Models: modelName,
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create(&Ability{
		Group: "default", Model: modelName, ChannelId: channelID, Enabled: false,
	}).Error)
	InitChannelCache()
	channelSyncLock.Lock()
	delete(channelsIDM, channelID)
	channelSyncLock.Unlock()

	require.True(t, enableAutoDisabledSingleKeyChannel(channelID, "single-probed-key", false, false))
	require.True(t, IsChannelEnabledForGroupModel("default", modelName, channelID))
}

func TestEnableAutoDisabledSingleKeyChannelRechecksLatestPolicy(t *testing.T) {
	tests := []struct {
		name       string
		channelID  int
		autoBan    int
		automatic  bool
		settings   dto.ChannelOtherSettings
		rawSetting string
		expected   bool
	}{
		{
			name: "auto enable disabled", channelID: 991019, autoBan: 1,
			settings: dto.ChannelOtherSettings{
				MonitorAutoEnableEnabled:    common.GetPointer(false),
				MonitorConsecutiveSuccesses: 1,
			},
		},
		{
			name: "automatic monitor disabled", channelID: 991020, autoBan: 1, automatic: true,
			settings: dto.ChannelOtherSettings{
				MonitorEnabled:              common.GetPointer(false),
				MonitorAutoEnableEnabled:    common.GetPointer(true),
				MonitorConsecutiveSuccesses: 1,
			},
		},
		{
			name: "automatic auto ban disabled", channelID: 991021, automatic: true,
			settings: dto.ChannelOtherSettings{
				MonitorEnabled:              common.GetPointer(true),
				MonitorAutoEnableEnabled:    common.GetPointer(true),
				MonitorConsecutiveSuccesses: 1,
			},
		},
		{
			name: "success threshold raised", channelID: 991022, autoBan: 1,
			settings: dto.ChannelOtherSettings{
				MonitorAutoEnableEnabled:    common.GetPointer(true),
				MonitorEnableThreshold:      common.GetPointer(2),
				MonitorConsecutiveSuccesses: 1,
			},
		},
		{
			name: "manual test ignores monitor and auto ban switches", channelID: 991023,
			settings: dto.ChannelOtherSettings{
				MonitorEnabled:              common.GetPointer(false),
				MonitorAutoEnableEnabled:    common.GetPointer(true),
				MonitorConsecutiveSuccesses: 1,
			},
			expected: true,
		},
		{
			name: "malformed settings are rejected without repair write", channelID: 991024, autoBan: 1,
			rawSetting: "{",
		},
	}

	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		if previousMemoryCacheEnabled {
			InitChannelCache()
		}
	})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, DB.Unscoped().Delete(&Channel{}, test.channelID).Error)
			t.Cleanup(func() {
				_ = DB.Unscoped().Delete(&Channel{}, test.channelID).Error
			})
			channel := &Channel{
				Id: test.channelID, Name: test.name, Status: common.ChannelStatusAutoDisabled,
				Key: "single-probed-key", Group: "default", Models: "monitor-policy-model",
				AutoBan: common.GetPointer(test.autoBan),
			}
			if test.rawSetting != "" {
				channel.OtherSettings = test.rawSetting
			} else {
				channel.SetOtherSettings(test.settings)
			}
			require.NoError(t, DB.Create(channel).Error)

			actual := EnableAutoDisabledSingleKeyChannel(test.channelID, "single-probed-key", test.automatic)
			require.Equal(t, test.expected, actual)

			var reloaded Channel
			require.NoError(t, DB.First(&reloaded, test.channelID).Error)
			if test.expected {
				require.Equal(t, common.ChannelStatusEnabled, reloaded.Status)
			} else {
				require.Equal(t, common.ChannelStatusAutoDisabled, reloaded.Status)
			}
		})
	}
}
