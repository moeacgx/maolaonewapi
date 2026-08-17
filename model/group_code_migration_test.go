package model

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"gorm.io/gorm"
)

func prepareGroupCodeMigrationTestDB(t *testing.T) {
	t.Helper()
	db := openGroupIdentityTestDB(t)
	oldOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	t.Cleanup(func() { common.OptionMap = oldOptionMap })
	if err := db.AutoMigrate(
		&Option{}, &Group{}, &GroupAlias{}, &AutoGroupMember{},
		&Channel{}, &ChannelGroupBinding{}, &Token{}, &TokenGroupBinding{},
		&User{}, &Ability{}, &SubscriptionPlan{}, &UserSubscription{},
		&PromptAuditConfig{}, &PromptAuditEvent{}, &PromptAuditQueueState{},
	); err != nil {
		t.Fatalf("迁移测试表失败: %v", err)
	}
	if err := EnsurePromptAuditDefaults(); err != nil {
		t.Fatalf("初始化安全审计配置失败: %v", err)
	}
}

func TestMigrateLegacyGroupCodesToIDsRebuildsAllCurrentReferences(t *testing.T) {
	prepareGroupCodeMigrationTestDB(t)
	defaultGroup := Group{Code: "default", Name: "默认", Ratio: 1, Status: GroupStatusActive}
	legacyGroup := Group{Code: "group_2", Name: "特价", Ratio: 0.8, Status: GroupStatusActive, UserSelectable: true}
	if err := DB.Create(&defaultGroup).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Create(&legacyGroup).Error; err != nil {
		t.Fatal(err)
	}
	targetCode := strconv.Itoa(legacyGroup.Id)

	channel := Channel{Name: "迁移渠道", Group: legacyGroup.Code + ",default", Models: "gpt-test", Status: common.ChannelStatusEnabled}
	if err := DB.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Create([]ChannelGroupBinding{{ChannelId: channel.Id, GroupId: legacyGroup.Id, Position: 0}, {ChannelId: channel.Id, GroupId: defaultGroup.Id, Position: 1}}).Error; err != nil {
		t.Fatal(err)
	}

	limit := 1.6
	token := Token{UserId: 9, Key: "group-code-migration-token", Name: "迁移令牌", Group: legacyGroup.Code + ",default", GroupMode: TokenGroupModeExplicit, GroupRatioLimits: `{"group_2":1.6}`}
	if err := DB.Create(&token).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Create([]TokenGroupBinding{{TokenId: token.Id, GroupId: legacyGroup.Id, Position: 0, RatioLimit: &limit}, {TokenId: token.Id, GroupId: defaultGroup.Id, Position: 1}}).Error; err != nil {
		t.Fatal(err)
	}

	user := User{
		Username: "migration-user", Password: "password", Group: legacyGroup.Code, GroupId: legacyGroup.Id,
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
	}
	if err := DB.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	ability := Ability{Group: legacyGroup.Code, GroupId: legacyGroup.Id, Model: "gpt-test", ChannelId: channel.Id, Enabled: true}
	if err := DB.Create(&ability).Error; err != nil {
		t.Fatal(err)
	}
	plan := SubscriptionPlan{Title: "迁移套餐", UpgradeGroup: legacyGroup.Code}
	if err := DB.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	subscription := UserSubscription{UserId: user.Id, UpgradeGroup: legacyGroup.Code, PrevUserGroup: legacyGroup.Code}
	if err := DB.Create(&subscription).Error; err != nil {
		t.Fatal(err)
	}
	sensitiveRulesValue := `{"rules":[` +
		`{"id":"group-rule","name":"分组规则","enabled":true,"action":"block","scope":"request","keywords":["blocked"],"target_type":"groups","group_codes":["group_2","default"]},` +
		`{"id":"channel-rule","name":"渠道规则","enabled":true,"action":"mask","scope":"response","replacement":"[MASK]","keywords":["secret"],"target_type":"channels","channel_ids":[7]}` +
		`]}`
	options := []Option{
		{Key: "GroupRatio", Value: `{"default":1,"group_2":0.8}`},
		{Key: "UserUsableGroups", Value: `{"default":"默认","group_2":"特价"}`},
		{Key: "GroupGroupRatio", Value: `{"default":{"group_2":0.9}}`},
		{Key: "TopupGroupRatio", Value: `{"group_2":2}`},
		{Key: "ModelRequestRateLimitGroup", Value: `{"group_2":[10,20]}`},
		{Key: PromptAuditOptionSensitiveRules, Value: sensitiveRulesValue},
	}
	if err := DB.Create(&options).Error; err != nil {
		t.Fatal(err)
	}
	previousSensitivePolicy := setting.GetSensitivePolicySnapshot()
	t.Cleanup(func() { setting.ReplaceSensitivePolicySnapshot(previousSensitivePolicy) })
	if err := setting.UpdateSensitiveRulesByJSONString(sensitiveRulesValue); err != nil {
		t.Fatal(err)
	}
	var auditConfig PromptAuditConfig
	if err := DB.First(&auditConfig, "id = ?", PromptAuditConfigID).Error; err != nil {
		t.Fatal(err)
	}
	initialAuditConfigVersion := auditConfig.ConfigVersion
	if err := DB.Model(&PromptAuditConfig{}).
		Where("id = ?", PromptAuditConfigID).
		Updates(map[string]interface{}{
			"upstream_policy_target_type":              "groups",
			"upstream_policy_group_codes":              `["group_2"]`,
			"cyber_policy_auto_ban_exempt_group_codes": `["group_2"]`,
		}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	historicalEvents := []PromptAuditEvent{
		{
			UserId: user.Id, GroupId: legacyGroup.Id, GroupCode: legacyGroup.Code,
			Source: promptAuditUpstreamPolicySource, ErrorCode: promptAuditCyberPolicyCode,
			CreatedAt: now, Categories: "[]", MatchedScanners: "[]", UnknownCategories: "[]",
		},
		{
			UserId: user.Id, GroupId: defaultGroup.Id, GroupCode: defaultGroup.Code,
			Source: promptAuditUpstreamPolicySource, ErrorCode: promptAuditCyberPolicyCode,
			CreatedAt: now, Categories: "[]", MatchedScanners: "[]", UnknownCategories: "[]",
		},
	}
	if err := DB.Create(&historicalEvents).Error; err != nil {
		t.Fatal(err)
	}

	preview, err := PreviewGroupCodeMigration()
	if err != nil {
		t.Fatalf("预检失败: %v", err)
	}
	if !preview.CanExecute || len(preview.Groups) != 1 || preview.Groups[0].TargetCode != targetCode {
		t.Fatalf("预检结果异常: %#v", preview)
	}
	if preview.AffectedSubscriptions != 1 {
		t.Fatalf("同一订阅的两个分组字段应按订阅去重统计，实际为 %d", preview.AffectedSubscriptions)
	}
	if err := DB.First(&auditConfig, "id = ?", PromptAuditConfigID).Error; err != nil {
		t.Fatal(err)
	}
	if auditConfig.UpstreamPolicyGroupCodes != `["group_2"]` ||
		auditConfig.CyberPolicyAutoBanExemptGroupCodes != `["group_2"]` ||
		auditConfig.ConfigVersion != initialAuditConfigVersion {
		t.Fatalf("预检不得改写安全审计配置: %#v", auditConfig)
	}
	previewSensitiveRules := setting.GetSensitivePolicySnapshot().Rules
	if len(previewSensitiveRules) != 2 ||
		len(previewSensitiveRules[0].GroupCodes) != 2 || previewSensitiveRules[0].GroupCodes[1] != legacyGroup.Code {
		t.Fatalf("预检不得发布屏蔽词运行时快照: %#v", previewSensitiveRules)
	}
	result, err := MigrateLegacyGroupCodesToIDs()
	if err != nil {
		t.Fatalf("执行迁移失败: %v", err)
	}
	if !result.Executed || len(result.Groups) != 1 {
		t.Fatalf("执行结果异常: %#v", result)
	}
	if result.Warning != "" {
		t.Fatalf("成功执行后不应保留仅用于预检的部署警告: %q", result.Warning)
	}

	var migrated Group
	if err := DB.First(&migrated, legacyGroup.Id).Error; err != nil {
		t.Fatal(err)
	}
	if migrated.Code != targetCode {
		t.Fatalf("分组 code 未跟随 ID: %q", migrated.Code)
	}
	var alias GroupAlias
	if err := DB.First(&alias, "alias = ?", legacyGroup.Code).Error; err != nil {
		t.Fatal(err)
	}
	if alias.GroupId != legacyGroup.Id {
		t.Fatalf("历史别名归属错误: %#v", alias)
	}
	if err := DB.First(&channel, channel.Id).Error; err != nil {
		t.Fatal(err)
	}
	if channel.Group != targetCode+",default" {
		t.Fatalf("渠道镜像错误: %q", channel.Group)
	}
	if err := DB.Unscoped().First(&token, token.Id).Error; err != nil {
		t.Fatal(err)
	}
	if token.Group != targetCode+",default" || token.GetGroupRatioLimitsMap()[targetCode] != limit {
		t.Fatalf("令牌镜像或倍率保护错误: group=%q limits=%q", token.Group, token.GroupRatioLimits)
	}
	if err := DB.First(&user, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if user.Group != targetCode {
		t.Fatalf("用户分组镜像错误: %q", user.Group)
	}
	ability = Ability{}
	if err := DB.First(&ability, "group_id = ?", legacyGroup.Id).Error; err != nil {
		t.Fatal(err)
	}
	if ability.Group != targetCode {
		t.Fatalf("能力分组镜像错误: %q", ability.Group)
	}
	if err := DB.First(&plan, plan.Id).Error; err != nil {
		t.Fatal(err)
	}
	if plan.UpgradeGroup != targetCode {
		t.Fatalf("套餐升级分组错误: %q", plan.UpgradeGroup)
	}
	if err := DB.First(&subscription, subscription.Id).Error; err != nil {
		t.Fatal(err)
	}
	if subscription.UpgradeGroup != targetCode || subscription.PrevUserGroup != targetCode {
		t.Fatalf("订阅分组快照错误: %#v", subscription)
	}
	if err := DB.First(&auditConfig, "id = ?", PromptAuditConfigID).Error; err != nil {
		t.Fatal(err)
	}
	var upstreamGroupCodes []string
	if err := common.UnmarshalJsonStr(auditConfig.UpstreamPolicyGroupCodes, &upstreamGroupCodes); err != nil {
		t.Fatal(err)
	}
	var exemptGroupCodes []string
	if err := common.UnmarshalJsonStr(auditConfig.CyberPolicyAutoBanExemptGroupCodes, &exemptGroupCodes); err != nil {
		t.Fatal(err)
	}
	if len(upstreamGroupCodes) != 1 || upstreamGroupCodes[0] != targetCode ||
		len(exemptGroupCodes) != 1 || exemptGroupCodes[0] != targetCode {
		t.Fatalf("安全审计分组引用未同步迁移: scope=%v exempt=%v", upstreamGroupCodes, exemptGroupCodes)
	}
	if auditConfig.ConfigVersion != initialAuditConfigVersion+1 {
		t.Fatalf("安全审计配置版本应只递增一次: got=%d want=%d", auditConfig.ConfigVersion, initialAuditConfigVersion+1)
	}
	var sensitiveRulesOption Option
	if err := DB.First(&sensitiveRulesOption, commonKeyCol+" = ?", PromptAuditOptionSensitiveRules).Error; err != nil {
		t.Fatal(err)
	}
	migratedSensitiveRules, err := setting.ParseSensitiveRulesJSONString(sensitiveRulesOption.Value)
	if err != nil {
		t.Fatal(err)
	}
	if len(migratedSensitiveRules) != 2 ||
		len(migratedSensitiveRules[0].GroupCodes) != 2 ||
		migratedSensitiveRules[0].GroupCodes[0] != targetCode ||
		migratedSensitiveRules[0].GroupCodes[1] != defaultGroup.Code {
		t.Fatalf("屏蔽词分组规则未同步迁移: %#v", migratedSensitiveRules)
	}
	if migratedSensitiveRules[1].TargetType != setting.SensitiveRuleTargetChannels ||
		len(migratedSensitiveRules[1].ChannelIds) != 1 || migratedSensitiveRules[1].ChannelIds[0] != 7 {
		t.Fatalf("非分组屏蔽词规则不应被改写: %#v", migratedSensitiveRules[1])
	}
	runtimeSensitiveRules := setting.GetSensitivePolicySnapshot().Rules
	if len(runtimeSensitiveRules) != 2 ||
		len(runtimeSensitiveRules[0].GroupCodes) != 2 || runtimeSensitiveRules[0].GroupCodes[0] != targetCode {
		t.Fatalf("迁移成功后未发布屏蔽词运行时快照: %#v", runtimeSensitiveRules)
	}
	var historicalEvent PromptAuditEvent
	if err := DB.First(&historicalEvent, historicalEvents[0].Id).Error; err != nil {
		t.Fatal(err)
	}
	if historicalEvent.GroupCode != legacyGroup.Code {
		t.Fatalf("审计事件必须保留发生时的分组快照: %q", historicalEvent.GroupCode)
	}

	windowStart, windowEnd := now-1, now+1
	counts, err := CountCyberPolicyEventsByUsers([]int{user.Id}, windowStart, windowEnd, PromptAuditCyberPolicyScope{
		TargetType: "groups", GroupCodes: []string{targetCode},
	})
	if err != nil {
		t.Fatal(err)
	}
	if counts[user.Id] != 1 {
		t.Fatalf("迁移后的官方风控范围应识别历史 code 事件: %v", counts)
	}
	whitelistScope := PromptAuditCyberPolicyScope{TargetType: "all", ExemptGroupCodes: []string{targetCode}}
	counts, err = CountCyberPolicyEventsByUsers([]int{user.Id}, windowStart, windowEnd, whitelistScope)
	if err != nil {
		t.Fatal(err)
	}
	if counts[user.Id] != 1 {
		t.Fatalf("迁移后的白名单应排除历史 code 事件: %v", counts)
	}
	count, disabled, err := DisableCommonUserOnCyberPolicyThreshold(user.Id, windowStart, windowEnd, 2, whitelistScope)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || disabled {
		t.Fatalf("历史白名单事件不得导致误封: count=%d disabled=%v", count, disabled)
	}

	postMigrationEvent := PromptAuditEvent{
		UserId: user.Id, GroupId: legacyGroup.Id, GroupCode: targetCode,
		Source: promptAuditUpstreamPolicySource, ErrorCode: promptAuditCyberPolicyCode,
		CreatedAt: now, Categories: "[]", MatchedScanners: "[]", UnknownCategories: "[]",
	}
	if err := DB.Create(&postMigrationEvent).Error; err != nil {
		t.Fatal(err)
	}
	staleWhitelistScope := PromptAuditCyberPolicyScope{TargetType: "all", ExemptGroupCodes: []string{legacyGroup.Code}}
	counts, err = CountCyberPolicyEventsByUsers([]int{user.Id}, windowStart, windowEnd, staleWhitelistScope)
	if err != nil {
		t.Fatal(err)
	}
	if counts[user.Id] != 1 {
		t.Fatalf("迁移期间的旧配置缓存也必须排除新 code 事件: %v", counts)
	}
	count, disabled, err = DisableCommonUserOnCyberPolicyThreshold(user.Id, windowStart, windowEnd, 2, staleWhitelistScope)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || disabled {
		t.Fatalf("旧白名单缓存不得因新 code 事件误封用户: count=%d disabled=%v", count, disabled)
	}
	counts, err = CountCyberPolicyEventsByUsers([]int{user.Id}, windowStart, windowEnd, PromptAuditCyberPolicyScope{
		TargetType: "groups", GroupCodes: []string{legacyGroup.Code},
	})
	if err != nil {
		t.Fatal(err)
	}
	if counts[user.Id] != 2 {
		t.Fatalf("旧官方风控范围缓存应同时匹配迁移前后事件: %v", counts)
	}
	var topup Option
	if err := DB.First(&topup, commonKeyCol+" = ?", "TopupGroupRatio").Error; err != nil {
		t.Fatal(err)
	}
	if topup.Value != `{"`+targetCode+`":2}` {
		t.Fatalf("充值倍率选项错误: %s", topup.Value)
	}

	repeated, err := PreviewGroupCodeMigration()
	if err != nil {
		t.Fatal(err)
	}
	if !repeated.CanExecute || len(repeated.Groups) != 0 {
		t.Fatalf("迁移应幂等: %#v", repeated)
	}
	if err := DB.First(&auditConfig, "id = ?", PromptAuditConfigID).Error; err != nil {
		t.Fatal(err)
	}
	if auditConfig.ConfigVersion != initialAuditConfigVersion+1 {
		t.Fatalf("幂等预检不得再次递增安全审计配置版本: %d", auditConfig.ConfigVersion)
	}
}

func TestMigrateLegacyGroupCodesToIDsBumpsConfigVersionForSensitiveRulesOnly(t *testing.T) {
	prepareGroupCodeMigrationTestDB(t)
	legacyGroup := Group{Code: "legacy-sensitive", Name: "屏蔽词分组", Ratio: 1, Status: GroupStatusActive}
	if err := DB.Create(&legacyGroup).Error; err != nil {
		t.Fatal(err)
	}
	rulesValue := `{"rules":[{"id":"sensitive-only","name":"屏蔽词规则","enabled":true,"action":"block","scope":"request","keywords":["blocked"],"target_type":"groups","group_codes":["legacy-sensitive"]}]}`
	if err := DB.Create(&Option{Key: PromptAuditOptionSensitiveRules, Value: rulesValue}).Error; err != nil {
		t.Fatal(err)
	}
	previousSensitivePolicy := setting.GetSensitivePolicySnapshot()
	t.Cleanup(func() { setting.ReplaceSensitivePolicySnapshot(previousSensitivePolicy) })
	if err := setting.UpdateSensitiveRulesByJSONString(rulesValue); err != nil {
		t.Fatal(err)
	}
	var config PromptAuditConfig
	if err := DB.First(&config, "id = ?", PromptAuditConfigID).Error; err != nil {
		t.Fatal(err)
	}
	initialVersion := config.ConfigVersion

	if _, err := MigrateLegacyGroupCodesToIDs(); err != nil {
		t.Fatal(err)
	}
	if err := DB.First(&config, "id = ?", PromptAuditConfigID).Error; err != nil {
		t.Fatal(err)
	}
	if config.ConfigVersion != initialVersion+1 {
		t.Fatalf("仅迁移屏蔽词分组规则也必须递增配置版本: got=%d want=%d", config.ConfigVersion, initialVersion+1)
	}
	if config.UpstreamPolicyGroupCodes != "[]" || config.CyberPolicyAutoBanExemptGroupCodes != "[]" {
		t.Fatalf("仅迁移屏蔽词规则不应改写其他安全审计范围: %#v", config)
	}
	var option Option
	if err := DB.First(&option, commonKeyCol+" = ?", PromptAuditOptionSensitiveRules).Error; err != nil {
		t.Fatal(err)
	}
	rules, err := setting.ParseSensitiveRulesJSONString(option.Value)
	if err != nil {
		t.Fatal(err)
	}
	targetCode := strconv.Itoa(legacyGroup.Id)
	if len(rules) != 1 || len(rules[0].GroupCodes) != 1 || rules[0].GroupCodes[0] != targetCode {
		t.Fatalf("屏蔽词分组规则未迁移到稳定 code: %#v", rules)
	}
	runtimeRules := setting.GetSensitivePolicySnapshot().Rules
	if len(runtimeRules) != 1 || len(runtimeRules[0].GroupCodes) != 1 || runtimeRules[0].GroupCodes[0] != targetCode {
		t.Fatalf("屏蔽词运行时快照未同步: %#v", runtimeRules)
	}
}

func TestGroupCodeMigrationRejectsInvalidSensitiveRulesAndLeavesStateUntouched(t *testing.T) {
	prepareGroupCodeMigrationTestDB(t)
	legacyGroup := Group{Code: "legacy-invalid-sensitive", Name: "非法屏蔽词规则", Ratio: 1, Status: GroupStatusActive}
	if err := DB.Create(&legacyGroup).Error; err != nil {
		t.Fatal(err)
	}
	const invalidRules = `{"rules":[`
	if err := DB.Create(&Option{Key: PromptAuditOptionSensitiveRules, Value: invalidRules}).Error; err != nil {
		t.Fatal(err)
	}
	var config PromptAuditConfig
	if err := DB.First(&config, "id = ?", PromptAuditConfigID).Error; err != nil {
		t.Fatal(err)
	}
	initialVersion := config.ConfigVersion

	preview, err := PreviewGroupCodeMigration()
	if err != nil {
		t.Fatal(err)
	}
	if preview.CanExecute || len(preview.Blockers) == 0 {
		t.Fatalf("非法 SensitiveRules 必须阻止预检: %#v", preview)
	}
	if _, err := MigrateLegacyGroupCodesToIDs(); err == nil {
		t.Fatal("执行阶段必须重新预检并拒绝非法 SensitiveRules")
	}
	var storedGroup Group
	if err := DB.First(&storedGroup, legacyGroup.Id).Error; err != nil {
		t.Fatal(err)
	}
	if storedGroup.Code != legacyGroup.Code {
		t.Fatalf("失败迁移不得修改分组 code: %q", storedGroup.Code)
	}
	var option Option
	if err := DB.First(&option, commonKeyCol+" = ?", PromptAuditOptionSensitiveRules).Error; err != nil {
		t.Fatal(err)
	}
	if option.Value != invalidRules {
		t.Fatalf("失败迁移不得改写 SensitiveRules: %q", option.Value)
	}
	if err := DB.First(&config, "id = ?", PromptAuditConfigID).Error; err != nil {
		t.Fatal(err)
	}
	if config.ConfigVersion != initialVersion {
		t.Fatalf("失败迁移不得递增配置版本: %d", config.ConfigVersion)
	}
}

func TestPreviewGroupCodeMigrationBlocksAbilityTargetCollision(t *testing.T) {
	prepareGroupCodeMigrationTestDB(t)
	defaultGroup := Group{Code: "default", Name: "默认", Ratio: 1, Status: GroupStatusActive}
	legacyGroup := Group{Code: "group_2", Name: "特价", Ratio: 0.8, Status: GroupStatusActive}
	if err := DB.Create(&defaultGroup).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Create(&legacyGroup).Error; err != nil {
		t.Fatal(err)
	}
	targetCode := strconv.Itoa(legacyGroup.Id)
	abilities := []Ability{
		{Group: legacyGroup.Code, GroupId: legacyGroup.Id, Model: "gpt-test", ChannelId: 9, Enabled: true},
		{Group: targetCode, GroupId: 999, Model: "gpt-test", ChannelId: 9, Enabled: true},
	}
	if err := DB.Create(&abilities).Error; err != nil {
		t.Fatal(err)
	}

	preview, err := PreviewGroupCodeMigration()
	if err != nil {
		t.Fatal(err)
	}
	if preview.CanExecute || len(preview.Blockers) == 0 {
		t.Fatalf("能力目标主键冲突未阻止迁移: %#v", preview)
	}
	if _, err := MigrateLegacyGroupCodesToIDs(); err == nil {
		t.Fatal("执行阶段必须重新预检并拒绝能力目标主键冲突")
	}
	var stored Group
	if err := DB.First(&stored, legacyGroup.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Code != legacyGroup.Code {
		t.Fatalf("失败迁移不应修改分组标识: %q", stored.Code)
	}
}

func TestPreviewGroupCodeMigrationKeepsVirtualAutoCode(t *testing.T) {
	prepareGroupCodeMigrationTestDB(t)
	groups := []Group{
		{Code: "default", Name: "默认", Ratio: 1, Status: GroupStatusActive},
		{Code: TokenGroupModeAuto, Name: "历史自动分组", Ratio: 1, Status: GroupStatusActive},
	}
	if err := DB.Create(&groups).Error; err != nil {
		t.Fatal(err)
	}

	preview, err := PreviewGroupCodeMigration()
	if err != nil {
		t.Fatal(err)
	}
	if !preview.CanExecute || len(preview.Groups) != 0 {
		t.Fatalf("虚拟 auto 不应进入实体分组 code 迁移: %#v", preview)
	}
}

func TestPreviewGroupCodeMigrationBlocksTargetAliasCollision(t *testing.T) {
	prepareGroupCodeMigrationTestDB(t)
	defaultGroup := Group{Code: "default", Name: "默认", Ratio: 1, Status: GroupStatusActive}
	legacyGroup := Group{Code: "group_2", Name: "特价", Ratio: 0.8, Status: GroupStatusActive}
	if err := DB.Create(&defaultGroup).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Create(&legacyGroup).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Create(&GroupAlias{Alias: strconv.Itoa(legacyGroup.Id), GroupId: defaultGroup.Id}).Error; err != nil {
		t.Fatal(err)
	}

	preview, err := PreviewGroupCodeMigration()
	if err != nil {
		t.Fatal(err)
	}
	if preview.CanExecute || len(preview.Blockers) == 0 {
		t.Fatalf("目标别名冲突未阻止迁移: %#v", preview)
	}
	if _, err := MigrateLegacyGroupCodesToIDs(); err == nil {
		t.Fatal("执行阶段必须重新预检并拒绝冲突")
	}
	var stored Group
	if err := DB.First(&stored, legacyGroup.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Code != legacyGroup.Code {
		t.Fatalf("失败迁移不应修改数据: %q", stored.Code)
	}
}

func TestGroupCodeMigrationRejectsInvalidPromptAuditGroupReferencesAndRollsBack(t *testing.T) {
	prepareGroupCodeMigrationTestDB(t)
	legacyGroup := Group{Code: "legacy-vip", Name: "VIP", Ratio: 1, Status: GroupStatusActive}
	if err := DB.Create(&legacyGroup).Error; err != nil {
		t.Fatal(err)
	}
	var config PromptAuditConfig
	if err := DB.First(&config, "id = ?", PromptAuditConfigID).Error; err != nil {
		t.Fatal(err)
	}
	initialVersion := config.ConfigVersion
	if err := DB.Model(&PromptAuditConfig{}).
		Where("id = ?", PromptAuditConfigID).
		Updates(map[string]interface{}{
			"upstream_policy_group_codes":              `["legacy-vip"]`,
			"cyber_policy_auto_ban_exempt_group_codes": `[`,
		}).Error; err != nil {
		t.Fatal(err)
	}

	preview, err := PreviewGroupCodeMigration()
	if err != nil {
		t.Fatal(err)
	}
	if preview.CanExecute || len(preview.Blockers) == 0 {
		t.Fatalf("非法安全审计分组 JSON 必须阻止预检: %#v", preview)
	}
	if _, err := MigrateLegacyGroupCodesToIDs(); err == nil {
		t.Fatal("执行阶段必须重新预检并拒绝非法安全审计分组 JSON")
	}

	var storedGroup Group
	if err := DB.First(&storedGroup, legacyGroup.Id).Error; err != nil {
		t.Fatal(err)
	}
	if storedGroup.Code != legacyGroup.Code {
		t.Fatalf("失败迁移不得修改分组 code: %q", storedGroup.Code)
	}
	if err := DB.First(&config, "id = ?", PromptAuditConfigID).Error; err != nil {
		t.Fatal(err)
	}
	if config.UpstreamPolicyGroupCodes != `["legacy-vip"]` ||
		config.CyberPolicyAutoBanExemptGroupCodes != `[` || config.ConfigVersion != initialVersion {
		t.Fatalf("失败迁移不得部分改写安全审计配置: %#v", config)
	}
	var aliasCount int64
	if err := DB.Model(&GroupAlias{}).Where("alias = ?", legacyGroup.Code).Count(&aliasCount).Error; err != nil {
		t.Fatal(err)
	}
	if aliasCount != 0 {
		t.Fatalf("失败迁移不得留下历史别名: %d", aliasCount)
	}
}

func TestApplyPromptAuditGroupCodeMigrationCASConflictRollsBackTransaction(t *testing.T) {
	prepareGroupCodeMigrationTestDB(t)
	group := Group{Code: "legacy-cas", Name: "CAS", Ratio: 1, Status: GroupStatusActive}
	if err := DB.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	oldRules := `{"rules":[{"id":"cas","name":"CAS","enabled":true,"action":"block","scope":"request","keywords":["blocked"],"target_type":"groups","group_codes":["legacy-cas"]}]}`
	if err := DB.Create(&Option{Key: PromptAuditOptionSensitiveRules, Value: oldRules}).Error; err != nil {
		t.Fatal(err)
	}
	var config PromptAuditConfig
	if err := DB.First(&config, "id = ?", PromptAuditConfigID).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Model(&PromptAuditConfig{}).
		Where("id = ?", PromptAuditConfigID).
		Update("config_version", config.ConfigVersion+1).Error; err != nil {
		t.Fatal(err)
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Group{}).Where("id = ?", group.Id).Update("code", strconv.Itoa(group.Id)).Error; err != nil {
			return err
		}
		newRules, err := rewriteSensitiveRuleGroupCodes(oldRules, map[string]string{group.Code: strconv.Itoa(group.Id)})
		if err != nil {
			return err
		}
		if err := tx.Model(&Option{}).
			Where(commonKeyCol+" = ?", PromptAuditOptionSensitiveRules).
			Update("value", newRules).Error; err != nil {
			return err
		}
		return applyPromptAuditGroupCodeMigration(tx, &promptAuditGroupCodeMigrationUpdate{
			expectedVersion: config.ConfigVersion,
			values: map[string]interface{}{
				"config_version":                           config.ConfigVersion + 1,
				"upstream_policy_group_codes":              `[` + strconv.Quote(strconv.Itoa(group.Id)) + `]`,
				"cyber_policy_auto_ban_exempt_group_codes": `[` + strconv.Quote(strconv.Itoa(group.Id)) + `]`,
			},
		})
	})
	if !errors.Is(err, ErrPromptAuditConfigConflict) {
		t.Fatalf("过期配置版本应触发 CAS 冲突: %v", err)
	}
	var stored Group
	if err := DB.First(&stored, group.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Code != group.Code {
		t.Fatalf("CAS 冲突必须回滚同事务内的分组修改: %q", stored.Code)
	}
	var option Option
	if err := DB.First(&option, commonKeyCol+" = ?", PromptAuditOptionSensitiveRules).Error; err != nil {
		t.Fatal(err)
	}
	if option.Value != oldRules {
		t.Fatalf("CAS 冲突必须回滚同事务内的 SensitiveRules: %q", option.Value)
	}
}
