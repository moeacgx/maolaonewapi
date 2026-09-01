package model

import (
	"errors"
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var ErrBenefitAmountDisplayChanged = errors.New("benefit_amount_display_changed")

// BenefitAmountDisplayContext 固定一次活动请求使用的单位换算环境。
// DisplayRate 表示 1 美元对应的展示单位数，CNYRate 表示当前美元兑人民币汇率。
type BenefitAmountDisplayContext struct {
	DisplayType  string
	QuotaPerUnit decimal.Decimal
	DisplayRate  decimal.Decimal
	CNYRate      decimal.Decimal
}

func CurrentBenefitAmountDisplayContext() BenefitAmountDisplayContext {
	general := operation_setting.GetGeneralSetting()
	quotaPerUnit := decimal.Zero
	if common.QuotaPerUnit > 0 && !math.IsNaN(common.QuotaPerUnit) && !math.IsInf(common.QuotaPerUnit, 0) {
		quotaPerUnit = decimal.NewFromFloat(common.QuotaPerUnit)
	}
	cnyRate := decimal.Zero
	if operation_setting.USDExchangeRate > 0 && !math.IsNaN(operation_setting.USDExchangeRate) && !math.IsInf(operation_setting.USDExchangeRate, 0) {
		cnyRate = decimal.NewFromFloat(operation_setting.USDExchangeRate)
	}
	displayRate := decimal.Zero
	if cnyRate.GreaterThan(decimal.Zero) {
		rate := operation_setting.GetUsdToCurrencyRate(operation_setting.USDExchangeRate)
		if rate > 0 && !math.IsNaN(rate) && !math.IsInf(rate, 0) {
			displayRate = decimal.NewFromFloat(rate)
		}
	}
	return BenefitAmountDisplayContext{
		DisplayType:  general.QuotaDisplayType,
		QuotaPerUnit: quotaPerUnit,
		DisplayRate:  displayRate,
		CNYRate:      cnyRate,
	}
}

func (ctx BenefitAmountDisplayContext) validateType() error {
	switch ctx.DisplayType {
	case operation_setting.QuotaDisplayTypeUSD, operation_setting.QuotaDisplayTypeCNY, operation_setting.QuotaDisplayTypeCustom, operation_setting.QuotaDisplayTypeTokens:
		return nil
	default:
		return errors.New("额度展示类型无效")
	}
}

func (ctx BenefitAmountDisplayContext) DisplayAmountToQuota(amount decimal.Decimal) (int64, error) {
	if err := ctx.validateType(); err != nil {
		return 0, err
	}
	if amount.LessThanOrEqual(decimal.Zero) {
		return 0, errors.New("额度必须大于 0")
	}
	if ctx.DisplayType == operation_setting.QuotaDisplayTypeTokens {
		if !amount.IsInteger() {
			return 0, errors.New("Tokens 额度必须为整数")
		}
		return common.WalletQuotaFromDecimalStrict(amount)
	}
	if amount.Exponent() < -2 {
		return 0, errors.New("金额最多只能保留两位小数")
	}
	if ctx.DisplayRate.LessThanOrEqual(decimal.Zero) || ctx.QuotaPerUnit.LessThanOrEqual(decimal.Zero) {
		return 0, errors.New("额度展示配置无效")
	}
	return common.WalletQuotaFromDecimalStrict(amount.Div(ctx.DisplayRate).Mul(ctx.QuotaPerUnit))
}

func (ctx BenefitAmountDisplayContext) DisplayAmountToCNYCents(amount decimal.Decimal) (int64, error) {
	if err := ctx.validateType(); err != nil {
		return 0, err
	}
	if amount.LessThanOrEqual(decimal.Zero) {
		return 0, errors.New("金额必须大于 0")
	}
	if ctx.CNYRate.LessThanOrEqual(decimal.Zero) {
		return 0, errors.New("人民币兑美元汇率配置无效")
	}
	if ctx.DisplayType != operation_setting.QuotaDisplayTypeTokens && amount.Exponent() < -2 {
		return 0, errors.New("金额最多只能保留两位小数")
	}
	var divisor decimal.Decimal
	if ctx.DisplayType == operation_setting.QuotaDisplayTypeTokens {
		if !amount.IsInteger() {
			return 0, errors.New("Tokens 额度必须为整数")
		}
		if ctx.QuotaPerUnit.LessThanOrEqual(decimal.Zero) {
			return 0, errors.New("额度展示配置无效")
		}
		divisor = ctx.QuotaPerUnit
	} else {
		if ctx.DisplayRate.LessThanOrEqual(decimal.Zero) {
			return 0, errors.New("额度展示配置无效")
		}
		divisor = ctx.DisplayRate
	}
	// 先乘汇率再除展示单位，避免例如 0.01 CNY / 7.5 * 7.5 的有限精度误差。
	centsNumerator := amount.Mul(ctx.CNYRate).Mul(decimal.NewFromInt(100))
	if centsNumerator.GreaterThan(decimal.Zero) && centsNumerator.LessThan(divisor) {
		return 0, errors.New("换算后的金额不足 0.01 元")
	}
	centsValue := centsNumerator.Div(divisor)
	cents := centsValue.Round(0)
	return common.WalletQuotaFromDecimalStrict(cents)
}

func (ctx BenefitAmountDisplayContext) QuotaToDisplayAmount(quota int64) (decimal.Decimal, error) {
	if err := ctx.validateType(); err != nil {
		return decimal.Zero, err
	}
	if quota < 0 {
		return decimal.Zero, errors.New("额度不能为负数")
	}
	if quota == 0 {
		return decimal.Zero, nil
	}
	if ctx.DisplayType == operation_setting.QuotaDisplayTypeTokens {
		return decimal.NewFromInt(quota), nil
	}
	if ctx.DisplayRate.LessThanOrEqual(decimal.Zero) || ctx.QuotaPerUnit.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, errors.New("额度展示配置无效")
	}
	return decimal.NewFromInt(quota).Div(ctx.QuotaPerUnit).Mul(ctx.DisplayRate).Round(2), nil
}

func (ctx BenefitAmountDisplayContext) CNYCentsToDisplayAmount(cents int64) (decimal.Decimal, error) {
	if err := ctx.validateType(); err != nil {
		return decimal.Zero, err
	}
	if cents < 0 {
		return decimal.Zero, errors.New("金额不能为负数")
	}
	if cents == 0 {
		return decimal.Zero, nil
	}
	if ctx.CNYRate.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, errors.New("人民币兑美元汇率配置无效")
	}
	usd := decimal.NewFromInt(cents).Div(decimal.NewFromInt(100)).Div(ctx.CNYRate)
	if ctx.DisplayType == operation_setting.QuotaDisplayTypeTokens {
		if ctx.QuotaPerUnit.LessThanOrEqual(decimal.Zero) {
			return decimal.Zero, errors.New("额度展示配置无效")
		}
		return usd.Mul(ctx.QuotaPerUnit).Round(0), nil
	}
	if ctx.DisplayRate.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, errors.New("额度展示配置无效")
	}
	return usd.Mul(ctx.DisplayRate).Round(2), nil
}

func migrateBenefitActivityQuotaConfig(db *gorm.DB) error {
	if db == nil {
		return errors.New("数据库连接为空")
	}
	var activities []BenefitActivity
	if err := db.Unscoped().Where(
		"(amount_mode = ? AND (total_quota = 0 OR fixed_quota = 0 OR min_quota = 0 OR max_quota = 0)) OR "+
			"(amount_mode = ? AND (total_quota = 0 OR min_quota = 0 OR max_quota = 0)) OR "+
			"COALESCE(amount_display_type_snapshot, '') = '' OR COALESCE(amount_display_rate_snapshot, '') = '' OR COALESCE(quota_per_unit_snapshot, '') = ''",
		BenefitAmountModeFixed, BenefitAmountModeRandom,
	).Find(&activities).Error; err != nil {
		return err
	}
	if len(activities) == 0 {
		return nil
	}
	if common.QuotaPerUnit <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) || operation_setting.USDExchangeRate <= 0 || math.IsNaN(operation_setting.USDExchangeRate) || math.IsInf(operation_setting.USDExchangeRate, 0) {
		return errors.New("福利活动迁移快照配置无效")
	}
	legacyRate := decimal.NewFromFloat(operation_setting.USDExchangeRate).String()
	legacyQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit).String()
	for i := range activities {
		activity := &activities[i]
		var shares []BenefitActivityShare
		if err := db.Where("activity_id = ?", activity.Id).Find(&shares).Error; err != nil {
			return err
		}
		if len(shares) > 0 {
			minQuota, maxQuota := shares[0].Quota, shares[0].Quota
			for _, share := range shares[1:] {
				if share.Quota < minQuota {
					minQuota = share.Quota
				}
				if share.Quota > maxQuota {
					maxQuota = share.Quota
				}
			}
			activity.MinQuota = minQuota
			activity.MaxQuota = maxQuota
			if minQuota == maxQuota {
				if activity.AmountMode == BenefitAmountModeFixed {
					activity.FixedQuota = minQuota
				}
			}
			if activity.TotalQuota == 0 {
				for _, share := range shares {
					activity.TotalQuota += share.Quota
				}
			}
		} else {
			quota := activity.TotalQuota
			if quota == 0 && activity.TotalAmountCents > 0 {
				converted, err := BenefitAmountCNYToQuota(activity.TotalAmountCents)
				if err != nil {
					return err
				}
				quota = converted
				activity.TotalQuota = converted
			}
			if activity.TotalCount > 0 && quota > 0 {
				if activity.AmountMode == BenefitAmountModeRandom && activity.MinAmountCents > 0 && activity.MaxAmountCents > 0 {
					minimum, err := BenefitAmountCNYToQuota(activity.MinAmountCents)
					if err != nil {
						return err
					}
					maximum, err := BenefitAmountCNYToQuota(activity.MaxAmountCents)
					if err != nil {
						return err
					}
					activity.MinQuota = minimum
					activity.MaxQuota = maximum
				} else if activity.AmountMode == BenefitAmountModeFixed {
					if quota%int64(activity.TotalCount) != 0 {
						return errors.New("福利活动旧草稿额度无法无损迁移")
					}
					activity.FixedQuota = quota / int64(activity.TotalCount)
					activity.MinQuota = activity.FixedQuota
					activity.MaxQuota = activity.FixedQuota
				}
			}
		}
		if activity.AmountMode == BenefitAmountModeFixed && activity.FixedQuota > 0 {
			activity.MinQuota = activity.FixedQuota
			activity.MaxQuota = activity.FixedQuota
		}
		if activity.TotalQuota <= 0 || activity.MinQuota <= 0 || activity.MaxQuota <= 0 || (activity.AmountMode == BenefitAmountModeFixed && activity.FixedQuota <= 0) {
			return errors.New("福利活动额度配置迁移失败")
		}
		if activity.AmountDisplayTypeSnapshot == "" {
			activity.AmountDisplayTypeSnapshot = operation_setting.QuotaDisplayTypeCNY
		}
		if activity.AmountDisplayRateSnapshot == "" {
			activity.AmountDisplayRateSnapshot = legacyRate
		}
		if activity.QuotaPerUnitSnapshot == "" {
			activity.QuotaPerUnitSnapshot = legacyQuotaPerUnit
		}
		if err := db.Unscoped().Model(activity).Updates(map[string]interface{}{
			"total_quota": activity.TotalQuota, "fixed_quota": activity.FixedQuota,
			"min_quota": activity.MinQuota, "max_quota": activity.MaxQuota,
			"amount_display_type_snapshot": activity.AmountDisplayTypeSnapshot,
			"amount_display_rate_snapshot": activity.AmountDisplayRateSnapshot,
			"quota_per_unit_snapshot":      activity.QuotaPerUnitSnapshot,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}
