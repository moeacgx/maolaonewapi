package dto

import (
	"fmt"
	"strings"
)

const (
	DynamicRoutingActionModelRedirect = "model_redirect"
	// DynamicRoutingActionResponsesImageToolBridge routes a forced Responses
	// image_generation tool call to an administrator-selected image model and
	// target endpoint. Unlike model_redirect it may change the request shape and
	// response handling.
	DynamicRoutingActionResponsesImageToolBridge = "responses_image_tool_bridge"
	// DynamicRoutingActionResponsesImageFunctionBridge injects a private
	// function into a Responses request. When the source model calls that
	// function, the relay performs a second Images API request.
	DynamicRoutingActionResponsesImageFunctionBridge = "responses_image_function_bridge"

	DynamicRoutingConditionReasoningEffort = "reasoning_effort"
	DynamicRoutingConditionRequestPrefix   = "request."

	// DynamicRoutingResponsesPath keeps the Responses request shape and sends it
	// to the target model's Responses endpoint.
	DynamicRoutingResponsesPath = "/v1/responses"
	// DynamicRoutingImageGenerationPath is the canonical OpenAI image operation.
	DynamicRoutingImageGenerationPath = "/v1/images/generations"

	DynamicRoutingOperatorEquals    = "equals"
	DynamicRoutingOperatorNotEquals = "not_equals"
	DynamicRoutingOperatorExists    = "exists"
	DynamicRoutingOperatorNotExists = "not_exists"

	MaxDynamicRoutingRules           = 100
	MaxDynamicRoutingConditions      = 8
	MaxDynamicRoutingPriority        = 1000
	MaxDynamicRoutingStringLength    = 256
	MaxDynamicRoutingRequestPathSize = 256
)

// DynamicRoutingConfig is the global dynamic routing configuration. Rules are
// evaluated only when Enabled is true, unless a channel explicitly enables its
// own override configuration.
type DynamicRoutingConfig struct {
	Enabled bool                 `json:"enabled"`
	Rules   []DynamicRoutingRule `json:"rules"`
}

// DynamicRoutingChannelConfig optionally overrides the global configuration for
// one channel. A nil Enabled inherits the global master switch. An explicit
// false disables dynamic routing on this channel, while an explicit true enables
// its rules even when the global switch is off.
type DynamicRoutingChannelConfig struct {
	Enabled *bool                `json:"enabled,omitempty"`
	Rules   []DynamicRoutingRule `json:"rules,omitempty"`
}

// DynamicRoutingRule matches a public model and request context, then applies
// the configured action. model_redirect only changes the final upstream model;
// responses_image_tool_bridge has its own request, response, and billing path.
type DynamicRoutingRule struct {
	ID           string                    `json:"id"`
	Enabled      bool                      `json:"enabled"`
	Action       string                    `json:"action,omitempty"`
	SourceModel  string                    `json:"source_model"`
	TargetModel  string                    `json:"target_model"`
	TargetPath   string                    `json:"target_path,omitempty"`
	SourceGroups []string                  `json:"source_groups,omitempty"`
	TargetGroup  string                    `json:"target_group,omitempty"`
	ChannelTypes []int                     `json:"channel_types,omitempty"`
	RequestPaths []string                  `json:"request_paths,omitempty"`
	Conditions   []DynamicRoutingCondition `json:"conditions,omitempty"`
	Priority     int                       `json:"priority,omitempty"`
}

// DynamicRoutingCondition is an AND condition within a rule. Field supports
// the normalized reasoning_effort and simple request.<json.path> lookups.
type DynamicRoutingCondition struct {
	Field    string `json:"field"`
	Operator string `json:"operator,omitempty"`
	Value    string `json:"value,omitempty"`
}

func (c *DynamicRoutingChannelConfig) Validate() error {
	if c == nil {
		return nil
	}
	return ValidateDynamicRoutingRules(c.Rules)
}

// ValidateDynamicRoutingRules rejects malformed or unsupported routes before
// they are persisted. Runtime matching still fails closed for stale configs.
func ValidateDynamicRoutingRules(rules []DynamicRoutingRule) error {
	if len(rules) > MaxDynamicRoutingRules {
		return fmt.Errorf("dynamic_routing.rules exceeds maximum of %d", MaxDynamicRoutingRules)
	}

	ids := make(map[string]struct{}, len(rules))
	for index, rule := range rules {
		if rule.Priority < -MaxDynamicRoutingPriority || rule.Priority > MaxDynamicRoutingPriority {
			return fmt.Errorf("dynamic_routing.rules[%d].priority must be between -%d and %d", index, MaxDynamicRoutingPriority, MaxDynamicRoutingPriority)
		}
		if len(rule.Conditions) > MaxDynamicRoutingConditions {
			return fmt.Errorf("dynamic_routing.rules[%d].conditions exceeds maximum of %d", index, MaxDynamicRoutingConditions)
		}
		if err := validateDynamicRoutingScope(index, rule); err != nil {
			return err
		}
		if err := validateDynamicRoutingConditions(index, rule.Conditions); err != nil {
			return err
		}
		if !rule.Enabled {
			continue
		}

		if strings.TrimSpace(rule.ID) == "" {
			return fmt.Errorf("dynamic_routing.rules[%d].id is required when enabled", index)
		}
		if len(rule.ID) > MaxDynamicRoutingStringLength {
			return fmt.Errorf("dynamic_routing.rules[%d].id is too long", index)
		}
		if _, exists := ids[rule.ID]; exists {
			return fmt.Errorf("dynamic_routing.rules[%d].id duplicates another enabled rule", index)
		}
		ids[rule.ID] = struct{}{}

		action := normalizeDynamicRoutingAction(rule.Action)
		if action != DynamicRoutingActionModelRedirect &&
			action != DynamicRoutingActionResponsesImageToolBridge &&
			action != DynamicRoutingActionResponsesImageFunctionBridge {
			return fmt.Errorf("dynamic_routing.rules[%d].action is not supported: %s", index, rule.Action)
		}
		if action == DynamicRoutingActionResponsesImageToolBridge || action == DynamicRoutingActionResponsesImageFunctionBridge {
			if len(rule.RequestPaths) != 1 || strings.TrimSpace(rule.RequestPaths[0]) != "/v1/responses" {
				return fmt.Errorf("dynamic_routing.rules[%d].request_paths must be exactly /v1/responses for %s", index, action)
			}
			if targetPath := EffectiveDynamicRoutingTargetPath(rule); !IsSupportedDynamicRoutingImageTargetPath(targetPath) {
				return fmt.Errorf("dynamic_routing.rules[%d].target_path is unsupported for %s", index, action)
			}
			if action == DynamicRoutingActionResponsesImageFunctionBridge &&
				EffectiveDynamicRoutingTargetPath(rule) != DynamicRoutingImageGenerationPath {
				return fmt.Errorf("dynamic_routing.rules[%d].target_path must be /v1/images/generations for %s", index, action)
			}
		}
		if strings.TrimSpace(rule.SourceModel) == "" {
			return fmt.Errorf("dynamic_routing.rules[%d].source_model is required when enabled", index)
		}
		if strings.TrimSpace(rule.TargetModel) == "" {
			return fmt.Errorf("dynamic_routing.rules[%d].target_model is required when enabled", index)
		}
		if len(rule.SourceModel) > MaxDynamicRoutingStringLength || len(rule.TargetModel) > MaxDynamicRoutingStringLength {
			return fmt.Errorf("dynamic_routing.rules[%d] model name is too long", index)
		}
		if len(rule.TargetGroup) > MaxDynamicRoutingStringLength {
			return fmt.Errorf("dynamic_routing.rules[%d].target_group is too long", index)
		}
		if strings.EqualFold(strings.TrimSpace(rule.TargetGroup), "auto") || strings.Contains(rule.TargetGroup, ",") {
			return fmt.Errorf("dynamic_routing.rules[%d].target_group must be one group code", index)
		}
	}

	return nil
}

// EffectiveDynamicRoutingTargetPath supplies the only supported image bridge
// destination for legacy rules that predate the target_path field.
func EffectiveDynamicRoutingTargetPath(rule DynamicRoutingRule) string {
	action := normalizeDynamicRoutingAction(rule.Action)
	if action != DynamicRoutingActionResponsesImageToolBridge && action != DynamicRoutingActionResponsesImageFunctionBridge {
		return strings.TrimSpace(rule.TargetPath)
	}
	if path := strings.TrimSpace(rule.TargetPath); path != "" {
		return path
	}
	return DynamicRoutingImageGenerationPath
}

// IsSupportedDynamicRoutingImageTargetPath limits bridge execution to request
// shapes implemented by the relay. Bare /v1/images/ is intentionally rejected
// because it is not a registered executable endpoint.
func IsSupportedDynamicRoutingImageTargetPath(path string) bool {
	switch strings.TrimSpace(path) {
	case DynamicRoutingResponsesPath, DynamicRoutingImageGenerationPath:
		return true
	default:
		return false
	}
}

func validateDynamicRoutingScope(index int, rule DynamicRoutingRule) error {
	sourceGroups := make(map[string]struct{}, len(rule.SourceGroups))
	for _, sourceGroup := range rule.SourceGroups {
		group := strings.TrimSpace(sourceGroup)
		if group == "" || len(group) > MaxDynamicRoutingStringLength || strings.EqualFold(group, "auto") || strings.Contains(group, ",") {
			return fmt.Errorf("dynamic_routing.rules[%d].source_groups contains an invalid group code", index)
		}
		if _, exists := sourceGroups[group]; exists {
			return fmt.Errorf("dynamic_routing.rules[%d].source_groups contains duplicates", index)
		}
		sourceGroups[group] = struct{}{}
	}

	channelTypes := make(map[int]struct{}, len(rule.ChannelTypes))
	for _, channelType := range rule.ChannelTypes {
		if channelType <= 0 {
			return fmt.Errorf("dynamic_routing.rules[%d].channel_types must contain positive channel type ids", index)
		}
		if _, exists := channelTypes[channelType]; exists {
			return fmt.Errorf("dynamic_routing.rules[%d].channel_types contains duplicates", index)
		}
		channelTypes[channelType] = struct{}{}
	}

	requestPaths := make(map[string]struct{}, len(rule.RequestPaths))
	for _, requestPath := range rule.RequestPaths {
		path := strings.TrimSpace(requestPath)
		if path == "" || !strings.HasPrefix(path, "/") || strings.Contains(path, "?") || len(path) > MaxDynamicRoutingRequestPathSize {
			return fmt.Errorf("dynamic_routing.rules[%d].request_paths contains an invalid path", index)
		}
		if _, exists := requestPaths[path]; exists {
			return fmt.Errorf("dynamic_routing.rules[%d].request_paths contains duplicates", index)
		}
		requestPaths[path] = struct{}{}
	}
	return nil
}

func validateDynamicRoutingConditions(index int, conditions []DynamicRoutingCondition) error {
	for conditionIndex, condition := range conditions {
		field := strings.TrimSpace(condition.Field)
		if field != DynamicRoutingConditionReasoningEffort && !strings.HasPrefix(field, DynamicRoutingConditionRequestPrefix) {
			return fmt.Errorf("dynamic_routing.rules[%d].conditions[%d].field is unsupported", index, conditionIndex)
		}
		if strings.HasPrefix(field, DynamicRoutingConditionRequestPrefix) && !isValidDynamicRoutingRequestField(strings.TrimPrefix(field, DynamicRoutingConditionRequestPrefix)) {
			return fmt.Errorf("dynamic_routing.rules[%d].conditions[%d].field must use a simple request JSON path", index, conditionIndex)
		}

		switch normalizeDynamicRoutingOperator(condition.Operator) {
		case DynamicRoutingOperatorEquals, DynamicRoutingOperatorNotEquals:
			if len(condition.Value) > MaxDynamicRoutingStringLength {
				return fmt.Errorf("dynamic_routing.rules[%d].conditions[%d].value is too long", index, conditionIndex)
			}
		case DynamicRoutingOperatorExists, DynamicRoutingOperatorNotExists:
		default:
			return fmt.Errorf("dynamic_routing.rules[%d].conditions[%d].operator is unsupported", index, conditionIndex)
		}
	}
	return nil
}

func normalizeDynamicRoutingAction(action string) string {
	action = strings.TrimSpace(action)
	if action == "" {
		return DynamicRoutingActionModelRedirect
	}
	return action
}

func normalizeDynamicRoutingOperator(operator string) string {
	operator = strings.ToLower(strings.TrimSpace(operator))
	if operator == "" {
		return DynamicRoutingOperatorEquals
	}
	return operator
}

func isValidDynamicRoutingRequestField(path string) bool {
	if path == "" || len(path) > MaxDynamicRoutingRequestPathSize || strings.HasPrefix(path, ".") || strings.HasSuffix(path, ".") || strings.Contains(path, "..") {
		return false
	}
	for _, character := range path {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}

// EffectiveDynamicRoutingAction exposes the backward-compatible default action
// to callers without allowing unsupported actions to execute.
func EffectiveDynamicRoutingAction(rule DynamicRoutingRule) string {
	return normalizeDynamicRoutingAction(rule.Action)
}

// EffectiveDynamicRoutingOperator exposes the backward-compatible default
// operator to callers without duplicating normalization rules.
func EffectiveDynamicRoutingOperator(condition DynamicRoutingCondition) string {
	return normalizeDynamicRoutingOperator(condition.Operator)
}
