package service

import (
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type recordingFundingSource struct {
	settleDeltas           []int
	preConsumed            []int
	refunds                int
	err                    error
	refundErr              error
	settleErr              error
	source                 string
	capacity               int
	additional             int
	consumed               int
	additionalReservations []int
	additionalRefunds      []int
}

type refundFailingBenefitFunding struct {
	*BenefitVoucherFunding
	refundErr error
}

func (f *refundFailingBenefitFunding) Refund() error { return f.refundErr }

func TestNewBillingSessionSkipsBenefitVoucherForInheritedGroup(t *testing.T) {
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	model.InitDBColumns()
	db, err := gorm.Open(sqlite.Open("file:billing_session_benefit_explicit_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLog)
		model.InitDBColumns()
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.Group{}, &model.GroupAlias{}, &model.User{}, &model.BenefitActivity{}, &model.BenefitUserVoucher{}))
	group := &model.Group{Code: "benefit", Name: "活动福利", Ratio: 1, Status: model.GroupStatusActive}
	require.NoError(t, db.Create(group).Error)
	user := &model.User{Id: 4321, Username: "benefit-session-user", Group: group.Code, GroupId: group.Id, Quota: 1000, CreatedAt: 1}
	require.NoError(t, db.Create(user).Error)
	now := time.Now().Unix()
	activity := &model.BenefitActivity{
		Name: "周末福利", GroupId: group.Id, Status: model.BenefitActivityStatusPublished,
		StartsAt: now - 60, EndsAt: now + 3600,
	}
	require.NoError(t, db.Create(activity).Error)
	require.NoError(t, db.Create(&model.BenefitUserVoucher{
		ActivityId: activity.Id, UserId: user.Id, RemainingQuota: 500,
		Status: model.BenefitVoucherStatusActive, ExpiresAt: now + 3600,
	}).Error)

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	common.SetContextKey(ctx, constant.ContextKeyBenefitGroupExplicit, false)
	info := &relaycommon.RelayInfo{
		UserId: user.Id, UsingGroup: group.Code, RequestId: "inherited-benefit-group",
		OriginModelName: "gpt-test", UserSetting: dto.UserSetting{BillingPreference: "wallet_only"},
	}
	session, apiErr := NewBillingSession(ctx, info, 0)
	require.Nil(t, apiErr)
	assert.Equal(t, BillingSourceWallet, session.funding.Source(), "继承用户分组时不应触发福利券")

	explicitCtx, _ := gin.CreateTestContext(nil)
	common.SetContextKey(explicitCtx, constant.ContextKeyBenefitGroupExplicit, true)
	explicitInfo := *info
	explicitInfo.RequestId = "explicit-benefit-group"
	explicitSession, explicitErr := NewBillingSession(explicitCtx, &explicitInfo, 0)
	require.Nil(t, explicitErr)
	assert.Equal(t, BillingSourceBenefitVoucher, explicitSession.funding.Source())
}

func (f *recordingFundingSource) Source() string {
	if f.source != "" {
		return f.source
	}
	return BillingSourceWallet
}

func (f *recordingFundingSource) PreConsume(amount int) error {
	if f.err != nil {
		return f.err
	}
	f.preConsumed = append(f.preConsumed, amount)
	f.consumed = amount
	return nil
}

func (f *recordingFundingSource) Capacity() int { return f.capacity }

func (f *recordingFundingSource) AdditionalCapacity() int {
	if f.additional >= 0 {
		return f.additional
	}
	return f.capacity
}

func (f *recordingFundingSource) PreConsumedAmount() int { return f.consumed }

func (f *recordingFundingSource) ReserveAdditional(amount int) error {
	f.additionalReservations = append(f.additionalReservations, amount)
	f.consumed += amount
	return nil
}

func (f *recordingFundingSource) RefundAdditional(amount int) error {
	f.additionalRefunds = append(f.additionalRefunds, amount)
	f.consumed -= amount
	return nil
}

func (f *recordingFundingSource) Settle(delta int) error {
	f.settleDeltas = append(f.settleDeltas, delta)
	if f.settleErr != nil {
		return f.settleErr
	}
	return nil
}

func (f *recordingFundingSource) Refund() error {
	f.refunds++
	return f.refundErr
}

func TestCompositeFundingUsesVoucherSubscriptionWalletOrder(t *testing.T) {
	voucher := &recordingFundingSource{source: BillingSourceBenefitVoucher, capacity: 30}
	subscription := &recordingFundingSource{source: BillingSourceSubscription, capacity: 40}
	wallet := &recordingFundingSource{source: BillingSourceWallet, capacity: -1}
	funding := NewCompositeFunding(voucher, subscription, wallet)

	require.NoError(t, funding.PreConsume(100))
	assert.Equal(t, []int{30}, voucher.preConsumed)
	assert.Equal(t, []int{40}, subscription.preConsumed)
	assert.Equal(t, []int{30}, wallet.preConsumed)

	require.NoError(t, funding.Settle(-30))
	assert.Equal(t, []int{-30}, wallet.settleDeltas)
}

func TestCompositeFundingRollsBackSourcesInReverseOrderWhenPreConsumeFails(t *testing.T) {
	voucher := &recordingFundingSource{source: BillingSourceBenefitVoucher, capacity: 30}
	subscription := &recordingFundingSource{source: BillingSourceSubscription, capacity: 40}
	wallet := &recordingFundingSource{source: BillingSourceWallet, capacity: -1, err: errors.New("wallet unavailable")}
	funding := NewCompositeFunding(voucher, subscription, wallet)

	err := funding.PreConsume(100)
	require.Error(t, err)
	assert.Equal(t, 1, voucher.refunds)
	assert.Equal(t, 1, subscription.refunds)
}

func TestCompositeFundingRollsBackEarlierSettlementWhenLaterSourceFails(t *testing.T) {
	voucher := &recordingFundingSource{source: BillingSourceBenefitVoucher, capacity: 30, additional: 0}
	subscription := &recordingFundingSource{source: BillingSourceSubscription, capacity: 40, additional: 20}
	wallet := &recordingFundingSource{source: BillingSourceWallet, capacity: 30, additional: 30, settleErr: errors.New("wallet settlement failed")}
	funding := NewCompositeFunding(voucher, subscription, wallet)
	require.NoError(t, funding.PreConsume(100))

	err := funding.Settle(40)
	require.Error(t, err)
	assert.Equal(t, []int{20, -20}, subscription.settleDeltas, "后续资金源失败时应回滚已成功补扣")
}

func TestCompositeFundingRollbackUsesCompensatingVoucherSettlement(t *testing.T) {
	voucher := &recordingFundingSource{source: BillingSourceBenefitVoucher, capacity: 30, additional: 30}
	subscription := &recordingFundingSource{source: BillingSourceSubscription, capacity: 30, additional: 10, settleErr: errors.New("subscription settlement failed")}
	funding := NewCompositeFunding(voucher, subscription)
	require.NoError(t, funding.PreConsume(30))

	err := funding.Settle(40)
	require.Error(t, err)
	assert.Equal(t, []int{30, -30}, voucher.settleDeltas)
}

func TestCompositeFundingRollbackRestoresRealVoucherLedgerAfterLaterSourceFailure(t *testing.T) {
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	model.InitDBColumns()
	db, err := gorm.Open(sqlite.Open("file:billing_session_voucher_rollback?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLog)
		model.InitDBColumns()
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.Group{}, &model.BenefitActivity{}, &model.BenefitUserVoucher{}, &model.BenefitVoucherLedger{}))
	group := &model.Group{Code: "rollback-group", Name: "回滚分组", Ratio: 1, Status: model.GroupStatusActive}
	require.NoError(t, db.Create(group).Error)
	activity := &model.BenefitActivity{
		Name: "回滚活动", GroupId: group.Id, Status: model.BenefitActivityStatusPublished,
		StartsAt: 900, EndsAt: 2000, TotalQuota: 40, TotalCount: 1,
	}
	require.NoError(t, db.Create(activity).Error)
	voucher := &model.BenefitUserVoucher{
		ActivityId: activity.Id, UserId: 44, OriginalQuota: 40, RemainingQuota: 40,
		Status: model.BenefitVoucherStatusActive, ExpiresAt: 2000,
	}
	require.NoError(t, db.Create(voucher).Error)

	voucherFunding := NewBenefitVoucherFunding("real-compensation", 44, group.Id)
	voucherFunding.now = func() int64 { return 1000 }
	subscription := &recordingFundingSource{
		source: BillingSourceSubscription, capacity: 30, additional: 10,
		settleErr: errors.New("subscription settlement failed"),
	}
	funding := NewCompositeFunding(voucherFunding, subscription)
	require.NoError(t, funding.PreConsume(30))

	require.Error(t, funding.Settle(20))
	var stored model.BenefitUserVoucher
	require.NoError(t, db.First(&stored, voucher.Id).Error)
	assert.Equal(t, int64(10), stored.RemainingQuota, "补偿后应保留初始预扣的 30 额度")
	assert.Zero(t, stored.UsedQuota, "补偿后不应留下结算使用量")
	var rollbackCount int64
	require.NoError(t, db.Model(&model.BenefitVoucherLedger{}).
		Where("request_id = ? AND type = ?", "real-compensation", model.BenefitLedgerTypeSettleRollback).
		Count(&rollbackCount).Error)
	assert.Equal(t, int64(1), rollbackCount)
	var settleCount int64
	require.NoError(t, db.Model(&model.BenefitVoucherLedger{}).
		Where("request_id = ? AND type = ?", "real-compensation", model.BenefitLedgerTypeSettleDelta).
		Count(&settleCount).Error)
	assert.Equal(t, int64(1), settleCount)

	require.NoError(t, model.RefundBenefitVoucherQuota("real-compensation", 1001))
	require.NoError(t, db.First(&stored, voucher.Id).Error)
	assert.Equal(t, int64(40), stored.RemainingQuota)
	assert.Zero(t, stored.UsedQuota)
	var refundCount int64
	require.NoError(t, db.Model(&model.BenefitVoucherLedger{}).
		Where("request_id = ? AND type = ?", "real-compensation", model.BenefitLedgerTypeRefund).
		Count(&refundCount).Error)
	assert.Equal(t, int64(1), refundCount)

	auditedFunding := NewBenefitVoucherFunding("real-refund-failure", 44, group.Id)
	auditedFunding.now = func() int64 { return 1002 }
	failingVoucher := &refundFailingBenefitFunding{
		BenefitVoucherFunding: auditedFunding,
		refundErr:             errors.New("voucher refund failed"),
	}
	laterFailure := &recordingFundingSource{
		source: BillingSourceSubscription, capacity: -1,
		err: errors.New("later source failed"),
	}
	err = NewCompositeFunding(failingVoucher, laterFailure).PreConsume(50)
	require.Error(t, err)
	assert.ErrorContains(t, err, "later source failed")
	assert.ErrorContains(t, err, "voucher refund failed")
	var retained model.BenefitVoucherLedger
	require.NoError(t, db.Where("request_id = ? AND type = ?", "real-refund-failure", model.BenefitLedgerTypePreConsume).First(&retained).Error)
	assert.Equal(t, int64(-40), retained.QuotaDelta, "退款失败时应保留可审计的预扣流水")
}

func TestCompositeFundingPreConsumeReturnsRefundFailure(t *testing.T) {
	voucher := &recordingFundingSource{source: BillingSourceBenefitVoucher, capacity: 30, refundErr: errors.New("voucher refund failed")}
	wallet := &recordingFundingSource{source: BillingSourceWallet, capacity: 0}
	funding := NewCompositeFunding(voucher, wallet)

	err := funding.PreConsume(50)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "voucher refund failed")
}

func TestBillingSessionBreakdownIncludesBenefitActivityID(t *testing.T) {
	voucher := &BenefitVoucherFunding{voucherID: 8, activityID: 9, consumed: 30, reserved: 30}
	funding := NewCompositeFunding(voucher)
	session := &BillingSession{funding: funding}

	breakdown := session.GetBreakdown()
	assert.Equal(t, 9, breakdown.ActivityID)
	assert.Equal(t, 8, breakdown.VoucherID)
}

func TestBillingSessionReservesAdditionalCompositeFundingInPriorityOrder(t *testing.T) {
	voucher := &recordingFundingSource{source: BillingSourceBenefitVoucher, capacity: 30, additional: 10}
	subscription := &recordingFundingSource{source: BillingSourceSubscription, capacity: 40, additional: 40}
	wallet := &recordingFundingSource{source: BillingSourceWallet, capacity: -1, additional: 100}
	funding := NewCompositeFunding(voucher, subscription, wallet)
	require.NoError(t, funding.PreConsume(100))
	session := &BillingSession{relayInfo: &relaycommon.RelayInfo{TokenQuotaExempt: true}, funding: funding, preConsumedQuota: 100}

	require.NoError(t, session.Reserve(140))
	assert.Equal(t, []int{10}, voucher.additionalReservations)
	assert.Equal(t, []int{30}, subscription.additionalReservations)
	assert.Empty(t, wallet.additionalReservations)
	assert.Equal(t, 30, session.extraReserved, "兼容订阅字段只应记录实际追加的订阅额度")
}

func TestCompositeFundingRefundsAdditionalReservationsBeforeInitialReservations(t *testing.T) {
	voucher := &recordingFundingSource{source: BillingSourceBenefitVoucher, capacity: 30, additional: 10}
	subscription := &recordingFundingSource{source: BillingSourceSubscription, capacity: 40, additional: 40}
	funding := NewCompositeFunding(voucher, subscription)
	require.NoError(t, funding.PreConsume(70))
	require.NoError(t, funding.ReserveAdditional(20))

	require.NoError(t, funding.Refund())
	assert.Equal(t, []int{10}, subscription.additionalRefunds)
	assert.Equal(t, []int{10}, voucher.additionalRefunds)
	assert.Equal(t, 1, subscription.refunds)
}

func TestBillingSessionFinalizesCompositeFundingWhenUsageMatchesPreConsume(t *testing.T) {
	voucher := &recordingFundingSource{source: BillingSourceBenefitVoucher, capacity: 100}
	subscription := &recordingFundingSource{source: BillingSourceSubscription, capacity: 100}
	funding := NewCompositeFunding(voucher, subscription)
	require.NoError(t, funding.PreConsume(100))

	session := &BillingSession{
		relayInfo:        &relaycommon.RelayInfo{TokenQuotaExempt: true},
		funding:          funding,
		preConsumedQuota: 100,
	}
	require.NoError(t, session.Settle(100))
	require.NoError(t, session.Settle(100))

	assert.Equal(t, []int{0}, voucher.settleDeltas, "预扣额与实际用量相等时仍需完成福利券结算")
	assert.Empty(t, subscription.settleDeltas)
}

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
