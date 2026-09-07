package service

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/assert"
)

func TestShouldChatCompletionsUseResponsesForRequest(t *testing.T) {
	policy := model_setting.ChatCompletionsToResponsesPolicy{}

	assert.True(t, ShouldChatCompletionsUseResponsesForRequest(
		policy, constant.ChannelTypeXai, true, 749, "grok-4.6",
	))
	assert.False(t, ShouldChatCompletionsUseResponsesForRequest(
		policy, constant.ChannelTypeXai, false, 749, "grok-4.6",
	))
	assert.False(t, ShouldChatCompletionsUseResponsesForRequest(
		policy, constant.ChannelTypeOpenAI, true, 749, "gpt-4o",
	))
}
