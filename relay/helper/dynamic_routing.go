package helper

import (
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/dynamic_routing_setting"
	routingreasoning "github.com/QuantumNous/new-api/setting/reasoning"
	"github.com/tidwall/gjson"
)

// applyDynamicModelRoute resolves the administrator-defined model_redirect
// action before the legacy static model_mapping is considered.
func applyDynamicModelRoute(info *relaycommon.RelayInfo, request dto.Request) bool {
	rule, matched := resolveDynamicModelRoute(info, request, dynamic_routing_setting.GetSettings())
	if !matched {
		return false
	}

	info.UpstreamModelName = strings.TrimSpace(rule.TargetModel)
	info.IsModelMapped = true
	info.IsDynamicModelRouted = true
	info.DynamicRoutingRuleID = rule.ID
	return true
}

func resolveDynamicModelRoute(
	info *relaycommon.RelayInfo,
	request dto.Request,
	globalConfig dto.DynamicRoutingConfig,
) (dto.DynamicRoutingRule, bool) {
	return resolveDynamicRoute(info, request, globalConfig, dto.DynamicRoutingActionModelRedirect)
}

// ResolveDynamicResponsesImageToolBridge resolves the special bridge action
// only when the Responses client explicitly selected image_generation. A mere
// tools declaration is intentionally insufficient: Codex can advertise that
// capability on ordinary text requests.
func ResolveDynamicResponsesImageToolBridge(
	info *relaycommon.RelayInfo,
	request dto.Request,
) (dto.DynamicRoutingRule, bool) {
	if !hasForcedResponsesImageGenerationTool(request) {
		return dto.DynamicRoutingRule{}, false
	}
	return resolveDynamicRoute(
		info,
		request,
		dynamic_routing_setting.GetSettings(),
		dto.DynamicRoutingActionResponsesImageToolBridge,
	)
}

// ResolveDynamicResponsesImageFunctionBridge resolves the two-stage image
// bridge. Unlike the native image_generation bridge, this action is triggered
// by a rule alone: the relay injects a private function tool into the source
// Responses request and waits for the model to call it.
//
// Streaming requests are supported by the Responses relay, which buffers the
// source SSE until it knows whether the private function was called.
func ResolveDynamicResponsesImageFunctionBridge(
	info *relaycommon.RelayInfo,
	request dto.Request,
) (dto.DynamicRoutingRule, bool) {
	responsesRequest, ok := request.(*dto.OpenAIResponsesRequest)
	if !ok || responsesRequest == nil {
		return dto.DynamicRoutingRule{}, false
	}
	return resolveDynamicRoute(
		info,
		request,
		dynamic_routing_setting.GetSettings(),
		dto.DynamicRoutingActionResponsesImageFunctionBridge,
	)
}

func resolveDynamicRoute(
	info *relaycommon.RelayInfo,
	request dto.Request,
	globalConfig dto.DynamicRoutingConfig,
	action string,
) (dto.DynamicRoutingRule, bool) {
	if info == nil || strings.TrimSpace(info.OriginModelName) == "" {
		return dto.DynamicRoutingRule{}, false
	}

	var channelConfig *dto.DynamicRoutingChannelConfig
	if info.ChannelMeta != nil {
		channelConfig = info.ChannelSetting.DynamicRouting
	}
	if channelConfig != nil {
		channelEnabled := globalConfig.Enabled
		if channelConfig.Enabled != nil {
			channelEnabled = *channelConfig.Enabled
			if !channelEnabled {
				return dto.DynamicRoutingRule{}, false
			}
		}
		if channelEnabled {
			candidates := dynamicRoutingCandidates(channelConfig.Rules, info, action)
			if len(candidates) > 0 {
				return matchDynamicRoutingCandidates(candidates, info, request)
			}
		}
	}

	if !globalConfig.Enabled {
		return dto.DynamicRoutingRule{}, false
	}
	return matchDynamicRoutingCandidates(dynamicRoutingCandidates(globalConfig.Rules, info, action), info, request)
}

// dynamicRoutingCandidates resolves model and scope only. A non-empty channel
// result intentionally suppresses global rules for the same model/scope, even
// if none of its request conditions match.
func dynamicRoutingCandidates(
	rules []dto.DynamicRoutingRule,
	info *relaycommon.RelayInfo,
	action string,
) []dto.DynamicRoutingRule {
	if info == nil {
		return nil
	}
	originModel := strings.TrimSpace(info.OriginModelName)
	requestPath := dynamicRoutingRequestPath(info)
	sourceGroup := strings.TrimSpace(info.UsingGroup)
	channelType := 0
	if info.ChannelMeta != nil {
		channelType = info.ChannelType
	}

	candidates := make([]dto.DynamicRoutingRule, 0, len(rules))
	for _, rule := range rules {
		if !rule.Enabled || dto.EffectiveDynamicRoutingAction(rule) != action {
			continue
		}
		if strings.TrimSpace(rule.SourceModel) != originModel || strings.TrimSpace(rule.TargetModel) == "" {
			continue
		}
		if len(rule.SourceGroups) > 0 && !containsDynamicRoutingSourceGroup(rule.SourceGroups, sourceGroup) {
			continue
		}
		if len(rule.ChannelTypes) > 0 && !containsDynamicRoutingChannelType(rule.ChannelTypes, channelType) {
			continue
		}
		if len(rule.RequestPaths) > 0 && !containsDynamicRoutingRequestPath(rule.RequestPaths, requestPath) {
			continue
		}
		candidates = append(candidates, rule)
	}

	sort.SliceStable(candidates, func(left, right int) bool {
		return candidates[left].Priority > candidates[right].Priority
	})
	return candidates
}

func hasForcedResponsesImageGenerationTool(request dto.Request) bool {
	responsesRequest, ok := request.(*dto.OpenAIResponsesRequest)
	if !ok || responsesRequest == nil {
		return false
	}

	hasImageTool := false
	for _, tool := range responsesRequest.GetToolsMap() {
		if strings.EqualFold(strings.TrimSpace(common.Interface2String(tool["type"])), dto.BuildInToolImageGeneration) {
			hasImageTool = true
			break
		}
	}
	if !hasImageTool || len(responsesRequest.ToolChoice) == 0 {
		return false
	}

	toolChoice := gjson.ParseBytes(responsesRequest.ToolChoice)
	if toolChoice.Type == gjson.String {
		return strings.EqualFold(strings.TrimSpace(toolChoice.String()), dto.BuildInToolImageGeneration)
	}
	return strings.EqualFold(
		strings.TrimSpace(toolChoice.Get("type").String()),
		dto.BuildInToolImageGeneration,
	)
}

func matchDynamicRoutingCandidates(
	candidates []dto.DynamicRoutingRule,
	info *relaycommon.RelayInfo,
	request dto.Request,
) (dto.DynamicRoutingRule, bool) {
	if len(candidates) == 0 {
		return dto.DynamicRoutingRule{}, false
	}

	var requestJSON []byte
	requestJSONReady := false
	requestJSONAvailable := false
	for _, rule := range candidates {
		if dynamicRoutingConditionsMatch(rule.Conditions, info, request, &requestJSON, &requestJSONReady, &requestJSONAvailable) {
			return rule, true
		}
	}
	return dto.DynamicRoutingRule{}, false
}

func dynamicRoutingConditionsMatch(
	conditions []dto.DynamicRoutingCondition,
	info *relaycommon.RelayInfo,
	request dto.Request,
	requestJSON *[]byte,
	requestJSONReady *bool,
	requestJSONAvailable *bool,
) bool {
	for _, condition := range conditions {
		actual, exists := dynamicRoutingConditionValue(condition, info, request, requestJSON, requestJSONReady, requestJSONAvailable)
		expected := condition.Value
		if strings.TrimSpace(condition.Field) == dto.DynamicRoutingConditionReasoningEffort {
			expected = normalizeDynamicRoutingReasoningEffort(expected)
		}
		switch dto.EffectiveDynamicRoutingOperator(condition) {
		case dto.DynamicRoutingOperatorExists:
			if !exists {
				return false
			}
		case dto.DynamicRoutingOperatorNotExists:
			if exists {
				return false
			}
		case dto.DynamicRoutingOperatorNotEquals:
			if !exists || actual == expected {
				return false
			}
		default:
			if !exists || actual != expected {
				return false
			}
		}
	}
	return true
}

func dynamicRoutingConditionValue(
	condition dto.DynamicRoutingCondition,
	info *relaycommon.RelayInfo,
	request dto.Request,
	requestJSON *[]byte,
	requestJSONReady *bool,
	requestJSONAvailable *bool,
) (string, bool) {
	field := strings.TrimSpace(condition.Field)
	if field == dto.DynamicRoutingConditionReasoningEffort {
		effort := dynamicRoutingReasoningEffort(info, request)
		if effort == "" {
			return "", false
		}
		return effort, true
	}
	if !strings.HasPrefix(field, dto.DynamicRoutingConditionRequestPrefix) || request == nil {
		return "", false
	}

	if !*requestJSONReady {
		*requestJSONReady = true
		encoded, err := common.Marshal(request)
		if err == nil {
			*requestJSON = encoded
			*requestJSONAvailable = true
		}
	}
	if !*requestJSONAvailable {
		return "", false
	}

	value := gjson.GetBytes(*requestJSON, strings.TrimPrefix(field, dto.DynamicRoutingConditionRequestPrefix))
	if !value.Exists() {
		return "", false
	}
	return value.String(), true
}

func dynamicRoutingReasoningEffort(info *relaycommon.RelayInfo, request dto.Request) string {
	if info != nil {
		if effort := normalizeDynamicRoutingReasoningEffort(info.ReasoningEffort); effort != "" {
			return effort
		}
	}
	var effort string
	switch req := request.(type) {
	case *dto.GeneralOpenAIRequest:
		if req == nil {
			return ""
		}
		if effort = normalizeDynamicRoutingReasoningEffort(req.ReasoningEffort); effort != "" {
			return effort
		}
		if len(req.Reasoning) > 0 {
			value := gjson.GetBytes(req.Reasoning, "effort")
			if value.Type == gjson.String {
				return normalizeDynamicRoutingReasoningEffort(value.String())
			}
		}
	case *dto.OpenAIResponsesRequest:
		if req != nil && req.Reasoning != nil {
			effort = req.Reasoning.Effort
		}
	case *dto.ClaudeRequest:
		if req != nil {
			effort = req.GetEfforts()
		}
	case *dto.GeminiChatRequest:
		if req != nil && req.GenerationConfig.ThinkingConfig != nil {
			effort = req.GenerationConfig.ThinkingConfig.ThinkingLevel
		}
	}
	return normalizeDynamicRoutingReasoningEffort(effort)
}

func normalizeDynamicRoutingReasoningEffort(effort string) string {
	return routingreasoning.NormalizeOpenAIReasoningEffort(effort)
}

func dynamicRoutingRequestPath(info *relaycommon.RelayInfo) string {
	if info == nil {
		return ""
	}
	path := info.OriginalRequestURLPath
	if path == "" {
		path = info.RequestURLPath
	}
	if queryIndex := strings.IndexByte(path, '?'); queryIndex >= 0 {
		return path[:queryIndex]
	}
	return path
}

func containsDynamicRoutingChannelType(channelTypes []int, target int) bool {
	for _, channelType := range channelTypes {
		if channelType == target {
			return true
		}
	}
	return false
}

func containsDynamicRoutingRequestPath(paths []string, target string) bool {
	for _, path := range paths {
		if strings.TrimSpace(path) == target {
			return true
		}
	}
	return false
}

func containsDynamicRoutingSourceGroup(groups []string, target string) bool {
	for _, group := range groups {
		if strings.EqualFold(strings.TrimSpace(group), target) {
			return true
		}
	}
	return false
}
