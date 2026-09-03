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

func TestBenefitDisplayAmountToCNYCentsRejectsPositiveSubCentConversions(t *testing.T) {
	tests := []struct {
		name   string
		amount decimal.Decimal
		ctx    BenefitAmountDisplayContext
	}{
		{
			name: "USD", amount: decimal.RequireFromString("0.01"),
			ctx: BenefitAmountDisplayContext{
				DisplayType:  operation_setting.QuotaDisplayTypeUSD,
				QuotaPerUnit: decimal.NewFromInt(500000), DisplayRate: decimal.NewFromInt(1), CNYRate: decimal.RequireFromString("0.01"),
			},
		},
		{
			name: "CUSTOM", amount: decimal.RequireFromString("0.01"),
			ctx: BenefitAmountDisplayContext{
				DisplayType:  operation_setting.QuotaDisplayTypeCustom,
				QuotaPerUnit: decimal.NewFromInt(500000), DisplayRate: decimal.NewFromInt(100), CNYRate: decimal.RequireFromString("7.5"),
			},
		},
		{
			name: "TOKENS", amount: decimal.NewFromInt(1),
			ctx: BenefitAmountDisplayContext{
				DisplayType:  operation_setting.QuotaDisplayTypeTokens,
				QuotaPerUnit: decimal.NewFromInt(500000), DisplayRate: decimal.NewFromInt(1), CNYRate: decimal.RequireFromString("7.5"),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.ctx.DisplayAmountToCNYCents(tc.amount)
			require.ErrorContains(t, err, "0.01")
		})
	}

	tokens := tests[2].ctx
	cents, err := tokens.DisplayAmountToCNYCents(decimal.NewFromInt(667))
	require.NoError(t, err)
	assert.Equal(t, int64(1), cents)

	cny := BenefitAmountDisplayContext{
		DisplayType:  operation_setting.QuotaDisplayTypeCNY,
		QuotaPerUnit: decimal.NewFromInt(500000),
		DisplayRate:  decimal.RequireFromString("7.5"),
		CNYRate:      decimal.RequireFromString("7.5"),
	}
	cents, err = cny.DisplayAmountToCNYCents(decimal.RequireFromString("0.01"))
	require.NoError(t, err)
	assert.Equal(t, int64(1), cents)
}

func TestMigrateBenefitActivityQuotaConfigIsIdempotentAcrossAmountModes(t *testing.T) {
	group := setupBenefitVoucherTestDB(t)
	fixed := &BenefitActivity{
		Name: "旧固定活动", GroupId: group.Id, Status: BenefitActivityStatusDraft,
		AmountMode: BenefitAmountModeFixed, TotalQuota: 300, TotalCount: 3,
	}
	random := &BenefitActivity{
		Name: "已迁移随机活动", GroupId: group.Id, Status: BenefitActivityStatusDraft,
		AmountMode: BenefitAmountModeRandom, TotalQuota: 300, TotalCount: 2,
		MinQuota: 100, MaxQuota: 200,
		AmountDisplayTypeSnapshot: operation_setting.QuotaDisplayTypeUSD,
		AmountDisplayRateSnapshot: "1", QuotaPerUnitSnapshot: "500000",
	}
	require.NoError(t, DB.Create(fixed).Error)
	require.NoError(t, DB.Create(random).Error)

	require.NoError(t, migrateBenefitActivityQuotaConfig(DB))
	var migratedFixed, migratedRandom BenefitActivity
	require.NoError(t, DB.First(&migratedFixed, fixed.Id).Error)
	require.NoError(t, DB.First(&migratedRandom, random.Id).Error)
	assert.Equal(t, int64(100), migratedFixed.FixedQuota)
	assert.NotEmpty(t, migratedFixed.AmountDisplayTypeSnapshot)
	assert.NotEmpty(t, migratedFixed.AmountDisplayRateSnapshot)
	assert.NotEmpty(t, migratedFixed.QuotaPerUnitSnapshot)
	assert.Zero(t, migratedRandom.FixedQuota, "随机活动的 fixed_quota=0 是合法配置")
	firstUpdatedAt := migratedFixed.UpdatedAt

	require.NoError(t, migrateBenefitActivityQuotaConfig(DB))
	var rerunFixed, rerunRandom BenefitActivity
	require.NoError(t, DB.First(&rerunFixed, fixed.Id).Error)
	require.NoError(t, DB.First(&rerunRandom, random.Id).Error)
	assert.Equal(t, firstUpdatedAt, rerunFixed.UpdatedAt)
	assert.Equal(t, migratedFixed.FixedQuota, rerunFixed.FixedQuota)
	assert.Equal(t, migratedRandom.MinQuota, rerunRandom.MinQuota)
	assert.Equal(t, migratedRandom.MaxQuota, rerunRandom.MaxQuota)
}
