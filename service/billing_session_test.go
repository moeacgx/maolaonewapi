package service

import (
	"testing"
	"time"

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
