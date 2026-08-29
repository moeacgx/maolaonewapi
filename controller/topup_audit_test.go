package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminCompleteTopUpReturnsSuccessAndWritesTransactionalAudit(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	user := &model.User{Id: 921, Username: "controller-audit-user", Status: common.UserStatusEnabled, Quota: 9}
	require.NoError(t, db.Create(user).Error)
	topUp := &model.TopUp{
		UserId: user.Id, Amount: 2, Money: 2, CreditedQuota: 11, PaidAmountCNY: 2,
		TradeNo: "controller-audit-order", PaymentMethod: "alipay", PaymentProvider: model.PaymentProviderEpay,
		CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())

	gin.SetMode(gin.TestMode)
	reqBody, err := common.Marshal(AdminCompleteTopupRequest{TradeNo: topUp.TradeNo})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/user/topup/complete", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	writer := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(writer)
	ctx.Request = req
	AdminCompleteTopUp(ctx)

	assert.Equal(t, http.StatusOK, writer.Code)
	assert.Contains(t, writer.Body.String(), `"success":true`)

	var log model.Log
	require.NoError(t, db.Where("type = ?", model.LogTypeTopup).First(&log).Error)
	other, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(9), adminInfo["balance_before"])
	assert.Equal(t, float64(11), adminInfo["credited_quota"])
	assert.Equal(t, float64(20), adminInfo["balance_after"])
	assert.Equal(t, topUp.TradeNo, adminInfo["trade_no"])
	assert.Equal(t, float64(2), adminInfo["paid_amount_cny"])
}
