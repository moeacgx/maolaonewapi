package model

import (
	"math"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func configureAffiliateTest(t *testing.T) {
	t.Helper()
	s := setting.GetAffiliateSetting()
	original := *s
	t.Cleanup(func() { *s = original })
	s.FirstLevelEnabled = true
	s.FirstLevelRatio = 10
	s.SecondLevelEnabled = true
	s.SecondLevelRatio = 5
	s.SettlementDelaySeconds = 60
	s.MinWithdrawalAmount = 0
	s.TriggerTopupEnabled = true
	s.TriggerSubscriptionEnabled = true
	s.FilterRedemptionTopupEnabled = false
	s.PayoutMethods = "usdt,alipay,wechat"
	s.ReviewEnabled = false
	s.AgreementEnabled = false
	s.InviterMinAccountAgeDays = 0
	s.InviterMinRechargeAmount = 0
	s.InviteeMinAccountAgeDays = 0
	s.InviteeMinRechargeAmount = 0
}

func createAffiliateTestUser(t *testing.T, id, inviterID, quota int) User {
	t.Helper()
	user := User{
		Id:        id,
		Username:  common.GetRandomString(12),
		AffCode:   common.GetRandomString(12),
		Status:    common.UserStatusEnabled,
		InviterId: inviterID,
		Quota:     quota,
	}
	require.NoError(t, DB.Create(&user).Error)
	return user
}

func loadAffiliateTestBalance(t *testing.T, userID int) AffiliateBalance {
	t.Helper()
	var balance AffiliateBalance
	require.NoError(t, DB.Where("user_id = ?", userID).First(&balance).Error)
	return balance
}

func assertAffiliateBalanceConserved(t *testing.T, balance AffiliateBalance) {
	t.Helper()
	assert.Equal(t,
		balance.TotalQuota,
		balance.PendingQuota+balance.AvailableQuota+balance.FrozenQuota+
			balance.RiskFrozenQuota+balance.WithdrawnQuota+balance.TransferredQuota,
	)
}

func TestAffiliateRewardSourceTupleIsIdempotentUnderDuplicateCallbacks(t *testing.T) {
	truncateTables(t)
	configureAffiliateTest(t)

	createAffiliateTestUser(t, 8101, 0, 0)
	createAffiliateTestUser(t, 8102, 8101, 0)
	createAffiliateTestUser(t, 8103, 8102, 0)

	const businessQuota = 10_000
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- CreateAffiliateRewardsForPayment(8103, AffiliateSourceTopUp, "duplicate-callback-order", businessQuota)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	var records []AffiliateRecord
	require.NoError(t, DB.Where("source_type = ? AND source_id = ?", AffiliateSourceTopUp, "duplicate-callback-order").Order("level ASC").Find(&records).Error)
	require.Len(t, records, 2)
	assert.Equal(t, 8102, records[0].UserId)
	assert.Equal(t, 1, records[0].Level)
	createAffiliateTestUser(t, 8104, 0, 0)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", 8103).Update("inviter_id", 8104).Error)
	require.NoError(t, CreateAffiliateRewardsForPayment(8103, AffiliateSourceTopUp, "duplicate-callback-order", businessQuota))
	require.NoError(t, DB.Where("source_type = ? AND source_id = ?", AffiliateSourceTopUp, "duplicate-callback-order").Order("level ASC").Find(&records).Error)
	require.Len(t, records, 2)

	assert.Equal(t, businessQuota, records[0].SourceQuota)
	assert.Equal(t, 1_000, records[0].RewardQuota)
	assert.Equal(t, 8101, records[1].UserId)
	assert.Equal(t, 2, records[1].Level)
	assert.Equal(t, 500, records[1].RewardQuota)

	first := loadAffiliateTestBalance(t, 8102)
	second := loadAffiliateTestBalance(t, 8101)
	assert.Equal(t, 1_000, first.PendingQuota)
	assert.Equal(t, 1_000, first.TotalQuota)
	assert.Equal(t, 500, second.PendingQuota)
	assert.Equal(t, 500, second.TotalQuota)
}

func TestAffiliateSettlementLocksBalanceBeforeRecordMutation(t *testing.T) {
	truncateTables(t)
	configureAffiliateTest(t)

	createAffiliateTestUser(t, 8105, 0, 0)
	createAffiliateTestUser(t, 8106, 8105, 0)
	require.NoError(t, CreateAffiliateRewardsForPayment(8106, AffiliateSourceTopUp, "settlement-lock-order", 10_000))
	require.NoError(t, DB.Model(&AffiliateRecord{}).
		Where("source_type = ? AND source_id = ?", AffiliateSourceTopUp, "settlement-lock-order").
		Update("available_time", common.GetTimestamp()-1).Error)

	events := make([]string, 0, 2)
	const queryCallback = "test:affiliate_settlement_balance_lock_order"
	const updateCallback = "test:affiliate_settlement_record_lock_order"
	queryRegistered := false
	updateRegistered := false
	t.Cleanup(func() {
		if queryRegistered {
			_ = DB.Callback().Query().Remove(queryCallback)
		}
		if updateRegistered {
			_ = DB.Callback().Update().Remove(updateCallback)
		}
	})
	require.NoError(t, DB.Callback().Query().Before("gorm:query").Register(queryCallback, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "affiliate_balances" {
			events = append(events, "balance")
		}
	}))
	queryRegistered = true
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(updateCallback, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "affiliate_records" {
			events = append(events, "record")
		}
	}))
	updateRegistered = true

	require.NoError(t, SettleMatureAffiliateRecords(8105))
	require.NoError(t, DB.Callback().Query().Remove(queryCallback))
	queryRegistered = false
	require.NoError(t, DB.Callback().Update().Remove(updateCallback))
	updateRegistered = false

	require.Contains(t, events, "balance")
	require.Contains(t, events, "record")
	assert.Less(t, indexOfAffiliateTestEvent(events, "balance"), indexOfAffiliateTestEvent(events, "record"), events)
}

func indexOfAffiliateTestEvent(events []string, target string) int {
	for index, event := range events {
		if event == target {
			return index
		}
	}
	return -1
}

func TestAffiliateSettlementWithdrawalTransitionsConserveBalance(t *testing.T) {
	truncateTables(t)
	configureAffiliateTest(t)

	createAffiliateTestUser(t, 8111, 0, 0)
	createAffiliateTestUser(t, 8112, 8111, 0)
	require.NoError(t, CreateAffiliateRewardsForPayment(8112, AffiliateSourceTopUp, "settlement-order", 10_000))
	require.NoError(t, DB.Model(&AffiliateRecord{}).
		Where("source_type = ? AND source_id = ?", AffiliateSourceTopUp, "settlement-order").
		Update("available_time", common.GetTimestamp()-1).Error)
	require.NoError(t, SettleMatureAffiliateRecords(8111))
	require.NoError(t, SettleMatureAffiliateRecords(8111))

	balance := loadAffiliateTestBalance(t, 8111)
	assert.Equal(t, 0, balance.PendingQuota)
	assert.Equal(t, 1_000, balance.AvailableQuota)
	assertAffiliateBalanceConserved(t, balance)

	require.NoError(t, DB.Create(&AffiliatePayoutAccount{UserId: 8111, AlipayAccount: "pay@example.test"}).Error)
	withdrawal, err := CreateAffiliateWithdrawal(8111, AffiliatePayoutMethodAlipay, 400)
	require.NoError(t, err)
	balance = loadAffiliateTestBalance(t, 8111)
	assert.Equal(t, 600, balance.AvailableQuota)
	assert.Equal(t, 400, balance.FrozenQuota)
	assertAffiliateBalanceConserved(t, balance)

	require.NoError(t, RejectAffiliateWithdrawal(withdrawal.Id, 1, "reject"))
	require.NoError(t, RejectAffiliateWithdrawal(withdrawal.Id, 1, "duplicate reject"))
	balance = loadAffiliateTestBalance(t, 8111)
	assert.Equal(t, 1_000, balance.AvailableQuota)
	assert.Zero(t, balance.FrozenQuota)
	assertAffiliateBalanceConserved(t, balance)

	withdrawal, err = CreateAffiliateWithdrawal(8111, AffiliatePayoutMethodAlipay, 400)
	require.NoError(t, err)
	require.NoError(t, MarkAffiliateWithdrawalPaid(withdrawal.Id, 1, "paid"))
	require.NoError(t, MarkAffiliateWithdrawalPaid(withdrawal.Id, 1, "duplicate paid"))
	balance = loadAffiliateTestBalance(t, 8111)
	assert.Equal(t, 600, balance.AvailableQuota)
	assert.Zero(t, balance.FrozenQuota)
	assert.Equal(t, 400, balance.WithdrawnQuota)
	assertAffiliateBalanceConserved(t, balance)
}

func TestAffiliateMinimumWithdrawalConversionRejectsUnsafeValues(t *testing.T) {
	originalQuotaPerUnit := common.QuotaPerUnit
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalExchangeRate := operation_setting.USDExchangeRate
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
		operation_setting.USDExchangeRate = originalExchangeRate
	})

	testCases := []struct {
		name         string
		displayType  string
		quotaPerUnit float64
		exchangeRate float64
		minimum      int
		quota        int
		wantErr      bool
	}{
		{name: "exact currency boundary", displayType: operation_setting.QuotaDisplayTypeUSD, quotaPerUnit: 10, exchangeRate: 1, minimum: 2, quota: 20},
		{name: "below currency boundary", displayType: operation_setting.QuotaDisplayTypeUSD, quotaPerUnit: 10, exchangeRate: 1, minimum: 2, quota: 19, wantErr: true},
		{name: "token minimum at saturation boundary", displayType: operation_setting.QuotaDisplayTypeTokens, quotaPerUnit: 10, exchangeRate: 1, minimum: common.MaxQuota, quota: 1, wantErr: true},
		{name: "oversized currency minimum", displayType: operation_setting.QuotaDisplayTypeUSD, quotaPerUnit: math.MaxFloat64, exchangeRate: 1, minimum: 2, quota: 1, wantErr: true},
		{name: "nan quota unit", displayType: operation_setting.QuotaDisplayTypeUSD, quotaPerUnit: math.NaN(), exchangeRate: 1, minimum: 1, quota: 1, wantErr: true},
		{name: "zero quota unit", displayType: operation_setting.QuotaDisplayTypeUSD, quotaPerUnit: 0, exchangeRate: 1, minimum: 1, quota: 1, wantErr: true},
		{name: "nan exchange rate", displayType: operation_setting.QuotaDisplayTypeCNY, quotaPerUnit: 10, exchangeRate: math.NaN(), minimum: 1, quota: 1, wantErr: true},
		{name: "minimum below one quota", displayType: operation_setting.QuotaDisplayTypeCNY, quotaPerUnit: 1, exchangeRate: math.MaxFloat64, minimum: 1, quota: 1, wantErr: true},
	}
	for index, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			truncateTables(t)
			configureAffiliateTest(t)
			common.QuotaPerUnit = testCase.quotaPerUnit
			operation_setting.GetGeneralSetting().QuotaDisplayType = testCase.displayType
			operation_setting.USDExchangeRate = testCase.exchangeRate
			setting.GetAffiliateSetting().MinWithdrawalAmount = testCase.minimum

			userID := 8191 + index
			createAffiliateTestUser(t, userID, 0, 0)
			require.NoError(t, DB.Create(&AffiliateBalance{UserId: userID, AvailableQuota: 1_000, TotalQuota: 1_000}).Error)
			require.NoError(t, DB.Create(&AffiliatePayoutAccount{UserId: userID, AlipayAccount: "pay@example.test"}).Error)

			withdrawal, err := CreateAffiliateWithdrawal(userID, AffiliatePayoutMethodAlipay, testCase.quota)
			if testCase.wantErr {
				require.Error(t, err)
				assert.Nil(t, withdrawal)
				var count int64
				require.NoError(t, DB.Model(&AffiliateWithdrawal{}).Where("user_id = ?", userID).Count(&count).Error)
				assert.Zero(t, count)
				balance := loadAffiliateTestBalance(t, userID)
				assert.Equal(t, 1_000, balance.AvailableQuota)
				assert.Zero(t, balance.FrozenQuota)
				assertAffiliateBalanceConserved(t, balance)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, withdrawal)
			assert.Equal(t, testCase.quota, withdrawal.Quota)
			balance := loadAffiliateTestBalance(t, userID)
			assert.Equal(t, 1_000-testCase.quota, balance.AvailableQuota)
			assert.Equal(t, testCase.quota, balance.FrozenQuota)
			assertAffiliateBalanceConserved(t, balance)
		})
	}
}

func TestAffiliateTransferUsesAtomicWalletCapacityAndSynchronizesLegacyFields(t *testing.T) {
	truncateTables(t)
	configureAffiliateTest(t)

	createAffiliateTestUser(t, 8121, 0, common.MaxQuota-50)
	require.NoError(t, DB.Create(&AffiliateBalance{UserId: 8121, AvailableQuota: 100, TotalQuota: 100}).Error)

	err := TransferAffiliateQuotaToBalance(8121, 100)
	require.ErrorIs(t, err, ErrTopUpQuotaLimitExceeded)
	balance := loadAffiliateTestBalance(t, 8121)
	assert.Equal(t, 100, balance.AvailableQuota)
	assert.Zero(t, balance.TransferredQuota)
	assertAffiliateBalanceConserved(t, balance)

	require.NoError(t, DB.Model(&User{}).Where("id = ?", 8121).Update("quota", 10).Error)
	require.NoError(t, TransferAffiliateQuotaToBalance(8121, 100))
	balance = loadAffiliateTestBalance(t, 8121)
	assert.Zero(t, balance.AvailableQuota)
	assert.Equal(t, 100, balance.TransferredQuota)
	assertAffiliateBalanceConserved(t, balance)

	var user User
	require.NoError(t, DB.Select("quota", "aff_quota", "aff_history").Where("id = ?", 8121).First(&user).Error)
	assert.Equal(t, 110, user.Quota)
	assert.Zero(t, user.AffQuota)
	assert.Equal(t, 100, user.AffHistoryQuota)
}

func TestAffiliateBalanceMigratesLegacyUserFieldsOnFirstAccess(t *testing.T) {
	truncateTables(t)
	configureAffiliateTest(t)

	createAffiliateTestUser(t, 8122, 0, 0)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", 8122).Updates(map[string]interface{}{
		"aff_quota":   700,
		"aff_history": 1_000,
	}).Error)
	balance, err := GetAffiliateBalance(8122)
	require.NoError(t, err)
	assert.Equal(t, 700, balance.AvailableQuota)
	assert.Equal(t, 1_000, balance.TotalQuota)
	assert.Equal(t, 300, balance.TransferredQuota)
	assertAffiliateBalanceConserved(t, *balance)

	var user User
	require.NoError(t, DB.Select("aff_quota", "aff_history").Where("id = ?", 8122).First(&user).Error)
	assert.Equal(t, 700, user.AffQuota)
	assert.Equal(t, 1_000, user.AffHistoryQuota)
}

func TestAffiliateInviterBindingRejectsSelfCycleAndDuplicateCount(t *testing.T) {
	truncateTables(t)
	configureAffiliateTest(t)

	first := createAffiliateTestUser(t, 8131, 8132, 0)
	second := createAffiliateTestUser(t, 8132, 0, 0)

	_, err := BindUserInviterByAffCode(first.Id, "", first.AffCode, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "自己")

	_, err = BindUserInviterByAffCode(second.Id, "", first.AffCode, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "循环")

	third := createAffiliateTestUser(t, 8133, 0, 0)
	result, err := BindUserInviterByAffCode(third.Id, "", second.AffCode, false)
	require.NoError(t, err)
	assert.True(t, result.Updated)
	result, err = BindUserInviterByAffCode(third.Id, "", second.AffCode, false)
	require.NoError(t, err)
	assert.False(t, result.Updated)

	var inviter User
	require.NoError(t, DB.Select("aff_count").Where("id = ?", second.Id).First(&inviter).Error)
	assert.Equal(t, 1, inviter.AffCount)
}

func TestAffiliateFraudClawbackConfiscatesOnce(t *testing.T) {
	truncateTables(t)
	configureAffiliateTest(t)

	createAffiliateTestUser(t, 8141, 0, 0)
	createAffiliateTestUser(t, 8142, 8141, 0)
	require.NoError(t, CreateAffiliateRewardsForPayment(8142, AffiliateSourceTopUp, "clawback-order", 10_000))
	require.NoError(t, DB.Model(&AffiliateRecord{}).
		Where("source_type = ? AND source_id = ?", AffiliateSourceTopUp, "clawback-order").
		Update("available_time", common.GetTimestamp()-1).Error)
	require.NoError(t, SettleMatureAffiliateRecords(8141))

	alert := AffiliateFraudAlert{
		InviterId:  8141,
		InviteeId:  8142,
		Status:     FraudAlertStatusDetected,
		DetectedAt: common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(&alert).Error)
	require.NoError(t, UnbindAffiliateRelationship(alert.Id, 1, true))

	balance := loadAffiliateTestBalance(t, 8141)
	assert.Zero(t, balance.AvailableQuota)
	assert.Equal(t, 1_000, balance.ConfiscatedQuota)
	assert.Zero(t, balance.TotalQuota)
	var record AffiliateRecord
	require.NoError(t, DB.Where("source_type = ? AND source_id = ?", AffiliateSourceTopUp, "clawback-order").First(&record).Error)
	assert.Equal(t, AffiliateRecordStatusConfiscated, record.Status)

	err := UnbindAffiliateRelationship(alert.Id, 1, true)
	require.Error(t, err)
	balance = loadAffiliateTestBalance(t, 8141)
	assert.Equal(t, 1_000, balance.ConfiscatedQuota)
}

func TestAffiliateFraudClawbackWaitsForWithdrawalTerminalState(t *testing.T) {
	testCases := []struct {
		name                  string
		approveBeforeClawback bool
		terminalStatus        string
		wantRecovered         int
		wantWithdrawn         int
		wantTotal             int
	}{
		{
			name:           "pending withdrawal is rejected before clawback",
			terminalStatus: AffiliateWithdrawalStatusRejected,
			wantRecovered:  1_000,
		},
		{
			name:                  "approved withdrawal is paid before clawback",
			approveBeforeClawback: true,
			terminalStatus:        AffiliateWithdrawalStatusPaid,
			wantRecovered:         600,
			wantWithdrawn:         400,
			wantTotal:             400,
		},
	}
	for index, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			truncateTables(t)
			configureAffiliateTest(t)
			inviterID := 8211 + index*2
			inviteeID := inviterID + 1
			createAffiliateTestUser(t, inviterID, 0, 0)
			createAffiliateTestUser(t, inviteeID, inviterID, 0)
			sourceID := "withdrawal-clawback-" + testCase.terminalStatus
			require.NoError(t, CreateAffiliateRewardsForPayment(inviteeID, AffiliateSourceTopUp, sourceID, 10_000))
			require.NoError(t, DB.Model(&AffiliateRecord{}).
				Where("source_type = ? AND source_id = ?", AffiliateSourceTopUp, sourceID).
				Update("available_time", common.GetTimestamp()-1).Error)
			require.NoError(t, SettleMatureAffiliateRecords(inviterID))
			require.NoError(t, DB.Create(&AffiliatePayoutAccount{UserId: inviterID, AlipayAccount: "pay@example.test"}).Error)
			withdrawal, err := CreateAffiliateWithdrawal(inviterID, AffiliatePayoutMethodAlipay, 400)
			require.NoError(t, err)
			if testCase.approveBeforeClawback {
				require.NoError(t, ApproveAffiliateWithdrawal(withdrawal.Id, 1, "approve"))
			}

			alert := AffiliateFraudAlert{
				InviterId:  inviterID,
				InviteeId:  inviteeID,
				Status:     FraudAlertStatusDetected,
				DetectedAt: common.GetTimestamp(),
			}
			require.NoError(t, DB.Create(&alert).Error)
			err = UnbindAffiliateRelationship(alert.Id, 1, true)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "提现申请")

			balance := loadAffiliateTestBalance(t, inviterID)
			assert.Equal(t, 600, balance.AvailableQuota)
			assert.Equal(t, 400, balance.FrozenQuota)
			assert.Zero(t, balance.ConfiscatedQuota)
			assertAffiliateBalanceConserved(t, balance)
			var unchangedRecord AffiliateRecord
			require.NoError(t, DB.Where("source_type = ? AND source_id = ?", AffiliateSourceTopUp, sourceID).First(&unchangedRecord).Error)
			assert.Equal(t, AffiliateRecordStatusAvailable, unchangedRecord.Status)
			var unchangedAlert AffiliateFraudAlert
			require.NoError(t, DB.First(&unchangedAlert, alert.Id).Error)
			assert.Equal(t, FraudAlertStatusDetected, unchangedAlert.Status)
			var unchangedInvitee User
			require.NoError(t, DB.Select("inviter_id").First(&unchangedInvitee, inviteeID).Error)
			assert.Equal(t, inviterID, unchangedInvitee.InviterId)

			switch testCase.terminalStatus {
			case AffiliateWithdrawalStatusRejected:
				require.NoError(t, RejectAffiliateWithdrawal(withdrawal.Id, 1, "reject before clawback"))
			case AffiliateWithdrawalStatusPaid:
				require.NoError(t, MarkAffiliateWithdrawalPaid(withdrawal.Id, 1, "pay before clawback"))
			default:
				t.Fatalf("unsupported terminal status %q", testCase.terminalStatus)
			}
			require.NoError(t, UnbindAffiliateRelationship(alert.Id, 1, true))

			balance = loadAffiliateTestBalance(t, inviterID)
			assert.Zero(t, balance.AvailableQuota)
			assert.Zero(t, balance.FrozenQuota)
			assert.Equal(t, testCase.wantRecovered, balance.ConfiscatedQuota)
			assert.Equal(t, testCase.wantWithdrawn, balance.WithdrawnQuota)
			assert.Equal(t, testCase.wantTotal, balance.TotalQuota)
			assertAffiliateBalanceConserved(t, balance)
			var finalizedWithdrawal AffiliateWithdrawal
			require.NoError(t, DB.First(&finalizedWithdrawal, withdrawal.Id).Error)
			assert.Equal(t, testCase.terminalStatus, finalizedWithdrawal.Status)
			var confiscatedRecord AffiliateRecord
			require.NoError(t, DB.Where("source_type = ? AND source_id = ?", AffiliateSourceTopUp, sourceID).First(&confiscatedRecord).Error)
			assert.Equal(t, AffiliateRecordStatusConfiscated, confiscatedRecord.Status)
			var resolvedAlert AffiliateFraudAlert
			require.NoError(t, DB.First(&resolvedAlert, alert.Id).Error)
			assert.Equal(t, FraudAlertStatusResolved, resolvedAlert.Status)
			assert.Equal(t, testCase.wantRecovered, resolvedAlert.ClawbackQuota)
		})
	}
}

func TestAffiliateRiskFreezeRoutesSettlementToRiskBalance(t *testing.T) {
	truncateTables(t)
	configureAffiliateTest(t)

	createAffiliateTestUser(t, 8151, 0, 0)
	createAffiliateTestUser(t, 8152, 8151, 0)
	_, err := ApplyAffiliateRiskAction(8151, 1, AffiliateRiskApplyRequest{
		FreezeAssets: true,
		Reason:       "risk review",
	})
	require.NoError(t, err)
	require.NoError(t, CreateAffiliateRewardsForPayment(8152, AffiliateSourceTopUp, "risk-settlement-order", 10_000))
	require.NoError(t, DB.Model(&AffiliateRecord{}).
		Where("source_type = ? AND source_id = ?", AffiliateSourceTopUp, "risk-settlement-order").
		Update("available_time", common.GetTimestamp()-1).Error)
	require.NoError(t, SettleMatureAffiliateRecords(8151))

	balance := loadAffiliateTestBalance(t, 8151)
	assert.Zero(t, balance.PendingQuota)
	assert.Zero(t, balance.AvailableQuota)
	assert.Equal(t, 1_000, balance.RiskFrozenQuota)
	assertAffiliateBalanceConserved(t, balance)

	_, err = RemoveAffiliateRiskAction(8151, 1, AffiliateRiskRemoveRequest{})
	require.NoError(t, err)
	balance = loadAffiliateTestBalance(t, 8151)
	assert.Equal(t, 1_000, balance.AvailableQuota)
	assert.Zero(t, balance.RiskFrozenQuota)
	assertAffiliateBalanceConserved(t, balance)
}

func TestAffiliateRegistrationCountsWithoutFixedInviterReward(t *testing.T) {
	truncateTables(t)
	configureAffiliateTest(t)

	paymentSetting := operation_setting.GetPaymentSetting()
	originalConfirmed := paymentSetting.ComplianceConfirmed
	originalTermsVersion := paymentSetting.ComplianceTermsVersion
	originalNewUserQuota := common.QuotaForNewUser
	originalInviteeQuota := common.QuotaForInvitee
	originalInviterQuota := common.QuotaForInviter
	t.Cleanup(func() {
		paymentSetting.ComplianceConfirmed = originalConfirmed
		paymentSetting.ComplianceTermsVersion = originalTermsVersion
		common.QuotaForNewUser = originalNewUserQuota
		common.QuotaForInvitee = originalInviteeQuota
		common.QuotaForInviter = originalInviterQuota
	})
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	common.QuotaForNewUser = 0
	common.QuotaForInvitee = 123
	common.QuotaForInviter = 456

	createAffiliateTestUser(t, 8161, 0, 0)
	invitee := User{
		Username: common.GetRandomString(12),
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
	}
	require.NoError(t, invitee.Insert(8161))

	var savedInvitee User
	require.NoError(t, DB.Where("id = ?", invitee.Id).First(&savedInvitee).Error)
	assert.Equal(t, 8161, savedInvitee.InviterId)
	assert.Equal(t, 123, savedInvitee.Quota)

	var inviter User
	require.NoError(t, DB.Where("id = ?", 8161).First(&inviter).Error)
	assert.Equal(t, 1, inviter.AffCount)
	assert.Zero(t, inviter.AffQuota)
	assert.Zero(t, inviter.AffHistoryQuota)
}

func TestAffiliateBusinessQuotaSnapshotsExcludeInvoiceFee(t *testing.T) {
	topup := &TopUp{
		CreditedQuota:        9_000,
		AffiliateSourceQuota: 5_000,
		InvoiceFeeAmount:     4,
	}
	assert.Equal(t, 5_000, topUpAffiliateSourceQuota(topup, topup.CreditedQuota))
	freeTopUp := &TopUp{
		CreditedQuota: 9_000,
		PromoCodeId:   1,
	}
	assert.Zero(t, topUpAffiliateSourceQuota(freeTopUp, 0))

	order := &SubscriptionOrder{
		Money:                9,
		AffiliateSourceQuota: 4_000,
		InvoiceFeeAmount:     5,
	}
	assert.Equal(t, 4_000, subscriptionOrderAffiliateSourceQuota(order))
}

func TestAffiliateFraudOverlapCanonicalizesPublicIPv6(t *testing.T) {
	truncateTables(t)
	configureAffiliateTest(t)

	createAffiliateTestUser(t, 8171, 0, 0)
	createAffiliateTestUser(t, 8172, 8171, 0)
	require.NoError(t, DB.Create(&UserIPRecord{UserId: 8171, Ip: "2001:4860:4860:0:0:0:0:8888", Action: "login"}).Error)
	require.NoError(t, DB.Create(&UserIPRecord{UserId: 8172, Ip: "2001:4860:4860::8888", Action: "register"}).Error)
	require.NoError(t, DB.Create(&UserIPRecord{UserId: 8171, Ip: "fd00::1", Action: "login"}).Error)
	require.NoError(t, DB.Create(&UserIPRecord{UserId: 8172, Ip: "fd00::1", Action: "register"}).Error)

	overlaps, err := GetIPOverlap(8171, 8172, 0)
	require.NoError(t, err)
	assert.Equal(t, []string{"2001:4860:4860::8888"}, overlaps)

	batch, err := GetIPOverlapBatch(8171, []int{8172}, 0)
	require.NoError(t, err)
	assert.Equal(t, []string{"2001:4860:4860::8888"}, batch[8172])
}

func TestAffiliateRechargeEligibilityUsesNormalizedBusinessCNY(t *testing.T) {
	truncateTables(t)
	configureAffiliateTest(t)

	originalPrice := operation_setting.Price
	operation_setting.Price = 7
	t.Cleanup(func() { operation_setting.Price = originalPrice })

	user := createAffiliateTestUser(t, 8181, 0, 0)
	topUps := []TopUp{
		{UserId: user.Id, Money: 10, ActualMoney: 10, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusSuccess, TradeNo: "aff-cny-stripe"},
		{UserId: user.Id, Money: 70, ActualMoney: 70, PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusSuccess, TradeNo: "aff-cny-epay"},
		{UserId: user.Id, Money: 60, ActualMoney: 50, PaidAmountCNY: 50, InvoiceFeeAmount: 10, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusSuccess, TradeNo: "aff-cny-snapshot"},
		{UserId: user.Id, PromoCodeId: 1, Money: 0, ActualMoney: 0, PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusSuccess, TradeNo: "aff-cny-free"},
	}
	for i := range topUps {
		require.NoError(t, DB.Create(&topUps[i]).Error)
	}

	total, err := GetUserTotalRechargeAmount(user.Id)
	require.NoError(t, err)
	assert.InDelta(t, 190, total, 0.000001)

	s := setting.GetAffiliateSetting()
	s.InviteeMinRechargeAmount = 190
	eligible, err := checkInviteeEligibility(DB, user.Id, user.CreatedAt, s)
	require.NoError(t, err)
	assert.True(t, eligible)
	s.InviteeMinRechargeAmount = 191
	eligible, err = checkInviteeEligibility(DB, user.Id, user.CreatedAt, s)
	require.NoError(t, err)
	assert.False(t, eligible)
}
