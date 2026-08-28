package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageCacheCreationJSONPresenceAndPrecedence(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantTokens  int
		wantPresent bool
		wantDetail  bool
	}{
		{
			name:        "absent",
			body:        `{"input_tokens":100,"input_tokens_details":{"cached_tokens":10}}`,
			wantTokens:  0,
			wantPresent: false,
			wantDetail:  false,
		},
		{
			name:        "detail precedence",
			body:        `{"cache_write_tokens":90,"prompt_tokens_details":{"cache_write_tokens":20},"input_tokens_details":{"cache_write_tokens":10,"cache_creation_tokens":30,"cached_creation_tokens":40}}`,
			wantTokens:  10,
			wantPresent: true,
			wantDetail:  true,
		},
		{
			name:        "top level precedence",
			body:        `{"cache_write_tokens":20,"cache_creation_input_tokens":30,"cache_write_input_tokens":40,"cache_creation_tokens":50}`,
			wantTokens:  20,
			wantPresent: true,
			wantDetail:  false,
		},
		{
			name:        "top level explicit zero wins",
			body:        `{"cache_write_tokens":0,"cache_creation_input_tokens":30}`,
			wantTokens:  0,
			wantPresent: true,
			wantDetail:  false,
		},
		{
			name:        "detail explicit zero wins over aliases",
			body:        `{"cache_write_tokens":90,"input_tokens_details":{"cache_write_tokens":0,"cache_creation_tokens":30,"cached_creation_tokens":40}}`,
			wantTokens:  0,
			wantPresent: true,
			wantDetail:  true,
		},
		{
			name:        "negative detail clamps and wins",
			body:        `{"cache_creation_input_tokens":90,"input_tokens_details":{"cache_write_tokens":-5,"cache_creation_tokens":30}}`,
			wantTokens:  0,
			wantPresent: true,
			wantDetail:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var usage Usage
			require.NoError(t, json.Unmarshal([]byte(tt.body), &usage))

			tokens, present := usage.GetCacheCreationTokensWithPresence()
			assert.Equal(t, tt.wantTokens, tokens)
			assert.Equal(t, tt.wantPresent, present)
			assert.Equal(t, tt.wantDetail, usage.HasAnyDetailCacheCreationTokensField())
		})
	}
}

func TestUsageCopyCacheCreationTokensPreservesExplicitZero(t *testing.T) {
	var source Usage
	require.NoError(t, json.Unmarshal([]byte(`{"cache_creation_input_tokens":19,"input_tokens_details":{"cache_write_tokens":0}}`), &source))

	destination := Usage{
		InputTokensDetails:          &InputTokenDetails{},
		ClaudeCacheCreation5mTokens: 7,
		ClaudeCacheCreation1hTokens: 11,
	}
	destination.CopyCacheCreationTokensFrom(&source)

	tokens, present := destination.GetCacheCreationTokensWithPresence()
	assert.Zero(t, tokens)
	assert.True(t, present)
	assert.True(t, destination.PromptTokensDetails.HasCacheWriteTokens)
	require.NotNil(t, destination.InputTokensDetails)
	assert.True(t, destination.InputTokensDetails.HasCacheWriteTokens)
	assert.True(t, destination.HasCacheWriteTokens)
	assert.Equal(t, 7, destination.ClaudeCacheCreation5mTokens)
	assert.Equal(t, 11, destination.ClaudeCacheCreation1hTokens)
}

func TestUsageCopyCacheCreationTokensWritesOneCanonicalAliasValue(t *testing.T) {
	var source Usage
	require.NoError(t, json.Unmarshal([]byte(`{"cache_write_tokens":90,"input_tokens_details":{"cache_creation_tokens":30,"cached_creation_tokens":40}}`), &source))

	destination := Usage{InputTokensDetails: &InputTokenDetails{}}
	destination.CopyCacheCreationTokensFrom(&source)

	assert.Equal(t, 30, destination.GetCacheCreationTokens())
	assert.Equal(t, 30, destination.CacheWriteTokens)
	assert.Equal(t, 30, destination.CacheCreationInputTokens)
	assert.Equal(t, 30, destination.PromptTokensDetails.CacheWriteTokens)
	assert.Equal(t, 30, destination.InputTokensDetails.CacheWriteTokens)
}

func TestUsageJSONUnmarshalKeepsResponseEnvelope(t *testing.T) {
	body := []byte(`{"id":"chatcmpl-test","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30,"cache_write_tokens":4}}`)

	var response OpenAITextResponse
	require.NoError(t, json.Unmarshal(body, &response))
	assert.Equal(t, "chatcmpl-test", response.Id)
	require.Len(t, response.Choices, 1)
	assert.Equal(t, 10, response.Usage.PromptTokens)
	assert.Equal(t, 4, response.Usage.GetCacheCreationTokens())
	assert.True(t, response.Usage.HasCacheWriteTokens)
}

func TestUsageJSONUnmarshalTracksCacheMetricPresence(t *testing.T) {
	var usage Usage
	require.NoError(t, json.Unmarshal([]byte(`{"prompt_tokens":0,"prompt_cache_hit_tokens":0,"prompt_tokens_details":{"cached_tokens":0}}`), &usage))

	assert.True(t, usage.HasPromptTokens)
	assert.True(t, usage.HasPromptCacheHitTokens)
	assert.True(t, usage.PromptTokensDetails.HasCachedTokens)
	assert.Zero(t, usage.PromptTokens)
	assert.Zero(t, usage.PromptCacheHitTokens)
	assert.Zero(t, usage.PromptTokensDetails.CachedTokens)

	var missing Usage
	require.NoError(t, json.Unmarshal([]byte(`{"prompt_tokens_details":{}}`), &missing))
	assert.False(t, missing.HasPromptTokens)
	assert.False(t, missing.HasPromptCacheHitTokens)
	assert.False(t, missing.PromptTokensDetails.HasCachedTokens)
}

func TestUsageJSONUnmarshalKeepsEmbeddingEnvelope(t *testing.T) {
	body := []byte(`{"object":"list","model":"embedding-test","data":[{"object":"embedding","index":0,"embedding":[0.1,0.2]}],"usage":{"prompt_tokens":10,"cache_creation_input_tokens":4}}`)

	var response EmbeddingResponse
	require.NoError(t, json.Unmarshal(body, &response))
	assert.Equal(t, "list", response.Object)
	assert.Equal(t, "embedding-test", response.Model)
	require.Len(t, response.Data, 1)
	assert.Equal(t, 10, response.Usage.PromptTokens)
	assert.Equal(t, 4, response.Usage.GetCacheCreationTokens())
}
