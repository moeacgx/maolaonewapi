package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingFundingSource struct {
	settleDeltas []int
}

func (f *recordingFundingSource) Source() string { return BillingSourceWallet }

func (f *recordingFundingSource) PreConsume(int) error { return nil }

func (f *recordingFundingSource) Settle(delta int) error {
	f.settleDeltas = append(f.settleDeltas, delta)
	return nil
}

func (f *recordingFundingSource) Refund() error { return nil }

func TestPostTextConsumeQuotaKeepsBillingOpenForZeroUsageRetry(t *testing.T) {
	truncate(t)

	gin.SetMode(gin.TestMode)
	ctx, info, billing := newRetryBillingRelayInfo()

	err := PostTextConsumeQuota(ctx, info, &dto.Usage{}, nil)

	require.Error(t, err)
	assert.Equal(t, types.ErrorCodeEmptyResponse, err.GetErrorCode())
	assert.Empty(t, billing.settleCalls, "a retryable zero-usage attempt must not settle billing")
	assert.True(t, billing.NeedsRefund(), "the reservation must remain open for retry or final refund")

	// 后续渠道成功时仍应使用同一个预扣会话完成结算。
	require.NoError(t, billing.Settle(40))
	assert.Equal(t, []int{40}, billing.settleCalls)
	assert.False(t, billing.NeedsRefund())
}

func TestPostTextConsumeQuotaRecordsOnlyEligibleAffinityUsage(t *testing.T) {
	truncate(t)
	isolateChannelAffinityUsageCacheForTest(t)

	tests := []struct {
		name      string
		usage     *dto.Usage
		stream    bool
		cancelled bool
		wantError bool
		wantTotal int64
		wantHit   int64
		wantCache int64
	}{
		{name: "zero usage failure", usage: &dto.Usage{}, wantError: true},
		{
			name:      "successful cache miss",
			usage:     &dto.Usage{PromptTokens: 100, CompletionTokens: 10, TotalTokens: 110},
			wantTotal: 1,
		},
		{
			name: "successful cache hit",
			usage: &dto.Usage{
				PromptTokens:        100,
				CompletionTokens:    10,
				TotalTokens:         110,
				PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 80},
			},
			wantTotal: 1,
			wantHit:   1,
			wantCache: 80,
		},
		{
			name:   "stream without terminal status",
			stream: true,
			usage: &dto.Usage{
				PromptTokens:        100,
				CompletionTokens:    10,
				TotalTokens:         110,
				PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 80},
			},
		},
		{
			name: "cancelled stream with partial usage",
			usage: &dto.Usage{
				PromptTokens:        100,
				CompletionTokens:    10,
				TotalTokens:         110,
				PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 80},
			},
			cancelled: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const usingGroup = "default"
			ruleName := t.Name()
			keyFP := test.name
			ctx := buildChannelAffinityStatsContextForTest(ruleName, usingGroup, keyFP)
			_, info, _ := newRetryBillingRelayInfo()
			info.FinalRequestRelayFormat = types.RelayFormatOpenAIResponses
			info.IsStream = test.stream || test.cancelled
			if test.cancelled {
				info.StreamStatus = relaycommon.NewStreamStatus()
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, nil)
			}

			err := PostTextConsumeQuota(ctx, info, test.usage, nil)

			if test.wantError {
				require.Error(t, err)
			} else {
				require.Nil(t, err)
			}
			stats := GetChannelAffinityUsageCacheStats(ruleName, usingGroup, keyFP)
			assert.Equal(t, test.wantTotal, stats.Total)
			assert.Equal(t, test.wantHit, stats.Hit)
			assert.Equal(t, test.wantCache, stats.CachedTokens)
			if test.wantTotal == 0 {
				assert.Zero(t, stats.LastSeenAt)
			}
		})
	}
}

func TestPostTextConsumeQuotaTreatsClientGoneAsNonBillableCancellation(t *testing.T) {
	truncate(t)

	ctx, info, billing := newRetryBillingRelayInfo()
	info.IsStream = true
	info.StreamStatus = relaycommon.NewStreamStatus()
	info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, context.Canceled)

	err := PostTextConsumeQuota(ctx, info, &dto.Usage{}, nil)

	require.Error(t, err)
	assert.Equal(t, types.ErrorCodeDoRequestFailed, err.GetErrorCode())
	assert.Empty(t, billing.settleCalls)
	assert.True(t, billing.NeedsRefund())

	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ?", info.UserId).Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Contains(t, logs[0].Content, "客户端已断开")
	assert.NotContains(t, logs[0].Content, "上游没有返回计费信息")
}

func TestBillingSessionSettlesSuccessfulZeroQuota(t *testing.T) {
	info := &relaycommon.RelayInfo{TokenQuotaExempt: true}
	funding := &recordingFundingSource{}
	session := &BillingSession{
		relayInfo:        info,
		funding:          funding,
		preConsumedQuota: 100,
		tokenConsumed:    100,
	}

	require.NoError(t, session.Settle(0))
	assert.False(t, session.NeedsRefund())
	assert.Equal(t, []int{-100}, funding.settleDeltas)
}

type retryBillingSettler struct {
	preConsumedQuota int
	settled          bool
	settleCalls      []int
}

func (s *retryBillingSettler) Settle(quota int) error {
	s.settleCalls = append(s.settleCalls, quota)
	s.settled = true
	return nil
}

func (*retryBillingSettler) Refund(*gin.Context) {}

func (s *retryBillingSettler) NeedsRefund() bool { return !s.settled }

func (s *retryBillingSettler) GetPreConsumedQuota() int { return s.preConsumedQuota }

func (*retryBillingSettler) Reserve(int) error { return nil }

func newRetryBillingRelayInfo() (*gin.Context, *relaycommon.RelayInfo, *retryBillingSettler) {
	ctx, _ := gin.CreateTestContext(nil)
	info := &relaycommon.RelayInfo{
		UserId:                1,
		TokenId:               1,
		OriginModelName:       "gpt-test",
		StartTime:             time.Now(),
		FinalPreConsumedQuota: 100,
		ChannelMeta:           &relaycommon.ChannelMeta{ChannelId: 1},
		PriceData: hosttypes.PriceData{
			ModelRatio:      1,
			CompletionRatio: 1,
			GroupRatioInfo:  hosttypes.GroupRatioInfo{GroupRatio: 1},
		},
	}
	billing := &retryBillingSettler{preConsumedQuota: 100}
	info.Billing = billing
	return ctx, info, billing
}

func TestPostAudioConsumeQuotaKeepsBillingOpenForZeroUsageRetry(t *testing.T) {
	truncate(t)

	ctx, info, billing := newRetryBillingRelayInfo()
	err := PostAudioConsumeQuota(ctx, info, &dto.Usage{}, "")

	require.Error(t, err)
	assert.Equal(t, types.ErrorCodeEmptyResponse, err.GetErrorCode())
	assert.Empty(t, billing.settleCalls)
	assert.True(t, billing.NeedsRefund())
}

func TestPostWssConsumeQuotaKeepsBillingOpenForZeroUsageRetry(t *testing.T) {
	truncate(t)

	ctx, info, billing := newRetryBillingRelayInfo()
	err := PostWssConsumeQuota(ctx, info, "gpt-test", &dto.RealtimeUsage{}, "")

	require.Error(t, err)
	assert.Equal(t, types.ErrorCodeEmptyResponse, err.GetErrorCode())
	assert.Empty(t, billing.settleCalls)
	assert.True(t, billing.NeedsRefund())
}
