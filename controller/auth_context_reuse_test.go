package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const authenticatedContextTestGroup = "zz-auth-context-group"

func setupAuthenticatedContextControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldRedisEnabled := common.RedisEnabled
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.Group{},
		&model.SubscriptionPlan{},
		&model.UserSubscription{},
	))
	model.InvalidatePricingCache()

	t.Cleanup(func() {
		model.InvalidatePricingCache()
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.RedisEnabled = oldRedisEnabled
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
	})

	return db
}

func withAuthenticatedContextGroupSettings(t *testing.T) {
	t.Helper()

	oldGroupRatio := ratio_setting.GroupRatio2JSONString()
	oldGroupGroupRatio := ratio_setting.GroupGroupRatio2JSONString()
	oldUserUsableGroups := setting.UserUsableGroups2JSONString()

	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(
		"{\"default\":1,\""+authenticatedContextTestGroup+"\":2}",
	))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(
		"{\""+authenticatedContextTestGroup+"\":{\"default\":3.5,\""+authenticatedContextTestGroup+"\":4.5}}",
	))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(
		"{\"default\":\"Default\"}",
	))
	model.InvalidatePricingCache()

	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(oldGroupGroupRatio))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldUserUsableGroups))
		model.InvalidatePricingCache()
	})
}

func newCanceledControllerRequest(method, target string) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	requestContext, cancel := context.WithCancel(request.Context())
	cancel()
	return request.WithContext(requestContext)
}

func decodeControllerPayload(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	return payload
}

func TestGetSubscriptionUsesAuthenticatedQuotaContext(t *testing.T) {
	db := setupAuthenticatedContextControllerTestDB(t)
	oldDisplayTokenStatEnabled := common.DisplayTokenStatEnabled
	common.DisplayTokenStatEnabled = false
	t.Cleanup(func() {
		common.DisplayTokenStatEnabled = oldDisplayTokenStatEnabled
	})

	const userID = 4101
	require.NoError(t, db.Create(&model.User{
		Id:        userID,
		Username:  "billing-context-user",
		Group:     "default",
		Quota:     100,
		UsedQuota: 20,
		Status:    common.UserStatusEnabled,
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = newCanceledControllerRequest(http.MethodGet, "/dashboard/billing/subscription")
	ctx.Set("id", userID)
	common.SetContextKey(ctx, constant.ContextKeyUserQuota, 300)

	GetSubscription(ctx)

	var response OpenAISubscriptionResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "billing_subscription", response.Object)
}

func TestGetUserModelsUsesAuthenticatedGroupContext(t *testing.T) {
	db := setupAuthenticatedContextControllerTestDB(t)
	require.NoError(t, db.Create(&model.Ability{
		Group:     authenticatedContextTestGroup,
		Model:     "zz-auth-context-user-model",
		ChannelId: 1,
		Enabled:   true,
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = newCanceledControllerRequest(http.MethodGet, "/api/user/models")
	ctx.Set("id", 4102)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, authenticatedContextTestGroup)

	GetUserModels(ctx)

	payload := decodeControllerPayload(t, recorder)
	require.Equal(t, true, payload["success"])
	require.Contains(t, payload["data"], "zz-auth-context-user-model")
}

func TestGetUserGroupsUsesAuthenticatedGroupContext(t *testing.T) {
	_ = setupAuthenticatedContextControllerTestDB(t)
	withAuthenticatedContextGroupSettings(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = newCanceledControllerRequest(http.MethodGet, "/api/user/self/groups")
	ctx.Set("id", 4103)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, authenticatedContextTestGroup)

	GetUserGroups(ctx)

	payload := decodeControllerPayload(t, recorder)
	require.Equal(t, true, payload["success"])
	groups, ok := payload["data"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, groups, authenticatedContextTestGroup)
}

func TestListModelsUsesAuthenticatedUserSettingContext(t *testing.T) {
	_ = setupAuthenticatedContextControllerTestDB(t)
	withSelfUseModeDisabled(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = newCanceledControllerRequest(http.MethodGet, "/v1/models")
	ctx.Set("id", 4104)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyUserSetting, dto.UserSetting{
		AcceptUnsetRatioModel: true,
	})
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimit, map[string]bool{
		"zz-auth-context-unpriced-model": true,
	})

	ListModels(ctx, constant.ChannelTypeOpenAI)

	ids := decodeListModelsResponse(t, recorder)
	require.Contains(t, ids, "zz-auth-context-unpriced-model")
}

func TestGetSubscriptionSelfUsesAuthenticatedUserSettingContext(t *testing.T) {
	_ = setupAuthenticatedContextControllerTestDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = newCanceledControllerRequest(http.MethodGet, "/api/subscription/self")
	ctx.Set("id", 4105)
	common.SetContextKey(ctx, constant.ContextKeyUserSetting, dto.UserSetting{
		BillingPreference: "wallet_only",
	})

	GetSubscriptionSelf(ctx)

	payload := decodeControllerPayload(t, recorder)
	require.Equal(t, true, payload["success"])
	data, ok := payload["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "wallet_only", data["billing_preference"])
}

func TestGetPricingUsesAuthenticatedGroupContext(t *testing.T) {
	_ = setupAuthenticatedContextControllerTestDB(t)
	withAuthenticatedContextGroupSettings(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = newCanceledControllerRequest(http.MethodGet, "/api/pricing")
	ctx.Set("id", 4106)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, authenticatedContextTestGroup)

	GetPricing(ctx)

	payload := decodeControllerPayload(t, recorder)
	require.Equal(t, true, payload["success"])
	groupRatio, ok := payload["group_ratio"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, 3.5, groupRatio["default"])
	require.Equal(t, 4.5, groupRatio[authenticatedContextTestGroup])
	usableGroup, ok := payload["usable_group"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, usableGroup, authenticatedContextTestGroup)
}

func TestGetPricingFallsBackForSessionOnlyUser(t *testing.T) {
	db := setupAuthenticatedContextControllerTestDB(t)
	withAuthenticatedContextGroupSettings(t)

	const userID = 4107
	require.NoError(t, db.Create(&model.User{
		Id:       userID,
		Username: "pricing-session-user",
		Group:    authenticatedContextTestGroup,
		Status:   common.UserStatusEnabled,
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/pricing", nil)
	ctx.Set("id", userID)

	GetPricing(ctx)

	payload := decodeControllerPayload(t, recorder)
	require.Equal(t, true, payload["success"])
	groupRatio, ok := payload["group_ratio"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, 3.5, groupRatio["default"])
	require.Equal(t, 4.5, groupRatio[authenticatedContextTestGroup])
}
