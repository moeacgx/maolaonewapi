package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
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

func TestCyberPolicyAutoBanUsesCurrentChannelGroupScope(t *testing.T) {
	db := setupCyberPolicyAutoBanTest(t)
	require.NoError(t, db.AutoMigrate(&model.Group{}))
	group := model.Group{Id: 7, Code: "vip", Name: "贵宾分组", Status: model.GroupStatusActive}
	require.NoError(t, db.Create(&group).Error)

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

	user := model.User{Username: "auto-ban-group-scope", Email: "group-scope@example.com", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
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
