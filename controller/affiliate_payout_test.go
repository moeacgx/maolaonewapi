package controller

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateAffiliateWithdrawalPayoutUsesFiatForAlipay(t *testing.T) {
	originalQuotaPerUnit := common.QuotaPerUnit
	originalExchangeRate := operation_setting.USDExchangeRate
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		operation_setting.USDExchangeRate = originalExchangeRate
	})
	common.QuotaPerUnit = 100
	operation_setting.USDExchangeRate = 7.2

	payout, err := calculateAffiliateWithdrawalPayout(model.AffiliatePayoutMethodAlipay, 1_000)

	require.NoError(t, err)
	assert.InDelta(t, 72, payout.Amount, 0.000001)
	assert.Equal(t, "CNY", payout.Currency)
	assert.InDelta(t, 72, payout.FiatAmount, 0.000001)
	assert.Equal(t, "usd-cny", payout.RateSource)
}

func TestCalculateAffiliateWithdrawalPayoutUsesConfiguredUsdtFallback(t *testing.T) {
	originalQuotaPerUnit := common.QuotaPerUnit
	originalExchangeRate := operation_setting.USDExchangeRate
	originalAuto := setting.OkpayAutoExchangeEnabled
	originalUsdtRate := setting.OkpayUsdtCnyRate
	originalExchange := setting.OkpayExchangeRate
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		operation_setting.USDExchangeRate = originalExchangeRate
		setting.OkpayAutoExchangeEnabled = originalAuto
		setting.OkpayUsdtCnyRate = originalUsdtRate
		setting.OkpayExchangeRate = originalExchange
	})
	common.QuotaPerUnit = 100
	operation_setting.USDExchangeRate = 7.2
	setting.OkpayAutoExchangeEnabled = false
	setting.OkpayUsdtCnyRate = 6.8
	setting.OkpayExchangeRate = 0

	payout, err := calculateAffiliateWithdrawalPayout(model.AffiliatePayoutMethodUSDT, 1_000)

	require.NoError(t, err)
	assert.InDelta(t, 10.58823529, payout.Amount, 0.00000001)
	assert.Equal(t, "USDT", payout.Currency)
	assert.InDelta(t, 6.8, payout.Rate, 0.000001)
	assert.Equal(t, "fallback", payout.RateSource)
}

func TestCalculateAffiliateWithdrawalPayoutRejectsMissingUsdtFallback(t *testing.T) {
	originalQuotaPerUnit := common.QuotaPerUnit
	originalExchangeRate := operation_setting.USDExchangeRate
	originalAuto := setting.OkpayAutoExchangeEnabled
	originalUsdtRate := setting.OkpayUsdtCnyRate
	originalExchange := setting.OkpayExchangeRate
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		operation_setting.USDExchangeRate = originalExchangeRate
		setting.OkpayAutoExchangeEnabled = originalAuto
		setting.OkpayUsdtCnyRate = originalUsdtRate
		setting.OkpayExchangeRate = originalExchange
	})
	common.QuotaPerUnit = 100
	operation_setting.USDExchangeRate = 7.2
	setting.OkpayAutoExchangeEnabled = false
	setting.OkpayUsdtCnyRate = math.NaN()
	setting.OkpayExchangeRate = 0

	payout, err := calculateAffiliateWithdrawalPayout(model.AffiliatePayoutMethodUSDT, 1_000)

	require.Error(t, err)
	assert.Zero(t, payout.Amount)
}
