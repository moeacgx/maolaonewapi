package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
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

type failingBillingFunding struct {
	source string
	err    error
}

func (f *failingBillingFunding) Source() string       { return f.source }
func (f *failingBillingFunding) PreConsume(int) error { return f.err }
func (f *failingBillingFunding) Settle(int) error     { return f.err }
func (f *failingBillingFunding) Refund() error        { return nil }

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

func TestRealtimeReservationExtendsInitialPreConsumeToTarget(t *testing.T) {
	truncate(t)
	const (
		userID       = 9213
		tokenID      = 9213
		tokenKey     = "realtime-initial-pre-consume"
		initialQuota = 100_000
	)
	seedUser(t, userID, initialQuota)
	seedToken(t, tokenID, userID, tokenKey, initialQuota)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
	ctx.Set("token_quota", initialQuota)
	info := &relaycommon.RelayInfo{
		TokenId: tokenID, TokenKey: tokenKey, UserId: userID,
		RequestId:       "realtime-initial-pre-consume",
		ForcePreConsume: true,
		UserSetting:     dto.UserSetting{BillingPreference: "wallet_only"},
	}

	require.Nil(t, PreConsumeBilling(ctx, 100, info))
	require.Equal(t, 100, info.Billing.GetPreConsumedQuota())
	require.Equal(t, initialQuota-100, getUserQuota(t, userID))
	require.Equal(t, initialQuota-100, getTokenRemainQuota(t, tokenID))

	require.NoError(t, info.Billing.ReserveRealtime(250))
	require.Equal(t, 250, info.Billing.GetPreConsumedQuota())
	require.Equal(t, initialQuota-250, getUserQuota(t, userID))
	require.Equal(t, initialQuota-250, getTokenRemainQuota(t, tokenID))

	require.NoError(t, info.Billing.Settle(250))
	require.Equal(t, initialQuota-250, getUserQuota(t, userID))
	require.Equal(t, initialQuota-250, getTokenRemainQuota(t, tokenID))
}

func TestBillingSessionGetPreConsumedQuotaConcurrentWithReserveRealtime(t *testing.T) {
	truncate(t)
	const (
		userID       = 9214
		initialQuota = 100_000
		finalTarget  = 200
	)
	seedUser(t, userID, initialQuota)
	session := &BillingSession{
		relayInfo: &relaycommon.RelayInfo{
			UserId: userID, RequestId: "concurrent-realtime-reservation", SkipTokenQuota: true,
		},
		funding: &WalletFunding{userId: userID},
		trusted: true,
	}

	start := make(chan struct{})
	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		for target := 1; target <= finalTarget; target++ {
			if err := session.ReserveRealtime(target); err != nil {
				errCh <- err
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < finalTarget*20; i++ {
			_ = session.GetPreConsumedQuota()
		}
	}()
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}

	require.Equal(t, finalTarget, session.GetPreConsumedQuota())
	require.Equal(t, initialQuota-finalTarget, getUserQuota(t, userID))
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

func TestRealtimeLegacyFundingFailureRetriesIdempotentTokenCompensation(t *testing.T) {
	truncate(t)
	const (
		userID       = 9209
		tokenID      = 9209
		tokenKey     = "legacy-funding-compensation"
		initialQuota = 100_000
		reserveQuota = 1_000
	)
	seedUser(t, userID, initialQuota)
	seedToken(t, tokenID, userID, tokenKey, initialQuota)

	fundingErr := errors.New("forced legacy wallet reserve failure")
	compensationErr := errors.New("forced first legacy token compensation failure")
	tokenUpdates := 0
	callbackName := "test:legacy_funding_compensation"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		switch tx.Statement.Table {
		case "tokens":
			tokenUpdates++
			if tokenUpdates == 2 {
				tx.AddError(compensationErr)
			}
		case "users":
			tx.AddError(fundingErr)
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Update().Remove(callbackName) })

	info := &relaycommon.RelayInfo{
		TokenId: tokenID, TokenKey: tokenKey, UserId: userID,
		RequestId: "legacy-funding-compensation", BillingSource: BillingSourceWallet,
	}
	err := reserveLegacyRealtimeQuota(info, reserveQuota)
	require.ErrorIs(t, err, fundingErr)
	require.ErrorIs(t, err, compensationErr)
	require.Equal(t, initialQuota, getUserQuota(t, userID))
	require.Equal(t, initialQuota, getTokenRemainQuota(t, tokenID))
}

func TestRealtimeLegacySubscriptionReservationClassifiesBusinessAndDatabaseErrors(t *testing.T) {
	truncate(t)
	const (
		userID       = 9215
		tokenID      = 9215
		subID        = 9215
		tokenKey     = "legacy-subscription-error-classification"
		initialQuota = 100_000
	)
	seedUser(t, userID, initialQuota)
	seedToken(t, tokenID, userID, tokenKey, initialQuota)
	seedSubscription(t, subID, userID, 100, 90)

	newRelayInfo := func(requestID string) *relaycommon.RelayInfo {
		return &relaycommon.RelayInfo{
			TokenId: tokenID, TokenKey: tokenKey, UserId: userID,
			RequestId: requestID, BillingSource: BillingSourceSubscription, SubscriptionId: subID,
		}
	}

	quotaErr := NewRealtimeQuotaError(
		reserveLegacyRealtimeQuota(newRelayInfo("legacy-subscription-quota-error"), 20),
	)
	require.Equal(t, http.StatusForbidden, quotaErr.StatusCode)
	require.Equal(t, types.ErrorCodeInsufficientUserQuota, quotaErr.GetErrorCode())
	require.Equal(t, initialQuota, getTokenRemainQuota(t, tokenID))
	require.EqualValues(t, 90, getSubscriptionUsed(t, subID))

	injectedErr := errors.New("forced legacy subscription database failure")
	callbackName := "test:legacy_subscription_reserve_database_failure"
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "user_subscriptions" {
			tx.AddError(injectedErr)
		}
	}))
	callbackRegistered := true
	t.Cleanup(func() {
		if callbackRegistered {
			_ = model.DB.Callback().Query().Remove(callbackName)
		}
	})

	databaseErr := NewRealtimeQuotaError(
		reserveLegacyRealtimeQuota(newRelayInfo("legacy-subscription-database-error"), 5),
	)
	require.Equal(t, http.StatusServiceUnavailable, databaseErr.StatusCode)
	require.Equal(t, types.ErrorCodeUpdateDataError, databaseErr.GetErrorCode())
	require.ErrorIs(t, databaseErr, injectedErr)
	require.NoError(t, model.DB.Callback().Query().Remove(callbackName))
	callbackRegistered = false

	require.Equal(t, initialQuota, getTokenRemainQuota(t, tokenID))
	require.EqualValues(t, 90, getSubscriptionUsed(t, subID))
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

func TestBillingSessionReserveFundingFailureRetriesIdempotentTokenCompensation(t *testing.T) {
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
	rollbackErr := errors.New("forced first token compensation failure")
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
	require.Equal(t, []string{"token", "wallet", "token", "token"}, updateOrder)
	require.False(t, session.HasPendingTokenCompensation())
	require.Equal(t, initialQuota, getUserQuota(t, userID))
	require.Equal(t, initialQuota, getTokenRemainQuota(t, tokenID))
}

func TestBillingSessionInitialFundingFailureRetriesIdempotentTokenCompensation(t *testing.T) {
	truncate(t)
	const (
		userID       = 9205
		tokenID      = 9205
		tokenKey     = "initial-session-funding-failure"
		initialQuota = 100_000
		reserveQuota = 1_000
	)
	seedUser(t, userID, initialQuota)
	seedToken(t, tokenID, userID, tokenKey, initialQuota)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	ctx.Set("token_quota", initialQuota)

	fundingErr := errors.New("forced initial wallet reserve failure")
	rollbackErr := errors.New("forced first token compensation failure")
	tokenUpdates := 0
	updateOrder := make([]string, 0, 3)
	callbackName := "test:reject_initial_funding_and_token_rollback"
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

	info := &relaycommon.RelayInfo{
		TokenId: tokenID, TokenKey: tokenKey, UserId: userID,
		RequestId:   "initial-session-funding-failure",
		UserSetting: dto.UserSetting{BillingPreference: "wallet_only"},
	}
	apiErr := PreConsumeBilling(ctx, reserveQuota, info)
	require.Error(t, apiErr)
	require.ErrorIs(t, apiErr, fundingErr)
	require.ErrorIs(t, apiErr, rollbackErr)
	require.Nil(t, info.Billing)
	require.Equal(t, []string{"token", "wallet", "token", "token"}, updateOrder)
	require.Equal(t, initialQuota, getUserQuota(t, userID))
	require.Equal(t, initialQuota, getTokenRemainQuota(t, tokenID))
}

func TestInitialCompensationKeySeparatesSessionsAndFundingAttempts(t *testing.T) {
	truncate(t)
	const (
		userID       = 9211
		tokenID      = 9211
		tokenKey     = "funding-attempt-compensation-key"
		initialQuota = 100_000
		reserveQuota = 1_000
	)
	seedUser(t, userID, initialQuota)
	seedToken(t, tokenID, userID, tokenKey, initialQuota)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	ctx.Set("token_quota", initialQuota)
	relayInfo := &relaycommon.RelayInfo{
		TokenId: tokenID, TokenKey: tokenKey, UserId: userID,
		RequestId: "funding-attempt-compensation-key",
	}

	for _, funding := range []FundingSource{
		&failingBillingFunding{source: BillingSourceSubscription, err: model.ErrSubscriptionQuotaInsufficient},
		&failingBillingFunding{source: BillingSourceWallet, err: errors.New("forced wallet failure")},
		&failingBillingFunding{source: BillingSourceWallet, err: errors.New("forced repeated wallet failure")},
	} {
		session := &BillingSession{relayInfo: relayInfo, funding: funding}
		require.Error(t, session.preConsume(ctx, reserveQuota))
	}

	require.Equal(t, initialQuota, getTokenRemainQuota(t, tokenID))
	var compensationCount int64
	require.NoError(t, model.DB.Model(&model.TokenQuotaCompensation{}).
		Where("token_id = ?", tokenID).Count(&compensationCount).Error)
	require.EqualValues(t, 3, compensationCount)
}

func TestReserveRetryAtSameTargetUsesDistinctCompensationOperation(t *testing.T) {
	truncate(t)
	const (
		userID       = 9212
		tokenID      = 9212
		tokenKey     = "same-target-compensation-key"
		initialQuota = 100_000
		reserveQuota = 1_000
	)
	seedUser(t, userID, initialQuota)
	seedToken(t, tokenID, userID, tokenKey, initialQuota)

	session := &BillingSession{
		relayInfo: &relaycommon.RelayInfo{
			TokenId: tokenID, TokenKey: tokenKey, UserId: userID,
			RequestId: "same-target-compensation-key",
		},
		funding: &failingBillingFunding{
			source: BillingSourceWallet,
			err:    errors.New("forced repeated wallet failure"),
		},
		trusted: true,
	}

	require.Error(t, session.ReserveRealtime(reserveQuota))
	require.Error(t, session.ReserveRealtime(reserveQuota))
	require.Equal(t, initialQuota, getTokenRemainQuota(t, tokenID))

	var compensationCount int64
	require.NoError(t, model.DB.Model(&model.TokenQuotaCompensation{}).
		Where("token_id = ?", tokenID).Count(&compensationCount).Error)
	require.EqualValues(t, 2, compensationCount)
}

func TestBillingSessionPendingTokenCompensationCompletesDuringSettlement(t *testing.T) {
	truncate(t)
	const (
		userID       = 9206
		tokenID      = 9206
		tokenKey     = "pending-token-compensation"
		initialQuota = 100_000
		reserveQuota = 1_000
	)
	seedUser(t, userID, initialQuota)
	seedToken(t, tokenID, userID, tokenKey, initialQuota)

	fundingErr := errors.New("forced persistent wallet reserve failure")
	compensationErr := errors.New("forced persistent token compensation failure")
	tokenUpdates := 0
	callbackName := "test:persistent_token_compensation_failure"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		switch tx.Statement.Table {
		case "tokens":
			tokenUpdates++
			if tokenUpdates >= 2 {
				tx.AddError(compensationErr)
			}
		case "users":
			tx.AddError(fundingErr)
		}
	}))
	callbackRegistered := true
	t.Cleanup(func() {
		if callbackRegistered {
			_ = model.DB.Callback().Update().Remove(callbackName)
		}
	})

	session := &BillingSession{
		relayInfo: &relaycommon.RelayInfo{
			TokenId: tokenID, TokenKey: tokenKey, UserId: userID,
			RequestId: "pending-token-compensation",
		},
		funding: &WalletFunding{userId: userID},
		trusted: true,
	}
	err := session.ReserveRealtime(reserveQuota)
	require.ErrorIs(t, err, fundingErr)
	require.ErrorIs(t, err, compensationErr)
	require.True(t, session.HasPendingTokenCompensation())
	require.Equal(t, initialQuota-reserveQuota, getTokenRemainQuota(t, tokenID))

	require.NoError(t, model.DB.Callback().Update().Remove(callbackName))
	callbackRegistered = false
	require.NoError(t, session.Settle(0))
	require.False(t, session.HasPendingTokenCompensation())
	require.Equal(t, initialQuota, getTokenRemainQuota(t, tokenID))
}

func TestSubscriptionFundingErrorUsesBusinessSentinelsOnly(t *testing.T) {
	for _, businessErr := range []error{
		model.ErrNoActiveSubscription,
		model.ErrSubscriptionQuotaInsufficient,
	} {
		apiErr := newSubscriptionFundingError(businessErr)
		require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
		require.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
	}

	textOnlyErr := errors.New("database failed with text: no active subscription")
	apiErr := newSubscriptionFundingError(textOnlyErr)
	require.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
	require.Equal(t, types.ErrorCodeUpdateDataError, apiErr.GetErrorCode())
	require.ErrorIs(t, apiErr, textOnlyErr)
}

func TestInitialSubscriptionDatabaseFailureReturns503AndRestoresToken(t *testing.T) {
	truncate(t)
	const (
		userID       = 9210
		tokenID      = 9210
		tokenKey     = "initial-subscription-database-failure"
		initialQuota = 100_000
		reserveQuota = 1_000
	)
	seedUser(t, userID, initialQuota)
	seedToken(t, tokenID, userID, tokenKey, initialQuota)

	injectedErr := errors.New("forced initial subscription query failure")
	callbackName := "test:initial_subscription_database_failure"
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "user_subscriptions" {
			tx.AddError(injectedErr)
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Query().Remove(callbackName) })

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	ctx.Set("token_quota", initialQuota)
	info := &relaycommon.RelayInfo{
		TokenId: tokenID, TokenKey: tokenKey, UserId: userID,
		RequestId:       "initial-subscription-database-failure",
		UserSetting:     dto.UserSetting{BillingPreference: "subscription_only"},
		OriginModelName: "test-model",
	}

	apiErr := PreConsumeBilling(ctx, reserveQuota, info)
	require.Error(t, apiErr)
	require.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
	require.Equal(t, types.ErrorCodeUpdateDataError, apiErr.GetErrorCode())
	require.ErrorIs(t, apiErr, injectedErr)
	require.Equal(t, initialQuota, getTokenRemainQuota(t, tokenID))
}

func TestSubscriptionFirstAvailabilityDatabaseFailureReturns503(t *testing.T) {
	truncate(t)
	injectedErr := errors.New("forced active subscription availability failure")
	callbackName := "test:subscription_availability_database_failure"
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "user_subscriptions" {
			tx.AddError(injectedErr)
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Query().Remove(callbackName) })

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		UserId:      9213,
		UserSetting: dto.UserSetting{BillingPreference: "subscription_first"},
	}

	session, apiErr := NewBillingSession(ctx, info, 1_000)
	require.Nil(t, session)
	require.Error(t, apiErr)
	require.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
	require.Equal(t, types.ErrorCodeQueryDataError, apiErr.GetErrorCode())
	require.ErrorIs(t, apiErr, injectedErr)
}

func TestBillingSessionSubscriptionReserveClassifiesQuotaAndDatabaseErrors(t *testing.T) {
	truncate(t)
	const (
		userID       = 9207
		tokenID      = 9207
		subID        = 9207
		tokenKey     = "subscription-reserve-error-classification"
		initialQuota = 100_000
	)
	seedUser(t, userID, initialQuota)
	seedToken(t, tokenID, userID, tokenKey, initialQuota)
	seedSubscription(t, subID, userID, 100, 90)

	newSession := func(requestID string) *BillingSession {
		return &BillingSession{
			relayInfo: &relaycommon.RelayInfo{
				TokenId: tokenID, TokenKey: tokenKey, UserId: userID,
				RequestId: requestID, SubscriptionId: subID,
			},
			funding: &SubscriptionFunding{subscriptionId: subID},
			trusted: true,
		}
	}

	quotaErr := NewRealtimeQuotaError(newSession("subscription-quota-error").ReserveRealtime(20))
	require.Equal(t, http.StatusForbidden, quotaErr.StatusCode)
	require.Equal(t, types.ErrorCodeInsufficientUserQuota, quotaErr.GetErrorCode())
	require.Equal(t, initialQuota, getTokenRemainQuota(t, tokenID))

	injectedErr := errors.New("forced subscription database failure")
	callbackName := "test:subscription_reserve_database_failure"
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "user_subscriptions" {
			tx.AddError(injectedErr)
		}
	}))
	callbackRegistered := true
	t.Cleanup(func() {
		if callbackRegistered {
			_ = model.DB.Callback().Query().Remove(callbackName)
		}
	})

	databaseErr := NewRealtimeQuotaError(newSession("subscription-database-error").ReserveRealtime(20))
	require.Equal(t, http.StatusServiceUnavailable, databaseErr.StatusCode)
	require.Equal(t, types.ErrorCodeUpdateDataError, databaseErr.GetErrorCode())
	require.ErrorIs(t, databaseErr, injectedErr)
	require.Equal(t, initialQuota, getTokenRemainQuota(t, tokenID))

	require.NoError(t, model.DB.Callback().Query().Remove(callbackName))
	callbackRegistered = false
}

func TestBillingSessionRejectsConflictingSettlementTarget(t *testing.T) {
	session := &BillingSession{
		relayInfo: &relaycommon.RelayInfo{SkipTokenQuota: true},
		funding:   &WalletFunding{},
	}
	require.NoError(t, session.Settle(0))
	err := session.Settle(1)
	require.ErrorContains(t, err, "settlement target conflict")
}

func TestBillingSessionRejectsNegativeSettlementTarget(t *testing.T) {
	session := &BillingSession{
		relayInfo: &relaycommon.RelayInfo{SkipTokenQuota: true},
		funding:   &WalletFunding{},
	}
	require.ErrorContains(t, session.Settle(-1), "must be non-negative")
	require.False(t, session.settleRequested)
}

func TestBillingSessionStartedSettlementRequiresExplicitRetry(t *testing.T) {
	truncate(t)
	const (
		userID       = 9208
		tokenID      = 9208
		tokenKey     = "settlement-intent-cannot-refund"
		initialQuota = 100_000
		preConsumed  = 1_000
	)
	seedUser(t, userID, initialQuota-preConsumed)
	seedToken(t, tokenID, userID, tokenKey, initialQuota-preConsumed)

	injectedErr := errors.New("forced settlement funding failure")
	userUpdates := 0
	tokenUpdates := 0
	callbackName := "test:settlement_intent_cannot_refund"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		switch tx.Statement.Table {
		case "users":
			userUpdates++
			tx.AddError(injectedErr)
		case "tokens":
			tokenUpdates++
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Update().Remove(callbackName) })

	session := &BillingSession{
		relayInfo: &relaycommon.RelayInfo{
			TokenId: tokenID, TokenKey: tokenKey, UserId: userID,
		},
		funding:          &WalletFunding{userId: userID, consumed: preConsumed},
		preConsumedQuota: preConsumed,
		tokenConsumed:    preConsumed,
	}
	require.ErrorIs(t, session.Settle(preConsumed/2), injectedErr)
	require.NoError(t, model.DB.Callback().Update().Remove(callbackName))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	session.Refund(ctx)

	require.Equal(t, 1, userUpdates)
	require.Zero(t, tokenUpdates)
	require.False(t, session.settled)
	require.Equal(t, initialQuota-preConsumed, getUserQuota(t, userID))
	require.Equal(t, initialQuota-preConsumed, getTokenRemainQuota(t, tokenID))
	require.ErrorContains(t, session.ReserveRealtime(preConsumed+1), "settlement already started")

	require.NoError(t, session.Settle(preConsumed/2))
	expectedQuota := initialQuota - preConsumed/2
	require.True(t, session.settled)
	require.Equal(t, expectedQuota, getUserQuota(t, userID))
	require.Equal(t, expectedQuota, getTokenRemainQuota(t, tokenID))
}

func TestBillingSessionExplicitRetryDoesNotRepeatFundingSettlement(t *testing.T) {
	truncate(t)
	const (
		userID       = 9213
		tokenID      = 9213
		tokenKey     = "settlement-token-step-retry"
		initialQuota = 100_000
		preConsumed  = 1_000
	)
	actualQuota := preConsumed / 2
	seedUser(t, userID, initialQuota-preConsumed)
	seedToken(t, tokenID, userID, tokenKey, initialQuota-preConsumed)

	injectedErr := errors.New("forced settlement token failure")
	callbackName := "test:settlement_token_step_retry"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "tokens" {
			tx.AddError(injectedErr)
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Update().Remove(callbackName) })

	session := &BillingSession{
		relayInfo: &relaycommon.RelayInfo{
			TokenId: tokenID, TokenKey: tokenKey, UserId: userID,
		},
		funding:          &WalletFunding{userId: userID, consumed: preConsumed},
		preConsumedQuota: preConsumed,
		tokenConsumed:    preConsumed,
	}
	require.ErrorIs(t, session.Settle(actualQuota), injectedErr)
	require.Equal(t, initialQuota-preConsumed+actualQuota, getUserQuota(t, userID))
	require.Equal(t, initialQuota-preConsumed, getTokenRemainQuota(t, tokenID))

	require.NoError(t, model.DB.Callback().Update().Remove(callbackName))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	session.Refund(ctx)
	require.False(t, session.settled)
	require.Equal(t, initialQuota-preConsumed+actualQuota, getUserQuota(t, userID))
	require.Equal(t, initialQuota-preConsumed, getTokenRemainQuota(t, tokenID))

	require.NoError(t, session.Settle(actualQuota))
	expectedQuota := initialQuota - actualQuota
	require.True(t, session.settled)
	require.Equal(t, expectedQuota, getUserQuota(t, userID))
	require.Equal(t, expectedQuota, getTokenRemainQuota(t, tokenID))
}
