package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateUserSettingPreservesIndependentPreferences(t *testing.T) {
	db := setupUserRoleManagementTestDB(t)
	user := &model.User{
		Username: "setting-preserve-user",
		Password: "password123",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	user.SetSetting(dto.UserSetting{
		SidebarModules:    `{"console":{"enabled":true}}`,
		BillingPreference: "wallet_first",
		Language:          "zh",
		NotifyType:        dto.NotifyTypeWebhook,
		WebhookUrl:        "https://old.example.com/hook",
		WebhookSecret:     "old-secret",
	})
	require.NoError(t, db.Create(user).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", user.Id)
	ctx.Set("role", common.RoleCommonUser)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/user/setting", strings.NewReader(`{
		"notify_type":"email",
		"quota_warning_threshold":1,
		"notification_email":"notify@example.com",
		"accept_unset_model_ratio_model":true,
		"record_ip_log":true
	}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	UpdateUserSetting(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var got model.User
	require.NoError(t, db.First(&got, user.Id).Error)
	setting := got.GetSetting()
	assert.Equal(t, `{"console":{"enabled":true}}`, setting.SidebarModules)
	assert.Equal(t, "wallet_first", setting.BillingPreference)
	assert.Equal(t, "zh", setting.Language)
	assert.Equal(t, dto.NotifyTypeEmail, setting.NotifyType)
	assert.Equal(t, "notify@example.com", setting.NotificationEmail)
	assert.Empty(t, setting.WebhookUrl)
	assert.Empty(t, setting.WebhookSecret)
}
