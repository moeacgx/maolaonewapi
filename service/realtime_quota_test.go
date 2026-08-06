package service

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRealtimeCumulativeReservationSettlesExactlyOnce(t *testing.T) {
	truncate(t)
	const (
		userID    = 9201
		tokenID   = 9201
		channelID = 9201
		tokenKey  = "realtime-cumulative-reservation"
		modelName = "gpt-4o-realtime-preview"
	)
	initialQuota := common.GetTrustQuota() + 100_000
	seedUser(t, userID, initialQuota)
	seedToken(t, tokenID, userID, tokenKey, initialQuota)
	seedChannel(t, channelID)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
	ctx.Set("token_name", "realtime-test-token")
	ctx.Set("token_quota", initialQuota)

	modelRatio, _, _ := ratio_setting.GetModelRatio(modelName)
	groupRatio := ratio_setting.GetGroupRatio("default")
	info := &relaycommon.RelayInfo{
		TokenId:         tokenID,
		TokenKey:        tokenKey,
		UserId:          userID,
		UsingGroup:      "default",
		UserGroup:       "default",
		StartTime:       time.Now(),
		OriginModelName: modelName,
		RequestId:       "realtime-cumulative-reservation",
		UserSetting:     dto.UserSetting{BillingPreference: "wallet_only"},
		PriceData: types.PriceData{
			ModelRatio: modelRatio,
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: groupRatio,
			},
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:         channelID,
			UpstreamModelName: modelName,
		},
	}

	require.Nil(t, PreConsumeBilling(ctx, 100, info))
	require.NotNil(t, info.Billing)
	require.Zero(t, info.Billing.GetPreConsumedQuota())
	require.Equal(t, initialQuota, getUserQuota(t, userID))
	require.Equal(t, initialQuota, getTokenRemainQuota(t, tokenID))

	firstUsage := &dto.RealtimeUsage{
		TotalTokens: 200,
		InputTokens: 200,
		InputTokenDetails: dto.InputTokenDetails{
			TextTokens: 200,
		},
	}
	require.NoError(t, PreWssConsumeQuota(ctx, info, firstUsage))
	firstReserved := info.Billing.GetPreConsumedQuota()
	firstExpected, clamp := calculateAudioQuota(QuotaInfo{
		InputDetails: TokenDetails{TextTokens: firstUsage.InputTokenDetails.TextTokens},
		ModelName:    modelName, ModelRatio: modelRatio, GroupRatio: groupRatio,
	})
	require.Nil(t, clamp)
	require.Equal(t, firstExpected, firstReserved)
	require.Equal(t, initialQuota-firstExpected, getUserQuota(t, userID))
	require.Equal(t, initialQuota-firstExpected, getTokenRemainQuota(t, tokenID))

	finalUsage := &dto.RealtimeUsage{
		TotalTokens: 400,
		InputTokens: 400,
		InputTokenDetails: dto.InputTokenDetails{
			TextTokens: 400,
		},
	}
	require.NoError(t, PreWssConsumeQuota(ctx, info, finalUsage))
	finalReserved := info.Billing.GetPreConsumedQuota()
	finalExpected, clamp := calculateAudioQuota(QuotaInfo{
		InputDetails: TokenDetails{TextTokens: finalUsage.InputTokenDetails.TextTokens},
		ModelName:    modelName, ModelRatio: modelRatio, GroupRatio: groupRatio,
	})
	require.Nil(t, clamp)
	require.Greater(t, finalExpected, firstExpected)
	require.Equal(t, finalExpected, finalReserved)
	require.Equal(t, initialQuota-finalExpected, getUserQuota(t, userID))
	require.Equal(t, initialQuota-finalExpected, getTokenRemainQuota(t, tokenID))

	PostWssConsumeQuota(ctx, info, modelName, finalUsage, "")
	require.Equal(t, initialQuota-finalExpected, getUserQuota(t, userID))
	require.Equal(t, initialQuota-finalExpected, getTokenRemainQuota(t, tokenID))
}

func TestRealtimeLegacyReservationDoesNotChargeWalletWhenTokenUpdateFails(t *testing.T) {
	truncate(t)
	const (
		userID       = 9202
		tokenID      = 9202
		channelID    = 9202
		tokenKey     = "realtime-legacy-reservation"
		modelName    = "gpt-4o-realtime-preview"
		initialQuota = 100_000
	)
	seedUser(t, userID, initialQuota)
	seedToken(t, tokenID, userID, tokenKey, initialQuota)
	seedChannel(t, channelID)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
	ctx.Set("token_name", "realtime-legacy-test-token")

	modelRatio, _, _ := ratio_setting.GetModelRatio(modelName)
	groupRatio := ratio_setting.GetGroupRatio("default")
	info := &relaycommon.RelayInfo{
		TokenId: tokenID, TokenKey: tokenKey, UserId: userID,
		UsingGroup: "default", UserGroup: "default", StartTime: time.Now(),
		OriginModelName: modelName, RequestId: "realtime-legacy-reservation",
		BillingSource: BillingSourceWallet,
		PriceData: types.PriceData{
			ModelRatio:     modelRatio,
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: groupRatio},
		},
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channelID, UpstreamModelName: modelName},
	}
	usage := &dto.RealtimeUsage{
		TotalTokens: 400, InputTokens: 400,
		InputTokenDetails: dto.InputTokenDetails{TextTokens: 400},
	}

	injectedErr := errors.New("forced token update failure")
	callbackName := "test:reject_realtime_token_update"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "tokens" {
			tx.AddError(injectedErr)
		}
	}))
	callbackRegistered := true
	t.Cleanup(func() {
		if callbackRegistered {
			_ = model.DB.Callback().Update().Remove(callbackName)
		}
	})

	err := PreWssConsumeQuota(ctx, info, usage)
	require.ErrorIs(t, err, injectedErr)
	require.Zero(t, info.FinalPreConsumedQuota)
	require.Equal(t, initialQuota, getUserQuota(t, userID))
	require.Equal(t, initialQuota, getTokenRemainQuota(t, tokenID))

	require.NoError(t, model.DB.Callback().Update().Remove(callbackName))
	callbackRegistered = false
	require.NoError(t, PreWssConsumeQuota(ctx, info, usage))
	expectedQuota, clamp := calculateAudioQuota(QuotaInfo{
		InputDetails: TokenDetails{TextTokens: usage.InputTokenDetails.TextTokens},
		ModelName:    modelName, ModelRatio: modelRatio, GroupRatio: groupRatio,
	})
	require.Nil(t, clamp)
	require.Equal(t, expectedQuota, info.FinalPreConsumedQuota)
	require.Equal(t, initialQuota-expectedQuota, getUserQuota(t, userID))
	require.Equal(t, initialQuota-expectedQuota, getTokenRemainQuota(t, tokenID))

	PostWssConsumeQuota(ctx, info, modelName, usage, "")
	require.Equal(t, initialQuota-expectedQuota, getUserQuota(t, userID))
	require.Equal(t, initialQuota-expectedQuota, getTokenRemainQuota(t, tokenID))
}

func TestBillingSessionReserveTokenFailureDoesNotChargeWallet(t *testing.T) {
	truncate(t)
	const (
		userID       = 9203
		tokenID      = 9203
		tokenKey     = "realtime-session-token-failure"
		initialQuota = 100_000
		reserveQuota = 1_000
	)
	seedUser(t, userID, initialQuota)
	seedToken(t, tokenID, userID, tokenKey, initialQuota)
	previousBatchUpdateEnabled := common.BatchUpdateEnabled
	previousRedisEnabled := common.RedisEnabled
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.BatchUpdateEnabled = previousBatchUpdateEnabled
		common.RedisEnabled = previousRedisEnabled
	})

	tokenErr := errors.New("forced session token reserve failure")
	userUpdates := 0
	callbackName := "test:reject_session_token_reserve"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		switch tx.Statement.Table {
		case "tokens":
			tx.AddError(tokenErr)
		case "users":
			userUpdates++
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Update().Remove(callbackName) })

	session := &BillingSession{
		relayInfo: &relaycommon.RelayInfo{TokenId: tokenID, TokenKey: tokenKey, UserId: userID},
		funding:   &WalletFunding{userId: userID},
		trusted:   true,
	}
	err := session.ReserveRealtime(reserveQuota)
	require.ErrorIs(t, err, tokenErr)
	require.Zero(t, userUpdates)
	require.Equal(t, initialQuota, getUserQuota(t, userID))
	require.Equal(t, initialQuota, getTokenRemainQuota(t, tokenID))
}

func TestBillingSessionReserveReportsFundingAndTokenRollbackFailures(t *testing.T) {
	truncate(t)
	const (
		userID       = 9204
		tokenID      = 9204
		tokenKey     = "realtime-session-rollback-failure"
		initialQuota = 100_000
		reserveQuota = 1_000
	)
	seedUser(t, userID, initialQuota)
	seedToken(t, tokenID, userID, tokenKey, initialQuota)
	previousBatchUpdateEnabled := common.BatchUpdateEnabled
	previousRedisEnabled := common.RedisEnabled
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.BatchUpdateEnabled = previousBatchUpdateEnabled
		common.RedisEnabled = previousRedisEnabled
	})

	fundingErr := errors.New("forced wallet reserve failure")
	rollbackErr := errors.New("forced token rollback failure")
	tokenUpdates := 0
	updateOrder := make([]string, 0, 3)
	callbackName := "test:reject_session_funding_and_token_rollback"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		switch tx.Statement.Table {
		case "tokens":
			tokenUpdates++
			updateOrder = append(updateOrder, "token")
			if tokenUpdates == 2 {
				tx.AddError(rollbackErr)
			}
		case "users":
			updateOrder = append(updateOrder, "wallet")
			tx.AddError(fundingErr)
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Update().Remove(callbackName) })

	session := &BillingSession{
		relayInfo: &relaycommon.RelayInfo{TokenId: tokenID, TokenKey: tokenKey, UserId: userID},
		funding:   &WalletFunding{userId: userID},
		trusted:   true,
	}
	err := session.ReserveRealtime(reserveQuota)
	require.ErrorIs(t, err, fundingErr)
	require.ErrorIs(t, err, rollbackErr)
	require.Equal(t, []string{"token", "wallet", "token"}, updateOrder)
	require.Equal(t, initialQuota, getUserQuota(t, userID))
	require.Equal(t, initialQuota-reserveQuota, getTokenRemainQuota(t, tokenID))
}
