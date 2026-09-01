package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBenefitDisplayAmountToQuotaSupportsAllDisplayTypes(t *testing.T) {
	tests := []struct {
		name        string
		displayType string
		amount      string
		rate        string
		quota       int64
	}{
		{name: "USD", displayType: operation_setting.QuotaDisplayTypeUSD, amount: "1.25", rate: "1", quota: 625000},
		{name: "CNY", displayType: operation_setting.QuotaDisplayTypeCNY, amount: "7.20", rate: "7.2", quota: 500000},
		{name: "CUSTOM", displayType: operation_setting.QuotaDisplayTypeCustom, amount: "2.00", rate: "2", quota: 500000},
		{name: "TOKENS", displayType: operation_setting.QuotaDisplayTypeTokens, amount: "500000", rate: "1", quota: 500000},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := BenefitAmountDisplayContext{
				DisplayType:  tc.displayType,
				QuotaPerUnit: decimal.NewFromInt(500000),
				DisplayRate:  decimal.RequireFromString(tc.rate),
			}
			got, err := ctx.DisplayAmountToQuota(decimal.RequireFromString(tc.amount))
			require.NoError(t, err)
			assert.Equal(t, tc.quota, got)
		})
	}
}

func TestBenefitDisplayAmountRejectsInvalidPrecisionAndBounds(t *testing.T) {
	currency := BenefitAmountDisplayContext{
		DisplayType:  operation_setting.QuotaDisplayTypeUSD,
		QuotaPerUnit: decimal.NewFromInt(500000),
		DisplayRate:  decimal.NewFromInt(1),
	}
	_, err := currency.DisplayAmountToQuota(decimal.RequireFromString("1.001"))
	require.ErrorContains(t, err, "两位小数")
	_, err = currency.DisplayAmountToQuota(decimal.Zero)
	require.Error(t, err)
	_, err = currency.DisplayAmountToQuota(decimal.RequireFromString("-1"))
	require.Error(t, err)

	tokens := currency
	tokens.DisplayType = operation_setting.QuotaDisplayTypeTokens
	_, err = tokens.DisplayAmountToQuota(decimal.RequireFromString("1.5"))
	require.ErrorContains(t, err, "Tokens")

	for _, invalid := range []BenefitAmountDisplayContext{
		{DisplayType: operation_setting.QuotaDisplayTypeUSD, QuotaPerUnit: decimal.Zero, DisplayRate: decimal.NewFromInt(1)},
		{DisplayType: operation_setting.QuotaDisplayTypeUSD, QuotaPerUnit: decimal.NewFromInt(1), DisplayRate: decimal.Zero},
	} {
		_, err = invalid.DisplayAmountToQuota(decimal.NewFromInt(1))
		require.Error(t, err)
	}
	_, err = tokens.DisplayAmountToQuota(decimal.NewFromInt(common.MaxWalletQuota).Add(decimal.NewFromInt(1)))
	require.Error(t, err)
}

func TestBenefitDisplayAmountToCNYCentsUsesPaymentCurrencySnapshot(t *testing.T) {
	ctx := BenefitAmountDisplayContext{
		DisplayType:  operation_setting.QuotaDisplayTypeCNY,
		QuotaPerUnit: decimal.NewFromInt(500000),
		DisplayRate:  decimal.RequireFromString("7.2"),
		CNYRate:      decimal.RequireFromString("7.5"),
	}
	cents, err := ctx.DisplayAmountToCNYCents(decimal.RequireFromString("7.20"))
	require.NoError(t, err)
	assert.Equal(t, int64(750), cents)

	tokens := ctx
	tokens.DisplayType = operation_setting.QuotaDisplayTypeTokens
	cents, err = tokens.DisplayAmountToCNYCents(decimal.NewFromInt(500000))
	require.NoError(t, err)
	assert.Equal(t, int64(750), cents)
}
