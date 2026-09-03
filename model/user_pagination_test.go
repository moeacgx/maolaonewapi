package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	users, total, err := SearchUsers("user", "", nil, nil, 20, 20, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.Equal(t, int64(42), total)
	assert.Equal(t, []int{21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40}, collectUserIDs(users))
}

func insertSearchUsersFixture(t *testing.T) {
	t.Helper()
	users := []*User{
		{
			Id: 146, Username: "target-user", Password: "password123",
			DisplayName: "146 target", Email: "target@example.com",
			Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "search-aff-146",
		},
		{
			Id: 147, Username: "user146", Password: "password123",
			DisplayName: "Username match", Email: "username@example.com",
			Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "search-aff-147",
		},
		{
			Id: 148, Username: "email-user", Password: "password123",
			DisplayName: "Email match", Email: "mail146@example.com",
			Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "search-aff-148",
		},
		{
			Id: 149, Username: "display-user", Password: "password123",
			DisplayName: "display146", Email: "display@example.com",
			Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "search-aff-149",
		},
	}
	for _, user := range users {
		require.NoError(t, DB.Create(user).Error)
	}
}

func TestSearchUsersByIDMatchesOnlyExactID(t *testing.T) {
	truncateTables(t)
	insertSearchUsersFixture(t)

	users, total, err := SearchUsersWithSort("146", "", nil, nil, 0, 20, NewUserSortOptions("id", "desc"), "id")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, users, 1)
	assert.Equal(t, 146, users[0].Id)
}

func TestSearchUsersByUsernameMatchesOnlyUsername(t *testing.T) {
	truncateTables(t)
	insertSearchUsersFixture(t)

	users, total, err := SearchUsersWithSort("146", "", nil, nil, 0, 20, NewUserSortOptions("id", "desc"), "username")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, users, 1)
	assert.Equal(t, 147, users[0].Id)
}

func TestSearchUsersAllKeepsIDAndTextMatches(t *testing.T) {
	truncateTables(t)
	insertSearchUsersFixture(t)

	users, total, err := SearchUsersWithSort("146", "", nil, nil, 0, 20, NewUserSortOptions("id", "desc"), "all")
	require.NoError(t, err)
	assert.Equal(t, int64(4), total)
	assert.Equal(t, []int{149, 148, 147, 146}, collectUserIDs(users))
}

func TestSearchUsersByIDRejectsInvalidAndOversizedKeywords(t *testing.T) {
	truncateTables(t)
	insertSearchUsersFixture(t)

	for _, keyword := range []string{"", "   ", "999", "not-an-id", "0", "-1", "2147483648", "999999999999999999999"} {
		users, total, err := SearchUsersWithSort(keyword, "", nil, nil, 0, 20, NewUserSortOptions("id", "desc"), "id")
		require.NoError(t, err, keyword)
		assert.Equal(t, int64(0), total, keyword)
		assert.Empty(t, users, keyword)
	}
}

func TestSearchUsersEmptyKeywordPreservesNonIDSearchTypes(t *testing.T) {
	truncateTables(t)
	insertSearchUsersFixture(t)

	for _, searchType := range []string{"all", "username", "unknown"} {
		users, total, err := SearchUsersWithSort("", "", nil, nil, 0, 20, NewUserSortOptions("id", "desc"), searchType)
		require.NoError(t, err, searchType)
		assert.Equal(t, int64(4), total, searchType)
		assert.Equal(t, []int{149, 148, 147, 146}, collectUserIDs(users), searchType)
	}
}
