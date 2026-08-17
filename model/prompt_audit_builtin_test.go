package model

import (
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSavePromptAuditBuiltinPolicyUsesCASAndKeepsOptionsAtomic(t *testing.T) {
	db := setupPromptAuditTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	oldOptionMap := common.OptionMap
	oldCheckEnabled := setting.CheckSensitiveEnabled
	oldPromptEnabled := setting.CheckSensitiveOnPromptEnabled
	oldRules := append([]setting.SensitiveRule(nil), setting.SensitiveRules...)
	oldRulesConfigured := setting.SensitiveRulesConfigured
	oldChannelIds := append([]int(nil), setting.SensitiveRuleChannelIds...)
	common.OptionMap = make(map[string]string)
	t.Cleanup(func() {
		common.OptionMap = oldOptionMap
		setting.CheckSensitiveEnabled = oldCheckEnabled
		setting.CheckSensitiveOnPromptEnabled = oldPromptEnabled
		setting.SensitiveRules = oldRules
		setting.SensitiveRulesConfigured = oldRulesConfigured
		setting.SensitiveRuleChannelIds = oldChannelIds
	})

	update := PromptAuditBuiltinPolicyUpdate{
		ExpectedVersion:                    1,
		UpstreamPolicyEnabled:              false,
		UpstreamPolicyTargetType:           "channels",
		UpstreamPolicyChannelIds:           `[3,7]`,
		UpstreamPolicyGroupCodes:           `["vip"]`,
		SensitiveWordAuditEnabled:          true,
		CyberPolicyAutoBanEnabled:          true,
		CyberPolicyAutoBanExemptGroupCodes: `["trusted"]`,
		CyberPolicyBanThreshold:            3,
		CyberPolicyWindowHours:             48,
		CheckSensitiveEnabled:              true,
		CheckSensitiveOnPromptEnabled:      false,
		SensitiveRules:                     `{"rules":[{"id":"rule-1","name":"Rule 1","enabled":true,"action":"block","scope":"request","keywords":["blocked"]}]}`,
		SensitiveRuleChannelIds:            `[7,3,7]`,
		UpdatedBy:                          11,
		ChangeSummary:                      `{"kind":"builtin_policy"}`,
	}
	require.NoError(t, SavePromptAuditBuiltinPolicy(update))

	row, _, err := LoadPromptAuditConfig()
	require.NoError(t, err)
	require.EqualValues(t, 2, row.ConfigVersion)
	require.False(t, row.UpstreamPolicyEnabled)
	require.Equal(t, "channels", row.UpstreamPolicyTargetType)
	require.JSONEq(t, `[3,7]`, row.UpstreamPolicyChannelIds)
	require.JSONEq(t, `["vip"]`, row.UpstreamPolicyGroupCodes)
	require.True(t, row.SensitiveWordAuditEnabled)
	require.True(t, row.CyberPolicyAutoBanEnabled)
	require.JSONEq(t, `["trusted"]`, row.CyberPolicyAutoBanExemptGroupCodes)
	require.Equal(t, 3, row.CyberPolicyBanThreshold)
	require.Equal(t, 48, row.CyberPolicyWindowHours)
	require.Equal(t, 11, row.UpdatedBy)
	require.False(t, setting.CheckSensitiveOnPromptEnabled)
	require.Equal(t, []int{3, 7}, setting.SensitiveRuleChannelIds)

	update.ExpectedVersion = 1
	update.CheckSensitiveEnabled = false
	update.SensitiveRules = `{"rules":[]}`
	require.ErrorIs(t, SavePromptAuditBuiltinPolicy(update), ErrPromptAuditConfigConflict)

	var enabledOption Option
	require.NoError(t, db.First(&enabledOption, commonKeyCol+" = ?", PromptAuditOptionCheckSensitiveEnabled).Error)
	require.Equal(t, "true", enabledOption.Value)
	var rulesOption Option
	require.NoError(t, db.First(&rulesOption, commonKeyCol+" = ?", PromptAuditOptionSensitiveRules).Error)
	require.Contains(t, rulesOption.Value, "blocked")
}

func TestBuiltinOptionWriteInvalidatesStaleAuditPolicyVersion(t *testing.T) {
	db := setupPromptAuditTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	oldOptionMap := common.OptionMap
	oldRules := append([]setting.SensitiveRule(nil), setting.SensitiveRules...)
	oldRulesConfigured := setting.SensitiveRulesConfigured
	oldChannelIds := append([]int(nil), setting.SensitiveRuleChannelIds...)
	common.OptionMap = make(map[string]string)
	t.Cleanup(func() {
		common.OptionMap = oldOptionMap
		setting.SensitiveRules = oldRules
		setting.SensitiveRulesConfigured = oldRulesConfigured
		setting.SensitiveRuleChannelIds = oldChannelIds
	})

	config, _, err := LoadPromptAuditConfig()
	require.NoError(t, err)
	require.EqualValues(t, 1, config.ConfigVersion)

	require.NoError(t, UpdateOption(PromptAuditOptionSensitiveRules,
		`{"rules":[{"id":"new-rule","name":"New","enabled":true,"action":"block","scope":"request","keywords":["new"]}]}`))
	require.NoError(t, UpdateOptionsBulk(map[string]string{
		PromptAuditOptionSensitiveRuleChannelIds: `[13]`,
	}))
	updated, _, err := LoadPromptAuditConfig()
	require.NoError(t, err)
	require.EqualValues(t, 3, updated.ConfigVersion)

	err = SavePromptAuditBuiltinPolicy(PromptAuditBuiltinPolicyUpdate{
		ExpectedVersion:               config.ConfigVersion,
		CyberPolicyBanThreshold:       10,
		CyberPolicyWindowHours:        720,
		CheckSensitiveEnabled:         true,
		CheckSensitiveOnPromptEnabled: true,
		SensitiveRules:                `{"rules":[]}`,
		SensitiveRuleChannelIds:       `[]`,
	})
	require.ErrorIs(t, err, ErrPromptAuditConfigConflict)

	var rules Option
	require.NoError(t, db.First(&rules, commonKeyCol+" = ?", PromptAuditOptionSensitiveRules).Error)
	require.Contains(t, rules.Value, "new-rule")
}

func TestUpdateOptionRejectsEnabledSensitiveRuleWithoutRoutingTargets(t *testing.T) {
	db := setupPromptAuditTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))

	err := UpdateOption(PromptAuditOptionSensitiveRules,
		`{"rules":[{"enabled":true,"keywords":["blocked"],"target_type":"channel_tags","channel_tags":[]}]}`)
	require.Error(t, err)

	var count int64
	require.NoError(t, db.Model(&Option{}).
		Where(commonKeyCol+" = ?", PromptAuditOptionSensitiveRules).
		Count(&count).Error)
	require.Zero(t, count)
}

func TestLoadOptionsFromDatabaseRejectsInvalidBuiltinSnapshotAtomically(t *testing.T) {
	db := setupPromptAuditTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))

	originalPolicy := setting.GetSensitivePolicySnapshot()
	originalOptionMap := common.OptionMap
	stablePolicy := setting.SensitivePolicySnapshot{
		CheckEnabled:         true,
		CheckOnPromptEnabled: true,
		Rules: []setting.SensitiveRule{{
			ID:         "stable-rule",
			Enabled:    true,
			Action:     setting.SensitiveRuleActionBlock,
			Scope:      setting.SensitiveRuleScopeRequest,
			Keywords:   []string{"stable"},
			TargetType: setting.SensitiveRuleTargetChannels,
			ChannelIds: []int{7},
		}},
		RulesConfigured:  true,
		LegacyChannelIds: []int{7},
	}
	setting.ReplaceSensitivePolicySnapshot(stablePolicy)
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{
		PromptAuditOptionCheckSensitiveEnabled:         "true",
		PromptAuditOptionCheckSensitiveOnPromptEnabled: "true",
		PromptAuditOptionSensitiveRules:                `{"rules":[{"id":"stable-rule"}]}`,
		PromptAuditOptionSensitiveRuleChannelIds:       `[7]`,
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		setting.ReplaceSensitivePolicySnapshot(originalPolicy)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	require.NoError(t, db.Create(&[]Option{
		{Key: PromptAuditOptionCheckSensitiveEnabled, Value: "false"},
		{Key: PromptAuditOptionCheckSensitiveOnPromptEnabled, Value: "false"},
		{Key: PromptAuditOptionSensitiveRules, Value: `{invalid`},
		{Key: PromptAuditOptionSensitiveRuleChannelIds, Value: `[99]`},
	}).Error)

	loadOptionsFromDatabase()

	require.Equal(t, stablePolicy, setting.GetSensitivePolicySnapshot())
	common.OptionMapRWMutex.RLock()
	require.Equal(t, "true", common.OptionMap[PromptAuditOptionCheckSensitiveEnabled])
	require.Equal(t, "true", common.OptionMap[PromptAuditOptionCheckSensitiveOnPromptEnabled])
	require.Equal(t, `{"rules":[{"id":"stable-rule"}]}`, common.OptionMap[PromptAuditOptionSensitiveRules])
	require.Equal(t, `[7]`, common.OptionMap[PromptAuditOptionSensitiveRuleChannelIds])
	common.OptionMapRWMutex.RUnlock()
}

func TestLoadOptionsFromDatabaseSerializesBuiltinGenerationWithUpdate(t *testing.T) {
	db := setupPromptAuditTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))

	originalPolicy := setting.GetSensitivePolicySnapshot()
	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	setting.ReplaceSensitivePolicySnapshot(setting.SensitivePolicySnapshot{
		Words: []string{"preserved-word"},
	})
	t.Cleanup(func() {
		setting.ReplaceSensitivePolicySnapshot(originalPolicy)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	loadedValues := map[string]string{
		PromptAuditOptionCheckSensitiveEnabled:         "true",
		PromptAuditOptionCheckSensitiveOnPromptEnabled: "true",
		PromptAuditOptionSensitiveRules:                `{"rules":[{"id":"loaded-rule","enabled":true,"action":"block","scope":"request","keywords":["loaded"],"target_type":"channels","channel_ids":[1]}]}`,
		PromptAuditOptionSensitiveRuleChannelIds:       `[1]`,
	}
	for key, value := range loadedValues {
		require.NoError(t, db.Create(&Option{Key: key, Value: value}).Error)
	}

	loadRead := make(chan struct{})
	releaseLoad := make(chan struct{})
	var blockLoad sync.Once
	const callbackName = "test:block_builtin_option_load_after_read"
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Statement.Schema.Name != "Option" {
			return
		}
		blockLoad.Do(func() {
			close(loadRead)
			<-releaseLoad
		})
	}))
	t.Cleanup(func() {
		require.NoError(t, db.Callback().Query().Remove(callbackName))
	})

	loadDone := make(chan struct{})
	go func() {
		loadOptionsFromDatabase()
		close(loadDone)
	}()
	<-loadRead
	if optionWriteMutex.TryLock() {
		optionWriteMutex.Unlock()
		close(releaseLoad)
		<-loadDone
		t.Fatal("database load released optionWriteMutex before publishing its builtin generation")
	}

	updatedValues := map[string]string{
		PromptAuditOptionCheckSensitiveEnabled:         "false",
		PromptAuditOptionCheckSensitiveOnPromptEnabled: "false",
		PromptAuditOptionSensitiveRules:                `{"rules":[{"id":"updated-rule","enabled":true,"action":"block","scope":"request","keywords":["updated"],"target_type":"channels","channel_ids":[2]}]}`,
		PromptAuditOptionSensitiveRuleChannelIds:       `[2]`,
	}
	updateStarted := make(chan struct{})
	updateDone := make(chan error, 1)
	go func() {
		close(updateStarted)
		updateDone <- UpdateOptionsBulk(updatedValues)
	}()
	<-updateStarted
	close(releaseLoad)
	<-loadDone
	require.NoError(t, <-updateDone)

	updatedRules, err := setting.ParseSensitiveRulesJSONString(updatedValues[PromptAuditOptionSensitiveRules])
	require.NoError(t, err)
	snapshot := setting.GetSensitivePolicySnapshot()
	require.False(t, snapshot.CheckEnabled)
	require.False(t, snapshot.CheckOnPromptEnabled)
	require.Equal(t, []string{"preserved-word"}, snapshot.Words)
	require.True(t, snapshot.RulesConfigured)
	require.Len(t, snapshot.Rules, 1)
	require.Equal(t, updatedRules[0].ID, snapshot.Rules[0].ID)
	require.Equal(t, updatedRules[0].Keywords, snapshot.Rules[0].Keywords)
	require.Empty(t, snapshot.Rules[0].GroupRefs)
	require.Equal(t, []int{2}, snapshot.LegacyChannelIds)

	common.OptionMapRWMutex.RLock()
	publishedValues := make(map[string]string, len(updatedValues))
	for key := range updatedValues {
		publishedValues[key] = common.OptionMap[key]
	}
	common.OptionMapRWMutex.RUnlock()
	require.Equal(t, updatedValues, publishedValues)
}
