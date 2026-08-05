package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestUserUpdateDoesNotOverwriteAccountingFields(t *testing.T) {
	truncateTables(t)

	oldAccessToken := "old-management-token"
	user := User{
		Username:        "quota-race-user",
		Password:        "old-password",
		DisplayName:     "before",
		Status:          common.UserStatusEnabled,
		AccessToken:     &oldAccessToken,
		Quota:           1000,
		UsedQuota:       20,
		RequestCount:    3,
		AffCount:        2,
		AffQuota:        100,
		AffHistoryQuota: 10,
		InviterId:       7,
		StripeCustomer:  "stripe-old",
		CreatedAt:       10,
		LastLoginAt:     100,
	}
	user.SetSetting(dto.UserSetting{Language: "en"})
	require.NoError(t, DB.Create(&user).Error)

	staleUser, err := GetUserById(user.Id, true)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
		"password":        "new-password",
		"access_token":    "new-management-token",
		"quota":           gorm.Expr("quota - ?", 400),
		"used_quota":      gorm.Expr("used_quota + ?", 400),
		"request_count":   gorm.Expr("request_count + ?", 1),
		"aff_count":       5,
		"aff_quota":       70,
		"aff_history":     40,
		"inviter_id":      8,
		"setting":         `{"language":"zh"}`,
		"stripe_customer": "stripe-new",
		"created_at":      20,
		"last_login_at":   200,
	}).Error)

	staleUser.DisplayName = "after"
	require.NoError(t, staleUser.Update(false))

	var got User
	require.NoError(t, DB.First(&got, user.Id).Error)
	assert.Equal(t, "after", got.DisplayName)
	assert.Equal(t, "new-password", got.Password)
	require.NotNil(t, got.AccessToken)
	assert.Equal(t, "new-management-token", *got.AccessToken)
	assert.Equal(t, 600, got.Quota)
	assert.Equal(t, 420, got.UsedQuota)
	assert.Equal(t, 4, got.RequestCount)
	assert.Equal(t, 5, got.AffCount)
	assert.Equal(t, 70, got.AffQuota)
	assert.Equal(t, 40, got.AffHistoryQuota)
	assert.Equal(t, 8, got.InviterId)
	assert.Equal(t, "zh", got.GetSetting().Language)
	assert.Equal(t, "stripe-new", got.StripeCustomer)
	assert.Equal(t, int64(20), got.CreatedAt)
	assert.Equal(t, int64(200), got.LastLoginAt)
}

func TestUpdateUserSettingColumnOnlyUpdatesSetting(t *testing.T) {
	truncateTables(t)

	user := User{
		Username:     "setting-race-user",
		Password:     "password",
		Status:       common.UserStatusEnabled,
		Quota:        1000,
		UsedQuota:    20,
		RequestCount: 3,
	}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
		"quota":         750,
		"used_quota":    270,
		"request_count": 4,
	}).Error)

	require.NoError(t, UpdateUserSettingColumn(user.Id, dto.UserSetting{Language: "zh"}))

	var got User
	require.NoError(t, DB.First(&got, user.Id).Error)
	assert.Equal(t, 750, got.Quota)
	assert.Equal(t, 270, got.UsedQuota)
	assert.Equal(t, 4, got.RequestCount)
	assert.Equal(t, "zh", got.GetSetting().Language)
}

func TestUpdateUserAccessTokenColumnOnlyUpdatesAccessToken(t *testing.T) {
	truncateTables(t)

	user := User{
		Username:     "access-token-race-user",
		Password:     "password",
		Status:       common.UserStatusEnabled,
		Quota:        1000,
		UsedQuota:    20,
		RequestCount: 3,
	}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
		"quota":         750,
		"used_quota":    270,
		"request_count": 4,
	}).Error)

	require.NoError(t, UpdateUserAccessTokenColumn(user.Id, "new-management-token"))

	var got User
	require.NoError(t, DB.First(&got, user.Id).Error)
	require.NotNil(t, got.AccessToken)
	assert.Equal(t, "new-management-token", *got.AccessToken)
	assert.Equal(t, 750, got.Quota)
	assert.Equal(t, 270, got.UsedQuota)
	assert.Equal(t, 4, got.RequestCount)
}

func TestFillAndRequiredColumnUpdateRejectMissingUser(t *testing.T) {
	truncateTables(t)

	missing := User{Id: 987654}
	require.ErrorIs(t, missing.FillUserById(), gorm.ErrRecordNotFound)
	require.ErrorIs(t, UpdateUserEmailColumn(missing.Id, "missing@example.com"), gorm.ErrRecordNotFound)
}

func TestFillAndRequiredColumnUpdateRejectSoftDeletedUser(t *testing.T) {
	truncateTables(t)

	user := User{
		Username: "soft-deleted-column-update",
		Password: "password",
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, DB.Delete(&user).Error)

	lookup := User{Id: user.Id}
	require.ErrorIs(t, lookup.FillUserById(), gorm.ErrRecordNotFound)
	require.ErrorIs(t, UpdateUserBuiltinOAuthBindingColumn(user.Id, "github", "deleted-user"), gorm.ErrRecordNotFound)

	var stored User
	require.NoError(t, DB.Unscoped().First(&stored, user.Id).Error)
	require.Empty(t, stored.GitHubId)
}

func TestMutateUserSettingRetriesWithoutLosingConcurrentFields(t *testing.T) {
	truncateTables(t)

	user := User{
		Username: "setting-cas-user",
		Password: "password",
		Status:   common.UserStatusEnabled,
	}
	user.SetSetting(dto.UserSetting{Language: "en"})
	require.NoError(t, DB.Create(&user).Error)

	firstRead := make(chan struct{})
	continueFirstUpdate := make(chan struct{})
	firstUpdateDone := make(chan error, 1)
	go func() {
		firstAttempt := true
		firstUpdateDone <- MutateUserSetting(user.Id, func(setting *dto.UserSetting) error {
			if firstAttempt {
				firstAttempt = false
				close(firstRead)
				<-continueFirstUpdate
			}
			setting.SidebarModules = `{"console":{"enabled":true}}`
			return nil
		})
	}()

	select {
	case <-firstRead:
	case <-time.After(5 * time.Second):
		t.Fatal("等待首次设置读取超时")
	}
	secondErr := MutateUserSetting(user.Id, func(setting *dto.UserSetting) error {
		setting.Language = "zh"
		return nil
	})
	close(continueFirstUpdate)
	require.NoError(t, secondErr)

	select {
	case err := <-firstUpdateDone:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("等待并发设置重试超时")
	}

	var got User
	require.NoError(t, DB.First(&got, user.Id).Error)
	setting := got.GetSetting()
	assert.Equal(t, "zh", setting.Language)
	assert.Equal(t, `{"console":{"enabled":true}}`, setting.SidebarModules)
}

func TestMutateUserSettingNoOpRetriesCacheInvalidation(t *testing.T) {
	truncateTables(t)

	user := User{
		Username: "setting-noop-cache-retry",
		Password: "password",
		Status:   common.UserStatusEnabled,
	}
	user.SetSetting(dto.UserSetting{Language: "zh"})
	require.NoError(t, DB.Create(&user).Error)

	invalidationCalls := 0
	err := mutateUserSettingWithInvalidation(
		user.Id,
		func(setting *dto.UserSetting) error {
			setting.Language = "zh"
			return nil
		},
		func(userId int) error {
			require.Equal(t, user.Id, userId)
			invalidationCalls++
			return nil
		},
	)

	require.NoError(t, err)
	require.Equal(t, 1, invalidationCalls)
}

func TestExactTextCASUsesBinaryComparisonOnMySQL(t *testing.T) {
	previousMySQL := common.UsingMySQL
	common.UsingMySQL = true
	t.Cleanup(func() { common.UsingMySQL = previousMySQL })

	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true, DryRun: true})
	require.NoError(t, err)

	settingSQL := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		return whereExactText(tx.Model(&User{}).Where("id = ?", 1), "setting", `{"language":"EN"}`).
			Update("setting", `{"language":"en"}`)
	})
	require.Contains(t, settingSQL, "BINARY setting = BINARY")

	groupSQL := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		return whereExactText(tx.Model(&User{}).Where("id = ?", 1), "`group`", "VIP").
			Update("group", "vip")
	})
	require.Contains(t, groupSQL, "BINARY `group` = BINARY")
}

func TestUserEditOmitsUnprovidedRoleAndGroupAndRejectsStaleCAS(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	user := User{
		Username: "edit-cas-user",
		Password: "password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    defaultGroup.Code,
		GroupId:  defaultGroup.Id,
	}
	require.NoError(t, DB.Create(&user).Error)

	staleUser, err := GetUserById(user.Id, true)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
		"role":     common.RoleAdminUser,
		"group":    vipGroup.Code,
		"group_id": vipGroup.Id,
	}).Error)

	staleUser.Username = "edit-cas-user-renamed"
	require.NoError(t, staleUser.Edit(false, UserEditOptions{Username: true}))
	var got User
	require.NoError(t, DB.First(&got, user.Id).Error)
	assert.Equal(t, "edit-cas-user-renamed", got.Username)
	assert.Equal(t, common.RoleAdminUser, got.Role)
	assert.Equal(t, vipGroup.Code, got.Group)
	assert.Equal(t, vipGroup.Id, got.GroupId)

	staleUser.Username = "must-not-bypass-role-guard"
	err = staleUser.Edit(false, UserEditOptions{
		Username:     true,
		GuardRole:    true,
		ExpectedRole: common.RoleCommonUser,
	})
	require.Error(t, err)
	require.NoError(t, DB.First(&got, user.Id).Error)
	assert.Equal(t, "edit-cas-user-renamed", got.Username)

	staleUser.Role = common.RoleCommonUser
	staleUser.Group = defaultGroup.Code
	err = staleUser.Edit(false, UserEditOptions{
		Role:          true,
		Group:         true,
		ExpectedRole:  common.RoleCommonUser,
		ExpectedGroup: defaultGroup.Code,
	})
	require.Error(t, err)
	require.NoError(t, DB.First(&got, user.Id).Error)
	assert.Equal(t, common.RoleAdminUser, got.Role)
	assert.Equal(t, vipGroup.Code, got.Group)
}

func TestUserUpdateWithPasswordOnlyChangesProfileFields(t *testing.T) {
	truncateTables(t)
	user := User{
		Username: "profile-password-user",
		Password: "old-password",
		Role:     common.RoleAdminUser,
		Status:   common.UserStatusEnabled,
		Quota:    900,
	}
	require.NoError(t, DB.Create(&user).Error)

	profile := User{
		Id:          user.Id,
		Username:    "profile-password-renamed",
		DisplayName: "Profile Renamed",
		Password:    "new-password",
	}
	require.NoError(t, profile.Update(true))

	var got User
	require.NoError(t, DB.First(&got, user.Id).Error)
	assert.Equal(t, "profile-password-renamed", got.Username)
	assert.Equal(t, "Profile Renamed", got.DisplayName)
	assert.True(t, common.ValidatePasswordAndHash("new-password", got.Password))
	assert.Equal(t, common.RoleAdminUser, got.Role)
	assert.Equal(t, common.UserStatusEnabled, got.Status)
	assert.Equal(t, 900, got.Quota)
}
