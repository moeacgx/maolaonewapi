package oairesponses

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIResponsesRequestToClaudeMessagesPreservesDecodedFunctionToolsAndBlockCacheControl(t *testing.T) {
	var request dto.OpenAIResponsesRequest
	require.NoError(t, kitutil.Unmarshal([]byte(`{
		"model":"claude-sonnet-4-6",
		"max_output_tokens":256,
		"input":[{"role":"user","content":[{
			"type":"input_text",
			"text":"cached prompt",
			"cache_control":{"type":"ephemeral","ttl":"1h"}
		}]}],
		"tools":[{
			"type":"function",
			"name":"lookup",
			"description":"Lookup data",
			"parameters":{"type":"object","properties":{"q":{"type":"string"}}}
		}]
	}`), &request))

	got, err := OpenAIResponsesRequestToClaudeMessages(context.Background(), nil, &request)
	require.NoError(t, err)
	require.Len(t, got.GetTools(), 1)
	tool, ok := got.GetTools()[0].(*dto.Tool)
	require.True(t, ok)
	assert.Equal(t, "lookup", tool.Name)
	assert.Equal(t, "Lookup data", tool.Description)
	assert.Equal(t, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"q": map[string]any{"type": "string"},
		},
	}, tool.InputSchema)

	require.Len(t, got.Messages, 1)
	parts, err := got.Messages[0].ParseContent()
	require.NoError(t, err)
	require.Len(t, parts, 1)
	assert.Equal(t, "cached prompt", parts[0].GetText())
	assert.JSONEq(t, `{"type":"ephemeral","ttl":"1h"}`, string(parts[0].CacheControl))
}

func TestOpenAIResponsesRequestToClaudeMessagesRejectsLossyCustomTools(t *testing.T) {
	maxTokens := uint(256)
	tests := []struct {
		name      string
		request   *dto.OpenAIResponsesRequest
		wantError string
	}{
		{
			name: "non-function tool declaration",
			request: &dto.OpenAIResponsesRequest{
				Model:           "claude-sonnet-4-6",
				MaxOutputTokens: &maxTokens,
				Input:           mustRawMessage(t, "hello"),
				Tools: mustRawMessage(t, []map[string]any{
					{"type": "custom", "name": "apply_patch"},
				}),
			},
			wantError: `Claude Messages cannot safely represent Responses tool type "custom"`,
		},
		{
			name: "custom tool call",
			request: &dto.OpenAIResponsesRequest{
				Model:           "claude-sonnet-4-6",
				MaxOutputTokens: &maxTokens,
				Input: mustRawMessage(t, []map[string]any{
					{"type": "custom_tool_call", "call_id": "call_1", "name": "apply_patch", "input": "patch"},
				}),
			},
			wantError: "Claude Messages cannot safely represent Responses custom tool items",
		},
		{
			name: "custom tool call output",
			request: &dto.OpenAIResponsesRequest{
				Model:           "claude-sonnet-4-6",
				MaxOutputTokens: &maxTokens,
				Input: mustRawMessage(t, []map[string]any{
					{"type": "custom_tool_call_output", "call_id": "call_1", "output": "ok"},
				}),
			},
			wantError: "Claude Messages cannot safely represent Responses custom tool items",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := OpenAIResponsesRequestToClaudeMessages(context.Background(), nil, tt.request)
			require.EqualError(t, err, tt.wantError)
		})
	}
}

func TestOpenAIResponsesRequestToClaudeMessagesPreflightsCustomToolsBeforeMediaConversion(t *testing.T) {
	maxTokens := uint(256)
	request := &dto.OpenAIResponsesRequest{
		Model:           "claude-sonnet-4-6",
		MaxOutputTokens: &maxTokens,
		Input: mustRawMessage(t, []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "input_image", "image_url": "http://[::1"},
				},
			},
			{"type": "custom_tool_call", "call_id": "call_1", "name": "apply_patch", "input": "patch"},
		}),
	}

	_, err := OpenAIResponsesRequestToClaudeMessages(context.Background(), nil, request)
	require.EqualError(t, err, "Claude Messages cannot safely represent Responses custom tool items")
}

func TestOpenAIResponsesRequestToClaudeMessagesCapsCacheControlInEmittedOrder(t *testing.T) {
	maxTokens := uint(256)
	cacheControl := map[string]any{"type": "ephemeral"}
	parts := func(prefix string) []map[string]any {
		return []map[string]any{
			{"type": "input_text", "text": prefix + "-1", "cache_control": cacheControl},
			{"type": "input_text", "text": prefix + "-2", "cache_control": cacheControl},
			{"type": "input_text", "text": prefix + "-3", "cache_control": cacheControl},
		}
	}
	request := &dto.OpenAIResponsesRequest{
		Model:           "claude-sonnet-4-6",
		MaxOutputTokens: &maxTokens,
		Input: mustRawMessage(t, []map[string]any{
			{"role": "user", "content": parts("user")},
			{"role": "system", "content": parts("system")},
		}),
	}

	got, err := OpenAIResponsesRequestToClaudeMessages(context.Background(), nil, request)
	require.NoError(t, err)
	system := got.ParseSystem()
	require.Len(t, system, 3)
	for i, expectedText := range []string{"system-1", "system-2", "system-3"} {
		assert.Equal(t, expectedText, system[i].GetText())
		assert.NotEmpty(t, system[i].CacheControl)
	}
	require.Len(t, got.Messages, 1)
	messageParts, err := got.Messages[0].ParseContent()
	require.NoError(t, err)
	require.Len(t, messageParts, 3)
	assert.Equal(t, "user-1", messageParts[0].GetText())
	assert.Equal(t, "user-2", messageParts[1].GetText())
	assert.Equal(t, "user-3", messageParts[2].GetText())
	assert.NotEmpty(t, messageParts[0].CacheControl)
	assert.Empty(t, messageParts[1].CacheControl)
	assert.Empty(t, messageParts[2].CacheControl)
}

func TestOpenAIResponsesRequestToClaudeMessagesHandlesEveryNormalizedReasoningEffort(t *testing.T) {
	maxTokens := uint(8192)
	tests := []struct {
		effort     string
		wantType   string
		wantBudget int
		wantError  bool
	}{
		{effort: "none", wantType: "disabled"},
		{effort: "minimal", wantType: "enabled", wantBudget: 1024},
		{effort: "low", wantType: "enabled", wantBudget: 1280},
		{effort: "medium", wantType: "enabled", wantBudget: 2048},
		{effort: "high", wantType: "enabled", wantBudget: 4096},
		{effort: "xhigh", wantError: true},
		{effort: "max", wantError: true},
		{effort: "ultra", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.effort, func(t *testing.T) {
			got, err := OpenAIResponsesRequestToClaudeMessages(context.Background(), nil, &dto.OpenAIResponsesRequest{
				Model:           "claude-sonnet-4-6",
				MaxOutputTokens: &maxTokens,
				Input:           mustRawMessage(t, "hello"),
				Reasoning:       &dto.Reasoning{Effort: tt.effort},
			})
			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "cannot represent")
				assert.Contains(t, err.Error(), tt.effort)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got.Thinking)
			assert.Equal(t, tt.wantType, got.Thinking.Type)
			assert.Equal(t, tt.wantBudget, got.Thinking.GetBudgetTokens())
		})
	}
}

func TestOpenAIResponsesRequestToClaudeMessagesMinimalReasoningRespectsMaxOutputTokens(t *testing.T) {
	tests := []struct {
		name            string
		maxOutputTokens *uint
		wantMaxTokens   uint
		wantError       bool
	}{
		{name: "absent", wantMaxTokens: 1280},
		{name: "low", maxOutputTokens: kitutil.GetPointer(uint(512)), wantError: true},
		{name: "equal to budget", maxOutputTokens: kitutil.GetPointer(uint(1024)), wantError: true},
		{name: "sufficient", maxOutputTokens: kitutil.GetPointer(uint(1025)), wantMaxTokens: 1025},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := OpenAIResponsesRequestToClaudeMessages(context.Background(), nil, &dto.OpenAIResponsesRequest{
				Model:           "claude-sonnet-4-6",
				MaxOutputTokens: tt.maxOutputTokens,
				Input:           mustRawMessage(t, "hello"),
				Reasoning:       &dto.Reasoning{Effort: "minimal"},
			})
			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "budget_tokens (1024)")
				assert.Contains(t, err.Error(), "max_output_tokens")
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got.MaxTokens)
			require.NotNil(t, got.Thinking)
			assert.Equal(t, tt.wantMaxTokens, *got.MaxTokens)
			assert.Equal(t, 1024, got.Thinking.GetBudgetTokens())
			assert.Less(t, uint(got.Thinking.GetBudgetTokens()), *got.MaxTokens)
		})
	}
}
