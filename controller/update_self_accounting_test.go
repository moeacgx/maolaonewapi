package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpdateSelfSettingBranchesDoNotOverwriteAccounting(t *testing.T) {
	testCases := []struct {
		name       string
		body       string
		assertions func(t *testing.T, setting dto.UserSetting)
	}{
		{
			name: "sidebar modules",
			body: `{"sidebar_modules":"{\"console\":{\"enabled\":true}}"}`,
			assertions: func(t *testing.T, setting dto.UserSetting) {
				require.Equal(t, `{"console":{"enabled":true}}`, setting.SidebarModules)
				require.Equal(t, "en", setting.Language)
			},
		},
		{
			name: "language",
			body: `{"language":"zh"}`,
			assertions: func(t *testing.T, setting dto.UserSetting) {
				require.Equal(t, "zh", setting.Language)
				require.Equal(t, `{"console":{"enabled":false}}`, setting.SidebarModules)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			db := setupUserRoleManagementTestDB(t)
			user := &model.User{
				Username:     "update-self-accounting-" + strings.ReplaceAll(testCase.name, " ", "-"),
				Password:     "password123",
				Role:         common.RoleCommonUser,
				Status:       common.UserStatusEnabled,
				Quota:        750,
				UsedQuota:    270,
				RequestCount: 4,
			}
			user.SetSetting(dto.UserSetting{
				Language:       "en",
				SidebarModules: `{"console":{"enabled":false}}`,
			})
			require.NoError(t, db.Create(user).Error)

			// 确定性复现旧实现的丢失更新时序：控制器读到旧快照后，
			// 计费链路先完成原子更新，再允许资料设置继续写库。
			var accountingUpdateOnce sync.Once
			var accountingUpdateErr error
			accountingUpdated := false
			require.NoError(t, db.Callback().Query().After("gorm:query").Register(
				"test:update_accounting_after_profile_snapshot",
				func(tx *gorm.DB) {
					if tx.Statement.Table != "users" {
						return
					}
					accountingUpdateOnce.Do(func() {
						accountingUpdated = true
						accountingUpdateErr = db.Session(&gorm.Session{NewDB: true}).
							Model(&model.User{}).
							Where("id = ?", user.Id).
							Updates(map[string]interface{}{
								"quota":         gorm.Expr("quota - ?", 200),
								"used_quota":    gorm.Expr("used_quota + ?", 200),
								"request_count": gorm.Expr("request_count + ?", 1),
							}).Error
					})
				},
			))

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Set("id", user.Id)
			ctx.Request = httptest.NewRequest(http.MethodPut, "/api/user/self", strings.NewReader(testCase.body))
			ctx.Request.Header.Set("Content-Type", "application/json")

			UpdateSelf(ctx)

			require.Equal(t, http.StatusOK, recorder.Code)
			require.True(t, accountingUpdated)
			require.NoError(t, accountingUpdateErr)
			var stored model.User
			require.NoError(t, db.First(&stored, user.Id).Error)
			require.Equal(t, 550, stored.Quota)
			require.Equal(t, 470, stored.UsedQuota)
			require.Equal(t, 5, stored.RequestCount)
			testCase.assertions(t, stored.GetSetting())
		})
	}
}

func TestUpdateSelfRejectsOversizedRequestBody(t *testing.T) {
	db := setupUserRoleManagementTestDB(t)
	user := &model.User{
		Username: "update-self-oversized-body",
		Password: "password123",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	user.SetSetting(dto.UserSetting{Language: "en"})
	require.NoError(t, db.Create(user).Error)

	body := `{"display_name":"` + strings.Repeat("x", int(maxUpdateSelfRequestBodyBytes)) + `"}`
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", user.Id)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/user/self", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	UpdateSelf(ctx)

	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
	response := decodeUserRoleManagementResponse(t, recorder)
	require.False(t, response.Success)

	var stored model.User
	require.NoError(t, db.First(&stored, user.Id).Error)
	require.Equal(t, "en", stored.GetSetting().Language)
}

func TestUpdateSelfRejectsInvalidOrOversizedSidebarModules(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		value string
	}{
		{name: "invalid json", value: "not-json"},
		{name: "oversized", value: strings.Repeat("x", maxSidebarModulesBytes+1)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			db := setupUserRoleManagementTestDB(t)
			user := &model.User{
				Username: "update-self-invalid-sidebar-" + strings.ReplaceAll(testCase.name, " ", "-"),
				Password: "password123",
				Role:     common.RoleCommonUser,
				Status:   common.UserStatusEnabled,
			}
			user.SetSetting(dto.UserSetting{SidebarModules: `{"console":{"enabled":true}}`})
			require.NoError(t, db.Create(user).Error)

			payload, err := common.Marshal(map[string]any{"sidebar_modules": testCase.value})
			require.NoError(t, err)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Set("id", user.Id)
			ctx.Request = httptest.NewRequest(http.MethodPut, "/api/user/self", strings.NewReader(string(payload)))
			ctx.Request.Header.Set("Content-Type", "application/json")

			UpdateSelf(ctx)

			require.Equal(t, http.StatusOK, recorder.Code)
			response := decodeUserRoleManagementResponse(t, recorder)
			require.False(t, response.Success)

			var stored model.User
			require.NoError(t, db.First(&stored, user.Id).Error)
			require.Equal(t, `{"console":{"enabled":true}}`, stored.GetSetting().SidebarModules)
		})
	}
}
