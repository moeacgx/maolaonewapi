package oaichat

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatCompletionsResponseToResponsesPreservesTextToolCallsAndUsage(t *testing.T) {
	chat := &dto.OpenAITextResponse{
		Id:      "chatcmpl_1",
		Model:   "gpt-test",
		Created: 456,
		Choices: []dto.OpenAITextResponseChoice{
			{
				Message:      assistantMessageWithTool("I will call.", "call_1", "lookup", `{"q":"x"}`),
				FinishReason: "tool_calls",
			},
		},
		Usage: dto.Usage{PromptTokens: 3, CompletionTokens: 5, TotalTokens: 8},
	}

	resp, usage, err := ChatCompletionsResponseToResponsesResponse(chat, "resp_1")
	require.NoError(t, err)
	require.NotNil(t, usage)

	assert.Equal(t, "resp_1", resp.ID)
	assert.Equal(t, "response", resp.Object)
	assert.Equal(t, `"completed"`, string(resp.Status))
	assert.Equal(t, 3, resp.Usage.InputTokens)
	assert.Equal(t, 5, resp.Usage.OutputTokens)
	require.Len(t, resp.Output, 2)
	assert.Equal(t, responsesOutputTypeMessage, resp.Output[0].Type)
	assert.Equal(t, "I will call.", resp.Output[0].Content[0].Text)
	assert.Equal(t, responsesOutputTypeFunctionCall, resp.Output[1].Type)
	assert.Equal(t, "call_1", resp.Output[1].CallId)
	assert.Equal(t, "lookup", resp.Output[1].Name)
	assert.Equal(t, `"{\"q\":\"x\"}"`, string(resp.Output[1].Arguments))
}

func TestChatCompletionsResponseToResponsesMapsIncompleteFinishReasons(t *testing.T) {
	tests := []struct {
		name         string
		finishReason string
		wantReason   string
	}{
		{name: "length", finishReason: "length", wantReason: responsesIncompleteReasonMaxTokens},
		{name: "content filter", finishReason: "content_filter", wantReason: responsesIncompleteReasonContentFilter},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, _, err := ChatCompletionsResponseToResponsesResponse(&dto.OpenAITextResponse{
				Id:    "chatcmpl_1",
				Model: "gpt-test",
				Choices: []dto.OpenAITextResponseChoice{
					{
						Message:      dto.Message{Role: "assistant", Content: "partial"},
						FinishReason: tt.finishReason,
					},
				},
			}, "resp_1")
			require.NoError(t, err)

			assert.Equal(t, `"incomplete"`, string(resp.Status))
			require.NotNil(t, resp.IncompleteDetails)
			assert.Equal(t, tt.wantReason, resp.IncompleteDetails.Reason)
			require.Len(t, resp.Output, 1)
			assert.Equal(t, "incomplete", resp.Output[0].Status)
		})
	}
}

func TestChatCompletionsStreamToResponsesEventsAggregatesUsageAndToolArgs(t *testing.T) {
	state := NewChatToResponsesStreamState("resp_1", "gpt-test")
	state.Created = 123
	toolIndex := 0

	var events []ChatToResponsesStreamEvent
	events = append(events, mustResponsesEventsFromChatChunk(t, state, &dto.ChatCompletionsStreamResponse{
		Id:      "chatcmpl_1",
		Model:   "gpt-test",
		Created: 123,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{Index: 0, Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Role: "assistant"}},
		},
	})...)
	events = append(events, mustResponsesEventsFromChatChunk(t, state, &dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{Index: 0, Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Content: lo.ToPtr("hello")}},
		},
	})...)
	events = append(events, mustResponsesEventsFromChatChunk(t, state, &dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{Index: 0, Delta: dto.ChatCompletionsStreamResponseChoiceDelta{ToolCalls: []dto.ToolCallResponse{
				{Index: &toolIndex, ID: "call_1", Type: "function", Function: dto.FunctionResponse{Name: "lookup"}},
			}}},
		},
	})...)
	events = append(events, mustResponsesEventsFromChatChunk(t, state, &dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{Index: 0, Delta: dto.ChatCompletionsStreamResponseChoiceDelta{ToolCalls: []dto.ToolCallResponse{
				{Index: &toolIndex, Function: dto.FunctionResponse{Arguments: `{"q":"x"}`}},
			}}},
		},
	})...)
	finishReason := "tool_calls"
	events = append(events, mustResponsesEventsFromChatChunk(t, state, &dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{Index: 0, FinishReason: &finishReason},
		},
	})...)
	events = append(events, mustResponsesEventsFromChatChunk(t, state, &dto.ChatCompletionsStreamResponse{
		Usage: &dto.Usage{PromptTokens: 2, CompletionTokens: 4, TotalTokens: 6},
	})...)
	events = append(events, FinalizeChatCompletionsStreamToResponses(state)...)

	require.Len(t, events, 10)
	assert.Equal(t, responsesEventCreated, events[0].Type)
	assert.Equal(t, responsesEventOutputTextDelta, events[2].Type)
	assert.Equal(t, "hello", events[2].Payload.Delta)
	assert.Equal(t, responsesEventFunctionArgsDelta, events[4].Type)
	assert.Equal(t, `{"q":"x"}`, events[4].Payload.Delta)
	assert.Equal(t, responsesEventCompleted, events[9].Type)
	require.NotNil(t, events[9].Payload.Response)
	assert.Equal(t, 6, events[9].Payload.Response.Usage.TotalTokens)
	require.Len(t, events[9].Payload.Response.Output, 2)
	assert.Equal(t, "hello", events[9].Payload.Response.Output[0].Content[0].Text)
	assert.Equal(t, `"{\"q\":\"x\"}"`, string(events[9].Payload.Response.Output[1].Arguments))
}

func mustResponsesEventsFromChatChunk(t *testing.T, state *ChatToResponsesStreamState, chunk *dto.ChatCompletionsStreamResponse) []ChatToResponsesStreamEvent {
	t.Helper()
	events, err := ChatCompletionsStreamChunkToResponsesEvents(chunk, state)
	require.NoError(t, err)
	return events
}
func TestUsageFromChatUsagePreservesExplicitZeroCacheCreation(t *testing.T) {
	var source dto.Usage
	require.NoError(t, json.Unmarshal([]byte(`{"prompt_tokens":100,"completion_tokens":5,"cache_creation_input_tokens":19,"prompt_tokens_details":{"cached_tokens":70,"cache_creation_tokens":0},"claude_cache_creation_5_m_tokens":7,"claude_cache_creation_1_h_tokens":11}`), &source))

	usage := UsageFromChatUsage(&source)

	tokens, present := usage.GetCacheCreationTokensWithPresence()
	assert.Zero(t, tokens)
	assert.True(t, present)
	require.NotNil(t, usage.InputTokensDetails)
	assert.Equal(t, 70, usage.InputTokensDetails.CachedTokens)
	assert.Equal(t, usage.InputTokensDetails.CachedTokens, usage.PromptTokensDetails.CachedTokens)
	assert.True(t, usage.InputTokensDetails.HasCacheWriteTokens)
	assert.True(t, usage.PromptTokensDetails.HasCacheWriteTokens)
	assert.Equal(t, 7, usage.ClaudeCacheCreation5mTokens)
	assert.Equal(t, 11, usage.ClaudeCacheCreation1hTokens)

	wire, err := json.Marshal(usage)
	require.NoError(t, err)
	var downstream dto.Usage
	require.NoError(t, json.Unmarshal(wire, &downstream))
	require.NotNil(t, downstream.InputTokensDetails)
	assert.Equal(t, 70, downstream.PromptTokensDetails.CachedTokens)
	assert.Equal(t, downstream.InputTokensDetails.CachedTokens, downstream.PromptTokensDetails.CachedTokens)
	assert.Equal(t, 30, downstream.InputTokens-downstream.PromptTokensDetails.CachedTokens)
	assert.Zero(t, downstream.GetCacheCreationTokens())
}

func TestUsageFromChatUsageUsesOneCacheWriteAliasAndMirrorsCacheReads(t *testing.T) {
	var source dto.Usage
	require.NoError(t, json.Unmarshal([]byte(`{"prompt_tokens":100,"completion_tokens":5,"cache_creation_input_tokens":13,"cache_write_input_tokens":17,"prompt_tokens_details":{"cached_tokens":25,"cache_write_tokens":7,"cache_creation_tokens":11}}`), &source))

	usage := UsageFromChatUsage(&source)

	require.NotNil(t, usage.InputTokensDetails)
	assert.Equal(t, 25, usage.InputTokensDetails.CachedTokens)
	assert.Equal(t, 25, usage.PromptTokensDetails.CachedTokens)
	assert.Equal(t, 7, usage.GetCacheCreationTokens())
	assert.Equal(t, 7, usage.PromptTokensDetails.CacheWriteTokens)
	assert.Equal(t, 7, usage.InputTokensDetails.CacheWriteTokens)
	assert.Equal(t, 7, usage.CacheCreationInputTokens)
	assert.Equal(t, 7, usage.CacheWriteInputTokens)
}

func TestUsageFromChatUsageKeepsAbsentCacheDetailsAbsent(t *testing.T) {
	usage := UsageFromChatUsage(&dto.Usage{PromptTokens: 100, CompletionTokens: 5})

	assert.Nil(t, usage.InputTokensDetails)
	assert.Zero(t, usage.PromptTokensDetails.CachedTokens)
	_, present := usage.GetCacheCreationTokensWithPresence()
	assert.False(t, present)
}
