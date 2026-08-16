package claude

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	rootconstant "github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertOpenAIResponsesRequestUsesRelaykitAndNormalizesSampling(t *testing.T) {
	c, _ := newClaudeBridgeContext(t)
	maxTokens := uint(256)
	temperature := 0.7
	topP := 0.9
	request := dto.OpenAIResponsesRequest{
		Model:           "claude-sonnet-4-6",
		MaxOutputTokens: &maxTokens,
		Temperature:     &temperature,
		TopP:            &topP,
		Input: mustClaudeBridgeRaw(t, []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": "cached prompt", "cache_control": map[string]any{"type": "ephemeral"}},
				},
			},
		}),
		Tools: mustClaudeBridgeRaw(t, []map[string]any{
			{"type": "function", "name": "lookup", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}},
		}),
	}
	info := newClaudeBridgeInfo(false)

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(c, info, request)
	require.NoError(t, err)
	claudeRequest, ok := converted.(*dto.ClaudeRequest)
	require.True(t, ok)
	assert.Nil(t, claudeRequest.Temperature)
	assert.Nil(t, claudeRequest.TopP)
	assert.Nil(t, claudeRequest.TopK)
	assert.Equal(t, types.RelayFormat(types.RelayFormatClaude), info.FinalRequestRelayFormat)
	require.Len(t, claudeRequest.GetTools(), 1)
	require.Len(t, claudeRequest.Messages, 1)
	parts, err := claudeRequest.Messages[0].ParseContent()
	require.NoError(t, err)
	require.Len(t, parts, 1)
	assert.JSONEq(t, `{"type":"ephemeral"}`, string(parts[0].CacheControl))
}

func TestConvertOpenAIResponsesRequestRejectsLossyCustomTool(t *testing.T) {
	c, _ := newClaudeBridgeContext(t)
	maxTokens := uint(256)
	_, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(c, newClaudeBridgeInfo(false), dto.OpenAIResponsesRequest{
		Model:           "claude-sonnet-4-6",
		MaxOutputTokens: &maxTokens,
		Input: mustClaudeBridgeRaw(t, []map[string]any{
			{"type": "custom_tool_call", "call_id": "call_1", "name": "apply_patch", "input": "patch"},
		}),
	})
	require.ErrorContains(t, err, "cannot safely represent")
}

func TestClaudeMessagesToResponsesHandlerPreservesOrderedOutputsAndCacheUsage(t *testing.T) {
	c, recorder := newClaudeBridgeContext(t)
	response := map[string]any{
		"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-sonnet-4-6",
		"content": []map[string]any{
			{"type": "thinking", "thinking": "reasoning"},
			{"type": "text", "text": "hello"},
			{"type": "tool_use", "id": "call_1", "name": "lookup", "input": map[string]any{"q": "x"}},
		},
		"stop_reason": "tool_use",
		"usage": map[string]any{
			"input_tokens": 6, "cache_creation_input_tokens": 3, "cache_read_input_tokens": 4, "output_tokens": 2,
			"cache_creation": map[string]any{"ephemeral_5m_input_tokens": 2, "ephemeral_1h_input_tokens": 1},
		},
	}
	body, err := common.Marshal(response)
	require.NoError(t, err)

	usage, relayErr := ClaudeMessagesToResponsesHandler(c, newClaudeBridgeInfo(false), &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	})
	require.Nil(t, relayErr)
	require.NotNil(t, usage)
	assert.Equal(t, 13, usage.PromptTokens)
	require.NotNil(t, usage.InputTokensDetails)
	assert.Equal(t, 4, usage.InputTokensDetails.CachedTokens)
	assert.Equal(t, 3, usage.PromptTokensDetails.CacheCreationTokensTotal())

	var result dto.OpenAIResponsesResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
	assert.Equal(t, "response", result.Object)
	require.Len(t, result.Output, 3)
	assert.Equal(t, "message", result.Output[0].Type)
	assert.Equal(t, "hello", result.Output[0].Content[0].Text)
	assert.Equal(t, "reasoning", result.Output[1].Type)
	assert.Equal(t, "reasoning", result.Output[1].Content[0].Text)
	assert.Equal(t, "function_call", result.Output[2].Type)
	assert.Equal(t, "lookup", result.Output[2].Name)
	require.NotNil(t, result.Usage)
	assert.Equal(t, 13, result.Usage.InputTokens)
	assert.Equal(t, 2, result.Usage.OutputTokens)
	assert.Equal(t, 3, result.Usage.InputTokensDetails.CacheCreationTokensTotal())
}

func TestClaudeMessagesToResponsesStreamHandlerEmitsOrderedToolAndCacheEvents(t *testing.T) {
	c, recorder := newClaudeBridgeContext(t)
	body := strings.Join([]string{
		`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-6","usage":{"input_tokens":6,"cache_creation_input_tokens":3,"cache_read_input_tokens":4,"output_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":2,"ephemeral_1h_input_tokens":1}}}}`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call_1","name":"lookup","input":{}}}`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"q\":\"x\"}"}}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":"opaque-stop"},"usage":{"output_tokens":2}}`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")

	oldTimeout := rootconstant.StreamingTimeout
	rootconstant.StreamingTimeout = 30
	t.Cleanup(func() { rootconstant.StreamingTimeout = oldTimeout })
	usage, relayErr := ClaudeMessagesToResponsesStreamHandler(c, newClaudeBridgeInfo(true), &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
	})
	require.Nil(t, relayErr)
	require.NotNil(t, usage)
	assert.Equal(t, 6, usage.PromptTokens)
	assert.Equal(t, 2, usage.CompletionTokens)
	assert.Equal(t, 4, usage.PromptTokensDetails.CachedTokens)
	assert.Equal(t, 3, usage.PromptTokensDetails.CacheCreationTokensTotal())

	output := recorder.Body.String()
	assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	requireOrderedClaudeBridgeEvents(t, output,
		"event: response.created",
		"event: response.output_item.added",
		"event: response.output_text.delta",
		"event: response.output_item.added",
		"event: response.function_call_arguments.delta",
		"event: response.output_text.done",
		"event: response.completed",
	)
	assert.Contains(t, output, `"name":"lookup"`)
	assert.Contains(t, output, `"input_tokens":13`)
	assert.Contains(t, output, `"output_tokens":2`)
}

func newClaudeBridgeContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(common.RequestIdKey, "claude-bridge-test")
	return c, recorder
}

func newClaudeBridgeInfo(stream bool) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		IsStream:        stream,
		RelayMode:       relayconstant.RelayModeResponses,
		RelayFormat:     types.RelayFormatOpenAIResponses,
		RequestURLPath:  "/v1/responses",
		DisablePing:     true,
		OriginModelName: "claude-sonnet-4-6",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "claude-sonnet-4-6",
		},
	}
}

func mustClaudeBridgeRaw(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := common.Marshal(value)
	require.NoError(t, err)
	return raw
}

func requireOrderedClaudeBridgeEvents(t *testing.T, body string, events ...string) {
	t.Helper()
	offset := 0
	for _, event := range events {
		index := strings.Index(body[offset:], event)
		require.NotEqualf(t, -1, index, "missing %q after byte %d", event, offset)
		offset += index + len(event)
	}
}
