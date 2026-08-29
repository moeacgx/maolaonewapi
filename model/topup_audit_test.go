package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readTopUpAuditInfo(t *testing.T, tradeNo string) map[string]interface{} {
	t.Helper()
	var log Log
	require.NoError(t, LOG_DB.Where("type = ?", LogTypeTopup).Order("id DESC").First(&log).Error)
	other, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, tradeNo, adminInfo["trade_no"])
	return adminInfo
}

func TestRechargeEpayRecordsTransactionalTopUpBalanceAudit(t *testing.T) {
	truncateTables(t)

	user := insertUserForPaymentGuardTest(t, 911, 7)
	topUp := &TopUp{
		UserId: user.Id, Amount: 2, Money: 2, CreditedQuota: 10, PaidAmountCNY: 2,
		TradeNo: "audit-epay-order", PaymentMethod: "alipay", PaymentProvider: PaymentProviderEpay,
		CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())

	alreadyDone, err := RechargeEpay(topUp.TradeNo, "alipay", "127.0.0.1")
	require.NoError(t, err)
	assert.False(t, alreadyDone)

	audit := readTopUpAuditInfo(t, topUp.TradeNo)
	assert.Equal(t, float64(7), audit["balance_before"])
	assert.Equal(t, float64(10), audit["credited_quota"])
	assert.Equal(t, float64(17), audit["balance_after"])
	assert.Equal(t, topUp.TradeNo, audit["trade_no"])
	assert.Equal(t, float64(2), audit["paid_amount_cny"])
}

func TestManualCompleteTopUpDoesNotLogDuplicateCallbackAudit(t *testing.T) {
	truncateTables(t)

	user := insertUserForPaymentGuardTest(t, 912, 5)
	topUp := &TopUp{
		UserId: user.Id, Amount: 2, Money: 2, CreditedQuota: 10, PaidAmountCNY: 2,
		TradeNo: "audit-manual-order", PaymentMethod: "alipay", PaymentProvider: PaymentProviderEpay,
		CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())

	require.NoError(t, ManualCompleteTopUp(topUp.TradeNo, "127.0.0.1"))
	var count int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("type = ?", LogTypeTopup).Count(&count).Error)
	assert.Equal(t, int64(1), count)

	require.NoError(t, ManualCompleteTopUp(topUp.TradeNo, "127.0.0.1"))
	require.NoError(t, LOG_DB.Model(&Log{}).Where("type = ?", LogTypeTopup).Count(&count).Error)
	assert.Equal(t, int64(1), count)

	audit := readTopUpAuditInfo(t, topUp.TradeNo)
	assert.Equal(t, float64(5), audit["balance_before"])
	assert.Equal(t, float64(10), audit["credited_quota"])
	assert.Equal(t, float64(15), audit["balance_after"])
}

func TestStripeTopUpRecordsPaidAmountAndBalanceAudit(t *testing.T) {
	truncateTables(t)

	user := insertUserForPaymentGuardTest(t, 913, 3)
	topUp := &TopUp{
		UserId: user.Id, Amount: 2, Money: 2, CreditedQuota: 8, PaidAmountCNY: 1.5,
		TradeNo: "audit-stripe-order", PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe,
		CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())

	require.NoError(t, Recharge(topUp.TradeNo, "cus_audit", "127.0.0.2"))
	audit := readTopUpAuditInfo(t, topUp.TradeNo)
	assert.Equal(t, float64(3), audit["balance_before"])
	assert.Equal(t, float64(8), audit["credited_quota"])
	assert.Equal(t, float64(11), audit["balance_after"])
	assert.Equal(t, topUp.TradeNo, audit["trade_no"])
	assert.Equal(t, float64(1.5), audit["paid_amount_cny"])
}

func TestPaymentAttemptTopUpRecordsAuditAndSkipsDuplicateCallback(t *testing.T) {
	truncateTables(t)

	user := insertUserForPaymentGuardTest(t, 914, 4)
	topUp := &TopUp{
		UserId: user.Id, Amount: 2, Money: 2, CreditedQuota: 6, PaidAmountCNY: 2,
		TradeNo: "audit-attempt-order", PaymentMethod: PaymentMethodOkpay, PaymentProvider: PaymentProviderOkpay,
		CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())
	attempt, err := CreateTopUpPaymentAttempt(topUp.TradeNo, PaymentProviderOkpay, PaymentMethodOkpay, "2.00000000", "USDT")
	require.NoError(t, err)
	require.NoError(t, MarkTopUpPaymentAttemptLaunched(attempt.Id, "provider-audit"))

	alreadyDone, err := CompleteTopUpPaymentAttempt(attempt.Id, topUp.TradeNo, PaymentProviderOkpay, PaymentMethodOkpay, "127.0.0.3")
	require.NoError(t, err)
	assert.False(t, alreadyDone)

	alreadyDone, err = CompleteTopUpPaymentAttempt(attempt.Id, topUp.TradeNo, PaymentProviderOkpay, PaymentMethodOkpay, "127.0.0.3")
	require.NoError(t, err)
	assert.True(t, alreadyDone)
	var count int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("type = ?", LogTypeTopup).Count(&count).Error)
	assert.Equal(t, int64(1), count)

	audit := readTopUpAuditInfo(t, topUp.TradeNo)
	assert.Equal(t, float64(4), audit["balance_before"])
	assert.Equal(t, float64(6), audit["credited_quota"])
	assert.Equal(t, float64(10), audit["balance_after"])
}

func TestFailedTopUpDoesNotRecordBalanceAudit(t *testing.T) {
	truncateTables(t)

	user := insertUserForPaymentGuardTest(t, 915, common.MaxWalletQuota-5)
	topUp := &TopUp{
		UserId: user.Id, Amount: 2, Money: 2, CreditedQuota: 10, PaidAmountCNY: 2,
		TradeNo: "audit-failed-order", PaymentMethod: "alipay", PaymentProvider: PaymentProviderEpay,
		CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())

	_, err := RechargeEpay(topUp.TradeNo, "alipay", "127.0.0.4")
	require.ErrorIs(t, err, ErrTopUpQuotaLimitExceeded)
	assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, topUp.TradeNo))
	var count int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("type = ?", LogTypeTopup).Count(&count).Error)
	assert.Zero(t, count)
}

func TestWaffoPancakeTopUpRecordsBalanceAuditOnce(t *testing.T) {
	truncateTables(t)

	user := insertUserForPaymentGuardTest(t, 916, 6)
	topUp := &TopUp{
		UserId: user.Id, Amount: 2, Money: 2, CreditedQuota: 4, PaidAmountCNY: 2,
		TradeNo: "audit-pancake-order", PaymentMethod: PaymentMethodWaffoPancake, PaymentProvider: PaymentProviderWaffoPancake,
		CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())

	require.NoError(t, RechargeWaffoPancake(topUp.TradeNo, "127.0.0.5"))
	require.NoError(t, RechargeWaffoPancake(topUp.TradeNo, "127.0.0.5"))
	var count int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("type = ?", LogTypeTopup).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	audit := readTopUpAuditInfo(t, topUp.TradeNo)
	assert.Equal(t, float64(6), audit["balance_before"])
	assert.Equal(t, float64(4), audit["credited_quota"])
	assert.Equal(t, float64(10), audit["balance_after"])
}
