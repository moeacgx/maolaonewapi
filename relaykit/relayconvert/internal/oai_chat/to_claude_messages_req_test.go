package oaichat

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIChatRequestToClaudeMessagesPreservesDecodedBlockCacheControlAndStops(t *testing.T) {
	var request dto.GeneralOpenAIRequest
	require.NoError(t, kitutil.Unmarshal([]byte(`{
		"model":"claude-sonnet-4-6",
		"max_tokens":256,
		"stop":["first-stop","second-stop"],
		"messages":[
			{"role":"system","content":[{"type":"text","text":"cached system","cache_control":{"type":"ephemeral","ttl":"1h"}}]},
			{"role":"user","content":[{"type":"text","text":"cached prompt","cache_control":{"type":"ephemeral","ttl":"1h"}}]}
		]
	}`), &request))

	got, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, request)
	require.NoError(t, err)
	assert.Equal(t, []string{"first-stop", "second-stop"}, got.StopSequences)

	system := got.ParseSystem()
	require.Len(t, system, 1)
	assert.Equal(t, "cached system", system[0].GetText())
	assert.JSONEq(t, `{"type":"ephemeral","ttl":"1h"}`, string(system[0].CacheControl))

	require.Len(t, got.Messages, 1)
	parts, err := got.Messages[0].ParseContent()
	require.NoError(t, err)
	require.Len(t, parts, 1)
	assert.Equal(t, "cached prompt", parts[0].GetText())
	assert.JSONEq(t, `{"type":"ephemeral","ttl":"1h"}`, string(parts[0].CacheControl))
}

func TestMessageParseContentPreservesDecodedCacheControlForRepresentableMedia(t *testing.T) {
	var request dto.GeneralOpenAIRequest
	require.NoError(t, kitutil.Unmarshal([]byte(`{
		"model":"claude-sonnet-4-6",
		"messages":[{"role":"user","content":[
			{"type":"text","text":"text","cache_control":{"type":"ephemeral"}},
			{"type":"image_url","image_url":{"url":"data:image/png;base64,aGVsbG8="},"cache_control":{"type":"ephemeral"}},
			{"type":"input_audio","input_audio":{"data":"aGVsbG8=","format":"wav"},"cache_control":{"type":"ephemeral"}},
			{"type":"file","file":{"filename":"note.pdf","file_data":"data:application/pdf;base64,aGVsbG8="},"cache_control":{"type":"ephemeral"}},
			{"type":"video_url","video_url":"https://example.test/video.mp4","cache_control":{"type":"ephemeral"}}
		]}]
	}`), &request))

	require.Len(t, request.Messages, 1)
	parts := request.Messages[0].ParseContent()
	require.Len(t, parts, 5)
	assert.Equal(t, dto.ContentTypeText, parts[0].Type)
	assert.Equal(t, dto.ContentTypeImageURL, parts[1].Type)
	assert.Equal(t, dto.ContentTypeInputAudio, parts[2].Type)
	assert.Equal(t, dto.ContentTypeFile, parts[3].Type)
	assert.Equal(t, dto.ContentTypeVideoUrl, parts[4].Type)
	for _, part := range parts {
		assert.JSONEq(t, `{"type":"ephemeral"}`, string(part.CacheControl))
	}
}

func TestOpenAIChatRequestToClaudeMessagesCapsCacheControlInEmittedOrder(t *testing.T) {
	maxTokens := uint(256)
	cacheControl := json.RawMessage(`{"type":"ephemeral"}`)
	parts := func(prefix string) []dto.MediaContent {
		return []dto.MediaContent{
			{Type: dto.ContentTypeText, Text: prefix + "-1", CacheControl: cacheControl},
			{Type: dto.ContentTypeText, Text: prefix + "-2", CacheControl: cacheControl},
			{Type: dto.ContentTypeText, Text: prefix + "-3", CacheControl: cacheControl},
		}
	}
	request := dto.GeneralOpenAIRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: &maxTokens,
		Messages: []dto.Message{
			{Role: "user", Content: parts("user")},
			{Role: "system", Content: parts("system")},
		},
	}

	got, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, request)
	require.NoError(t, err)
	system := got.ParseSystem()
	require.Len(t, system, 3)
	for i, part := range system {
		assert.Equal(t, "system-"+string(rune('1'+i)), part.GetText())
		assert.NotEmpty(t, part.CacheControl)
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

func TestOpenAIChatRequestToClaudeMessagesHandlesEveryNormalizedReasoningEffort(t *testing.T) {
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
			got, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, dto.GeneralOpenAIRequest{
				Model:           "claude-sonnet-4-6",
				MaxTokens:       &maxTokens,
				ReasoningEffort: tt.effort,
				Messages:        []dto.Message{{Role: "user", Content: "hello"}},
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

func TestOpenAIChatRequestToClaudeMessagesReasoningBudgetsUseFinalMaxTokens(t *testing.T) {
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
			got, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, dto.GeneralOpenAIRequest{
				Model:           "claude-sonnet-4-6",
				ReasoningEffort: effort.name,
				Messages:        []dto.Message{{Role: "user", Content: "hello"}},
			})
			require.NoError(t, err)
			require.NotNil(t, got.MaxTokens)
			require.NotNil(t, got.Thinking)
			assert.Equal(t, effort.implicitMaxTokens, *got.MaxTokens)
			assert.Equal(t, int(effort.budget), got.Thinking.GetBudgetTokens())
			assert.Less(t, uint(got.Thinking.GetBudgetTokens()), *got.MaxTokens)
		})

		t.Run(effort.name+"/explicit sufficient", func(t *testing.T) {
			maxTokens := effort.budget + 1
			got, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, dto.GeneralOpenAIRequest{
				Model:           "claude-sonnet-4-6",
				MaxTokens:       &maxTokens,
				ReasoningEffort: effort.name,
				Messages:        []dto.Message{{Role: "user", Content: "hello"}},
			})
			require.NoError(t, err)
			require.NotNil(t, got.MaxTokens)
			require.NotNil(t, got.Thinking)
			assert.Equal(t, maxTokens, *got.MaxTokens)
			assert.Less(t, uint(got.Thinking.GetBudgetTokens()), *got.MaxTokens)
		})

		for _, maxTokens := range []uint{effort.budget, effort.budget - 1} {
			t.Run(fmt.Sprintf("%s/explicit invalid %d", effort.name, maxTokens), func(t *testing.T) {
				got, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, dto.GeneralOpenAIRequest{
					Model:           "claude-sonnet-4-6",
					MaxTokens:       &maxTokens,
					ReasoningEffort: effort.name,
					Messages:        []dto.Message{{Role: "user", Content: "hello"}},
				})
				require.Error(t, err)
				assert.Nil(t, got)
				assert.Contains(t, err.Error(), fmt.Sprintf("budget_tokens (%d)", effort.budget))
				assert.Contains(t, err.Error(), fmt.Sprintf("max_tokens (%d)", maxTokens))
			})
		}
	}
}

func TestOpenAIChatRequestToClaudeMessagesBoundsRawReasoningMaxTokens(t *testing.T) {
	tests := []struct {
		name      string
		budget    int64
		wantError bool
	}{
		{name: "boundary minus one", budget: dto.MaxTokensLimit - 1},
		{name: "boundary", budget: dto.MaxTokensLimit, wantError: true},
		{name: "negative", budget: -1, wantError: true},
		{name: "oversized", budget: dto.MaxTokensLimit + 1, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, dto.GeneralOpenAIRequest{
				Model:     "claude-sonnet-4-6",
				Messages:  []dto.Message{{Role: "user", Content: "hello"}},
				Reasoning: json.RawMessage(fmt.Sprintf(`{"max_tokens":%d}`, tt.budget)),
			})
			if tt.wantError {
				require.Error(t, err)
				assert.Nil(t, got)
				assert.Contains(t, err.Error(), "reasoning.max_tokens is invalid")
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got.MaxTokens)
			require.NotNil(t, got.Thinking)
			assert.EqualValues(t, dto.MaxTokensLimit-1, got.Thinking.GetBudgetTokens())
			assert.EqualValues(t, dto.MaxTokensLimit, *got.MaxTokens)
			assert.Less(t, uint(got.Thinking.GetBudgetTokens()), *got.MaxTokens)
			assert.LessOrEqual(t, *got.MaxTokens, uint(dto.MaxTokensLimit))
		})
	}
}

func TestOpenAIChatRequestToClaudeMessagesThinkingAdapterValidatesFinalMaxTokens(t *testing.T) {
	meta := &convmeta.Values{Options: &convmeta.Options{Claude: convmeta.ClaudeOptions{
		ThinkingAdapterEnabled:                true,
		ThinkingAdapterBudgetTokensPercentage: 0.8,
	}}}
	clientMaxTokens := uint(1000)
	got, err := OpenAIChatRequestToClaudeMessages(context.Background(), meta, dto.GeneralOpenAIRequest{
		Model:     "claude-sonnet-4-6-thinking",
		MaxTokens: &clientMaxTokens,
		Messages:  []dto.Message{{Role: "user", Content: "hello"}},
	})
	require.NoError(t, err)
	require.NotNil(t, got.MaxTokens)
	require.NotNil(t, got.Thinking)
	assert.Equal(t, uint(1280), *got.MaxTokens)
	assert.Equal(t, 1024, got.Thinking.GetBudgetTokens())
	assert.Less(t, uint(got.Thinking.GetBudgetTokens()), *got.MaxTokens)
}

func TestOpenAIChatRequestToClaudeMessagesEffortSuffixOnlyRewritesCanonicalOpusModels(t *testing.T) {
	maxTokens := uint(8192)
	tests := []struct {
		name         string
		model        string
		wantModel    string
		wantThinking bool
	}{
		{name: "opus 4.7 exact", model: "claude-opus-4-7-high", wantModel: "claude-opus-4-7", wantThinking: true},
		{name: "opus 4.7 dated", model: "claude-opus-4-7-20260801-high", wantModel: "claude-opus-4-7-20260801", wantThinking: true},
		{name: "custom extra segment collision", model: "claude-opus-4-7-enterprise-high", wantModel: "claude-opus-4-7-enterprise-high"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, dto.GeneralOpenAIRequest{
				Model:     tt.model,
				MaxTokens: &maxTokens,
				Messages:  []dto.Message{{Role: "user", Content: "hello"}},
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantModel, got.Model)
			if !tt.wantThinking {
				assert.Nil(t, got.Thinking)
				assert.Empty(t, got.OutputConfig)
				return
			}
			require.NotNil(t, got.Thinking)
			assert.Equal(t, "adaptive", got.Thinking.Type)
			assert.Equal(t, "summarized", got.Thinking.Display)
			assert.JSONEq(t, `{"effort":"high"}`, string(got.OutputConfig))
		})
	}
}

func TestOpenAIChatRequestToClaudeMessagesThinkingSuffixOnlyRewritesSupportedFamilies(t *testing.T) {
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
			got, err := OpenAIChatRequestToClaudeMessages(context.Background(), meta, dto.GeneralOpenAIRequest{
				Model:     tt.model,
				MaxTokens: &maxTokens,
				Messages:  []dto.Message{{Role: "user", Content: "hello"}},
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
func TestOpenAIChatRequestToClaudeMessagesPreservesSuffixUsingOriginAndFallback(t *testing.T) {
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
			got, err := OpenAIChatRequestToClaudeMessages(context.Background(), meta, dto.GeneralOpenAIRequest{
				Model:     tt.model,
				MaxTokens: &maxTokens,
				Messages:  []dto.Message{{Role: "user", Content: "hello"}},
			})
			require.NoError(t, err)
			assert.Equal(t, tt.model, got.Model)
			require.NotNil(t, got.Thinking)
		})
	}
}
