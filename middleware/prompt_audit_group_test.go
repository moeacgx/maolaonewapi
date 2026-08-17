package middleware

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type promptAuditAuthenticatedGroupResponse struct {
	ShouldAudit   bool   `json:"should_audit"`
	GroupId       int    `json:"group_id"`
	GroupCode     string `json:"group_code"`
	GroupName     string `json:"group_name"`
	ContextGroup  int    `json:"context_group_id"`
	ContextEmail  string `json:"context_email"`
	ContextUserId int    `json:"context_user_id"`
}

func TestPromptAuditResolveGroupScopeUsesExplicitCandidates(t *testing.T) {
	c := promptAuditGroupTestContext()
	common.SetContextKey(c, constant.ContextKeyUserGroupId, 1)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default,vip")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "default,vip")
	common.SetContextKey(c, constant.ContextKeyTokenGroupMode, "explicit")
	common.SetContextKey(c, constant.ContextKeyTokenGroupIds, []int{1, 2})
	common.SetContextKey(c, constant.ContextKeyTokenGroupDetails, []model.GroupReference{
		{Id: 1, Code: "default", Name: "默认分组"},
		{Id: 2, Code: "vip", Name: "贵宾分组"},
	})

	shouldAudit, groupId, groupCode, groupName := promptAuditResolveGroupScope(c, &service.PromptAuditConfig{GroupIds: []int{2}})
	require.True(t, shouldAudit)
	require.Equal(t, 2, groupId)
	require.Equal(t, "vip", groupCode)
	require.Equal(t, "pre_allocation:vip", groupName)

	shouldAudit, _, _, _ = promptAuditResolveGroupScope(c, &service.PromptAuditConfig{GroupIds: []int{3}})
	require.False(t, shouldAudit)
}

func TestPromptAuditResolveGroupScopeAuditsAutoFailSafe(t *testing.T) {
	c := promptAuditGroupTestContext()
	common.SetContextKey(c, constant.ContextKeyUserGroupId, 1)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "auto")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "auto")
	common.SetContextKey(c, constant.ContextKeyTokenGroupMode, "auto")

	shouldAudit, groupId, groupCode, groupName := promptAuditResolveGroupScope(c, &service.PromptAuditConfig{GroupIds: []int{99}})
	require.True(t, shouldAudit)
	require.Zero(t, groupId)
	require.Empty(t, groupCode)
	require.Equal(t, "pre_allocation:auto", groupName)
}

func TestPromptAuditResolveGroupScopeUsesInheritedUserGroup(t *testing.T) {
	c := promptAuditGroupTestContext()
	common.SetContextKey(c, constant.ContextKeyUserGroupId, 7)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "standard")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "standard")
	common.SetContextKey(c, constant.ContextKeyTokenGroupMode, "inherit")

	shouldAudit, groupId, groupCode, groupName := promptAuditResolveGroupScope(c, &service.PromptAuditConfig{GroupIds: []int{7}})
	require.True(t, shouldAudit)
	require.Equal(t, 7, groupId)
	require.Empty(t, groupCode)
	require.Equal(t, "standard", groupName)

	shouldAudit, _, _, _ = promptAuditResolveGroupScope(c, &service.PromptAuditConfig{GroupIds: []int{8}})
	require.False(t, shouldAudit)
}

func TestPromptAuditResolveGroupScopeFailsSafeForLegacyUserCacheWithoutGroupID(t *testing.T) {
	c := promptAuditGroupTestContext()
	common.SetContextKey(c, constant.ContextKeyUserGroup, "standard")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "standard")
	common.SetContextKey(c, constant.ContextKeyTokenGroupMode, "inherit")

	shouldAudit, groupId, groupCode, groupName := promptAuditResolveGroupScope(c, &service.PromptAuditConfig{
		AllGroups: false, GroupIds: []int{99},
	})
	require.True(t, shouldAudit)
	require.Zero(t, groupId)
	require.Empty(t, groupCode)
	require.Equal(t, "pre_allocation:standard", groupName)
}

func TestPromptAuditResolveGroupScopeFailsSafeForLegacyMultiGroupWithoutIds(t *testing.T) {
	c := promptAuditGroupTestContext()
	common.SetContextKey(c, constant.ContextKeyUserGroupId, 1)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "default,vip")
	common.SetContextKey(c, constant.ContextKeyTokenGroupMode, "inherit")

	shouldAudit, groupId, groupCode, groupName := promptAuditResolveGroupScope(c, &service.PromptAuditConfig{
		AllGroups: false, GroupIds: []int{99},
	})
	require.True(t, shouldAudit)
	require.Zero(t, groupId)
	require.Empty(t, groupCode)
	require.Equal(t, "pre_allocation:default,vip", groupName)
}

func TestPromptAuditResolveGroupScopeUsesPlaygroundRequestedGroup(t *testing.T) {
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "prompt-audit-groups.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Group{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	vip := model.Group{Code: "vip", Name: "VIP", Status: model.GroupStatusActive}
	require.NoError(t, db.Create(&vip).Error)

	c := promptAuditGroupTestContext()
	common.SetContextKey(c, constant.ContextKeyUserGroupId, 1)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenGroupMode, "inherit")

	shouldAudit, groupId, groupCode, groupName := promptAuditResolveGroupScope(
		c, &service.PromptAuditConfig{GroupIds: []int{vip.Id}}, "vip",
	)
	require.True(t, shouldAudit)
	require.Equal(t, vip.Id, groupId)
	require.Equal(t, vip.Code, groupCode)
	require.Equal(t, "pre_allocation:vip", groupName)

	shouldAudit, _, _, _ = promptAuditResolveGroupScope(
		c, &service.PromptAuditConfig{GroupIds: []int{vip.Id + 1}}, "vip",
	)
	require.False(t, shouldAudit)
}

func TestPromptAuditAuthMiddlewareWritesStableUserGroupContext(t *testing.T) {
	setupDashboardAuthMiddlewareTest(t)
	pat := "prompt-audit-user-pat"
	user := &model.User{
		Id: 2088, Username: "prompt-audit-user", Email: "prompt-audit@example.com",
		Group: "vip", GroupId: 73, Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, AccessToken: &pat, AuthVersion: 1,
		AffCode: "prompt-audit-user-aff",
	}
	require.NoError(t, model.DB.Create(user).Error)
	bundle, err := service.CreateLoginSession(user.Id, "password", "127.0.0.1", "prompt-audit-group-test")
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	router := gin.New()

	handler := func(c *gin.Context) {
		shouldAudit, groupId, groupCode, groupName := promptAuditResolveGroupScope(
			c, &service.PromptAuditConfig{GroupIds: []int{user.GroupId}},
		)
		c.JSON(http.StatusOK, promptAuditAuthenticatedGroupResponse{
			ShouldAudit:   shouldAudit,
			GroupId:       groupId,
			GroupCode:     groupCode,
			GroupName:     groupName,
			ContextGroup:  common.GetContextKeyInt(c, constant.ContextKeyUserGroupId),
			ContextEmail:  common.GetContextKeyString(c, constant.ContextKeyUserEmail),
			ContextUserId: common.GetContextKeyInt(c, constant.ContextKeyUserId),
		})
	}
	router.POST("/playground", UserAuth(), handler)
	router.POST("/canvas", UserAuth(), handler)

	for _, testCase := range []struct {
		name  string
		path  string
		token string
	}{
		{name: "playground session jwt", path: "/playground", token: bundle.AccessToken},
		{name: "canvas personal access token", path: "/canvas", token: pat},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, testCase.path, nil)
			request.Header.Set("Authorization", "Bearer "+testCase.token)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

			var response promptAuditAuthenticatedGroupResponse
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.True(t, response.ShouldAudit)
			require.Equal(t, user.GroupId, response.GroupId)
			require.Empty(t, response.GroupCode)
			require.Equal(t, user.Group, response.GroupName)
			require.Equal(t, user.GroupId, response.ContextGroup)
			require.Equal(t, user.Email, response.ContextEmail)
			require.Equal(t, user.Id, response.ContextUserId)
		})
	}
}

func promptAuditGroupTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	return c
}
