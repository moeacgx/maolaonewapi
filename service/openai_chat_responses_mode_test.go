package service

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/assert"
)

func TestShouldChatCompletionsUseResponsesPolicyRequiresExplicitMatch(t *testing.T) {
	empty := model_setting.ChatCompletionsToResponsesPolicy{}
	assert.False(t, ShouldChatCompletionsUseResponsesPolicy(empty, 749, constant.ChannelTypeXai, "grok-4.6"))

	configured := model_setting.ChatCompletionsToResponsesPolicy{
		Enabled:       true,
		ChannelIDs:    []int{749},
		ModelPatterns: []string{`^grok-4\.6$`},
	}
	assert.True(t, ShouldChatCompletionsUseResponsesPolicy(configured, 749, constant.ChannelTypeXai, "grok-4.6"))
	assert.False(t, ShouldChatCompletionsUseResponsesPolicy(configured, 749, constant.ChannelTypeXai, "grok-imagine-image"))
}

func TestShouldChatCompletionsUseResponsesForRequestUsesExplicitPolicy(t *testing.T) {
	policy := model_setting.ChatCompletionsToResponsesPolicy{}

	assert.False(t, ShouldChatCompletionsUseResponsesForRequest(
		policy, constant.ChannelTypeOpenAI, true, 357, "grok-4.5", "grok-4.5",
	))

	configured := model_setting.ChatCompletionsToResponsesPolicy{
		Enabled:       true,
		ChannelIDs:    []int{357},
		ModelPatterns: []string{`^grok-4\.5$`},
	}
	assert.True(t, ShouldChatCompletionsUseResponsesForRequest(
		configured, constant.ChannelTypeOpenAI, true, 357, "grok-4.5", "grok-4.5",
	))
	assert.True(t, ShouldChatCompletionsUseResponsesForRequest(
		configured, constant.ChannelTypeOpenAI, true, 357, "my-grok-alias", "grok-4.5",
	))
	assert.False(t, ShouldChatCompletionsUseResponsesForRequest(
		configured, constant.ChannelTypeOpenAI, true, 358, "grok-4.5", "grok-4.5",
	))
	assert.False(t, ShouldChatCompletionsUseResponsesForRequest(
		configured, constant.ChannelTypeOpenAI, true, 357, "gpt-4o", "gpt-4o",
	))
}
