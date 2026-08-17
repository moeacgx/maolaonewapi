package oaichat

import (
	"context"
	"encoding/json"
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

func TestOpenAIChatRequestToClaudeMessagesMinimalReasoningRespectsMaxTokens(t *testing.T) {
	tests := []struct {
		name          string
		maxTokens     *uint
		wantMaxTokens uint
		wantError     bool
	}{
		{name: "absent", wantMaxTokens: 1280},
		{name: "low", maxTokens: kitutil.GetPointer(uint(512)), wantError: true},
		{name: "equal to budget", maxTokens: kitutil.GetPointer(uint(1024)), wantError: true},
		{name: "sufficient", maxTokens: kitutil.GetPointer(uint(1025)), wantMaxTokens: 1025},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, dto.GeneralOpenAIRequest{
				Model:           "claude-sonnet-4-6",
				MaxTokens:       tt.maxTokens,
				ReasoningEffort: "minimal",
				Messages:        []dto.Message{{Role: "user", Content: "hello"}},
			})
			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "budget_tokens (1024)")
				assert.Contains(t, err.Error(), "max_tokens")
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

func TestOpenAIChatRequestToClaudeMessagesThinkingSuffixOnlyRewritesSupportedFamilies(t *testing.T) {
	meta := &convmeta.Values{
		Options: &convmeta.Options{
			Claude: convmeta.ClaudeOptions{
				ThinkingAdapterEnabled:                true,
				ThinkingAdapterBudgetTokensPercentage: 0.8,
			},
		},
	}
	maxTokens := uint(8192)
	tests := []struct {
		name         string
		model        string
		wantModel    string
		wantType     string
		wantThinking bool
	}{
		{
			name:         "opus 4.6 thinking",
			model:        "claude-opus-4-6-thinking",
			wantModel:    "claude-opus-4-6",
			wantType:     "enabled",
			wantThinking: true,
		},
		{
			name:         "opus 4.8 dated alias thinking",
			model:        "claude-opus-4-8-20260801-thinking",
			wantModel:    "claude-opus-4-8-20260801",
			wantType:     "adaptive",
			wantThinking: true,
		},
		{
			name:      "opus 4.60 collision",
			model:     "claude-opus-4-60-thinking",
			wantModel: "claude-opus-4-60-thinking",
		},
		{
			name:      "unrelated slash Claude collision",
			model:     "claude/custom-thinking",
			wantModel: "claude/custom-thinking",
		},
		{
			name:      "unrelated Claude collision",
			model:     "claude-custom-thinking",
			wantModel: "claude-custom-thinking",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := OpenAIChatRequestToClaudeMessages(context.Background(), meta, dto.GeneralOpenAIRequest{
				Model:     tt.model,
				MaxTokens: &maxTokens,
				Messages:  []dto.Message{{Role: "user", Content: "hello"}},
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantModel, got.Model)
			if tt.wantThinking {
				require.NotNil(t, got.Thinking)
				assert.Equal(t, tt.wantType, got.Thinking.Type)
			} else {
				assert.Nil(t, got.Thinking)
			}
		})
	}
}
