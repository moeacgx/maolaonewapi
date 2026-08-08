package ratio_setting

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
)

type ModelRoutePriceVariantMap map[string]map[string]ModelPriceVariantConfig

var modelRoutePriceVariantMap = types.NewRWMap[string, map[string]ModelPriceVariantConfig]()

func normalizeModelPriceRoute(route string) string {
	route = strings.ToLower(strings.Trim(strings.TrimSpace(route), "/"))
	route = strings.ReplaceAll(route, "_", ".")
	route = strings.ReplaceAll(route, "/", ".")
	switch route {
	case "edit", "edits", "image.edits", "images.edits", "v1.images.edits", "canvas.v1.images.edits":
		return ModelPriceRouteImageEdit
	default:
		return route
	}
}

func parseModelRoutePriceVariants(jsonStr string) (ModelRoutePriceVariantMap, error) {
	configs := make(ModelRoutePriceVariantMap)
	if err := common.UnmarshalJsonStr(jsonStr, &configs); err != nil {
		return nil, err
	}
	if configs == nil {
		return nil, fmt.Errorf("model route price variants must be a JSON object")
	}
	normalized := make(ModelRoutePriceVariantMap, len(configs))
	for modelName, routeConfigs := range configs {
		trimmedName := strings.TrimSpace(modelName)
		if trimmedName == "" {
			return nil, fmt.Errorf("model route price variant model name cannot be empty")
		}
		if routeConfigs == nil {
			return nil, fmt.Errorf("model %s route price variants must be a JSON object", trimmedName)
		}
		normalizedRoutes := make(map[string]ModelPriceVariantConfig, len(routeConfigs))
		for route, config := range routeConfigs {
			normalizedRoute := normalizeModelPriceRoute(route)
			if normalizedRoute == "" {
				return nil, fmt.Errorf("model %s route price variant route cannot be empty", trimmedName)
			}
			if config.Inherited {
				continue
			}
			normalizedConfig, err := normalizeModelPriceVariantConfig(trimmedName+" "+normalizedRoute, config)
			if err != nil {
				return nil, err
			}
			normalizedRoutes[normalizedRoute] = normalizedConfig
		}
		if len(normalizedRoutes) > 0 {
			normalized[trimmedName] = normalizedRoutes
		}
	}
	return normalized, nil
}

func CheckModelRoutePriceVariantsJSONString(jsonStr string) error {
	_, err := parseModelRoutePriceVariants(jsonStr)
	return err
}

func UpdateModelRoutePriceVariantsByJSONString(jsonStr string) error {
	configs, err := parseModelRoutePriceVariants(jsonStr)
	if err != nil {
		return err
	}
	normalized, err := common.Marshal(configs)
	if err != nil {
		return err
	}
	return types.LoadFromJsonStringWithCallback(modelRoutePriceVariantMap, string(normalized), InvalidateExposedDataCache)
}

func GetModelRoutePriceVariantConfig(modelName string, route string) (ModelPriceVariantConfig, bool) {
	modelName = FormatMatchingModelName(modelName)
	route = normalizeModelPriceRoute(route)
	if modelName == "" || route == "" {
		return ModelPriceVariantConfig{}, false
	}
	routes, ok := modelRoutePriceVariantMap.Get(modelName)
	if !ok {
		return ModelPriceVariantConfig{}, false
	}
	config, ok := routes[route]
	return cloneModelPriceVariantConfig(config), ok
}

func GetModelRoutePriceVariantsCopy() ModelRoutePriceVariantMap {
	configs := make(ModelRoutePriceVariantMap)
	for modelName, routeConfigs := range modelRoutePriceVariantMap.ReadAll() {
		if len(routeConfigs) == 0 {
			continue
		}
		clonedRoutes := make(map[string]ModelPriceVariantConfig, len(routeConfigs))
		for route, config := range routeConfigs {
			clonedRoutes[route] = cloneModelPriceVariantConfig(config)
		}
		configs[modelName] = clonedRoutes
	}
	return configs
}

func ModelRoutePriceVariants2JSONString() string {
	data, err := common.Marshal(GetModelRoutePriceVariantsCopy())
	if err != nil {
		return "{}"
	}
	return string(data)
}

func MatchModelRoutePriceVariant(modelName string, route string, dimensions map[string]string) ModelPriceVariantMatch {
	config, configured := GetModelRoutePriceVariantConfig(modelName, route)
	result := ModelPriceVariantMatch{Configured: configured}
	if !configured {
		return result
	}

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
