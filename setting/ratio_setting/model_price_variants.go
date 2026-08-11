package ratio_setting

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
)

const (
	ModelPriceVariantResolution     = "resolution"
	ModelPriceVariantQuality        = "quality"
	ModelPriceExtraParamInputImages = "input_images"
	ModelPriceRouteImageEdit        = "image.edit"
)

// ModelPriceVariantRule matches a request specification to a final fixed unit price.
// Price uses the same unit as ModelPriceUnit: request means per request, second means per second.
type ModelPriceVariantRule struct {
	Resolution string   `json:"resolution,omitempty"`
	Quality    string   `json:"quality,omitempty"`
	Price      *float64 `json:"price"`
}

// ModelPriceExtraParamRule adds a per-unit surcharge for an additional request parameter.
// Base is the quantity included in the matched base price.
type ModelPriceExtraParamRule struct {
	Key       string   `json:"key"`
	Base      float64  `json:"base,omitempty"`
	UnitPrice *float64 `json:"unit_price"`
}

// ModelPriceVariantConfig declares whether a fixed-price model varies by request specification.
// Even with both switches disabled, the config can disable built-in hidden variant ratios.
type ModelPriceVariantConfig struct {
	ResolutionEnabled bool                       `json:"resolution_enabled"`
	QualityEnabled    bool                       `json:"quality_enabled"`
	Rules             []ModelPriceVariantRule    `json:"rules,omitempty"`
	ExtraParams       []ModelPriceExtraParamRule `json:"extra_params,omitempty"`
	Inherited         bool                       `json:"inherited,omitempty"`
}

type ModelPriceVariantMatch struct {
	Configured bool
	Matched    bool
	Price      float64
	Resolution string
	Quality    string
}

type ModelPriceExtraParamCharge struct {
	Key        string
	Value      float64
	Base       float64
	UnitPrice  float64
	ExtraUnits float64
	Price      float64
}

type defaultVariantRatios struct {
	Resolution map[string]float64
}

// Built-in configs only store relative ratios. Display prices are derived from the
// administrator's current ModelPrice so upgrades keep local custom base prices.
var defaultModelPriceVariantRatios = map[string]defaultVariantRatios{
	"grok-imagine-video": {
		Resolution: map[string]float64{
			"480p": 1,
			"720p": 0.07 / 0.05,
		},
	},
	"grok-imagine-video-1.5": {
		Resolution: map[string]float64{
			"480p":  1,
			"720p":  0.14 / 0.08,
			"1080p": 0.25 / 0.08,
		},
	},
}

// Only administrator overrides are stored here. Built-ins are merged by getters.
var modelPriceVariantOverrideMap = types.NewRWMap[string, ModelPriceVariantConfig]()

func normalizeVariantResolution(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "480", "480p", "sd":
		return "480p"
	case "720", "720p", "hd":
		return "720p"
	case "1080", "1080p", "fhd", "full-hd", "full_hd":
		return "1080p"
	case "2k":
		return "2k"
	case "4k":
		return "4k"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func normalizeVariantQuality(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeExtraParamKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeModelPriceVariantConfig(modelName string, config ModelPriceVariantConfig) (ModelPriceVariantConfig, error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return ModelPriceVariantConfig{}, fmt.Errorf("model price variant model name cannot be empty")
	}
	extraParams, err := normalizeModelPriceExtraParamRules(modelName, config.ExtraParams)
	if err != nil {
		return ModelPriceVariantConfig{}, err
	}
	config.ExtraParams = extraParams

	if !config.ResolutionEnabled && !config.QualityEnabled {
		config.Rules = nil
		return config, nil
	}
	if len(config.Rules) == 0 {
		return ModelPriceVariantConfig{}, fmt.Errorf("model %s enables variant pricing but has no rules", modelName)
	}

	normalizedRules := make([]ModelPriceVariantRule, 0, len(config.Rules))
	seen := make(map[string]struct{}, len(config.Rules))
	for index, rule := range config.Rules {
		if rule.Price == nil || math.IsNaN(*rule.Price) || math.IsInf(*rule.Price, 0) || *rule.Price < 0 {
			return ModelPriceVariantConfig{}, fmt.Errorf("model %s rule %d has invalid price", modelName, index+1)
		}
		if config.ResolutionEnabled {
			rule.Resolution = normalizeVariantResolution(rule.Resolution)
			if rule.Resolution == "" {
				return ModelPriceVariantConfig{}, fmt.Errorf("model %s rule %d misses resolution", modelName, index+1)
			}
		} else {
			rule.Resolution = ""
		}
		if config.QualityEnabled {
			rule.Quality = normalizeVariantQuality(rule.Quality)
			if rule.Quality == "" {
				return ModelPriceVariantConfig{}, fmt.Errorf("model %s rule %d misses quality", modelName, index+1)
			}
		} else {
			rule.Quality = ""
		}

		key := rule.Resolution + "\x00" + rule.Quality
		if _, exists := seen[key]; exists {
			return ModelPriceVariantConfig{}, fmt.Errorf("model %s has duplicate variant rule %s/%s", modelName, rule.Resolution, rule.Quality)
		}
		seen[key] = struct{}{}
		normalizedRules = append(normalizedRules, rule)
	}
	config.Rules = normalizedRules
	return config, nil
}

func normalizeModelPriceExtraParamRules(modelName string, rules []ModelPriceExtraParamRule) ([]ModelPriceExtraParamRule, error) {
	if len(rules) == 0 {
		return nil, nil
	}
	normalized := make([]ModelPriceExtraParamRule, 0, len(rules))
	seen := make(map[string]struct{}, len(rules))
	for index, rule := range rules {
		rule.Key = normalizeExtraParamKey(rule.Key)
		if rule.Key == "" {
			return nil, fmt.Errorf("model %s extra param rule %d misses key", modelName, index+1)
		}
		if _, exists := seen[rule.Key]; exists {
			return nil, fmt.Errorf("model %s has duplicate extra param rule %s", modelName, rule.Key)
		}
		seen[rule.Key] = struct{}{}
		if math.IsNaN(rule.Base) || math.IsInf(rule.Base, 0) || rule.Base < 0 {
			return nil, fmt.Errorf("model %s extra param rule %d has invalid base", modelName, index+1)
		}
		if rule.UnitPrice == nil || math.IsNaN(*rule.UnitPrice) || math.IsInf(*rule.UnitPrice, 0) || *rule.UnitPrice < 0 {
			return nil, fmt.Errorf("model %s extra param rule %d has invalid unit_price", modelName, index+1)
		}
		unitPrice := *rule.UnitPrice
		rule.UnitPrice = &unitPrice
		normalized = append(normalized, rule)
	}
	return normalized, nil
}

func parseModelPriceVariants(jsonStr string) (map[string]ModelPriceVariantConfig, error) {
	configs := make(map[string]ModelPriceVariantConfig)
	if err := common.UnmarshalJsonStr(jsonStr, &configs); err != nil {
		return nil, err
	}
	if configs == nil {
		return nil, fmt.Errorf("model price variants must be a JSON object")
	}
	normalized := make(map[string]ModelPriceVariantConfig, len(configs))
	for modelName, config := range configs {
		trimmedName := strings.TrimSpace(modelName)
		if config.Inherited {
			continue
		}
		normalizedConfig, err := normalizeModelPriceVariantConfig(trimmedName, config)
		if err != nil {
			return nil, err
		}
		normalized[trimmedName] = normalizedConfig
	}
	return normalized, nil
}

func CheckModelPriceVariantsJSONString(jsonStr string) error {
	_, err := parseModelPriceVariants(jsonStr)
	return err
}

func UpdateModelPriceVariantsByJSONString(jsonStr string) error {
	configs, err := parseModelPriceVariants(jsonStr)
	if err != nil {
		return err
	}
	normalized, err := common.Marshal(configs)
	if err != nil {
		return err
	}
	return types.LoadFromJsonStringWithCallback(modelPriceVariantOverrideMap, string(normalized), InvalidateExposedDataCache)
}

func cloneModelPriceVariantConfig(config ModelPriceVariantConfig) ModelPriceVariantConfig {
	cloned := config
	cloned.Rules = make([]ModelPriceVariantRule, len(config.Rules))
	for index, rule := range config.Rules {
		cloned.Rules[index] = rule
		if rule.Price != nil {
			price := *rule.Price
			cloned.Rules[index].Price = &price
		}
	}
	cloned.ExtraParams = make([]ModelPriceExtraParamRule, len(config.ExtraParams))
	for index, rule := range config.ExtraParams {
		cloned.ExtraParams[index] = rule
		if rule.UnitPrice != nil {
			unitPrice := *rule.UnitPrice
			cloned.ExtraParams[index].UnitPrice = &unitPrice
		}
	}
	return cloned
}

func buildDefaultModelPriceVariantConfig(modelName string) (ModelPriceVariantConfig, bool) {
	defaults, ok := defaultModelPriceVariantRatios[modelName]
	if !ok {
		return ModelPriceVariantConfig{}, false
	}
	basePrice, ok := GetModelPrice(modelName, false)
	if !ok || basePrice < 0 {
		basePrice = defaultModelPrice[modelName]
	}
	resolutions := make([]string, 0, len(defaults.Resolution))
	for resolution := range defaults.Resolution {
		resolutions = append(resolutions, resolution)
	}
	resolutionOrder := map[string]int{"480p": 1, "720p": 2, "1080p": 3, "2k": 4, "4k": 5}
	sort.Slice(resolutions, func(i, j int) bool {
		left, leftKnown := resolutionOrder[resolutions[i]]
		right, rightKnown := resolutionOrder[resolutions[j]]
		if leftKnown && rightKnown {
			return left < right
		}
		if leftKnown != rightKnown {
			return leftKnown
		}
		return resolutions[i] < resolutions[j]
	})
	rules := make([]ModelPriceVariantRule, 0, len(resolutions))
	for _, resolution := range resolutions {
		price := basePrice * defaults.Resolution[resolution]
		rules = append(rules, ModelPriceVariantRule{
			Resolution: resolution,
			Price:      &price,
		})
	}
	return ModelPriceVariantConfig{ResolutionEnabled: true, Rules: rules, Inherited: true}, true
}

func GetModelPriceVariantConfig(modelName string) (ModelPriceVariantConfig, bool) {
	modelName = FormatMatchingModelName(modelName)
	if config, ok := modelPriceVariantOverrideMap.Get(modelName); ok {
		return cloneModelPriceVariantConfig(config), true
	}
	config, ok := buildDefaultModelPriceVariantConfig(modelName)
	return cloneModelPriceVariantConfig(config), ok
}

func GetModelPriceVariantsCopy() map[string]ModelPriceVariantConfig {
	configs := make(map[string]ModelPriceVariantConfig)
	for modelName := range defaultModelPriceVariantRatios {
		if config, ok := buildDefaultModelPriceVariantConfig(modelName); ok {
			configs[modelName] = config
		}
	}
	for modelName, config := range modelPriceVariantOverrideMap.ReadAll() {
		configs[modelName] = cloneModelPriceVariantConfig(config)
	}
	return configs
}

func ModelPriceVariants2JSONString() string {
	data, err := common.Marshal(GetModelPriceVariantsCopy())
	if err != nil {
		return "{}"
	}
	return string(data)
}

func MatchModelPriceVariant(modelName string, dimensions map[string]string) ModelPriceVariantMatch {
	config, configured := GetModelPriceVariantConfig(modelName)
	result := ModelPriceVariantMatch{Configured: configured}
	if !configured {
		return result
	}

	return matchModelPriceVariantConfig(config, dimensions, result)
}

func matchModelPriceVariantConfig(config ModelPriceVariantConfig, dimensions map[string]string, result ModelPriceVariantMatch) ModelPriceVariantMatch {
	result.Resolution = normalizeVariantResolution(dimensions[ModelPriceVariantResolution])
	result.Quality = normalizeVariantQuality(dimensions[ModelPriceVariantQuality])
	if !config.ResolutionEnabled && !config.QualityEnabled {
		return result
	}
	if config.ResolutionEnabled && result.Resolution == "" {
		return result
	}
	if config.QualityEnabled && result.Quality == "" {
		return result
	}
	for _, rule := range config.Rules {
		if config.ResolutionEnabled && rule.Resolution != result.Resolution {
			continue
		}
		if config.QualityEnabled && rule.Quality != result.Quality {
			continue
		}
		result.Matched = true
		result.Price = *rule.Price
		return result
	}
	return result
}

func MatchModelPriceVariantConfig(config ModelPriceVariantConfig, dimensions map[string]string) ModelPriceVariantMatch {
	return matchModelPriceVariantConfig(config, dimensions, ModelPriceVariantMatch{Configured: true})
}

func CalculateModelPriceExtraParamCharges(config ModelPriceVariantConfig, params map[string]float64) []ModelPriceExtraParamCharge {
	if len(config.ExtraParams) == 0 || len(params) == 0 {
		return nil
	}
	normalizedParams := make(map[string]float64, len(params))
	for key, value := range params {
		normalizedKey := normalizeExtraParamKey(key)
		if normalizedKey == "" || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			continue
		}
		normalizedParams[normalizedKey] = value
	}
	if len(normalizedParams) == 0 {
		return nil
	}
	charges := make([]ModelPriceExtraParamCharge, 0, len(config.ExtraParams))
	for _, rule := range config.ExtraParams {
		if rule.UnitPrice == nil {
			continue
		}
		value, ok := normalizedParams[rule.Key]
		if !ok {
			continue
		}
		extraUnits := value - rule.Base
		if extraUnits < 0 {
			extraUnits = 0
		}
		unitPrice := *rule.UnitPrice
		charges = append(charges, ModelPriceExtraParamCharge{
			Key:        rule.Key,
			Value:      value,
			Base:       rule.Base,
			UnitPrice:  unitPrice,
			ExtraUnits: extraUnits,
			Price:      extraUnits * unitPrice,
		})
	}
	return charges
}
