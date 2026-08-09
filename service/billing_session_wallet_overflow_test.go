package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedSubscriptionPlanForWalletOverflowTest(t *testing.T, id int, allowOverflow bool, totalAmount int64) *model.SubscriptionPlan {
	t.Helper()
	plan := &model.SubscriptionPlan{
		Id:                  id,
		Title:               "Overflow Plan",
		PriceAmount:         9.99,
		DurationUnit:        model.SubscriptionDurationMonth,
		DurationValue:       1,
		TotalAmount:         totalAmount,
		AllowWalletOverflow: &allowOverflow,
	}
	require.NoError(t, model.DB.Create(plan).Error)
	return plan
}

func createActiveSubscriptionFromPlanForWalletOverflowTest(t *testing.T, userID int, plan *model.SubscriptionPlan) int {
	t.Helper()
	var subID int
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		sub, err := model.CreateUserSubscriptionFromPlanTx(tx, userID, plan, "test")
		if err != nil {
			return err
		}
		subID = sub.Id
		return nil
	}))
	return subID
}

func TestNewBillingSessionFallsBackToWalletWhenSubscriptionAllowsOverflow(t *testing.T) {
	truncate(t)
	gin.SetMode(gin.TestMode)

	userID := 9101
	tokenID := 9102
	planID := 9103
	preConsumedQuota := 300

	seedUser(t, userID, 1000)
	seedToken(t, tokenID, userID, "sk-wallet-overflow-allow", 1000)
	plan := seedSubscriptionPlanForWalletOverflowTest(t, planID, true, 100)
	subID := createActiveSubscriptionFromPlanForWalletOverflowTest(t, userID, plan)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		UserId:          userID,
		TokenId:         tokenID,
		TokenKey:        "sk-wallet-overflow-allow",
		SkipTokenQuota:  true,
		RequestId:       "req-wallet-overflow-allow",
		OriginModelName: "test-model",
		UserSetting: dto.UserSetting{
			BillingPreference: "subscription_first",
		},
	}

	session, apiErr := NewBillingSession(c, info, preConsumedQuota)
	require.Nil(t, apiErr)
	require.NotNil(t, session)
	assert.Equal(t, BillingSourceWallet, info.BillingSource)
	assert.Equal(t, preConsumedQuota, info.FinalPreConsumedQuota)
	assert.Equal(t, 700, getUserQuota(t, userID))
	assert.Equal(t, 1000, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, int64(0), getSubscriptionUsed(t, subID))
}

func TestNewBillingSessionKeepsSubscriptionErrorWhenWalletOverflowBlocked(t *testing.T) {
	truncate(t)
	gin.SetMode(gin.TestMode)

	userID := 9201
	tokenID := 9202
	planID := 9203
	preConsumedQuota := 300

	seedUser(t, userID, 1000)
	seedToken(t, tokenID, userID, "sk-wallet-overflow-block", 1000)
	plan := seedSubscriptionPlanForWalletOverflowTest(t, planID, false, 100)
	subID := createActiveSubscriptionFromPlanForWalletOverflowTest(t, userID, plan)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		UserId:          userID,
		TokenId:         tokenID,
		TokenKey:        "sk-wallet-overflow-block",
		SkipTokenQuota:  true,
		RequestId:       "req-wallet-overflow-block",
		OriginModelName: "test-model",
		UserSetting: dto.UserSetting{
			BillingPreference: "subscription_first",
		},
	}

	session, apiErr := NewBillingSession(c, info, preConsumedQuota)
	require.Nil(t, session)
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.MessageForClient(), "订阅额度不足")
	assert.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
	assert.Equal(t, 1000, getUserQuota(t, userID))
	assert.Equal(t, 1000, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, int64(0), getSubscriptionUsed(t, subID))
	assert.Empty(t, info.BillingSource)
	assert.Zero(t, info.FinalPreConsumedQuota)
}
