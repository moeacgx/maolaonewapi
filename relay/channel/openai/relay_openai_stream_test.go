package openai

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupOaiStreamTest(body io.Reader) (*gin.Context, *httptest.ResponseRecorder, *relaycommon.RelayInfo, *http.Response) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{},
		RelayFormat: types.RelayFormatOpenAI,
		RelayMode:   relayconstant.RelayModeChatCompletions,
		StartTime:   time.Now(),
	}

	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(body)}
	return c, recorder, info, resp
}

func chatCompletionSSE(data string) string {
	return fmt.Sprintf("data: %s\n", data)
}

func TestOaiStreamHandlerForwardsCurrentChunkImmediately(t *testing.T) {
	pr, pw := io.Pipe()
	c, recorder, info, resp := setupOaiStreamTest(pr)

	done := make(chan *types.NewAPIError, 1)
	go func() {
		_, err := OaiStreamHandler(c, info, resp)
		done <- err
	}()

	firstChunk := `{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"first"},"finish_reason":null}]}`
	_, err := pw.Write([]byte(chatCompletionSSE(firstChunk)))
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return strings.Contains(recorder.Body.String(), `"content":"first"`)
	}, 500*time.Millisecond, 10*time.Millisecond, "first content chunk should be sent before the next SSE chunk")

	_, err = pw.Write([]byte("data: [DONE]\n"))
	require.NoError(t, err)
	require.NoError(t, pw.Close())
	require.Nil(t, <-done)
}

func TestOaiStreamHandlerDoesNotDuplicateLastChunk(t *testing.T) {
	body := strings.Join([]string{
		chatCompletionSSE(`{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"last"},"finish_reason":null}]}`),
		"data: [DONE]\n",
	}, "")
	c, recorder, info, resp := setupOaiStreamTest(strings.NewReader(body))

	usage, err := OaiStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)

	output := recorder.Body.String()
	assert.Equal(t, 1, strings.Count(output, `"content":"last"`))
	assert.Contains(t, output, "[DONE]")
}

func TestOaiStreamHandlerSkipsUsageOnlyChunkWhenNotRequested(t *testing.T) {
	body := strings.Join([]string{
		chatCompletionSSE(`{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"body"},"finish_reason":null}]}`),
		chatCompletionSSE(`{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`),
		"data: [DONE]\n",
	}, "")
	c, recorder, info, resp := setupOaiStreamTest(strings.NewReader(body))
	info.ShouldIncludeUsage = false

	usage, err := OaiStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.Equal(t, &dto.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}, usage)

	output := recorder.Body.String()
	assert.Contains(t, output, `"content":"body"`)
	assert.NotContains(t, output, `"usage":{"prompt_tokens":3`)
	assert.Contains(t, output, "[DONE]")
}

func TestOaiStreamHandlerDoesNotBillEmptyStream(t *testing.T) {
	body := strings.Join([]string{
		chatCompletionSSE(`{"id":"chatcmpl-empty","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`),
		chatCompletionSSE(`{"id":"chatcmpl-empty","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"content":""},"finish_reason":"stop"}]}`),
		chatCompletionSSE(`{"id":"chatcmpl-empty","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[],"usage":{"prompt_tokens":291300,"completion_tokens":0,"total_tokens":291300}}`),
		"data: [DONE]\n",
	}, "")
	c, _, info, resp := setupOaiStreamTest(strings.NewReader(body))
	info.ShouldIncludeUsage = true

	usage, err := OaiStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	assert.Equal(t, 0, usage.PromptTokens)
	assert.Equal(t, 0, usage.CompletionTokens)
	assert.Equal(t, 0, usage.TotalTokens)
}

func TestOaiStreamHandlerReturnsCapacityErrorBeforeWriting(t *testing.T) {
	body := chatCompletionSSE(`{"error":{"type":"server_error","code":"server_error","message":"Selected model is at capacity. Please try a different model."}}`)
	c, recorder, info, resp := setupOaiStreamTest(strings.NewReader(body))

	usage, relayErr := OaiStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, relayErr)
	require.True(t, types.IsUpstreamCapacityError(relayErr))
	require.Equal(t, http.StatusTooManyRequests, relayErr.StatusCode)
	require.Equal(t, types.UpstreamCapacityClientMessage, relayErr.ToOpenAIError().Message)
	require.Equal(t, http.StatusOK, relayErr.OriginalStatusCode)
	require.False(t, c.Writer.Written())
	require.Empty(t, recorder.Body.String())
	require.Equal(t, 1, info.ReceivedResponseCount)
}

func TestOaiStreamHandlerRetriesCapacityErrorAfterRoleOnlyChunk(t *testing.T) {
	body := strings.Join([]string{
		chatCompletionSSE(`{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`),
		chatCompletionSSE(`{"error":{"type":"server_error","code":"server_error","message":"Selected model is at capacity. Please try a different model."}}`),
	}, "")
	c, recorder, info, resp := setupOaiStreamTest(strings.NewReader(body))

	usage, relayErr := OaiStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, relayErr)
	require.True(t, types.IsUpstreamCapacityError(relayErr))
	require.False(t, c.Writer.Written())
	require.Empty(t, recorder.Body.String())
}

func TestOaiStreamHandlerFlushesRoleBeforeActualOutput(t *testing.T) {
	body := strings.Join([]string{
		chatCompletionSSE(`{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`),
		chatCompletionSSE(`{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}]}`),
		"data: [DONE]\n",
	}, "")
	c, recorder, info, resp := setupOaiStreamTest(strings.NewReader(body))

	usage, relayErr := OaiStreamHandler(c, info, resp)

	require.Nil(t, relayErr)
	require.NotNil(t, usage)
	responseBody := recorder.Body.String()
	roleIndex := strings.Index(responseBody, `"role":"assistant"`)
	contentIndex := strings.Index(responseBody, `"content":"hello"`)
	require.GreaterOrEqual(t, roleIndex, 0)
	require.Greater(t, contentIndex, roleIndex)
}

func TestOaiStreamHandlerStopsRetryAfterRoleBufferLimit(t *testing.T) {
	chunks := make([]string, 0, maxProvisionalChatStreamChunks+2)
	for index := 0; index <= maxProvisionalChatStreamChunks; index++ {
		chunks = append(chunks, chatCompletionSSE(fmt.Sprintf(
			`{"id":"chatcmpl-%d","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
			index,
		)))
	}
	chunks = append(chunks, chatCompletionSSE(`{"error":{"type":"server_error","code":"server_error","message":"Selected model is at capacity. Please try a different model."}}`))
	c, recorder, info, resp := setupOaiStreamTest(strings.NewReader(strings.Join(chunks, "")))

	usage, relayErr := OaiStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, relayErr)
	require.True(t, types.IsUpstreamCapacityError(relayErr))
	require.True(t, c.Writer.Written())
	require.Equal(t, maxProvisionalChatStreamChunks+1, strings.Count(recorder.Body.String(), `"role":"assistant"`))
	require.Equal(t, 1, strings.Count(recorder.Body.String(), types.UpstreamCapacityClientMessage))
}

func TestOaiStreamHandlerStopsRetryAfterRoleBufferByteLimit(t *testing.T) {
	largeID := strings.Repeat("x", maxProvisionalChatStreamBytes)
	body := strings.Join([]string{
		chatCompletionSSE(fmt.Sprintf(
			`{"id":"%s","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
			largeID,
		)),
		chatCompletionSSE(`{"error":{"type":"server_error","code":"server_error","message":"Selected model is at capacity. Please try a different model."}}`),
	}, "")
	c, recorder, info, resp := setupOaiStreamTest(strings.NewReader(body))

	usage, relayErr := OaiStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, relayErr)
	require.True(t, types.IsUpstreamCapacityError(relayErr))
	require.True(t, c.Writer.Written())
	require.Greater(t, recorder.Body.Len(), maxProvisionalChatStreamBytes)
	require.Equal(t, 1, strings.Count(recorder.Body.String(), types.UpstreamCapacityClientMessage))
}

func TestProvisionalChatStreamResponseRejectsUnmodeledOutputFields(t *testing.T) {
	tests := []struct {
		name        string
		delta       string
		choiceExtra string
		provisional bool
	}{
		{name: "role only", delta: `{"role":"assistant"}`, provisional: true},
		{name: "empty delta", delta: `{}`, provisional: true},
		{name: "empty content", delta: `{"content":""}`, provisional: true},
		{name: "audio", delta: `{"audio":{"id":"audio_1","data":"AAAA"}}`},
		{name: "legacy function call", delta: `{"function_call":{"name":"lookup","arguments":"{}"}}`},
		{name: "refusal", delta: `{"refusal":"blocked"}`},
		{name: "logprobs", delta: `{}`, choiceExtra: `,"logprobs":{"content":[{"token":"x"}]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := fmt.Sprintf(
				`{"id":"chatcmpl-test","object":"chat.completion.chunk","choices":[{"index":0,"delta":%s%s,"finish_reason":null}]}`,
				tt.delta,
				tt.choiceExtra,
			)
			var response dto.ChatCompletionsStreamResponse
			require.NoError(t, common.UnmarshalJsonStr(data, &response))
			require.Equal(t, tt.provisional, isProvisionalChatStreamResponse(data, &response))
		})
	}
}

func TestOaiStreamHandlerForwardsUnmodeledAudioBeforeCapacityError(t *testing.T) {
	body := strings.Join([]string{
		chatCompletionSSE(`{"id":"chatcmpl-audio","object":"chat.completion.chunk","created":1,"model":"gpt-audio","choices":[{"index":0,"delta":{"audio":{"id":"audio_1","data":"AAAA"}},"finish_reason":null}]}`),
		chatCompletionSSE(`{"error":{"type":"server_error","code":"server_error","message":"Selected model is at capacity. Please try a different model."}}`),
	}, "")
	c, recorder, info, resp := setupOaiStreamTest(strings.NewReader(body))
	info.UpstreamModelName = "gpt-audio"

	usage, relayErr := OaiStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, relayErr)
	require.True(t, types.IsUpstreamCapacityError(relayErr))
	require.True(t, c.Writer.Written())
	require.Contains(t, recorder.Body.String(), `"audio":{"id":"audio_1","data":"AAAA"}`)
	require.Equal(t, 1, strings.Count(recorder.Body.String(), types.UpstreamCapacityClientMessage))
}

func TestOaiStreamHandlerForwardsCapacityErrorAfterActualOutput(t *testing.T) {
	body := strings.Join([]string{
		chatCompletionSSE(`{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":null}]}`),
		chatCompletionSSE(`{"error":{"type":"server_error","code":"server_error","message":"Selected model is at capacity. Please try a different model."}}`),
	}, "")
	c, recorder, info, resp := setupOaiStreamTest(strings.NewReader(body))

	usage, relayErr := OaiStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, relayErr)
	require.True(t, types.IsUpstreamCapacityError(relayErr))
	require.True(t, c.Writer.Written())
	responseBody := recorder.Body.String()
	require.Contains(t, responseBody, `"content":"partial"`)
	require.Equal(t, 1, strings.Count(responseBody, types.UpstreamCapacityClientMessage))
	require.NotContains(t, responseBody, "[DONE]")
}

func TestOaiStreamHandlerRetriesCapacityErrorWhileFirstOutputIsHeld(t *testing.T) {
	body := strings.Join([]string{
		chatCompletionSSE(`{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"Master"},"finish_reason":null}]}`),
		chatCompletionSSE(`{"error":{"type":"server_error","code":"server_error","message":"Selected model is at capacity. Please try a different model."}}`),
	}, "")
	c, recorder, info, resp := setupOaiStreamTest(strings.NewReader(body))
	withOpenAIStreamSensitiveRule(t, c, "Master Key")

	usage, relayErr := OaiStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, relayErr)
	require.True(t, types.IsUpstreamCapacityError(relayErr))
	require.False(t, c.Writer.Written())
	require.Empty(t, recorder.Body.String())
}

func TestOaiStreamHandlerKeepsToolCallChunkWithUsageWhenNotRequested(t *testing.T) {
	body := strings.Join([]string{
		chatCompletionSSE(`{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":null}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`),
		"data: [DONE]\n",
	}, "")
	c, recorder, info, resp := setupOaiStreamTest(strings.NewReader(body))
	info.ShouldIncludeUsage = false

	usage, err := OaiStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.Equal(t, &dto.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}, usage)

	output := recorder.Body.String()
	assert.Contains(t, output, `"tool_calls"`)
	assert.Contains(t, output, `"name":"lookup"`)
	assert.Contains(t, output, `[DONE]`)
}

func TestOaiStreamHandlerClaudeSingleContentChunkClosesStream(t *testing.T) {
	body := strings.Join([]string{
		chatCompletionSSE(`{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}]}`),
		"data: [DONE]\n",
	}, "")
	c, recorder, info, resp := setupOaiStreamTest(strings.NewReader(body))
	info.RelayFormat = types.RelayFormatClaude
	info.ClaudeConvertInfo = &relaycommon.ClaudeConvertInfo{
		LastMessagesType: relaycommon.LastMessageTypeNone,
	}

	usage, err := OaiStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)

	output := recorder.Body.String()
	assert.Contains(t, output, "event: message_start")
	assert.Contains(t, output, "event: content_block_start")
	assert.Contains(t, output, "event: content_block_delta")
	assert.Contains(t, output, "event: content_block_stop")
	assert.Contains(t, output, "event: message_delta")
	assert.Contains(t, output, "event: message_stop")
	assert.Contains(t, output, `"stop_reason":"end_turn"`)
	assert.Equal(t, 1, strings.Count(output, `"text":"hello"`))
}
