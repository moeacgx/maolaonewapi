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

func requestAllTopUps(t *testing.T, keyword string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(writer)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/topup?p=1&page_size=10&keyword="+keyword, nil)
	GetAllTopUps(ctx)
	return writer
}

func TestGetAllTopUpsReturnsCurrentUsername(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	user := &model.User{Id: 923, Username: "billing-history-user", Status: common.UserStatusEnabled, AffCode: "billing-history-aff"}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, (&model.TopUp{
		UserId: user.Id, Amount: 10, Money: 10, TradeNo: "billing-history-order",
		PaymentMethod: "alipay", PaymentProvider: model.PaymentProviderEpay,
		CreateTime: common.GetTimestamp(), Status: common.TopUpStatusSuccess,
	}).Insert())

	recorder := requestAllTopUps(t, "")
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Items []struct {
				Username string `json:"username"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Len(t, response.Data.Items, 1)
	assert.Equal(t, user.Username, response.Data.Items[0].Username)
}

func TestSearchAllTopUpsMatchesUsername(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	user := &model.User{Id: 924, Username: "billing-search-user", Status: common.UserStatusEnabled, AffCode: "billing-search-aff"}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, (&model.TopUp{
		UserId: user.Id, Amount: 10, Money: 10, TradeNo: "billing-search-order",
		PaymentMethod: "alipay", PaymentProvider: model.PaymentProviderEpay,
		CreateTime: common.GetTimestamp(), Status: common.TopUpStatusSuccess,
	}).Insert())

	recorder := requestAllTopUps(t, user.Username)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.Equal(t, 1, response.Data.Total)
}
