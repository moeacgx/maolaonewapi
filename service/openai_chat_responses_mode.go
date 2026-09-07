package service

import (
	"regexp"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"
)

// Chat→Responses upgrade policy is host routing logic (it decides *whether*
// to convert, reading host settings), so it lives here, not in relayconvert.

var chatResponsesRegexCache sync.Map // map[string]*regexp.Regexp

func matchAnyModelPattern(patterns []string, model string) bool {
	if len(patterns) == 0 || model == "" {
		return false
	}
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		re, ok := chatResponsesRegexCache.Load(pattern)
		if !ok {
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				// Treat invalid patterns as non-matching to avoid breaking runtime traffic.
				continue
			}
			re = compiled
			chatResponsesRegexCache.Store(pattern, re)
		}
		if re.(*regexp.Regexp).MatchString(model) {
			return true
		}
	}
	return false
}

func ShouldChatCompletionsUseResponsesPolicy(policy model_setting.ChatCompletionsToResponsesPolicy, channelID int, channelType int, model string) bool {
	if !policy.IsChannelEnabled(channelID, channelType) {
		return false
	}
	return matchAnyModelPattern(policy.ModelPatterns, model)
}

func ShouldChatCompletionsUseResponsesGlobal(channelID int, channelType int, model string) bool {
	return ShouldChatCompletionsUseResponsesPolicy(
		model_setting.GetGlobalSettings().ChatCompletionsToResponsesPolicy,
		channelID,
		channelType,
		model,
	)
}

// isGrokModel 判断可能需要上游 Responses 请求结构的文本 Grok 模型。
func isGrokModel(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(normalized, "grok-") || strings.HasPrefix(normalized, "xai/grok-")
}

// ShouldChatCompletionsUseResponsesForRequest 先应用显式全局策略，再对
// OpenAI 兼容或原生 xAI 渠道中的 Grok 操练场请求应用窄范围兼容兜底。
func ShouldChatCompletionsUseResponsesForRequest(policy model_setting.ChatCompletionsToResponsesPolicy, channelType int, isPlayground bool, channelID int, model string) bool {
	if ShouldChatCompletionsUseResponsesPolicy(policy, channelID, channelType, model) {
		return true
	}
	if !isPlayground || !isGrokModel(model) {
		return false
	}
	return channelType == constant.ChannelTypeOpenAI || channelType == constant.ChannelTypeXai
}
