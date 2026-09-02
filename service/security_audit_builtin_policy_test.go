package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/require"
)

func TestSecurityAuditBuiltinPolicyMigratesLegacyWordsWithoutDeletingThem(t *testing.T) {
	db := setupPromptAuditServiceTest(t, false, false, nil)
	require.NoError(t, db.AutoMigrate(&model.Option{}))

	oldOptionMap := common.OptionMap
	oldCheckEnabled := setting.CheckSensitiveEnabled
	oldPromptEnabled := setting.CheckSensitiveOnPromptEnabled
	oldWords := append([]string(nil), setting.SensitiveWords...)
	oldRules := append([]setting.SensitiveRule(nil), setting.SensitiveRules...)
	oldRulesConfigured := setting.SensitiveRulesConfigured
	oldChannelIds := append([]int(nil), setting.SensitiveRuleChannelIds...)
	common.OptionMap = make(map[string]string)
	setting.SensitiveRules = nil
	setting.SensitiveRulesConfigured = false
	setting.SensitiveRuleChannelIds = []int{9}
	t.Cleanup(func() {
		common.OptionMap = oldOptionMap
		setting.CheckSensitiveEnabled = oldCheckEnabled
		setting.CheckSensitiveOnPromptEnabled = oldPromptEnabled
		setting.SensitiveWords = oldWords
		setting.SensitiveRules = oldRules
		setting.SensitiveRulesConfigured = oldRulesConfigured
		setting.SensitiveRuleChannelIds = oldChannelIds
	})
	require.NoError(t, db.Create(&model.Option{Key: "SensitiveWords", Value: "legacy-word\n旧词"}).Error)
	setting.SensitiveWordsFromString("legacy-word\n旧词")
	common.OptionMap["SensitiveWords"] = "legacy-word\n旧词"
	setting.SensitiveRulesConfigured = false

	policy, err := GetSecurityAuditBuiltinPolicy()
	require.NoError(t, err)
	require.False(t, policy.CyberPolicyAutoBanEnabled)
	require.Empty(t, policy.CyberPolicyAutoBanExemptGroupCodes)
	require.Equal(t, 10, policy.CyberPolicyBanThreshold)
	require.Equal(t, 720, policy.CyberPolicyWindowHours)
	require.Equal(t, []string{PromptAuditPolicySourceCyber}, policy.PolicyActionSources)
	require.Equal(t, PromptAuditUpstreamPolicyTargetAll, policy.UpstreamPolicyTargetType)
	require.Empty(t, policy.UpstreamPolicyChannelIds)
	require.Empty(t, policy.UpstreamPolicyGroupCodes)
	require.True(t, policy.UsesLegacySensitiveWords)
	require.Contains(t, policy.SensitiveRules, "legacy-word")
	require.Contains(t, policy.SensitiveRules, "旧词")

	disabled := false
	autoBanEnabled := true
	banThreshold := 1
	windowHours := 24
	updated, err := SaveSecurityAuditBuiltinPolicy(SecurityAuditBuiltinPolicyUpdateRequest{
		ExpectedConfigVersion:     policy.ConfigVersion,
		CheckSensitiveEnabled:     &disabled,
		CyberPolicyAutoBanEnabled: &autoBanEnabled,
		CyberPolicyBanThreshold:   &banThreshold,
		CyberPolicyWindowHours:    &windowHours,
	}, 23)
	require.NoError(t, err)
	require.EqualValues(t, policy.ConfigVersion+1, updated.ConfigVersion)
	require.False(t, updated.CheckSensitiveEnabled)
	require.True(t, updated.CyberPolicyAutoBanEnabled)
	require.Equal(t, 1, updated.CyberPolicyBanThreshold)
	require.Equal(t, 24, updated.CyberPolicyWindowHours)
	require.False(t, updated.UsesLegacySensitiveWords)
	require.Equal(t, "legacy-word\n旧词", updated.SensitiveWords)
	require.Contains(t, updated.SensitiveRules, "legacy-word")

	var legacy model.Option
	require.NoError(t, db.First(&legacy, "`key` = ?", "SensitiveWords").Error)
	require.Equal(t, "legacy-word\n旧词", legacy.Value)

	_, err = SaveSecurityAuditBuiltinPolicy(SecurityAuditBuiltinPolicyUpdateRequest{
		ExpectedConfigVersion: policy.ConfigVersion,
		CheckSensitiveEnabled: &disabled,
	}, 23)
	require.ErrorIs(t, err, model.ErrPromptAuditConfigConflict)

	invalidThreshold := 0
	_, err = SaveSecurityAuditBuiltinPolicy(SecurityAuditBuiltinPolicyUpdateRequest{
		ExpectedConfigVersion:   updated.ConfigVersion,
		CyberPolicyBanThreshold: &invalidThreshold,
	}, 23)
	require.ErrorContains(t, err, "自动封禁阈值")
}

func TestSaveSecurityAuditBuiltinPolicyPersistsPolicyActionSources(t *testing.T) {
	db := setupPromptAuditServiceTest(t, false, false, nil)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	policy, err := GetSecurityAuditBuiltinPolicy()
	require.NoError(t, err)
	sources := []string{PromptAuditPolicySourceBiologicalRisk, PromptAuditPolicySourceCyber, PromptAuditPolicySourceCyber}
	updated, err := SaveSecurityAuditBuiltinPolicy(SecurityAuditBuiltinPolicyUpdateRequest{
		ExpectedConfigVersion: policy.ConfigVersion,
		PolicyActionSources:   &sources,
	}, 23)
	require.NoError(t, err)
	require.Equal(t, []string{PromptAuditPolicySourceBiologicalRisk, PromptAuditPolicySourceCyber}, updated.PolicyActionSources)
	empty := []string{}
	updated, err = SaveSecurityAuditBuiltinPolicy(SecurityAuditBuiltinPolicyUpdateRequest{
		ExpectedConfigVersion: updated.ConfigVersion,
		PolicyActionSources:   &empty,
	}, 23)
	require.NoError(t, err)
	require.Empty(t, updated.PolicyActionSources)

	invalid := []string{"unknown"}
	_, err = SaveSecurityAuditBuiltinPolicy(SecurityAuditBuiltinPolicyUpdateRequest{
		ExpectedConfigVersion: updated.ConfigVersion,
		PolicyActionSources:   &invalid,
	}, 23)
	require.ErrorContains(t, err, "处置来源无效")
}

func TestSaveSecurityAuditBuiltinPolicyPreservesUpstreamPolicyScopeSelections(t *testing.T) {
	db := setupPromptAuditServiceTest(t, false, false, nil)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.Group{}))
	require.NoError(t, db.Create(&model.Group{Code: "beta", Name: "Beta", Status: model.GroupStatusActive}).Error)
	require.NoError(t, db.Create(&model.Group{Code: "vip", Name: "VIP", Status: model.GroupStatusActive}).Error)
	isolateSecurityAuditBuiltinOptionState(t)
	policy, err := GetSecurityAuditBuiltinPolicy()
	require.NoError(t, err)

	channels := PromptAuditUpstreamPolicyTargetChannels
	channelIds := []int{9, 3, 9, 0}
	groupCodes := []string{" vip ", "beta", "vip", "", "auto"}
	updated, err := SaveSecurityAuditBuiltinPolicy(SecurityAuditBuiltinPolicyUpdateRequest{
		ExpectedConfigVersion:    policy.ConfigVersion,
		UpstreamPolicyTargetType: &channels,
		UpstreamPolicyChannelIds: &channelIds,
		UpstreamPolicyGroupCodes: &groupCodes,
	}, 23)
	require.NoError(t, err)
	require.Equal(t, PromptAuditUpstreamPolicyTargetChannels, updated.UpstreamPolicyTargetType)
	require.Equal(t, []int{3, 9}, updated.UpstreamPolicyChannelIds)
	require.Equal(t, []string{"beta", "vip"}, updated.UpstreamPolicyGroupCodes)

	groups := PromptAuditUpstreamPolicyTargetGroups
	updated, err = SaveSecurityAuditBuiltinPolicy(SecurityAuditBuiltinPolicyUpdateRequest{
		ExpectedConfigVersion:    updated.ConfigVersion,
		UpstreamPolicyTargetType: &groups,
	}, 23)
	require.NoError(t, err)
	require.Equal(t, PromptAuditUpstreamPolicyTargetGroups, updated.UpstreamPolicyTargetType)
	require.Equal(t, []int{3, 9}, updated.UpstreamPolicyChannelIds)
	require.Equal(t, []string{"beta", "vip"}, updated.UpstreamPolicyGroupCodes)
}

func TestSaveSecurityAuditBuiltinPolicyPersistsAutoBanGroupWhitelist(t *testing.T) {
	db := setupPromptAuditServiceTest(t, false, false, nil)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.Group{}))
	require.NoError(t, db.Create(&model.Group{Code: "trusted", Name: "信任分组", Status: model.GroupStatusActive}).Error)
	require.NoError(t, db.Create(&model.Group{Code: "disabled", Name: "停用分组", Status: model.GroupStatusActive}).Error)
	require.NoError(t, db.Model(&model.Group{}).Where("code = ?", "disabled").Update("status", model.GroupStatusDisabled).Error)
	isolateSecurityAuditBuiltinOptionState(t)
	policy, err := GetSecurityAuditBuiltinPolicy()
	require.NoError(t, err)

	whitelist := []string{" trusted ", "", "  ", "trusted"}
	updated, err := SaveSecurityAuditBuiltinPolicy(SecurityAuditBuiltinPolicyUpdateRequest{
		ExpectedConfigVersion:              policy.ConfigVersion,
		CyberPolicyAutoBanExemptGroupCodes: &whitelist,
	}, 23)
	require.NoError(t, err)
	require.Equal(t, []string{"trusted"}, updated.CyberPolicyAutoBanExemptGroupCodes)

	row, _, err := model.LoadPromptAuditConfig()
	require.NoError(t, err)
	require.JSONEq(t, `["trusted"]`, row.CyberPolicyAutoBanExemptGroupCodes)

	require.NoError(t, db.Model(&model.Group{}).Where("code = ?", "trusted").Update("status", model.GroupStatusDisabled).Error)
	threshold := updated.CyberPolicyBanThreshold + 1
	preserved := append([]string(nil), updated.CyberPolicyAutoBanExemptGroupCodes...)
	updated, err = SaveSecurityAuditBuiltinPolicy(SecurityAuditBuiltinPolicyUpdateRequest{
		ExpectedConfigVersion:              updated.ConfigVersion,
		CyberPolicyAutoBanExemptGroupCodes: &preserved,
		CyberPolicyBanThreshold:            &threshold,
	}, 23)
	require.NoError(t, err)
	require.Equal(t, threshold, updated.CyberPolicyBanThreshold)
	require.Equal(t, []string{"trusted"}, updated.CyberPolicyAutoBanExemptGroupCodes)

	missing := []string{"missing-group"}
	_, err = SaveSecurityAuditBuiltinPolicy(SecurityAuditBuiltinPolicyUpdateRequest{
		ExpectedConfigVersion:              updated.ConfigVersion,
		CyberPolicyAutoBanExemptGroupCodes: &missing,
	}, 23)
	require.ErrorContains(t, err, "cyber_policy 自动封禁分组白名单")

	disabled := []string{"disabled"}
	_, err = SaveSecurityAuditBuiltinPolicy(SecurityAuditBuiltinPolicyUpdateRequest{
		ExpectedConfigVersion:              updated.ConfigVersion,
		CyberPolicyAutoBanExemptGroupCodes: &disabled,
	}, 23)
	require.ErrorContains(t, err, "不存在或已停用")

	invalid := []string{"auto"}
	_, err = SaveSecurityAuditBuiltinPolicy(SecurityAuditBuiltinPolicyUpdateRequest{
		ExpectedConfigVersion:              updated.ConfigVersion,
		CyberPolicyAutoBanExemptGroupCodes: &invalid,
	}, 23)
	require.ErrorContains(t, err, "白名单编码无效")

	removed := []string{}
	updated, err = SaveSecurityAuditBuiltinPolicy(SecurityAuditBuiltinPolicyUpdateRequest{
		ExpectedConfigVersion:              updated.ConfigVersion,
		CyberPolicyAutoBanExemptGroupCodes: &removed,
	}, 23)
	require.NoError(t, err)
	require.Empty(t, updated.CyberPolicyAutoBanExemptGroupCodes)
}

func TestSaveSecurityAuditBuiltinPolicyRejectsEmptyActiveUpstreamPolicyScope(t *testing.T) {
	db := setupPromptAuditServiceTest(t, false, false, nil)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	isolateSecurityAuditBuiltinOptionState(t)
	policy, err := GetSecurityAuditBuiltinPolicy()
	require.NoError(t, err)

	channels := PromptAuditUpstreamPolicyTargetChannels
	emptyChannelIds := []int{}
	_, err = SaveSecurityAuditBuiltinPolicy(SecurityAuditBuiltinPolicyUpdateRequest{
		ExpectedConfigVersion:    policy.ConfigVersion,
		UpstreamPolicyTargetType: &channels,
		UpstreamPolicyChannelIds: &emptyChannelIds,
	}, 23)
	require.ErrorContains(t, err, "至少需要选择一个渠道")

	groups := PromptAuditUpstreamPolicyTargetGroups
	emptyGroupCodes := []string{" ", ""}
	_, err = SaveSecurityAuditBuiltinPolicy(SecurityAuditBuiltinPolicyUpdateRequest{
		ExpectedConfigVersion:    policy.ConfigVersion,
		UpstreamPolicyTargetType: &groups,
		UpstreamPolicyGroupCodes: &emptyGroupCodes,
	}, 23)
	require.ErrorContains(t, err, "至少需要选择一个业务分组")
}

func TestSaveSecurityAuditBuiltinPolicyRejectsUnknownUpstreamGroup(t *testing.T) {
	db := setupPromptAuditServiceTest(t, false, false, nil)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.Group{}))
	isolateSecurityAuditBuiltinOptionState(t)
	policy, err := GetSecurityAuditBuiltinPolicy()
	require.NoError(t, err)
	targetType := PromptAuditUpstreamPolicyTargetGroups
	groupCodes := []string{"missing-group"}
	_, err = SaveSecurityAuditBuiltinPolicy(SecurityAuditBuiltinPolicyUpdateRequest{
		ExpectedConfigVersion:    policy.ConfigVersion,
		UpstreamPolicyTargetType: &targetType,
		UpstreamPolicyGroupCodes: &groupCodes,
	}, 23)
	require.ErrorContains(t, err, "不存在或已停用")
}

func isolateSecurityAuditBuiltinOptionState(t *testing.T) {
	t.Helper()
	oldOptionMap := common.OptionMap
	oldCheckEnabled := setting.CheckSensitiveEnabled
	oldPromptEnabled := setting.CheckSensitiveOnPromptEnabled
	oldWords := append([]string(nil), setting.SensitiveWords...)
	oldRules := append([]setting.SensitiveRule(nil), setting.SensitiveRules...)
	oldRulesConfigured := setting.SensitiveRulesConfigured
	oldChannelIds := append([]int(nil), setting.SensitiveRuleChannelIds...)
	common.OptionMap = make(map[string]string)
	t.Cleanup(func() {
		common.OptionMap = oldOptionMap
		setting.CheckSensitiveEnabled = oldCheckEnabled
		setting.CheckSensitiveOnPromptEnabled = oldPromptEnabled
		setting.SensitiveWords = oldWords
		setting.SensitiveRules = oldRules
		setting.SensitiveRulesConfigured = oldRulesConfigured
		setting.SensitiveRuleChannelIds = oldChannelIds
	})
}

func TestSavePromptAuditConfigPreservesBuiltinPolicyWhenFieldsAreOmitted(t *testing.T) {
	db := setupPromptAuditServiceTest(t, false, false, nil)
	require.NoError(t, db.AutoMigrate(&model.Group{}))
	require.NoError(t, db.Create(&model.Group{Code: "trusted", Name: "信任分组", Status: model.GroupStatusActive}).Error)
	row, endpoints, err := model.LoadPromptAuditConfig()
	require.NoError(t, err)
	row.UpstreamPolicyEnabled = true
	row.SensitiveWordAuditEnabled = true
	row.CyberPolicyAutoBanEnabled = true
	row.CyberPolicyAutoBanExemptGroupCodes = `["trusted"]`
	row.CyberPolicyBanThreshold = 7
	row.CyberPolicyWindowHours = 96
	require.NoError(t, model.SavePromptAuditConfig(row.ConfigVersion, row, endpoints))
	InvalidatePromptAuditConfig()

	current, err := GetPublicPromptAuditConfig()
	require.NoError(t, err)
	req := promptAuditUpdateRequestFromConfig(current)
	req.UpstreamPolicyEnabled = nil
	req.SensitiveWordAuditEnabled = nil
	for i := range req.Endpoints {
		req.Endpoints[i].TokenAction = PromptAuditTokenKeep
	}
	updated, err := SavePromptAuditConfig(req, 31)
	require.NoError(t, err)
	require.True(t, updated.UpstreamPolicyEnabled)
	require.True(t, updated.SensitiveWordAuditEnabled)
	require.True(t, updated.CyberPolicyAutoBanEnabled)
	require.Equal(t, []string{"trusted"}, updated.CyberPolicyAutoBanExemptGroupCodes)
	require.Equal(t, 7, updated.CyberPolicyBanThreshold)
	require.Equal(t, 96, updated.CyberPolicyWindowHours)

	emptyWhitelist := []string{}
	req = promptAuditUpdateRequestFromConfig(updated)
	req.CyberPolicyAutoBanExemptGroupCodes = &emptyWhitelist
	for i := range req.Endpoints {
		req.Endpoints[i].TokenAction = PromptAuditTokenKeep
	}
	updated, err = SavePromptAuditConfig(req, 31)
	require.NoError(t, err)
	require.Empty(t, updated.CyberPolicyAutoBanExemptGroupCodes)
}

func TestCyberPolicyAutoBanRequiresUpstreamPolicyEventRecording(t *testing.T) {
	setupPromptAuditServiceTest(t, false, false, nil)
	policy, err := GetSecurityAuditBuiltinPolicy()
	require.NoError(t, err)

	disabled := false
	enabled := true
	_, err = SaveSecurityAuditBuiltinPolicy(SecurityAuditBuiltinPolicyUpdateRequest{
		ExpectedConfigVersion:     policy.ConfigVersion,
		UpstreamPolicyEnabled:     &disabled,
		CyberPolicyAutoBanEnabled: &enabled,
	}, 23)
	require.ErrorContains(t, err, "必须先启用上游安全策略事件记录")

	current, err := GetPublicPromptAuditConfig()
	require.NoError(t, err)
	req := promptAuditUpdateRequestFromConfig(current)
	req.UpstreamPolicyEnabled = &disabled
	req.CyberPolicyAutoBanEnabled = &enabled
	for i := range req.Endpoints {
		req.Endpoints[i].TokenAction = PromptAuditTokenKeep
	}
	_, err = SavePromptAuditConfig(req, 23)
	require.ErrorContains(t, err, "必须先启用上游安全策略事件记录")
}

func TestSaveSecurityAuditBuiltinPolicyPersistsPerRuleAndCombinedScopes(t *testing.T) {
	db := setupPromptAuditServiceTest(t, false, false, nil)
	require.NoError(t, db.AutoMigrate(&model.Option{}))

	oldOptionMap := common.OptionMap
	oldRules := append([]setting.SensitiveRule(nil), setting.SensitiveRules...)
	oldRulesConfigured := setting.SensitiveRulesConfigured
	oldChannelIds := append([]int(nil), setting.SensitiveRuleChannelIds...)
	common.OptionMap = make(map[string]string)
	setting.SensitiveRules = nil
	setting.SensitiveRulesConfigured = true
	setting.SensitiveRuleChannelIds = []int{99}
	t.Cleanup(func() {
		common.OptionMap = oldOptionMap
		setting.SensitiveRules = oldRules
		setting.SensitiveRulesConfigured = oldRulesConfigured
		setting.SensitiveRuleChannelIds = oldChannelIds
	})

	policy, err := GetSecurityAuditBuiltinPolicy()
	require.NoError(t, err)
	raw := `{"rules":[
		{"id":"channels","name":"Channels","enabled":true,"action":"block","scope":"request","keywords":["one"],"target_type":"channels","channel_ids":[9,3,9]},
		{"id":"groups","name":"Groups","enabled":true,"action":"block","scope":"request","keywords":["two"],"target_type":"channel_tags","channel_tags":[" backup ","primary","backup"]},
		{"id":"routes","name":"Routes","enabled":true,"action":"block","scope":"request","keywords":["three"],"target_type":"routes","channel_ids":[12,3,12],"group_codes":[" group-b ","group-a","group-b"]}
	]}`
	updated, err := SaveSecurityAuditBuiltinPolicy(SecurityAuditBuiltinPolicyUpdateRequest{
		ExpectedConfigVersion: policy.ConfigVersion,
		SensitiveRules:        &raw,
	}, 42)
	require.NoError(t, err)
	rules, err := setting.ParseSensitiveRulesJSONString(updated.SensitiveRules)
	require.NoError(t, err)
	require.Len(t, rules, 3)
	require.Equal(t, []int{3, 9}, rules[0].ChannelIds)
	require.Equal(t, setting.SensitiveRuleTargetChannels, rules[0].TargetType)
	require.Equal(t, []string{"backup", "primary"}, rules[1].ChannelTags)
	require.Equal(t, setting.SensitiveRuleTargetChannelTags, rules[1].TargetType)
	require.Equal(t, setting.SensitiveRuleTargetRoutes, rules[2].TargetType)
	require.Equal(t, []int{3, 12}, rules[2].ChannelIds)
	require.Equal(t, []string{"group-a", "group-b"}, rules[2].GroupCodes)
	// 旧全局渠道仍保留，供尚未显式迁移的规则和回滚版本使用。
	require.Equal(t, "[99]", updated.SensitiveRuleChannelIds)

	row, _, err := model.LoadPromptAuditConfig()
	require.NoError(t, err)
	var combinedSummary struct {
		TargetChannelCount int `json:"sensitive_rule_target_channel_count"`
		TargetTagCount     int `json:"sensitive_rule_target_tag_count"`
		TargetGroupCount   int `json:"sensitive_rule_target_group_count"`
	}
	require.NoError(t, common.UnmarshalJsonStr(row.ChangeSummary, &combinedSummary))
	require.Equal(t, 3, combinedSummary.TargetChannelCount)
	require.Equal(t, 2, combinedSummary.TargetTagCount)
	require.Equal(t, 2, combinedSummary.TargetGroupCount)

	legacyRaw := `{"rules":[
		{"id":"channels","name":"Channels","enabled":true,"action":"block","scope":"request","keywords":["one"],"target_type":"channels","channel_ids":[9,3]},
		{"id":"tags","name":"Tags","enabled":true,"action":"block","scope":"request","keywords":["two"],"target_type":"channel_tags","channel_tags":["backup","primary"]},
		{"id":"legacy","name":"Legacy","enabled":true,"action":"block","scope":"request","keywords":["three"]}
	]}`
	legacyChannelIds := `[8,7,8]`
	updated, err = SaveSecurityAuditBuiltinPolicy(SecurityAuditBuiltinPolicyUpdateRequest{
		ExpectedConfigVersion:   updated.ConfigVersion,
		SensitiveRules:          &legacyRaw,
		SensitiveRuleChannelIds: &legacyChannelIds,
	}, 43)
	require.NoError(t, err)
	require.Equal(t, "[7,8]", updated.SensitiveRuleChannelIds)

	row, _, err = model.LoadPromptAuditConfig()
	require.NoError(t, err)
	var summary struct {
		TargetChannelCount int `json:"sensitive_rule_target_channel_count"`
		TargetTagCount     int `json:"sensitive_rule_target_tag_count"`
	}
	require.NoError(t, common.UnmarshalJsonStr(row.ChangeSummary, &summary))
	require.Equal(t, 4, summary.TargetChannelCount)
	require.Equal(t, 2, summary.TargetTagCount)
}

func TestSaveSecurityAuditBuiltinPolicyRejectsEnabledExplicitRuleWithoutTargets(t *testing.T) {
	db := setupPromptAuditServiceTest(t, false, false, nil)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	policy, err := GetSecurityAuditBuiltinPolicy()
	require.NoError(t, err)
	raw := `{"rules":[{"id":"empty","name":"Empty","enabled":true,"action":"block","scope":"request","keywords":["blocked"],"target_type":"channels"}]}`

	_, err = SaveSecurityAuditBuiltinPolicy(SecurityAuditBuiltinPolicyUpdateRequest{
		ExpectedConfigVersion: policy.ConfigVersion,
		SensitiveRules:        &raw,
	}, 42)
	require.ErrorContains(t, err, "必须至少选择一个渠道")
}
