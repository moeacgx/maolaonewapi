package types

import (
	"fmt"
	"math"

	"github.com/shopspring/decimal"
)

type GroupRatioInfo struct {
	GroupRatio        float64
	GroupSpecialRatio float64
	HasSpecialRatio   bool
}

type ModelPriceUnit string

const (
	ModelPriceUnitRequest ModelPriceUnit = "request"
	ModelPriceUnitSecond  ModelPriceUnit = "second"
)

type PriceData struct {
	FreeModel            bool
	ModelPrice           float64
	ModelPriceUnit       ModelPriceUnit
	ModelRatio           float64
	CompletionRatio      float64
	CacheRatio           float64
	CacheCreationRatio   float64
	CacheCreation5mRatio float64
	CacheCreation1hRatio float64
	ImageRatio           float64
	AudioRatio           float64
	AudioCompletionRatio float64
	otherRatios          map[string]float64
	BillingMeta          map[string]string
	UsePrice             bool
	Quota                int // 固定单价任务的最终额度（MJ / Task，可按请求或按秒）
	QuotaToPreConsume    int // 按量计费的预消耗额度
	GroupRatioInfo       GroupRatioInfo
}

func (p *PriceData) AddOtherRatio(key string, ratio float64) {
	if key == "" || !isValidOtherRatio(ratio) {
		return
	}
	if p.otherRatios == nil {
		p.otherRatios = make(map[string]float64)
	}
	p.otherRatios[key] = ratio
}

func (p *PriceData) ReplaceOtherRatios(ratios map[string]float64) bool {
	p.otherRatios = nil
	for key, ratio := range ratios {
		p.AddOtherRatio(key, ratio)
	}
	return len(p.otherRatios) > 0
}

func (p *PriceData) RemoveOtherRatio(key string) {
	delete(p.otherRatios, key)
}

func (p *PriceData) HasOtherRatio(key string) bool {
	ratio, ok := p.otherRatios[key]
	return ok && isValidOtherRatio(ratio)
}

func (p *PriceData) GetOtherRatio(key string) (float64, bool) {
	ratio, ok := p.otherRatios[key]
	if !ok || !isValidOtherRatio(ratio) {
		return 0, false
	}
	return ratio, true
}

func (p *PriceData) OtherRatios() map[string]float64 {
	if len(p.otherRatios) == 0 {
		return nil
	}
	ratios := make(map[string]float64, len(p.otherRatios))
	for key, ratio := range p.otherRatios {
		if isValidOtherRatio(ratio) {
			ratios[key] = ratio
		}
	}
	if len(ratios) == 0 {
		return nil
	}
	return ratios
}

func (p *PriceData) OtherRatioMultiplier() float64 {
	multiplier := 1.0
	for _, ratio := range p.otherRatios {
		if isValidOtherRatio(ratio) && ratio != 1 {
			multiplier *= ratio
		}
	}
	return multiplier
}

func (p *PriceData) AddBillingMeta(key string, value string) {
	if key == "" || value == "" {
		return
	}
	if p.BillingMeta == nil {
		p.BillingMeta = make(map[string]string)
	}
	p.BillingMeta[key] = value
}

func (p *PriceData) ApplyOtherRatiosToFloat(value float64) float64 {
	return value * p.OtherRatioMultiplier()
}

func (p *PriceData) ApplyOtherRatiosToDecimal(value decimal.Decimal) decimal.Decimal {
	for _, ratio := range p.otherRatios {
		if isValidOtherRatio(ratio) && ratio != 1 {
			value = value.Mul(decimal.NewFromFloat(ratio))
		}
	}
	return value
}

func (p *PriceData) RemoveOtherRatiosFromFloat(value float64) float64 {
	for _, ratio := range p.otherRatios {
		if isValidOtherRatio(ratio) && ratio != 1 {
			value /= ratio
		}
	}
	return value
}

// ShouldApplyTaskRatio 判断任务附加倍率是否参与固定价格计算。
// 按次固定价不会乘视频秒数；按秒固定价与倍率计费保持原有行为。
func (p *PriceData) ShouldApplyTaskRatio(key string) bool {
	if key != "seconds" || !p.UsePrice {
		return true
	}
	return p.ModelPriceUnit == ModelPriceUnitSecond
}

func (p *PriceData) ApplyTaskRatiosToFloat(value float64) float64 {
	for key, ratio := range p.otherRatios {
		if !p.ShouldApplyTaskRatio(key) {
			continue
		}
		if isValidOtherRatio(ratio) && ratio != 1 {
			value *= ratio
		}
	}
	return value
}

func (p *PriceData) RemoveTaskRatiosFromFloat(value float64) float64 {
	for key, ratio := range p.otherRatios {
		if !p.ShouldApplyTaskRatio(key) {
			continue
		}
		if isValidOtherRatio(ratio) && ratio != 1 {
			value /= ratio
		}
	}
	return value
}

func isValidOtherRatio(ratio float64) bool {
	return ratio > 0 && !math.IsInf(ratio, 1) && !math.IsNaN(ratio)
}

func (p *PriceData) ToSetting() string {
	return fmt.Sprintf("ModelPrice: %f, ModelPriceUnit: %s, ModelRatio: %f, CompletionRatio: %f, CacheRatio: %f, GroupRatio: %f, UsePrice: %t, CacheCreationRatio: %f, CacheCreation5mRatio: %f, CacheCreation1hRatio: %f, QuotaToPreConsume: %d, ImageRatio: %f, AudioRatio: %f, AudioCompletionRatio: %f", p.ModelPrice, p.ModelPriceUnit, p.ModelRatio, p.CompletionRatio, p.CacheRatio, p.GroupRatioInfo.GroupRatio, p.UsePrice, p.CacheCreationRatio, p.CacheCreation5mRatio, p.CacheCreation1hRatio, p.QuotaToPreConsume, p.ImageRatio, p.AudioRatio, p.AudioCompletionRatio)
}
