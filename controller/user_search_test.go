package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func performSearchUsersRequest(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	SearchUsers(c)
	return recorder
}

func TestSearchUsersControllerForwardsIDSearchType(t *testing.T) {
	db := setupManageUserTestDB(t)
	users := []model.User{
		{Id: 146, Username: "target-user", Password: "password123", DisplayName: "146 target", Email: "target@example.com", Group: "default", AffCode: "controller-search-aff-146"},
		{Id: 147, Username: "user146", Password: "password123", DisplayName: "Username match", Email: "username@example.com", Group: "default", AffCode: "controller-search-aff-147"},
	}
	for index := range users {
		users[index].Role = common.RoleCommonUser
		users[index].Status = common.UserStatusEnabled
		require.NoError(t, db.Create(&users[index]).Error)
	}

	recorder := performSearchUsersRequest(t, "/api/user/search?keyword=146&search_type=id&p=1&page_size=20")
	assert.Equal(t, http.StatusOK, recorder.Code)

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Total int          `json:"total"`
			Items []model.User `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Equal(t, 1, response.Data.Total)
	require.Len(t, response.Data.Items, 1)
	assert.Equal(t, 146, response.Data.Items[0].Id)
}

func TestSearchUsersControllerReturnsEmptyForEmptyIDKeyword(t *testing.T) {
	db := setupManageUserTestDB(t)
	user := model.User{
		Id: 146, Username: "target-user", Password: "password123", DisplayName: "Target user",
		Email: "target@example.com", Group: "default", AffCode: "controller-search-empty-id",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
	}
	require.NoError(t, db.Create(&user).Error)

	recorder := performSearchUsersRequest(t, "/api/user/search?keyword=&search_type=id&p=1&page_size=20")
	require.Equal(t, http.StatusOK, recorder.Code)

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Total int          `json:"total"`
			Items []model.User `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.Zero(t, response.Data.Total)
	assert.Empty(t, response.Data.Items)
}
