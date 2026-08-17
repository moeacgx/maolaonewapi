package oairesponses

import (
	"context"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
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

func TestOpenAIResponsesRequestToClaudeMessagesReasoningBudgetsUseFinalMaxTokens(t *testing.T) {
	efforts := []struct {
		name              string
		budget            uint
		implicitMaxTokens uint
	}{
		{name: "minimal", budget: 1024, implicitMaxTokens: 1280},
		{name: "low", budget: 1280, implicitMaxTokens: 1281},
		{name: "medium", budget: 2048, implicitMaxTokens: 2049},
		{name: "high", budget: 4096, implicitMaxTokens: 4097},
	}

	for _, effort := range efforts {
		t.Run(effort.name+"/implicit max", func(t *testing.T) {
			got, err := OpenAIResponsesRequestToClaudeMessages(context.Background(), nil, &dto.OpenAIResponsesRequest{
				Model:     "claude-sonnet-4-6",
				Input:     mustRawMessage(t, "hello"),
				Reasoning: &dto.Reasoning{Effort: effort.name},
			})
			require.NoError(t, err)
			require.NotNil(t, got.MaxTokens)
			require.NotNil(t, got.Thinking)
			assert.Equal(t, effort.implicitMaxTokens, *got.MaxTokens)
			assert.Equal(t, int(effort.budget), got.Thinking.GetBudgetTokens())
			assert.Less(t, uint(got.Thinking.GetBudgetTokens()), *got.MaxTokens)
		})

		t.Run(effort.name+"/explicit sufficient", func(t *testing.T) {
			maxOutputTokens := effort.budget + 1
			got, err := OpenAIResponsesRequestToClaudeMessages(context.Background(), nil, &dto.OpenAIResponsesRequest{
				Model:           "claude-sonnet-4-6",
				MaxOutputTokens: &maxOutputTokens,
				Input:           mustRawMessage(t, "hello"),
				Reasoning:       &dto.Reasoning{Effort: effort.name},
			})
			require.NoError(t, err)
			require.NotNil(t, got.MaxTokens)
			require.NotNil(t, got.Thinking)
			assert.Equal(t, maxOutputTokens, *got.MaxTokens)
			assert.Less(t, uint(got.Thinking.GetBudgetTokens()), *got.MaxTokens)
		})

		for _, maxOutputTokens := range []uint{effort.budget, effort.budget - 1} {
			t.Run(fmt.Sprintf("%s/explicit invalid %d", effort.name, maxOutputTokens), func(t *testing.T) {
				got, err := OpenAIResponsesRequestToClaudeMessages(context.Background(), nil, &dto.OpenAIResponsesRequest{
					Model:           "claude-sonnet-4-6",
					MaxOutputTokens: &maxOutputTokens,
					Input:           mustRawMessage(t, "hello"),
					Reasoning:       &dto.Reasoning{Effort: effort.name},
				})
				require.Error(t, err)
				assert.Nil(t, got)
				assert.Contains(t, err.Error(), fmt.Sprintf("budget_tokens (%d)", effort.budget))
				assert.Contains(t, err.Error(), fmt.Sprintf("max_output_tokens (%d)", maxOutputTokens))
			})
		}
	}
}

func TestOpenAIResponsesRequestToClaudeMessagesThinkingAdapterValidatesFinalMaxTokens(t *testing.T) {
	meta := &convmeta.Values{Options: &convmeta.Options{Claude: convmeta.ClaudeOptions{
		ThinkingAdapterEnabled:                true,
		ThinkingAdapterBudgetTokensPercentage: 0.8,
	}}}
	clientMaxTokens := uint(1000)
	got, err := OpenAIResponsesRequestToClaudeMessages(context.Background(), meta, &dto.OpenAIResponsesRequest{
		Model:           "claude-sonnet-4-6-thinking",
		MaxOutputTokens: &clientMaxTokens,
		Input:           mustRawMessage(t, "hello"),
	})
	require.NoError(t, err)
	require.NotNil(t, got.MaxTokens)
	require.NotNil(t, got.Thinking)
	assert.Equal(t, uint(1280), *got.MaxTokens)
	assert.Equal(t, 1024, got.Thinking.GetBudgetTokens())
	assert.Less(t, uint(got.Thinking.GetBudgetTokens()), *got.MaxTokens)
}

func TestOpenAIResponsesRequestToClaudeMessagesThinkingSuffixOnlyRewritesSupportedFamilies(t *testing.T) {
	maxTokens := uint(8192)
	tests := []struct {
		name         string
		model        string
		wantModel    string
		wantType     string
		wantThinking bool
		preserve     bool
	}{
		{name: "sonnet 4.6", model: "claude-sonnet-4-6-thinking", wantModel: "claude-sonnet-4-6", wantType: "enabled", wantThinking: true},
		{name: "sonnet 4.6 dated", model: "claude-sonnet-4-6-20260801-thinking", wantModel: "claude-sonnet-4-6-20260801", wantType: "enabled", wantThinking: true},
		{name: "sonnet 4.6 dated preserved", model: "claude-sonnet-4-6-20260801-thinking", wantModel: "claude-sonnet-4-6-20260801-thinking", wantType: "enabled", wantThinking: true, preserve: true},
		{name: "sonnet 4.5", model: "claude-sonnet-4-5-thinking", wantModel: "claude-sonnet-4-5", wantType: "enabled", wantThinking: true},
		{name: "sonnet 4.5 dated", model: "claude-sonnet-4-5-20250929-thinking", wantModel: "claude-sonnet-4-5-20250929", wantType: "enabled", wantThinking: true},
		{name: "claude 3.7 sonnet", model: "claude-3-7-sonnet-thinking", wantModel: "claude-3-7-sonnet", wantType: "enabled", wantThinking: true},
		{name: "claude 3.7 sonnet dated", model: "claude-3-7-sonnet-20250219-thinking", wantModel: "claude-3-7-sonnet-20250219", wantType: "enabled", wantThinking: true},
		{name: "opus 4.6", model: "claude-opus-4-6-thinking", wantModel: "claude-opus-4-6", wantType: "enabled", wantThinking: true},
		{name: "opus 4.6 dated", model: "claude-opus-4-6-20260801-thinking", wantModel: "claude-opus-4-6-20260801", wantType: "enabled", wantThinking: true},
		{name: "opus 4.7", model: "claude-opus-4-7-thinking", wantModel: "claude-opus-4-7", wantType: "adaptive", wantThinking: true},
		{name: "opus 4.7 dated", model: "claude-opus-4-7-20260801-thinking", wantModel: "claude-opus-4-7-20260801", wantType: "adaptive", wantThinking: true},
		{name: "opus 4.8", model: "claude-opus-4-8-thinking", wantModel: "claude-opus-4-8", wantType: "adaptive", wantThinking: true},
		{name: "opus 4.8 dated preserved", model: "claude-opus-4-8-20260801-thinking", wantModel: "claude-opus-4-8-20260801-thinking", wantType: "adaptive", wantThinking: true, preserve: true},
		{name: "family custom collision", model: "claude-sonnet-4-6-custom-thinking", wantModel: "claude-sonnet-4-6-custom-thinking"},
		{name: "dated alias custom collision", model: "claude-opus-4-8-20260801-custom-thinking", wantModel: "claude-opus-4-8-20260801-custom-thinking"},
		{name: "opus 4.60 collision", model: "claude-opus-4-60-thinking", wantModel: "claude-opus-4-60-thinking"},
		{name: "unrelated slash Claude collision", model: "claude/custom-thinking", wantModel: "claude/custom-thinking"},
		{name: "unrelated Claude collision", model: "claude-custom-thinking", wantModel: "claude-custom-thinking"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := &convmeta.Values{Options: &convmeta.Options{
				Claude: convmeta.ClaudeOptions{
					ThinkingAdapterEnabled:                true,
					ThinkingAdapterBudgetTokensPercentage: 0.8,
				},
				PreserveThinkingSuffix: func(string) bool { return tt.preserve },
			}}
			got, err := OpenAIResponsesRequestToClaudeMessages(context.Background(), meta, &dto.OpenAIResponsesRequest{
				Model:           tt.model,
				MaxOutputTokens: &maxTokens,
				Input:           mustRawMessage(t, "hello"),
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantModel, got.Model)
			if !tt.wantThinking {
				assert.Nil(t, got.Thinking)
				return
			}
			require.NotNil(t, got.Thinking)
			assert.Equal(t, tt.wantType, got.Thinking.Type)
			if tt.wantType == "adaptive" {
				assert.JSONEq(t, `{"effort":"high"}`, string(got.OutputConfig))
				return
			}
			assert.Less(t, uint(got.Thinking.GetBudgetTokens()), *got.MaxTokens)
		})
	}
}
func TestOpenAIResponsesRequestToClaudeMessagesPreservesSuffixUsingOriginAndFallback(t *testing.T) {
	maxTokens := uint(8192)
	tests := []struct {
		name        string
		model       string
		originModel string
		blacklist   string
	}{
		{name: "canonical effort suffix", model: "claude-opus-4-7-high", originModel: "claude-opus-4-7-high", blacklist: "claude-opus-4-7-high"},
		{name: "mapped origin effort alias", model: "claude-opus-4-7-high", originModel: "provider/opus-high", blacklist: "provider/opus-high"},
		{name: "standalone effort fallback", model: "claude-opus-4-7-high", blacklist: "claude-opus-4-7-high"},
		{name: "canonical thinking suffix", model: "claude-sonnet-4-6-thinking", originModel: "claude-sonnet-4-6-thinking", blacklist: "claude-sonnet-4-6-thinking"},
		{name: "mapped origin thinking alias", model: "claude-sonnet-4-6-thinking", originModel: "provider/sonnet-thinking", blacklist: "provider/sonnet-thinking"},
		{name: "standalone thinking fallback", model: "claude-sonnet-4-6-thinking", blacklist: "claude-sonnet-4-6-thinking"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := &convmeta.Values{OriginModelName: tt.originModel, Options: &convmeta.Options{
				Claude: convmeta.ClaudeOptions{
					ThinkingAdapterEnabled:                true,
					ThinkingAdapterBudgetTokensPercentage: 0.8,
				},
				PreserveThinkingSuffix: func(model string) bool { return model == tt.blacklist },
			}}
			got, err := OpenAIResponsesRequestToClaudeMessages(context.Background(), meta, &dto.OpenAIResponsesRequest{
				Model:           tt.model,
				MaxOutputTokens: &maxTokens,
				Input:           mustRawMessage(t, "hello"),
			})
			require.NoError(t, err)
			assert.Equal(t, tt.model, got.Model)
			require.NotNil(t, got.Thinking)
		})
	}
}
