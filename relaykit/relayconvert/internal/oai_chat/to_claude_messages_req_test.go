package oaichat

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIChatRequestToClaudeMessagesPreservesBlockCacheControlAndStops(t *testing.T) {
	maxTokens := uint(256)
	cacheControl := json.RawMessage(`{"type":"ephemeral","ttl":"1h"}`)
	request := dto.GeneralOpenAIRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: &maxTokens,
		Stop:      []interface{}{"first-stop", "second-stop"},
		Messages: []dto.Message{
			{
				Role: "system",
				Content: []dto.MediaContent{
					{Type: dto.ContentTypeText, Text: "cached system", CacheControl: cacheControl},
				},
			},
			{
				Role: "user",
				Content: []dto.MediaContent{
					{Type: dto.ContentTypeText, Text: "cached prompt", CacheControl: cacheControl},
				},
			},
		},
	}

	got, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, request)
	require.NoError(t, err)
	assert.Equal(t, []string{"first-stop", "second-stop"}, got.StopSequences)

	system := got.ParseSystem()
	require.Len(t, system, 1)
	assert.Equal(t, "cached system", system[0].GetText())
	assert.JSONEq(t, string(cacheControl), string(system[0].CacheControl))

	require.Len(t, got.Messages, 1)
	parts, err := got.Messages[0].ParseContent()
	require.NoError(t, err)
	require.Len(t, parts, 1)
	assert.Equal(t, "cached prompt", parts[0].GetText())
	assert.JSONEq(t, string(cacheControl), string(parts[0].CacheControl))
}
