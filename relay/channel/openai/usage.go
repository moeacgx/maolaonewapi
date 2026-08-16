package openai

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/tidwall/sjson"
)

func normalizeOpenAIUsageTokenCounts(usage *dto.Usage) bool {
	if usage == nil {
		return false
	}
	modified := false
	if usage.PromptTokens == 0 && usage.InputTokens > 0 {
		usage.PromptTokens = usage.InputTokens
		modified = true
	}
	if usage.CompletionTokens == 0 {
		switch {
		case usage.OutputTokens > 0:
			usage.CompletionTokens = usage.OutputTokens
			modified = true
		case usage.PromptTokens > 0 && usage.TotalTokens > usage.PromptTokens:
			usage.CompletionTokens = usage.TotalTokens - usage.PromptTokens
			modified = true
		}
	}
	if usage.PromptTokens == 0 && usage.CompletionTokens > 0 && usage.TotalTokens > usage.CompletionTokens {
		usage.PromptTokens = usage.TotalTokens - usage.CompletionTokens
		modified = true
	}
	calculatedTotal := usage.PromptTokens + usage.CompletionTokens
	if calculatedTotal > 0 && usage.TotalTokens != calculatedTotal {
		usage.TotalTokens = calculatedTotal
		modified = true
	}
	return modified
}

func normalizeAndValidateOpenAIUsage(usage *dto.Usage) bool {
	normalizeOpenAIUsageTokenCounts(usage)
	return usage != nil && (usage.PromptTokens != 0 || usage.CompletionTokens != 0)
}

func fillMissingOpenAIChatUsage(usage *dto.Usage, promptTokens, completionTokens int) bool {
	modified := normalizeOpenAIUsageTokenCounts(usage)
	if usage == nil {
		return modified
	}
	if usage.PromptTokens == 0 && promptTokens > 0 {
		usage.PromptTokens = promptTokens
		modified = true
	}
	if normalizeOpenAIUsageTokenCounts(usage) {
		modified = true
	}
	if usage.CompletionTokens == 0 && completionTokens > 0 {
		usage.CompletionTokens = completionTokens
		modified = true
	}
	if normalizeOpenAIUsageTokenCounts(usage) {
		modified = true
	}
	return modified
}

func patchOpenAIChatUsage(data []byte, usage *dto.Usage) ([]byte, error) {
	if usage == nil {
		return data, nil
	}
	patched, err := sjson.SetBytes(data, "usage.prompt_tokens", usage.PromptTokens)
	if err != nil {
		return nil, err
	}
	patched, err = sjson.SetBytes(patched, "usage.completion_tokens", usage.CompletionTokens)
	if err != nil {
		return nil, err
	}
	return sjson.SetBytes(patched, "usage.total_tokens", usage.TotalTokens)
}

func normalizeOpenAIStreamUsageData(data string) string {
	var streamResponse dto.ChatCompletionsStreamResponse
	if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil || streamResponse.Usage == nil {
		return data
	}
	if !normalizeOpenAIUsageTokenCounts(streamResponse.Usage) {
		return data
	}
	patched, err := patchOpenAIChatUsage(common.StringToByteSlice(data), streamResponse.Usage)
	if err != nil {
		return data
	}
	return string(patched)
}

func applyUsagePostProcessing(info *relaycommon.RelayInfo, usage *dto.Usage, responseBody []byte) {
	if info == nil || usage == nil {
		return
	}

	switch info.ChannelType {
	case constant.ChannelTypeDeepSeek:
		if usage.PromptTokensDetails.CachedTokens == 0 && usage.PromptCacheHitTokens != 0 {
			usage.PromptTokensDetails.CachedTokens = usage.PromptCacheHitTokens
		}
	case constant.ChannelTypeZhipu_v4:
		// 智普的cached_tokens在标准位置: usage.prompt_tokens_details.cached_tokens
		if usage.PromptTokensDetails.CachedTokens == 0 {
			if usage.InputTokensDetails != nil && usage.InputTokensDetails.CachedTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.InputTokensDetails.CachedTokens
			} else if cachedTokens, ok := extractCachedTokensFromBody(responseBody); ok {
				usage.PromptTokensDetails.CachedTokens = cachedTokens
			} else if usage.PromptCacheHitTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.PromptCacheHitTokens
			}
		}
	case constant.ChannelTypeMoonshot:
		// Moonshot的cached_tokens在非标准位置: choices[].usage.cached_tokens
		if usage.PromptTokensDetails.CachedTokens == 0 {
			if usage.InputTokensDetails != nil && usage.InputTokensDetails.CachedTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.InputTokensDetails.CachedTokens
			} else if cachedTokens, ok := extractMoonshotCachedTokensFromBody(responseBody); ok {
				usage.PromptTokensDetails.CachedTokens = cachedTokens
			} else if cachedTokens, ok := extractCachedTokensFromBody(responseBody); ok {
				usage.PromptTokensDetails.CachedTokens = cachedTokens
			} else if usage.PromptCacheHitTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.PromptCacheHitTokens
			}
		}
	case constant.ChannelTypeOpenAI:
		if usage.PromptTokensDetails.CachedTokens == 0 {
			if cachedTokens, ok := extractLlamaCachedTokensFromBody(responseBody); ok {
				usage.PromptTokensDetails.CachedTokens = cachedTokens
			}
		}
	}
}

func extractCachedTokensFromBody(body []byte) (int, bool) {
	if len(body) == 0 {
		return 0, false
	}

	var payload struct {
		Usage struct {
			PromptTokensDetails struct {
				CachedTokens *int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			CachedTokens         *int `json:"cached_tokens"`
			PromptCacheHitTokens *int `json:"prompt_cache_hit_tokens"`
		} `json:"usage"`
	}

	if err := common.Unmarshal(body, &payload); err != nil {
		return 0, false
	}

	if payload.Usage.PromptTokensDetails.CachedTokens != nil {
		return *payload.Usage.PromptTokensDetails.CachedTokens, true
	}
	if payload.Usage.CachedTokens != nil {
		return *payload.Usage.CachedTokens, true
	}
	if payload.Usage.PromptCacheHitTokens != nil {
		return *payload.Usage.PromptCacheHitTokens, true
	}
	return 0, false
}

// extractMoonshotCachedTokensFromBody 从Moonshot的非标准位置提取cached_tokens
// Moonshot的流式响应格式: {"choices":[{"usage":{"cached_tokens":111}}]}
func extractMoonshotCachedTokensFromBody(body []byte) (int, bool) {
	if len(body) == 0 {
		return 0, false
	}

	var payload struct {
		Choices []struct {
			Usage struct {
				CachedTokens *int `json:"cached_tokens"`
			} `json:"usage"`
		} `json:"choices"`
	}

	if err := common.Unmarshal(body, &payload); err != nil {
		return 0, false
	}

	// 遍历choices查找cached_tokens
	for _, choice := range payload.Choices {
		if choice.Usage.CachedTokens != nil && *choice.Usage.CachedTokens > 0 {
			return *choice.Usage.CachedTokens, true
		}
	}

	return 0, false
}

// extractLlamaCachedTokensFromBody 从llama.cpp的非标准位置提取cache_n
func extractLlamaCachedTokensFromBody(body []byte) (int, bool) {
	if len(body) == 0 {
		return 0, false
	}

	var payload struct {
		Timings struct {
			CachedTokens *int `json:"cache_n"`
		} `json:"timings"`
	}

	if err := common.Unmarshal(body, &payload); err != nil {
		return 0, false
	}

	if payload.Timings.CachedTokens == nil {
		return 0, false
	}
	return *payload.Timings.CachedTokens, true
}
