package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useCanvasBillingRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	server := miniredis.RunT(t)
	previousEnabled := common.RedisEnabled
	previousClient := common.RDB
	common.RedisEnabled = true
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	require.NoError(t, common.RDB.Ping(context.Background()).Err())
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RedisEnabled = previousEnabled
		common.RDB = previousClient
	})
	return server
}

func assertNoCanvasBillingTokenState(t *testing.T, server *miniredis.Miniredis) {
	t.Helper()
	var tokenRows int64
	require.NoError(t, model.DB.Model(&model.Token{}).Count(&tokenRows).Error)
	assert.Zero(t, tokenRows)
	for _, key := range server.Keys() {
		assert.False(t, strings.HasPrefix(key, "token:"), "unexpected token cache key %q", key)
	}
}

func waitForCanvasBillingValue(t *testing.T, load func() int, expected int) {
	t.Helper()
	require.Eventually(t, func() bool { return load() == expected }, 2*time.Second, 10*time.Millisecond)
}

func TestCanvasTokenQuotaExemptWalletLifecycle(t *testing.T) {
	truncate(t)
	seedUser(t, 8101, 1_000)
	server := useCanvasBillingRedis(t)
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)

	info := &relaycommon.RelayInfo{
		UserId:           8101,
		TokenQuotaExempt: true,
		RequestId:        "canvas-wallet-settle",
		OriginModelName:  "priced-canvas-model",
		UserSetting:      dto.UserSetting{BillingPreference: "wallet_only"},
	}
	session, apiErr := NewBillingSession(ctx, info, 100)
	require.Nil(t, apiErr)
	require.NoError(t, session.Reserve(160))
	require.NoError(t, session.Settle(120))
	var wallet model.User
	require.NoError(t, model.DB.Select("quota").First(&wallet, info.UserId).Error)
	assert.Equal(t, 880, wallet.Quota)
	assert.Equal(t, 160, info.FinalPreConsumedQuota)
	assert.Zero(t, session.tokenConsumed)
	assertNoCanvasBillingTokenState(t, server)

	refundInfo := &relaycommon.RelayInfo{
		UserId:           info.UserId,
		TokenQuotaExempt: true,
		RequestId:        "canvas-wallet-refund",
		OriginModelName:  info.OriginModelName,
		UserSetting:      dto.UserSetting{BillingPreference: "wallet_only"},
	}
	refundSession, apiErr := NewBillingSession(ctx, refundInfo, 80)
	require.Nil(t, apiErr)
	assert.True(t, refundSession.NeedsRefund())
	refundSession.Refund(ctx)
	waitForCanvasBillingValue(t, func() int {
		value, loadErr := model.GetUserQuota(info.UserId, false)
		if loadErr != nil {
			return -1
		}
		return value
	}, 880)
	assertNoCanvasBillingTokenState(t, server)
}

func TestCanvasTokenQuotaExemptSubscriptionLifecycle(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.SubscriptionPlan{}, &model.SubscriptionPreConsumeRecord{}))
	seedUser(t, 8102, 1_000)
	now := time.Now().Unix()
	plan := &model.SubscriptionPlan{Id: 8202, Title: "Canvas plan", Enabled: true, TotalAmount: 2_000, QuotaResetPeriod: model.SubscriptionResetNever}
	require.NoError(t, model.DB.Create(plan).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{
		Id: 8302, UserId: 8102, PlanId: plan.Id, AmountTotal: 2_000,
		Status: "active", StartTime: now - 60, EndTime: now + 3600,
	}).Error)
	server := useCanvasBillingRedis(t)
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)

	info := &relaycommon.RelayInfo{
		UserId:           8102,
		TokenQuotaExempt: true,
		RequestId:        "canvas-subscription-settle",
		OriginModelName:  "priced-canvas-model",
		UserSetting:      dto.UserSetting{BillingPreference: "subscription_only"},
	}
	session, apiErr := NewBillingSession(ctx, info, 100)
	require.Nil(t, apiErr)
	require.NoError(t, session.Reserve(160))
	require.NoError(t, session.Settle(120))
	loadUsed := func() int {
		var subscription model.UserSubscription
		if err := model.DB.First(&subscription, 8302).Error; err != nil {
			return -1
		}
		return int(subscription.AmountUsed)
	}
	assert.Equal(t, 120, loadUsed())
	assert.Equal(t, BillingSourceSubscription, info.BillingSource)
	assert.Zero(t, session.tokenConsumed)
	assertNoCanvasBillingTokenState(t, server)

	refundInfo := &relaycommon.RelayInfo{
		UserId:           info.UserId,
		TokenQuotaExempt: true,
		RequestId:        "canvas-subscription-refund",
		OriginModelName:  info.OriginModelName,
		UserSetting:      dto.UserSetting{BillingPreference: "subscription_only"},
	}
	refundSession, apiErr := NewBillingSession(ctx, refundInfo, 80)
	require.Nil(t, apiErr)
	assert.True(t, refundSession.NeedsRefund())
	refundSession.Refund(ctx)
	waitForCanvasBillingValue(t, loadUsed, 120)
	assertNoCanvasBillingTokenState(t, server)
}

func TestTokenIDZeroStillFailsWithoutTrustedCanvasExemption(t *testing.T) {
	truncate(t)
	seedUser(t, 8103, 1_000)
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	info := &relaycommon.RelayInfo{
		UserId:          8103,
		RequestId:       "normal-token-zero",
		OriginModelName: "priced-model",
		UserSetting:     dto.UserSetting{BillingPreference: "wallet_only"},
	}

	session, apiErr := NewBillingSession(ctx, info, 100)
	assert.Nil(t, session)
	require.NotNil(t, apiErr)
	assert.Equal(t, 403, apiErr.StatusCode)
	quota, err := model.GetUserQuota(info.UserId, false)
	require.NoError(t, err)
	assert.Equal(t, 1_000, quota)
}

func TestCanvasTokenQuotaExemptLegacyFallbackKeepsFunding(t *testing.T) {
	truncate(t)
	seedUser(t, 8104, 1_000)
	server := useCanvasBillingRedis(t)
	info := &relaycommon.RelayInfo{UserId: 8104, TokenQuotaExempt: true, BillingSource: BillingSourceWallet}
	require.NoError(t, PostConsumeQuota(info, 100, 0, false))
	require.NoError(t, PostConsumeQuota(info, -40, 100, false))
	quota, err := model.GetUserQuota(info.UserId, false)
	require.NoError(t, err)
	assert.Equal(t, 940, quota)
	assertNoCanvasBillingTokenState(t, server)
}
