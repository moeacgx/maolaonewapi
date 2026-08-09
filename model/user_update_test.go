package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUserUpdateDoesNotOverwriteConcurrentAccountingOrTokenChanges(t *testing.T) {
	truncateTables(t)

	accessToken := "original-token"
	user := &User{
		Id:              1,
		Username:        "quota-race-user",
		Password:        "password",
		DisplayName:     "before",
		Role:            common.RoleCommonUser,
		Status:          common.UserStatusEnabled,
		Group:           "default",
		Quota:           1000,
		UsedQuota:       200,
		RequestCount:    7,
		AffCode:         "race-aff",
		AffCount:        2,
		AffQuota:        800,
		AffHistoryQuota: 1200,
		AccessToken:     &accessToken,
	}
	require.NoError(t, DB.Create(user).Error)

	stale, err := GetUserById(user.Id, true)
	require.NoError(t, err)

	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
		"quota":         gorm.Expr("quota + ?", 500),
		"used_quota":    gorm.Expr("used_quota + ?", 300),
		"request_count": gorm.Expr("request_count + ?", 4),
		"aff_count":     gorm.Expr("aff_count + ?", 1),
		"aff_quota":     gorm.Expr("aff_quota - ?", 500),
		"aff_history":   gorm.Expr("aff_history + ?", 500),
		"access_token":  "rotated-token",
	}).Error)

	stale.DisplayName = "after"
	require.NoError(t, stale.Update(false))

	var got User
	require.NoError(t, DB.First(&got, user.Id).Error)
	assert.Equal(t, "after", got.DisplayName)
	assert.Equal(t, 1500, got.Quota)
	assert.Equal(t, 500, got.UsedQuota)
	assert.Equal(t, 11, got.RequestCount)
	assert.Equal(t, 3, got.AffCount)
	assert.Equal(t, 300, got.AffQuota)
	assert.Equal(t, 1700, got.AffHistoryQuota)
	assert.Equal(t, "rotated-token", got.GetAccessToken())
}

func TestUpdateUserAccessTokenOnlyUpdatesAccessToken(t *testing.T) {
	truncateTables(t)

	accessToken := "before-token"
	user := &User{
		Id:          2,
		Username:    "access-token-rotate-user",
		Password:    "password",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		Quota:       900,
		UsedQuota:   100,
		AffCode:     "token-aff",
		AccessToken: &accessToken,
	}
	require.NoError(t, DB.Create(user).Error)

	require.NoError(t, UpdateUserAccessToken(user.Id, "after-token"))

	var got User
	require.NoError(t, DB.First(&got, user.Id).Error)
	assert.Equal(t, "after-token", got.GetAccessToken())
	assert.Equal(t, 900, got.Quota)
	assert.Equal(t, 100, got.UsedQuota)
}

func TestInviteUserUsesAtomicCounterExpression(t *testing.T) {
	truncateTables(t)

	oldQuotaForInviter := common.QuotaForInviter
	common.QuotaForInviter = 250
	t.Cleanup(func() { common.QuotaForInviter = oldQuotaForInviter })

	inviter := &User{
		Id:              3,
		Username:        "invite-accounting-user",
		Password:        "password",
		Role:            common.RoleCommonUser,
		Status:          common.UserStatusEnabled,
		Group:           "default",
		AffCode:         "invite-aff",
		AffCount:        2,
		AffQuota:        300,
		AffHistoryQuota: 700,
	}
	require.NoError(t, DB.Create(inviter).Error)

	require.NoError(t, inviteUser(inviter.Id))

	var got User
	require.NoError(t, DB.First(&got, inviter.Id).Error)
	assert.Equal(t, 3, got.AffCount)
	assert.Equal(t, 300, got.AffQuota)
	assert.Equal(t, 700, got.AffHistoryQuota)
}

func insertUsersForPaginationTest(t *testing.T, total int) {
	t.Helper()
	for id := 1; id <= total; id++ {
		user := &User{
			Id:          id,
			Username:    fmt.Sprintf("user%02d", id),
			Password:    "password123",
			DisplayName: fmt.Sprintf("User %02d", id),
			Email:       fmt.Sprintf("user%02d@example.com", id),
			Role:        common.RoleCommonUser,
			Status:      common.UserStatusEnabled,
			Group:       "default",
			AffCode:     fmt.Sprintf("aff%02d", id),
		}
		require.NoError(t, DB.Create(user).Error)
	}
}

func collectUserIDs(users []*User) []int {
	ids := make([]int, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.Id)
	}
	return ids
}

func TestGetAllUsersSortsBeforePagination(t *testing.T) {
	truncateTables(t)
	insertUsersForPaginationTest(t, 42)

	pageOne, total, err := GetAllUsers(&common.PageInfo{Page: 1, PageSize: 20}, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.Equal(t, int64(42), total)
	assert.Equal(t, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}, collectUserIDs(pageOne))

	pageTwo, total, err := GetAllUsers(&common.PageInfo{Page: 2, PageSize: 20}, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.Equal(t, int64(42), total)
	assert.Equal(t, []int{21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40}, collectUserIDs(pageTwo))

	pageThree, total, err := GetAllUsers(&common.PageInfo{Page: 3, PageSize: 20}, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.Equal(t, int64(42), total)
	assert.Equal(t, []int{41, 42}, collectUserIDs(pageThree))
}

func TestSearchUsersSortsBeforePagination(t *testing.T) {
	truncateTables(t)
	insertUsersForPaginationTest(t, 42)

	users, total, err := SearchUsersWithSort("user", "", nil, nil, 20, 20, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.Equal(t, int64(42), total)
	assert.Equal(t, []int{21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40}, collectUserIDs(users))
}
