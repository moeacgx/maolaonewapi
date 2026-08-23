package dynamic_routing_setting

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/config"
)

var dynamicRoutingSetting = dto.DynamicRoutingConfig{
	Enabled: false,
	Rules:   []dto.DynamicRoutingRule{},
}

func init() {
	config.GlobalConfig.Register("dynamic_routing", &dynamicRoutingSetting)
}

// GetSettings returns a shallow immutable snapshot for the relay hot path.
func GetSettings() dto.DynamicRoutingConfig {
	settings := dynamicRoutingSetting
	settings.Rules = append([]dto.DynamicRoutingRule(nil), dynamicRoutingSetting.Rules...)
	return settings
}

func ValidateRulesJSON(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed[0] != '[' {
		return fmt.Errorf("dynamic_routing.rules must be a JSON array")
	}
	var rules []dto.DynamicRoutingRule
	if err := common.UnmarshalJsonStr(raw, &rules); err != nil {
		return err
	}
	return dto.ValidateDynamicRoutingRules(rules)
}
