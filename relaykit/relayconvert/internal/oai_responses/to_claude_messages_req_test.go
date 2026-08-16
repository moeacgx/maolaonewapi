package oairesponses

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIResponsesRequestToClaudeMessagesPreservesFunctionToolsAndBlockCacheControl(t *testing.T) {
	maxTokens := uint(256)
	request := &dto.OpenAIResponsesRequest{
		Model:           "claude-sonnet-4-6",
		MaxOutputTokens: &maxTokens,
		Input: mustRawMessage(t, []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type":          "input_text",
						"text":          "cached prompt",
						"cache_control": map[string]any{"type": "ephemeral", "ttl": "1h"},
					},
				},
			},
		}),
		Tools: mustRawMessage(t, []map[string]any{
			{
				"type":        "function",
				"name":        "lookup",
				"description": "Lookup data",
				"parameters": map[string]any{
					"type":       "object",
					"properties": map[string]any{"q": map[string]any{"type": "string"}},
				},
			},
		}),
	}

	got, err := OpenAIResponsesRequestToClaudeMessages(context.Background(), nil, request)
	require.NoError(t, err)
	require.Len(t, got.GetTools(), 1)
	tool, ok := got.GetTools()[0].(*dto.Tool)
	require.True(t, ok)
	assert.Equal(t, "lookup", tool.Name)

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
		name    string
		request *dto.OpenAIResponsesRequest
	}{
		{
			name: "custom tool declaration",
			request: &dto.OpenAIResponsesRequest{
				Model:           "claude-sonnet-4-6",
				MaxOutputTokens: &maxTokens,
				Input:           mustRawMessage(t, "hello"),
				Tools: mustRawMessage(t, []map[string]any{
					{"type": "custom", "name": "apply_patch"},
				}),
			},
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
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := OpenAIResponsesRequestToClaudeMessages(context.Background(), nil, tt.request)
			require.ErrorContains(t, err, "cannot safely represent")
		})
	}
}
