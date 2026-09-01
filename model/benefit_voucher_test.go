package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBenefitAmountCNYToQuotaUsesCNYDisplayAmount(t *testing.T) {
	originalRate := operation_setting.USDExchangeRate
	originalQuotaPerUnit := common.QuotaPerUnit
	operation_setting.USDExchangeRate = 7.5
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		operation_setting.USDExchangeRate = originalRate
		common.QuotaPerUnit = originalQuotaPerUnit
	})

	quota, err := BenefitAmountCNYToQuota(750)
	require.NoError(t, err)
	assert.Equal(t, int64(500000), quota)

	quota, err = BenefitAmountCNYToQuota(1)
	require.NoError(t, err)
	assert.Equal(t, int64(667), quota)
}

func TestBenefitAmountCNYToQuotaRejectsInvalidAmount(t *testing.T) {
	_, err := BenefitAmountCNYToQuota(0)
	require.Error(t, err)
}

func TestSplitBenefitSharesPreservesRandomBudgetAndBounds(t *testing.T) {
	shares, err := SplitBenefitShares(BenefitShareSplitInput{
		Mode:             BenefitAmountModeRandom,
		TotalAmountCents: 1000,
		TotalCount:       4,
		MinAmountCents:   100,
		MaxAmountCents:   700,
		QuotaPerCent:     10,
		RandomIntn: func(max int) int {
			return max / 2
		},
	})
	require.NoError(t, err)
	require.Len(t, shares, 4)

	var totalAmount int64
	var totalQuota int64
	for _, share := range shares {
		assert.GreaterOrEqual(t, share.AmountCents, int64(100))
		assert.LessOrEqual(t, share.AmountCents, int64(700))
		assert.Equal(t, share.AmountCents*10, share.Quota)
		totalAmount += share.AmountCents
		totalQuota += share.Quota
	}
	assert.Equal(t, int64(1000), totalAmount)
	assert.Equal(t, int64(10000), totalQuota)
}

func TestSplitBenefitSharesRejectsUnsatisfiableRandomBounds(t *testing.T) {
	_, err := SplitBenefitShares(BenefitShareSplitInput{
		Mode:             BenefitAmountModeRandom,
		TotalAmountCents: 100,
		TotalCount:       2,
		MinAmountCents:   60,
		MaxAmountCents:   80,
		QuotaPerCent:     10,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1.20")
	assert.Contains(t, err.Error(), "1.60")
}

func TestSplitBenefitSharesRequiresExactFixedBudget(t *testing.T) {
	shares, err := SplitBenefitShares(BenefitShareSplitInput{
		Mode:             BenefitAmountModeFixed,
		TotalAmountCents: 300,
		TotalCount:       3,
		FixedAmountCents: 100,
		QuotaPerCent:     25,
	})
	require.NoError(t, err)
	require.Len(t, shares, 3)
	for _, share := range shares {
		assert.Equal(t, BenefitShareAllocation{AmountCents: 100, Quota: 2500}, share)
	}

	_, err = SplitBenefitShares(BenefitShareSplitInput{
		Mode:             BenefitAmountModeFixed,
		TotalAmountCents: 301,
		TotalCount:       3,
		FixedAmountCents: 100,
		QuotaPerCent:     25,
	})
	require.Error(t, err)
}

func TestSaveGroupConfigPersistsSingleUserConcurrencyLimit(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&Option{}, &Group{}, &GroupAlias{}, &AutoGroupMember{},
		&Channel{}, &Token{}, &User{},
	))

	group := &Group{
		Code: "benefit", Name: "活动福利", Ratio: 1,
		Status: GroupStatusActive, CreatedTime: 1, UpdatedTime: 1,
	}
	require.NoError(t, db.Create(group).Error)

	require.NoError(t, SaveGroupConfig([]GroupConfig{{
		Id: group.Id, Code: group.Code, Name: group.Name, Ratio: 1,
		Status: GroupStatusActive, SingleUserConcurrencyLimit: 3,
	}}, nil))

	var stored Group
	require.NoError(t, db.First(&stored, group.Id).Error)
	assert.Equal(t, 3, stored.SingleUserConcurrencyLimit)
	assert.Equal(t, 3, stored.ToConfig(nil).SingleUserConcurrencyLimit)
}

func TestSaveGroupConfigRejectsNegativeSingleUserConcurrencyLimit(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&Option{}, &Group{}, &GroupAlias{}, &AutoGroupMember{},
		&Channel{}, &Token{}, &User{},
	))
	group := &Group{Code: "benefit", Name: "活动福利", Ratio: 1, Status: GroupStatusActive}
	require.NoError(t, db.Create(group).Error)

	err := SaveGroupConfig([]GroupConfig{{
		Id: group.Id, Code: group.Code, Name: group.Name, Ratio: 1,
		Status: GroupStatusActive, SingleUserConcurrencyLimit: -1,
	}}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "并发")
}

func setupBenefitVoucherTestDB(t *testing.T) *Group {
	t.Helper()
	db := openGroupIdentityTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&Group{}, &BenefitActivity{}, &BenefitActivityShare{},
		&BenefitUserVoucher{}, &BenefitVoucherLedger{}, &User{}, &TopUp{},
	))
	group := &Group{Code: "benefit", Name: "活动福利", Ratio: 1, Status: GroupStatusActive}
	require.NoError(t, db.Create(group).Error)
	return group
}

func newFixedBenefitActivity(groupID int, startsAt, endsAt int64) *BenefitActivity {
	return &BenefitActivity{
		Name: "周末福利", GroupId: groupID,
		AmountMode:       BenefitAmountModeFixed,
		TotalAmountCents: 300, TotalQuota: 7501, TotalCount: 3,
		FixedAmountCents: 100, PersonalValidSeconds: 3600,
		StartsAt: startsAt, EndsAt: endsAt,
	}
}

func TestDeleteBenefitActivitiesByIDsOnlyArchivesHistoricalActivities(t *testing.T) {
	group := setupBenefitVoucherTestDB(t)
	ended := &BenefitActivity{Name: "已结束", GroupId: group.Id, Status: BenefitActivityStatusEnded}
	active := &BenefitActivity{Name: "进行中", GroupId: group.Id, Status: BenefitActivityStatusPublished}
	require.NoError(t, DB.Create(ended).Error)
	require.NoError(t, DB.Create(active).Error)

	deleted, skipped, err := DeleteBenefitActivitiesByIDs([]int{ended.Id, active.Id, ended.Id, 999999}, 1000)
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)
	assert.EqualValues(t, 2, skipped)

	var archived BenefitActivity
	require.NoError(t, DB.Unscoped().First(&archived, ended.Id).Error)
	assert.True(t, archived.DeletedAt.Valid)
	var visible BenefitActivity
	require.NoError(t, DB.First(&visible, active.Id).Error)
	assert.False(t, visible.DeletedAt.Valid)
}

func TestPublishBenefitActivityCreatesExactSharesAndRejectsOverlap(t *testing.T) {
	group := setupBenefitVoucherTestDB(t)
	first := newFixedBenefitActivity(group.Id, 1000, 2000)
	require.NoError(t, CreateBenefitActivity(first, 11, 900))

	published, err := PublishBenefitActivity(first.Id, 11, 950)
	require.NoError(t, err)
	assert.Equal(t, BenefitActivityStatusPublished, published.Status)
	assert.Equal(t, int64(950), published.PublishedAt)

	var shares []BenefitActivityShare
	require.NoError(t, DB.Where("activity_id = ?", first.Id).Order("id ASC").Find(&shares).Error)
	require.Len(t, shares, 3)
	var totalAmount int64
	var totalQuota int64
	for _, share := range shares {
		assert.Equal(t, int64(100), share.AmountCents)
		assert.Equal(t, BenefitShareStatusAvailable, share.Status)
		totalAmount += share.AmountCents
		totalQuota += share.Quota
	}
	assert.Equal(t, int64(300), totalAmount)
	assert.Equal(t, int64(7501), totalQuota)

	overlap := newFixedBenefitActivity(group.Id, 1500, 2500)
	require.NoError(t, CreateBenefitActivity(overlap, 11, 900))
	_, err = PublishBenefitActivity(overlap.Id, 11, 960)
	require.ErrorIs(t, err, ErrBenefitActivityOverlap)
}

func TestReserveBenefitVoucherQuotaExtendsExistingRequestReservation(t *testing.T) {
	group := setupBenefitVoucherTestDB(t)
	now := int64(1000)
	activity := &BenefitActivity{
		Name: "追加预扣", GroupId: group.Id, Status: BenefitActivityStatusPublished,
		StartsAt: now - 10, EndsAt: now + 1000, TotalAmountCents: 100,
		TotalQuota: 100, TotalCount: 1, FixedAmountCents: 100,
	}
	require.NoError(t, DB.Create(activity).Error)
	voucher := &BenefitUserVoucher{
		ActivityId: activity.Id, UserId: 44, OriginalQuota: 100,
		RemainingQuota: 100, Status: BenefitVoucherStatusActive, ExpiresAt: now + 1000,
	}
	require.NoError(t, DB.Create(voucher).Error)

	first, err := ReserveBenefitVoucherQuota("reserve-extension", 44, group.Id, 30, now)
	require.NoError(t, err)
	assert.Equal(t, int64(30), first.Reserved)
	second, err := ReserveBenefitVoucherQuota("reserve-extension", 44, group.Id, 50, now)
	require.NoError(t, err)
	assert.Equal(t, int64(50), second.Reserved)

	var stored BenefitUserVoucher
	require.NoError(t, DB.First(&stored, voucher.Id).Error)
	assert.Equal(t, int64(50), stored.RemainingQuota)
	var ledger BenefitVoucherLedger
	require.NoError(t, DB.Where("request_id = ? AND type = ?", "reserve-extension", BenefitLedgerTypePreConsume).First(&ledger).Error)
	assert.Equal(t, int64(-50), ledger.QuotaDelta)
}

func TestReserveBenefitVoucherQuotaAllowsAdditionalReserveAfterUnusedTermination(t *testing.T) {
	group := setupBenefitVoucherTestDB(t)
	now := int64(1000)
	activity := &BenefitActivity{
		Name: "终止后追加", GroupId: group.Id, Status: BenefitActivityStatusTerminated,
		TerminateMode: BenefitTerminateModeUnused, StartsAt: now - 10, EndsAt: now + 1000,
		TotalAmountCents: 100, TotalQuota: 100, TotalCount: 1, FixedAmountCents: 100,
	}
	require.NoError(t, DB.Create(activity).Error)
	voucher := &BenefitUserVoucher{
		ActivityId: activity.Id, UserId: 44, OriginalQuota: 100,
		RemainingQuota: 100, Status: BenefitVoucherStatusActive, ExpiresAt: now + 1000,
	}
	require.NoError(t, DB.Create(voucher).Error)

	first, err := ReserveBenefitVoucherQuota("terminated-extension", 44, group.Id, 30, now)
	require.NoError(t, err)
	assert.Equal(t, int64(30), first.Reserved)
	second, err := ReserveBenefitVoucherQuota("terminated-extension", 44, group.Id, 50, now)
	require.NoError(t, err)
	assert.Equal(t, int64(50), second.Reserved)

	_, err = ReserveBenefitVoucherQuota("terminated-at-end", 44, group.Id, 1, activity.EndsAt)
	require.ErrorIs(t, err, ErrBenefitVoucherUnavailable)
	require.NoError(t, DB.Model(voucher).Update("expires_at", now+100).Error)
	_, err = ReserveBenefitVoucherQuota("terminated-at-expiry", 44, group.Id, 1, now+100)
	require.ErrorIs(t, err, ErrBenefitVoucherUnavailable)
}

func TestRefundBenefitVoucherAdditionalIsIdempotentAndAuditable(t *testing.T) {
	group := setupBenefitVoucherTestDB(t)
	now := int64(1000)
	activity := &BenefitActivity{
		Name: "追加退款", GroupId: group.Id, Status: BenefitActivityStatusPublished,
		StartsAt: now - 10, EndsAt: now + 1000, TotalAmountCents: 100,
		TotalQuota: 100, TotalCount: 1, FixedAmountCents: 100,
	}
	require.NoError(t, DB.Create(activity).Error)
	voucher := &BenefitUserVoucher{
		ActivityId: activity.Id, UserId: 44, OriginalQuota: 100,
		RemainingQuota: 100, Status: BenefitVoucherStatusActive, ExpiresAt: now + 1000,
	}
	require.NoError(t, DB.Create(voucher).Error)

	_, err := ReserveBenefitVoucherQuota("additional-refund", 44, group.Id, 50, now)
	require.NoError(t, err)
	require.NoError(t, RefundBenefitVoucherAdditional("additional-refund", 30, now+1))
	require.NoError(t, RefundBenefitVoucherAdditional("additional-refund", 30, now+2))
	_, err = ReserveBenefitVoucherQuota("additional-refund", 44, group.Id, 50, now+3)
	require.NoError(t, err)
	require.NoError(t, RefundBenefitVoucherAdditional("additional-refund", 40, now+4))

	var stored BenefitUserVoucher
	require.NoError(t, DB.First(&stored, voucher.Id).Error)
	assert.Equal(t, int64(90), stored.RemainingQuota)
	var pre BenefitVoucherLedger
	require.NoError(t, DB.Where("request_id = ? AND type = ?", "additional-refund", BenefitLedgerTypePreConsume).First(&pre).Error)
	assert.Equal(t, int64(-10), pre.QuotaDelta)
	var refund BenefitVoucherLedger
	require.NoError(t, DB.Where("request_id = ? AND type = ?", "additional-refund", BenefitLedgerTypeRefundAdditional).First(&refund).Error)
	assert.Equal(t, int64(70), refund.QuotaDelta)
}

func TestRollbackBenefitVoucherSettlementRestoresPreConsumeState(t *testing.T) {
	group := setupBenefitVoucherTestDB(t)
	now := int64(1000)
	activity := &BenefitActivity{
		Name: "结算补偿", GroupId: group.Id, Status: BenefitActivityStatusPublished,
		StartsAt: now - 10, EndsAt: now + 1000, TotalAmountCents: 100,
		TotalQuota: 100, TotalCount: 1, FixedAmountCents: 100,
	}
	require.NoError(t, DB.Create(activity).Error)
	voucher := &BenefitUserVoucher{
		ActivityId: activity.Id, UserId: 44, OriginalQuota: 100,
		RemainingQuota: 100, Status: BenefitVoucherStatusActive, ExpiresAt: now + 1000,
	}
	require.NoError(t, DB.Create(voucher).Error)

	_, err := ReserveBenefitVoucherQuota("settlement-compensation", 44, group.Id, 30, now)
	require.NoError(t, err)
	require.NoError(t, SettleBenefitVoucherQuota("settlement-compensation", 20, now+1))
	require.NoError(t, RollbackBenefitVoucherSettlement("settlement-compensation", 20, now+2))
	require.NoError(t, RollbackBenefitVoucherSettlement("settlement-compensation", 20, now+3))

	var stored BenefitUserVoucher
	require.NoError(t, DB.First(&stored, voucher.Id).Error)
	assert.Equal(t, int64(70), stored.RemainingQuota)
	assert.Zero(t, stored.UsedQuota, "结算补偿后应恢复到初始预扣状态")
	var rollbackCount int64
	require.NoError(t, DB.Model(&BenefitVoucherLedger{}).Where("request_id = ? AND type = ?", "settlement-compensation", BenefitLedgerTypeSettleRollback).Count(&rollbackCount).Error)
	assert.Equal(t, int64(1), rollbackCount)
}

func TestRollbackBenefitVoucherSettlementSupportsNegativeSettlement(t *testing.T) {
	group := setupBenefitVoucherTestDB(t)
	now := int64(1000)
	activity := &BenefitActivity{
		Name: "负差额补偿", GroupId: group.Id, Status: BenefitActivityStatusPublished,
		StartsAt: now - 10, EndsAt: now + 1000, TotalAmountCents: 100,
		TotalQuota: 100, TotalCount: 1, FixedAmountCents: 100,
	}
	require.NoError(t, DB.Create(activity).Error)
	voucher := &BenefitUserVoucher{
		ActivityId: activity.Id, UserId: 44, OriginalQuota: 100,
		RemainingQuota: 100, Status: BenefitVoucherStatusActive, ExpiresAt: now + 1000,
	}
	require.NoError(t, DB.Create(voucher).Error)

	_, err := ReserveBenefitVoucherQuota("negative-compensation", 44, group.Id, 30, now)
	require.NoError(t, err)
	require.NoError(t, SettleBenefitVoucherQuota("negative-compensation", -10, now+1))
	require.NoError(t, RollbackBenefitVoucherSettlement("negative-compensation", -10, now+2))

	var stored BenefitUserVoucher
	require.NoError(t, DB.First(&stored, voucher.Id).Error)
	assert.Equal(t, int64(70), stored.RemainingQuota)
	assert.Zero(t, stored.UsedQuota)
}

func TestPublishedBenefitActivityRejectsDraftFieldUpdates(t *testing.T) {
	group := setupBenefitVoucherTestDB(t)
	activity := newFixedBenefitActivity(group.Id, 1000, 2000)
	require.NoError(t, CreateBenefitActivity(activity, 11, 900))
	_, err := PublishBenefitActivity(activity.Id, 11, 950)
	require.NoError(t, err)

	activity.TotalAmountCents = 600
	err = UpdateBenefitActivityDraft(activity, 12, 960)
	require.ErrorIs(t, err, ErrBenefitActivityNotDraft)
}

func TestUpdateBenefitActivityMetadataAllowsOnlyDisplayFieldsAfterPublish(t *testing.T) {
	group := setupBenefitVoucherTestDB(t)
	activity := newFixedBenefitActivity(group.Id, 1000, 2000)
	require.NoError(t, CreateBenefitActivity(activity, 11, 900))
	_, err := PublishBenefitActivity(activity.Id, 11, 950)
	require.NoError(t, err)

	updated, err := UpdateBenefitActivityMetadata(activity.Id, "新名称", "新的说明", 12, 960)
	require.NoError(t, err)
	assert.Equal(t, "新名称", updated.Name)
	assert.Equal(t, "新的说明", updated.Description)
	assert.Equal(t, int64(300), updated.TotalAmountCents)

	var stored BenefitActivity
	require.NoError(t, DB.First(&stored, activity.Id).Error)
	assert.Equal(t, int64(1000), stored.StartsAt)
	assert.Equal(t, int64(2000), stored.EndsAt)
}

func TestTerminateUnusedKeepsClaimedVoucherActive(t *testing.T) {
	group := setupBenefitVoucherTestDB(t)
	activity := newFixedBenefitActivity(group.Id, 1000, 3000)
	require.NoError(t, CreateBenefitActivity(activity, 11, 900))
	_, err := PublishBenefitActivity(activity.Id, 11, 950)
	require.NoError(t, err)

	var shares []BenefitActivityShare
	require.NoError(t, DB.Where("activity_id = ?", activity.Id).Order("id ASC").Find(&shares).Error)
	voucher := &BenefitUserVoucher{
		ActivityId: activity.Id, ShareId: shares[0].Id, UserId: 51,
		OriginalAmountCents: shares[0].AmountCents,
		OriginalQuota:       shares[0].Quota, RemainingQuota: shares[0].Quota,
		Status: BenefitVoucherStatusActive, ClaimedAt: 1000, ExpiresAt: 2500,
	}
	require.NoError(t, DB.Create(voucher).Error)
	require.NoError(t, DB.Model(&BenefitActivityShare{}).Where("id = ?", shares[0].Id).Updates(map[string]interface{}{
		"status": BenefitShareStatusClaimed, "claimed_by_user_id": voucher.UserId,
		"claimed_voucher_id": voucher.Id, "claimed_at": 1000,
	}).Error)

	require.NoError(t, TerminateBenefitActivity(activity.Id, 99, BenefitTerminateModeUnused, "活动调整", 1200))

	var storedVoucher BenefitUserVoucher
	require.NoError(t, DB.First(&storedVoucher, voucher.Id).Error)
	assert.Equal(t, BenefitVoucherStatusActive, storedVoucher.Status)
	assert.Equal(t, voucher.OriginalQuota, storedVoucher.RemainingQuota)

	var available, voided int64
	require.NoError(t, DB.Model(&BenefitActivityShare{}).Where("activity_id = ? AND status = ?", activity.Id, BenefitShareStatusAvailable).Count(&available).Error)
	require.NoError(t, DB.Model(&BenefitActivityShare{}).Where("activity_id = ? AND status = ?", activity.Id, BenefitShareStatusVoided).Count(&voided).Error)
	assert.Zero(t, available)
	assert.Equal(t, int64(2), voided)
	availableQuota, err := GetBenefitVoucherAvailableQuota(voucher.UserId, group.Id, 1200)
	require.NoError(t, err)
	assert.Equal(t, voucher.OriginalQuota, availableQuota, "unused 终止后已领取券仍应可用到活动结束")
}

func TestTerminateAllVoidsRemainingVoucherBalance(t *testing.T) {
	group := setupBenefitVoucherTestDB(t)
	activity := newFixedBenefitActivity(group.Id, 1000, 3000)
	require.NoError(t, CreateBenefitActivity(activity, 11, 900))
	_, err := PublishBenefitActivity(activity.Id, 11, 950)
	require.NoError(t, err)

	var share BenefitActivityShare
	require.NoError(t, DB.Where("activity_id = ?", activity.Id).First(&share).Error)
	voucher := &BenefitUserVoucher{
		ActivityId: activity.Id, ShareId: share.Id, UserId: 51,
		OriginalAmountCents: share.AmountCents, OriginalQuota: share.Quota,
		RemainingQuota: share.Quota - 100, UsedQuota: 100,
		Status: BenefitVoucherStatusActive, ClaimedAt: 1000, ExpiresAt: 2500,
	}
	require.NoError(t, DB.Create(voucher).Error)
	require.NoError(t, DB.Model(&BenefitActivityShare{}).Where("id = ?", share.Id).Update("status", BenefitShareStatusClaimed).Error)

	require.NoError(t, TerminateBenefitActivity(activity.Id, 99, BenefitTerminateModeAll, "紧急停止", 1200))

	var stored BenefitUserVoucher
	require.NoError(t, DB.First(&stored, voucher.Id).Error)
	assert.Equal(t, BenefitVoucherStatusVoided, stored.Status)
	assert.Zero(t, stored.RemainingQuota)
	assert.Equal(t, int64(100), stored.UsedQuota)

	var ledger BenefitVoucherLedger
	require.NoError(t, DB.Where("voucher_id = ? AND type = ?", voucher.Id, BenefitLedgerTypeVoid).First(&ledger).Error)
	assert.Equal(t, -voucher.RemainingQuota, ledger.QuotaDelta)
	assert.Zero(t, ledger.BalanceAfter)
}

func TestBenefitActivityReportExpiresDueVoucherBeforeAggregation(t *testing.T) {
	group := setupBenefitVoucherTestDB(t)
	activity := newFixedBenefitActivity(group.Id, 1000, 3000)
	require.NoError(t, CreateBenefitActivity(activity, 11, 900))
	_, err := PublishBenefitActivity(activity.Id, 11, 950)
	require.NoError(t, err)

	var shares []BenefitActivityShare
	require.NoError(t, DB.Where("activity_id = ?", activity.Id).Order("id ASC").Find(&shares).Error)
	voucher := &BenefitUserVoucher{
		ActivityId: activity.Id, ShareId: shares[0].Id, UserId: 51,
		OriginalAmountCents: shares[0].AmountCents, OriginalQuota: shares[0].Quota,
		RemainingQuota: shares[0].Quota - 100, UsedQuota: 100,
		Status: BenefitVoucherStatusActive, ClaimedAt: 1000, ExpiresAt: 1100,
	}
	require.NoError(t, DB.Create(voucher).Error)
	require.NoError(t, DB.Model(&BenefitActivityShare{}).Where("id = ?", shares[0].Id).Update("status", BenefitShareStatusClaimed).Error)

	report, err := GetBenefitActivityReport(activity.Id, 1200)
	require.NoError(t, err)
	assert.Equal(t, activity.TotalQuota, report.TotalQuota)
	assert.Equal(t, shares[1].Quota+shares[2].Quota, report.UndistributedQuota)
	assert.Equal(t, voucher.OriginalQuota, report.DistributedQuota)
	assert.Equal(t, int64(100), report.UsedQuota)
	assert.Equal(t, voucher.RemainingQuota, report.ExpiredUnusedQuota)

	var stored BenefitUserVoucher
	require.NoError(t, DB.First(&stored, voucher.Id).Error)
	assert.Equal(t, BenefitVoucherStatusExpired, stored.Status)
}

func TestBenefitActivityReportCountsVoidedVoucherUnusedQuotaFromOriginalBalance(t *testing.T) {
	group := setupBenefitVoucherTestDB(t)
	activity := newFixedBenefitActivity(group.Id, 1000, 3000)
	require.NoError(t, CreateBenefitActivity(activity, 11, 900))
	_, err := PublishBenefitActivity(activity.Id, 11, 950)
	require.NoError(t, err)

	var share BenefitActivityShare
	require.NoError(t, DB.Where("activity_id = ?", activity.Id).First(&share).Error)
	voucher := &BenefitUserVoucher{
		ActivityId: activity.Id, ShareId: share.Id, UserId: 51,
		OriginalAmountCents: share.AmountCents, OriginalQuota: share.Quota,
		RemainingQuota: share.Quota - 100, UsedQuota: 100,
		Status: BenefitVoucherStatusActive, ClaimedAt: 1000, ExpiresAt: 2500,
	}
	require.NoError(t, DB.Create(voucher).Error)
	require.NoError(t, DB.Model(&BenefitActivityShare{}).Where("id = ?", share.Id).Update("status", BenefitShareStatusClaimed).Error)
	require.NoError(t, TerminateBenefitActivity(activity.Id, 99, BenefitTerminateModeAll, "紧急停止", 1200))

	report, err := GetBenefitActivityReport(activity.Id, 1200)
	require.NoError(t, err)
	assert.Equal(t, sharesUnusedQuota(t, activity.Id)+voucher.OriginalQuota-voucher.UsedQuota, report.ExpiredUnusedQuota)
}

func sharesUnusedQuota(t *testing.T, activityID int) int64 {
	t.Helper()
	var shares []BenefitActivityShare
	require.NoError(t, DB.Where("activity_id = ? AND status = ?", activityID, BenefitShareStatusVoided).Find(&shares).Error)
	var total int64
	for _, share := range shares {
		total += share.Quota
	}
	return total
}

func TestEndBenefitActivityExpiresUnclaimedSharesImmediately(t *testing.T) {
	group := setupBenefitVoucherTestDB(t)
	activity := newFixedBenefitActivity(group.Id, 1000, 3000)
	require.NoError(t, CreateBenefitActivity(activity, 11, 900))
	_, err := PublishBenefitActivity(activity.Id, 11, 950)
	require.NoError(t, err)

	_, err = TransitionBenefitActivity(activity.Id, 11, BenefitActivityStatusEnded, 1200)
	require.NoError(t, err)

	var available, expired int64
	require.NoError(t, DB.Model(&BenefitActivityShare{}).Where("activity_id = ? AND status = ?", activity.Id, BenefitShareStatusAvailable).Count(&available).Error)
	require.NoError(t, DB.Model(&BenefitActivityShare{}).Where("activity_id = ? AND status = ?", activity.Id, BenefitShareStatusExpired).Count(&expired).Error)
	assert.Zero(t, available)
	assert.Equal(t, int64(3), expired)
}

func createBenefitClaimUser(t *testing.T, groupID, id int, createdAt int64, username string) *User {
	t.Helper()
	if createdAt == 0 {
		createdAt = 1
	}
	user := &User{
		Id: id, Username: username, Password: "password123",
		GroupId: groupID, CreatedAt: createdAt, Status: 1,
		AffCode: username + "-aff",
	}
	require.NoError(t, DB.Create(user).Error)
	return user
}

func TestBenefitEligibilityUsesSuccessfulPaidSnapshotsAndRegistrationAge(t *testing.T) {
	group := setupBenefitVoucherTestDB(t)
	activity := newFixedBenefitActivity(group.Id, 1000, 3000)
	activity.ClaimPaidThresholdCents = 500
	require.NoError(t, CreateBenefitActivity(activity, 11, 900))
	published, err := PublishBenefitActivity(activity.Id, 11, 950)
	require.NoError(t, err)

	user := createBenefitClaimUser(t, group.Id, 51, 0, "eligible-user")
	require.NoError(t, DB.Create(&TopUp{UserId: user.Id, Money: 6, ActualMoney: 6, PaidAmountCNY: 6, Status: "success", TradeNo: "paid-1"}).Error)
	require.NoError(t, DB.Create(&TopUp{UserId: user.Id, Money: 8, ActualMoney: 8, PaidAmountCNY: 8, Status: "pending", TradeNo: "pending-1"}).Error)

	eligibility, err := GetBenefitClaimEligibility(user.Id, published, 2000)
	require.NoError(t, err)
	assert.True(t, eligibility.Eligible)
	assert.Equal(t, int64(600), eligibility.PaidAmountCents)

	tooNew := createBenefitClaimUser(t, group.Id, 52, 1900, "new-user")
	eligibility, err = GetBenefitClaimEligibility(tooNew.Id, published, 2000)
	require.NoError(t, err)
	assert.False(t, eligibility.Eligible)
	assert.Equal(t, BenefitClaimReasonIneligible, eligibility.Reason)
}

func TestClaimBenefitActivityEnforcesSingleVoucherAndSoldOut(t *testing.T) {
	group := setupBenefitVoucherTestDB(t)
	activity := newFixedBenefitActivity(group.Id, 1000, 3000)
	require.NoError(t, CreateBenefitActivity(activity, 11, 900))
	_, err := PublishBenefitActivity(activity.Id, 11, 950)
	require.NoError(t, err)

	firstUser := createBenefitClaimUser(t, group.Id, 51, 0, "first-user")
	secondUser := createBenefitClaimUser(t, group.Id, 52, 0, "second-user")
	require.NoError(t, DB.Create(&TopUp{UserId: firstUser.Id, Money: 2, ActualMoney: 2, PaidAmountCNY: 2, Status: "success", TradeNo: "paid-first"}).Error)
	require.NoError(t, DB.Create(&TopUp{UserId: secondUser.Id, Money: 2, ActualMoney: 2, PaidAmountCNY: 2, Status: "success", TradeNo: "paid-second"}).Error)

	claimed, err := ClaimBenefitActivity(activity.Id, firstUser.Id, 2000)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	assert.Equal(t, BenefitVoucherStatusActive, claimed.Status)

	_, err = ClaimBenefitActivity(activity.Id, firstUser.Id, 2001)
	require.ErrorIs(t, err, ErrBenefitAlreadyClaimed)

	// Remove the other two shares so the next eligible user races for the last one.
	require.NoError(t, DB.Model(&BenefitActivityShare{}).Where("activity_id = ? AND status = ?", activity.Id, BenefitShareStatusAvailable).Where("id <> ?", claimed.ShareId).Update("status", BenefitShareStatusClaimed).Error)
	_, err = ClaimBenefitActivity(activity.Id, secondUser.Id, 2002)
	require.ErrorIs(t, err, ErrBenefitSoldOut)
}

func TestBenefitClaimRejectsOutsideActivityWindow(t *testing.T) {
	group := setupBenefitVoucherTestDB(t)
	activity := newFixedBenefitActivity(group.Id, 1000, 3000)
	require.NoError(t, CreateBenefitActivity(activity, 11, 900))
	_, err := PublishBenefitActivity(activity.Id, 11, 950)
	require.NoError(t, err)
	user := createBenefitClaimUser(t, group.Id, 51, 0, "window-user")
	require.NoError(t, DB.Create(&TopUp{UserId: user.Id, Money: 2, ActualMoney: 2, PaidAmountCNY: 2, Status: "success", TradeNo: "paid-window"}).Error)

	for _, now := range []int64{999, 3000} {
		_, err = ClaimBenefitActivity(activity.Id, user.Id, now)
		require.ErrorIs(t, err, ErrBenefitActivityNotClaimable)
	}
}

func TestBenefitVoucherReservationSettlementAndRefundAreIdempotent(t *testing.T) {
	group := setupBenefitVoucherTestDB(t)
	activity := newFixedBenefitActivity(group.Id, 1000, 3000)
	require.NoError(t, CreateBenefitActivity(activity, 11, 900))
	_, err := PublishBenefitActivity(activity.Id, 11, 950)
	require.NoError(t, err)
	user := createBenefitClaimUser(t, group.Id, 51, 1, "ledger-user")
	require.NoError(t, DB.Create(&TopUp{UserId: user.Id, Money: 2, ActualMoney: 2, PaidAmountCNY: 2, Status: "success", TradeNo: "ledger-paid"}).Error)
	voucher, err := ClaimBenefitActivity(activity.Id, user.Id, 2000)
	require.NoError(t, err)

	reservation, err := ReserveBenefitVoucherQuota("request-ledger", user.Id, group.Id, 500, 2100)
	require.NoError(t, err)
	assert.Equal(t, int64(500), reservation.Reserved)
	reservationAgain, err := ReserveBenefitVoucherQuota("request-ledger", user.Id, group.Id, 500, 2101)
	require.NoError(t, err)
	assert.Equal(t, reservation.Reserved, reservationAgain.Reserved)

	require.NoError(t, SettleBenefitVoucherQuota("request-ledger", -100, 2200))
	require.NoError(t, SettleBenefitVoucherQuota("request-ledger", -100, 2201))
	var settled BenefitUserVoucher
	require.NoError(t, DB.First(&settled, voucher.Id).Error)
	assert.Equal(t, int64(400), settled.UsedQuota)
	assert.Equal(t, voucher.OriginalQuota-400, settled.RemainingQuota)

	refundReservation, err := ReserveBenefitVoucherQuota("request-refund", user.Id, group.Id, 100, 2300)
	require.NoError(t, err)
	assert.Equal(t, int64(100), refundReservation.Reserved)
	require.NoError(t, RefundBenefitVoucherQuota("request-refund", 2301))
	require.NoError(t, RefundBenefitVoucherQuota("request-refund", 2302))
	var refunded BenefitUserVoucher
	require.NoError(t, DB.First(&refunded, voucher.Id).Error)
	assert.Equal(t, int64(400), refunded.UsedQuota)
	assert.Equal(t, voucher.OriginalQuota-400, refunded.RemainingQuota)
}

func TestRefundBenefitVoucherQuotaDoesNotRestoreVoidedVoucher(t *testing.T) {
	group := setupBenefitVoucherTestDB(t)
	now := int64(1000)
	activity := &BenefitActivity{
		Name: "作废退款", GroupId: group.Id, Status: BenefitActivityStatusPublished,
		StartsAt: now - 10, EndsAt: now + 1000, TotalAmountCents: 100,
		TotalQuota: 100, TotalCount: 1, FixedAmountCents: 100,
	}
	require.NoError(t, DB.Create(activity).Error)
	voucher := &BenefitUserVoucher{
		ActivityId: activity.Id, UserId: 44, OriginalQuota: 100,
		RemainingQuota: 100, Status: BenefitVoucherStatusActive, ExpiresAt: now + 1000,
	}
	require.NoError(t, DB.Create(voucher).Error)
	_, err := ReserveBenefitVoucherQuota("voided-refund", 44, group.Id, 50, now)
	require.NoError(t, err)
	require.NoError(t, VoidBenefitVoucher(voucher.Id, 99, "管理员作废", now+1))

	// 预扣发生在券进入终态之前，退款不能恢复作废券的余额或状态。
	require.NoError(t, RefundBenefitVoucherQuota("voided-refund", now+2))
	var stored BenefitUserVoucher
	require.NoError(t, DB.First(&stored, voucher.Id).Error)
	assert.Equal(t, int64(0), stored.RemainingQuota)
	assert.Equal(t, BenefitVoucherStatusVoided, stored.Status)
	var refund BenefitVoucherLedger
	require.NoError(t, DB.Where("request_id = ? AND type = ?", "voided-refund", BenefitLedgerTypeRefund).First(&refund).Error)
	assert.Zero(t, refund.QuotaDelta)
	assert.Contains(t, refund.Metadata, "not_restored")
}

func TestRefundBenefitVoucherQuotaDoesNotRestoreExpiredVoucher(t *testing.T) {
	group := setupBenefitVoucherTestDB(t)
	now := int64(1000)
	activity := &BenefitActivity{
		Name: "过期退款", GroupId: group.Id, Status: BenefitActivityStatusPublished,
		StartsAt: now - 10, EndsAt: now + 1000, TotalAmountCents: 100,
		TotalQuota: 100, TotalCount: 1, FixedAmountCents: 100,
	}
	require.NoError(t, DB.Create(activity).Error)
	voucher := &BenefitUserVoucher{
		ActivityId: activity.Id, UserId: 44, OriginalQuota: 100,
		RemainingQuota: 100, Status: BenefitVoucherStatusActive, ExpiresAt: now + 1,
	}
	require.NoError(t, DB.Create(voucher).Error)
	_, err := ReserveBenefitVoucherQuota("expired-refund", 44, group.Id, 50, now)
	require.NoError(t, err)
	require.NoError(t, DB.Model(voucher).Updates(map[string]interface{}{
		"status":     BenefitVoucherStatusExpired,
		"updated_at": now + 2,
	}).Error)

	require.NoError(t, RefundBenefitVoucherQuota("expired-refund", now+3))
	var stored BenefitUserVoucher
	require.NoError(t, DB.First(&stored, voucher.Id).Error)
	assert.Equal(t, int64(50), stored.RemainingQuota)
	assert.Equal(t, BenefitVoucherStatusExpired, stored.Status)
	var refund BenefitVoucherLedger
	require.NoError(t, DB.Where("request_id = ? AND type = ?", "expired-refund", BenefitLedgerTypeRefund).First(&refund).Error)
	assert.Zero(t, refund.QuotaDelta)
	assert.Contains(t, refund.Metadata, "not_restored")
}
