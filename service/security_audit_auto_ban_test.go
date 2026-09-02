package service

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCyberPolicyAutoBanTest(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupPromptAuditServiceTest(t, false, false, nil)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}))
	oldLogDB := model.LOG_DB
	oldRedisEnabled := common.RedisEnabled
	model.LOG_DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.LOG_DB = oldLogDB
		common.RedisEnabled = oldRedisEnabled
	})
	return db
}

func configureCyberPolicyAutoBan(t *testing.T, enabled bool, threshold, windowHours int) {
	t.Helper()
	row, endpoints, err := model.LoadPromptAuditConfig()
	require.NoError(t, err)
	row.CyberPolicyAutoBanEnabled = enabled
	row.CyberPolicyBanThreshold = threshold
	row.CyberPolicyWindowHours = windowHours
	require.NoError(t, model.SavePromptAuditConfig(row.ConfigVersion, row, endpoints))
	InvalidatePromptAuditConfig()
}

func configureCyberSessionBlock(t *testing.T, enabled bool, ttlSeconds int) {
	t.Helper()
	row, endpoints, err := model.LoadPromptAuditConfig()
	require.NoError(t, err)
	row.UpstreamPolicyEnabled = true
	row.CyberSessionBlockEnabled = enabled
	row.CyberSessionBlockTTLSeconds = ttlSeconds
	require.NoError(t, model.SavePromptAuditConfig(row.ConfigVersion, row, endpoints))
	InvalidatePromptAuditConfig()
}

func newCyberSessionTestContext(tokenID int, requestID string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Set(common.RequestIdKey, requestID)
	common.SetContextKey(c, constant.ContextKeyTokenId, tokenID)
	return c
}

func recordCyberPolicyForUser(t *testing.T, user model.User, requestId string) {
	recordCyberPolicyForUserInGroup(t, user, requestId, "")
}

func recordCyberPolicyForUserInGroup(t *testing.T, user model.User, requestId, groupCode string) {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Set(common.RequestIdKey, requestId)
	if groupCode != "" {
		common.SetContextKey(c, constant.ContextKeySelectedChannelGroup, groupCode)
		common.SetContextKey(c, constant.ContextKeySelectedChannel, &model.Channel{Id: 31, Name: "分组渠道"})
	}
	common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
	common.SetContextKey(c, constant.ContextKeyUserName, user.Username)
	common.SetContextKey(c, constant.ContextKeyUserEmail, user.Email)
	snapshot, err := BuildPromptAuditTextSnapshot(PromptAuditRequest{
		RequestId: requestId, UserId: user.Id, Username: user.Username, UserEmail: user.Email,
		Endpoint: "/v1/responses", Protocol: "openai_responses", Model: "gpt-test",
	}, "自动处置测试提示词")
	require.NoError(t, err)
	SetSecurityAuditRequestSnapshot(c, snapshot)
	require.True(t, RecordUpstreamPolicyPayload(c,
		[]byte(`{"response":{"error":{"code":"cyber_policy"}}}`), "response"))
}
func resetCyberSessionBlocksForTest(t *testing.T) {
	t.Helper()
	cyberSessionBlocks.mu.Lock()
	previousEntries := cyberSessionBlocks.entries
	cyberSessionBlocks.entries = nil
	cyberSessionBlocks.mu.Unlock()
	t.Cleanup(func() {
		cyberSessionBlocks.mu.Lock()
		cyberSessionBlocks.entries = previousEntries
		cyberSessionBlocks.mu.Unlock()
	})
}

func TestCyberSessionBlockUsesExplicitSessionIdentityAndApiKeyIsolation(t *testing.T) {
	db := setupCyberPolicyAutoBanTest(t)
	resetCyberSessionBlocksForTest(t)
	configureCyberSessionBlock(t, true, 60)

	body := []byte(`{"model":"gpt-test","prompt_cache_key":"session-a"}`)
	initial := newCyberSessionTestContext(777, "req-cyber-session-block-1")
	CacheCyberSessionBlockKey(initial, body)
	snapshot, err := BuildPromptAuditTextSnapshot(PromptAuditRequest{
		RequestId: "req-cyber-session-block-1", TokenId: 777,
		Endpoint: "/v1/responses", Protocol: "openai_responses", Model: "gpt-test",
	}, "cyber session block source prompt")
	require.NoError(t, err)
	SetSecurityAuditRequestSnapshot(initial, snapshot)
	require.True(t, RecordUpstreamPolicyPayload(initial,
		[]byte(`{"response":{"error":{"code":"cyber_policy"}}}`), "response"))

	cfg, err := GetPromptAuditConfig(initial.Request.Context())
	require.NoError(t, err)
	repeat := newCyberSessionTestContext(777, "req-cyber-session-block-repeat")
	require.True(t, IsCyberSessionBlocked(repeat, cfg, body))

	otherToken := newCyberSessionTestContext(778, "req-cyber-session-block-other-token")
	require.False(t, IsCyberSessionBlocked(otherToken, cfg, body))

	otherSession := newCyberSessionTestContext(777, "req-cyber-session-block-other-session")
	require.False(t, IsCyberSessionBlocked(otherSession, cfg,
		[]byte(`{"model":"gpt-test","prompt_cache_key":"session-b"}`)))

	withoutSession := newCyberSessionTestContext(777, "req-cyber-session-block-no-session")
	require.False(t, IsCyberSessionBlocked(withoutSession, cfg,
		[]byte(`{"model":"gpt-test","messages":[{"role":"user","content":"hello"}]}`)))

	var count int64
	require.NoError(t, db.Model(&model.PromptAuditEvent{}).
		Where("source = ? AND error_code = ?", PromptAuditSourceUpstreamPolicy, upstreamCyberPolicyCode).
		Count(&count).Error)
	require.EqualValues(t, 1, count, "local session-block checks must not create new upstream_policy events")
}

func TestCyberSessionBlockHeaderIdentityWorksWithoutBodyFallback(t *testing.T) {
	setupCyberPolicyAutoBanTest(t)
	resetCyberSessionBlocksForTest(t)
	configureCyberSessionBlock(t, true, 60)

	initial := newCyberSessionTestContext(888, "req-cyber-session-header-1")
	initial.Request.Header.Set("X-Session-Id", "explicit-session")
	CacheCyberSessionBlockKey(initial, nil)
	snapshot, err := BuildPromptAuditTextSnapshot(PromptAuditRequest{
		RequestId: "req-cyber-session-header-1", TokenId: 888,
		Endpoint: "/v1/responses", Protocol: "openai_responses", Model: "gpt-test",
	}, "cyber session header source prompt")
	require.NoError(t, err)
	SetSecurityAuditRequestSnapshot(initial, snapshot)
	require.True(t, RecordUpstreamPolicyPayload(initial,
		[]byte(`{"error":{"code":"cyber_policy"}}`), "response"))

	cfg, err := GetPromptAuditConfig(initial.Request.Context())
	require.NoError(t, err)
	repeat := newCyberSessionTestContext(888, "req-cyber-session-header-repeat")
	repeat.Request.Header.Set("X-Session-Id", "explicit-session")
	require.True(t, IsCyberSessionBlocked(repeat, cfg, nil))

	missing := newCyberSessionTestContext(888, "req-cyber-session-header-missing")
	require.False(t, IsCyberSessionBlocked(missing, cfg, nil))
}

func TestSelectedPolicySourcesControlBiologicalRiskActions(t *testing.T) {
	db := setupCyberPolicyAutoBanTest(t)
	resetCyberSessionBlocksForTest(t)
	row, endpoints, err := model.LoadPromptAuditConfig()
	require.NoError(t, err)
	row.UpstreamPolicyEnabled = true
	row.CyberSessionBlockEnabled = true
	row.CyberSessionBlockTTLSeconds = 60
	row.CyberPolicyAutoBanEnabled = true
	row.CyberPolicyBanThreshold = 1
	row.CyberPolicyWindowHours = 720
	sources, marshalErr := common.Marshal([]string{PromptAuditPolicySourceBiologicalRisk})
	require.NoError(t, marshalErr)
	row.PolicyActionSources = string(sources)
	require.NoError(t, model.SavePromptAuditConfig(row.ConfigVersion, row, endpoints))
	InvalidatePromptAuditConfig()

	user := model.User{Username: "biological-risk-action", Email: "biological-risk-action@example.com", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	c := newCyberSessionTestContext(911, "req-biological-risk-action")
	common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
	common.SetContextKey(c, constant.ContextKeyUserName, user.Username)
	common.SetContextKey(c, constant.ContextKeyUserEmail, user.Email)
	body := []byte(`{"model":"gpt-test","prompt_cache_key":"biological-session"}`)
	CacheCyberSessionBlockKey(c, body)
	snapshot, snapshotErr := BuildPromptAuditTextSnapshot(PromptAuditRequest{
		RequestId: "req-biological-risk-action", UserId: user.Id, TokenId: 911,
		Endpoint: "/v1/responses", Protocol: "openai_responses", Model: "gpt-test",
	}, "生物风险来源处置测试")
	require.NoError(t, snapshotErr)
	SetSecurityAuditRequestSnapshot(c, snapshot)
	biologicalErr := types.NewOpenAIError(errors.New("This content was flagged for possible biological risk."), types.ErrorCodeBadResponseStatusCode, 500)
	require.True(t, RecordUpstreamPolicyError(c, biologicalErr, "response"))

	cfg, cfgErr := GetPromptAuditConfig(c.Request.Context())
	require.NoError(t, cfgErr)
	repeat := newCyberSessionTestContext(911, "req-biological-risk-action-repeat")
	require.True(t, IsCyberSessionBlocked(repeat, cfg, body))
	var loaded model.User
	require.NoError(t, db.First(&loaded, "id = ?", user.Id).Error)
	require.Equal(t, common.UserStatusDisabled, loaded.Status)
}

func TestCyberPolicyAutoBanUsesCurrentChannelGroupScope(t *testing.T) {
	db := setupCyberPolicyAutoBanTest(t)
	require.NoError(t, db.AutoMigrate(&model.Group{}))
	group := model.Group{Id: 7, Code: "vip", Name: "贵宾分组", Status: model.GroupStatusActive}
	require.NoError(t, db.Create(&group).Error)
	defaultGroup := model.Group{Code: "default", Name: "默认分组", Status: model.GroupStatusActive}
	require.NoError(t, db.Create(&defaultGroup).Error)

	row, endpoints, err := model.LoadPromptAuditConfig()
	require.NoError(t, err)
	row.CyberPolicyAutoBanEnabled = true
	row.CyberPolicyBanThreshold = 2
	row.CyberPolicyWindowHours = 720
	row.UpstreamPolicyTargetType = PromptAuditUpstreamPolicyTargetGroups
	groupCodes, marshalErr := common.Marshal([]string{"vip"})
	require.NoError(t, marshalErr)
	row.UpstreamPolicyGroupCodes = string(groupCodes)
	require.NoError(t, model.SavePromptAuditConfig(row.ConfigVersion, row, endpoints))
	InvalidatePromptAuditConfig()

	user := model.User{Username: "auto-ban-group-scope", Email: "group-scope@example.com", GroupId: defaultGroup.Id, Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.PromptAuditEvent{
		UserId: user.Id, GroupId: 99, GroupCode: "standard", ChannelId: 10,
		Source: PromptAuditSourceUpstreamPolicy, ErrorCode: upstreamCyberPolicyCode,
		CreatedAt: now, ExpiresAt: now + 3600,
	}).Error)

	recordCyberPolicyForUserInGroup(t, user, "req-group-scope-1", "vip")
	var loaded model.User
	require.NoError(t, db.First(&loaded, "id = ?", user.Id).Error)
	require.Equal(t, common.UserStatusEnabled, loaded.Status)

	recordCyberPolicyForUserInGroup(t, user, "req-group-scope-2", "vip")
	require.NoError(t, db.First(&loaded, "id = ?", user.Id).Error)
	require.Equal(t, common.UserStatusDisabled, loaded.Status)
}

func TestCyberPolicyAutoBanExemptsWhitelistedGroups(t *testing.T) {
	db := setupCyberPolicyAutoBanTest(t)
	require.NoError(t, db.AutoMigrate(&model.Group{}))
	require.NoError(t, db.Create(&model.Group{Id: 8, Code: "trusted", Name: "信任分组", Status: model.GroupStatusActive}).Error)
	defaultGroup := model.Group{Code: "default", Name: "默认分组", Status: model.GroupStatusActive}
	require.NoError(t, db.Create(&defaultGroup).Error)

	row, endpoints, err := model.LoadPromptAuditConfig()
	require.NoError(t, err)
	row.CyberPolicyAutoBanEnabled = true
	row.CyberPolicyBanThreshold = 2
	row.CyberPolicyWindowHours = 720
	exemptGroupCodes, marshalErr := common.Marshal([]string{"trusted"})
	require.NoError(t, marshalErr)
	row.CyberPolicyAutoBanExemptGroupCodes = string(exemptGroupCodes)
	require.NoError(t, model.SavePromptAuditConfig(row.ConfigVersion, row, endpoints))
	// 保存后停用分组不得让白名单失效，否则会静默放大封禁范围。
	require.NoError(t, db.Model(&model.Group{}).Where("code = ?", "trusted").Update("status", model.GroupStatusDisabled).Error)
	InvalidatePromptAuditConfig()

	user := model.User{Username: "auto-ban-whitelist", Email: "whitelist@example.com", GroupId: defaultGroup.Id, Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)

	recordCyberPolicyForUserInGroup(t, user, "req-whitelist-1", "trusted")
	recordCyberPolicyForUserInGroup(t, user, "req-whitelist-2", "trusted")
	var loaded model.User
	require.NoError(t, db.First(&loaded, "id = ?", user.Id).Error)
	require.Equal(t, common.UserStatusEnabled, loaded.Status)

	recordCyberPolicyForUserInGroup(t, user, "req-standard-1", "standard")
	require.NoError(t, db.First(&loaded, "id = ?", user.Id).Error)
	require.Equal(t, common.UserStatusEnabled, loaded.Status)
	recordCyberPolicyForUserInGroup(t, user, "req-standard-2", "standard")
	require.NoError(t, db.First(&loaded, "id = ?", user.Id).Error)
	require.Equal(t, common.UserStatusDisabled, loaded.Status)

	var eventCount int64
	require.NoError(t, db.Model(&model.PromptAuditEvent{}).Where("user_id = ?", user.Id).Count(&eventCount).Error)
	require.EqualValues(t, 4, eventCount, "白名单只免除自动封禁，不能吞掉审计事件")
}

func TestBuildPromptAuditCyberPolicyScopeKeepsSavedWhitelistCodes(t *testing.T) {
	scope, err := BuildPromptAuditCyberPolicyScope(&PromptAuditConfig{
		UpstreamPolicyTargetType:           PromptAuditUpstreamPolicyTargetAll,
		CyberPolicyAutoBanExemptGroupCodes: []string{" retired-trusted ", "retired-trusted"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"retired-trusted"}, scope.ExemptGroupCodes)
}

func TestCyberPolicyAutoBanStaleWhitelistAliasNeverTriggersBan(t *testing.T) {
	db := setupCyberPolicyAutoBanTest(t)
	require.NoError(t, db.AutoMigrate(&model.Group{}, &model.GroupAlias{}))
	group := model.Group{Id: 8, Code: "8", Name: "信任分组", Status: model.GroupStatusActive}
	require.NoError(t, db.Create(&group).Error)
	require.NoError(t, db.Create(&model.GroupAlias{Alias: "trusted", GroupId: group.Id}).Error)
	defaultGroup := model.Group{Code: "default", Name: "默认分组", Status: model.GroupStatusActive}
	require.NoError(t, db.Create(&defaultGroup).Error)

	user := model.User{Username: "auto-ban-stale-whitelist", GroupId: defaultGroup.Id, Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	now := time.Now().Unix()
	priorEvent := model.PromptAuditEvent{
		UserId: user.Id, GroupCode: "standard",
		Source: PromptAuditSourceUpstreamPolicy, ErrorCode: upstreamCyberPolicyCode,
		CreatedAt: now, ExpiresAt: now + 3600,
	}
	require.NoError(t, db.Create(&priorEvent).Error)
	currentWhitelistEvent := model.PromptAuditEvent{
		UserId: user.Id, GroupId: group.Id, GroupCode: group.Code,
		Source: PromptAuditSourceUpstreamPolicy, ErrorCode: upstreamCyberPolicyCode,
		CreatedAt: now, ExpiresAt: now + 3600,
	}
	require.NoError(t, db.Create(&currentWhitelistEvent).Error)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	cfg := &PromptAuditConfig{
		ConfigVersion:                      1,
		UpstreamPolicyEnabled:              true,
		UpstreamPolicyTargetType:           PromptAuditUpstreamPolicyTargetAll,
		CyberPolicyAutoBanEnabled:          true,
		CyberPolicyAutoBanExemptGroupCodes: []string{"trusted"},
		CyberPolicyBanThreshold:            1,
		CyberPolicyWindowHours:             720,
	}
	applyCyberPolicyAutoBan(c, cfg, &currentWhitelistEvent)

	var loaded model.User
	require.NoError(t, db.First(&loaded, "id = ?", user.Id).Error)
	require.Equal(t, common.UserStatusEnabled, loaded.Status,
		"迁移期间旧白名单 code 必须识别新 code 当前事件，不能让该事件触发封禁")
}

func TestUpstreamPolicyGroupScopeAcceptsHistoricalAliasDuringMigration(t *testing.T) {
	db := setupCyberPolicyAutoBanTest(t)
	require.NoError(t, db.AutoMigrate(&model.Group{}, &model.GroupAlias{}))
	group := model.Group{Id: 9, Code: "9", Name: "迁移分组", Status: model.GroupStatusActive}
	require.NoError(t, db.Create(&group).Error)
	require.NoError(t, db.Create(&model.GroupAlias{Alias: "legacy-scope", GroupId: group.Id}).Error)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeySelectedChannelGroup, group.Code)
	require.True(t, upstreamPolicyScopeIncludesSelectedChannel(c, &PromptAuditConfig{
		UpstreamPolicyTargetType: PromptAuditUpstreamPolicyTargetGroups,
		UpstreamPolicyGroupCodes: []string{"legacy-scope"},
	}))
}

func TestCyberPolicyAutoBanCountsOnlyPersistedExactEvents(t *testing.T) {
	db := setupCyberPolicyAutoBanTest(t)
	configureCyberPolicyAutoBan(t, true, 2, 720)
	user := model.User{Username: "auto-ban-common", Email: "common@example.com", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)

	now := time.Now().Unix()
	fixtures := []model.PromptAuditEvent{
		{UserId: user.Id, Source: PromptAuditSourceGuard, ErrorCode: upstreamCyberPolicyCode, CreatedAt: now, ExpiresAt: now + 3600},
		{UserId: user.Id, Source: PromptAuditSourceUpstreamPolicy, ErrorCode: "ordinary_error", CreatedAt: now, ExpiresAt: now + 3600},
		{UserId: user.Id, Source: PromptAuditSourceUpstreamPolicy, ErrorCode: upstreamCyberPolicyCode, CreatedAt: now - 721*3600, ExpiresAt: now + 3600},
		{UserId: user.Id, Source: PromptAuditSourceUpstreamPolicy, ErrorCode: upstreamCyberPolicyCode, CreatedAt: now + 3600, ExpiresAt: now + 7200},
	}
	for i := range fixtures {
		require.NoError(t, db.Create(&fixtures[i]).Error)
	}

	recordCyberPolicyForUser(t, user, "req-auto-ban-1")
	var loaded model.User
	require.NoError(t, db.First(&loaded, "id = ?", user.Id).Error)
	require.Equal(t, common.UserStatusEnabled, loaded.Status)

	recordCyberPolicyForUser(t, user, "req-auto-ban-2")
	require.NoError(t, db.First(&loaded, "id = ?", user.Id).Error)
	require.Equal(t, common.UserStatusDisabled, loaded.Status)
	require.Greater(t, loaded.AuthVersion, user.AuthVersion, "auto-ban must invalidate stale authenticated user snapshots")

	var logCount int64
	require.NoError(t, db.Model(&model.Log{}).
		Where("user_id = ? AND type = ?", user.Id, model.LogTypeManage).Count(&logCount).Error)
	require.EqualValues(t, 1, logCount)
	var log model.Log
	require.NoError(t, db.First(&log, "user_id = ? AND type = ?", user.Id, model.LogTypeManage).Error)
	require.Contains(t, log.Content, "自动禁用用户")
	require.NotContains(t, log.Other, "自动处置测试提示词")

	// 用户已经禁用时，后续精确事件仍可留痕，但不能重复处置或刷管理日志。
	recordCyberPolicyForUser(t, user, "req-auto-ban-3")
	require.NoError(t, db.Model(&model.Log{}).
		Where("user_id = ? AND type = ?", user.Id, model.LogTypeManage).Count(&logCount).Error)
	require.EqualValues(t, 1, logCount)
}

func TestCyberPolicyAutoBanNeverDisablesPrivilegedUsers(t *testing.T) {
	db := setupCyberPolicyAutoBanTest(t)
	configureCyberPolicyAutoBan(t, true, 1, 720)
	users := []model.User{
		{Username: "auto-ban-admin", AffCode: "auto-ban-admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled},
		{Username: "auto-ban-root", AffCode: "auto-ban-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled},
	}
	for i := range users {
		require.NoError(t, db.Create(&users[i]).Error)
		recordCyberPolicyForUser(t, users[i], "req-privileged-"+users[i].Username)
		var loaded model.User
		require.NoError(t, db.First(&loaded, "id = ?", users[i].Id).Error)
		require.Equal(t, common.UserStatusEnabled, loaded.Status)
	}

	var logCount int64
	require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeManage).Count(&logCount).Error)
	require.Zero(t, logCount)
}

func TestCyberPolicyAutoBanRequiresSuccessfulEventPersistence(t *testing.T) {
	db := setupCyberPolicyAutoBanTest(t)
	configureCyberPolicyAutoBan(t, true, 1, 720)
	user := model.User{Username: "auto-ban-persist", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Migrator().DropTable(&model.PromptAuditEvent{}))

	recordCyberPolicyForUser(t, user, "req-persist-failed")
	var loaded model.User
	require.NoError(t, db.First(&loaded, "id = ?", user.Id).Error)
	require.Equal(t, common.UserStatusEnabled, loaded.Status)

	var logCount int64
	require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeManage).Count(&logCount).Error)
	require.Zero(t, logCount)
}
