package openai

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeOpenAIUsageTokenCounts(t *testing.T) {
	tests := []struct {
		name     string
		usage    dto.Usage
		modified bool
		prompt   int
		output   int
		total    int
	}{
		{name: "standard", usage: dto.Usage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30}, prompt: 10, output: 20, total: 30},
		{name: "input-output aliases", usage: dto.Usage{InputTokens: 10, OutputTokens: 20, TotalTokens: 30}, modified: true, prompt: 10, output: 20, total: 30},
		{name: "derive output from total", usage: dto.Usage{PromptTokens: 10, TotalTokens: 30}, modified: true, prompt: 10, output: 20, total: 30},
		{name: "derive input from total", usage: dto.Usage{CompletionTokens: 20, TotalTokens: 30}, modified: true, prompt: 10, output: 20, total: 30},
		{name: "standard fields win", usage: dto.Usage{PromptTokens: 10, InputTokens: 11, CompletionTokens: 20, OutputTokens: 21, TotalTokens: 40}, modified: true, prompt: 10, output: 20, total: 30},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			modified := normalizeOpenAIUsageTokenCounts(&test.usage)
			require.Equal(t, test.modified, modified)
			require.Equal(t, test.prompt, test.usage.PromptTokens)
			require.Equal(t, test.output, test.usage.CompletionTokens)
			require.Equal(t, test.total, test.usage.TotalTokens)
		})
	}
}

func TestFillMissingOpenAIChatUsagePrefersUpstreamTotal(t *testing.T) {
	usage := &dto.Usage{TotalTokens: 30}
	require.True(t, fillMissingOpenAIChatUsage(usage, 10, 99))
	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 20, usage.CompletionTokens)
	require.Equal(t, 30, usage.TotalTokens)
}

func TestOpenaiHandlerNormalizesAliasUsageAndPreservesUnknownFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"id":"chatcmpl-test","object":"chat.completion","model":"gpt-test","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"input_tokens":788,"output_tokens":589,"total_tokens":1377,"vendor_metric":42}}`)
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAI,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: "gpt-test"},
	}
	usage, apiErr := OpenaiHandler(c, info, &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body))})
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, 788, usage.PromptTokens)
	require.Equal(t, 589, usage.CompletionTokens)
	require.Equal(t, 1377, usage.TotalTokens)
	assert.EqualValues(t, 589, gjson.GetBytes(recorder.Body.Bytes(), "usage.completion_tokens").Int())
	assert.EqualValues(t, 42, gjson.GetBytes(recorder.Body.Bytes(), "usage.vendor_metric").Int())
}

func TestNormalizeOpenAIStreamUsageDataPatchesAliasesOnly(t *testing.T) {
	data := `{"id":"chunk","choices":[],"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5,"vendor_metric":9}}`
	got := normalizeOpenAIStreamUsageData(data)
	assert.EqualValues(t, 3, gjson.Get(got, "usage.prompt_tokens").Int())
	assert.EqualValues(t, 2, gjson.Get(got, "usage.completion_tokens").Int())
	assert.EqualValues(t, 5, gjson.Get(got, "usage.total_tokens").Int())
	assert.EqualValues(t, 9, gjson.Get(got, "usage.vendor_metric").Int())
}

func TestOpenAIImageUsageNormalizationPreservesDetails(t *testing.T) {
	usage := &dto.Usage{InputTokens: 3, OutputTokens: 4, InputTokensDetails: &dto.InputTokenDetails{ImageTokens: 2, TextTokens: 1}}
	normalizeOpenAIUsage(usage)
	require.Equal(t, 3, usage.PromptTokens)
	require.Equal(t, 4, usage.CompletionTokens)
	require.Equal(t, 7, usage.TotalTokens)
	require.Equal(t, 2, usage.PromptTokensDetails.ImageTokens)
}

func TestOpenAIUsageJSONRoundTripUsesCommonJSON(t *testing.T) {
	var usage dto.Usage
	require.NoError(t, common.Unmarshal([]byte(`{"input_tokens":1,"output_tokens":2,"total_tokens":3}`), &usage))
	require.True(t, normalizeAndValidateOpenAIUsage(&usage))
	assert.Equal(t, 1, usage.PromptTokens)
	assert.Equal(t, 2, usage.CompletionTokens)
}

func TestOaiStreamHandlerDoesNotReinjectAudioUsageFromSecondLastChunk(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	originalStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 60
	t.Cleanup(func() { constant.StreamingTimeout = originalStreamingTimeout })
	info := &relaycommon.RelayInfo{
		ChannelMeta:        &relaycommon.ChannelMeta{UpstreamModelName: "gpt-audio-preview"},
		RelayFormat:        types.RelayFormatOpenAI,
		ShouldIncludeUsage: true,
	}
	body := "data: {\"id\":\"audio\",\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n" +
		"data: {\"id\":\"audio\",\"choices\":[],\"usage\":{\"input_tokens\":2,\"output_tokens\":3,\"total_tokens\":5}}\n\n" +
		"data: {\"id\":\"audio\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"

	usage, apiErr := OaiStreamHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	})
	require.Nil(t, apiErr)
	require.Equal(t, 5, usage.TotalTokens)
	require.Equal(t, 1, bytes.Count(recorder.Body.Bytes(), []byte(`"usage":`)))
}

func TestOaiStreamHandlerKeepsFinalToolCallChunkWithUsageWhenNotRequested(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	originalStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 60
	t.Cleanup(func() { constant.StreamingTimeout = originalStreamingTimeout })
	info := &relaycommon.RelayInfo{
		ChannelMeta:        &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.6-sol"},
		RelayFormat:        types.RelayFormatOpenAI,
		ShouldIncludeUsage: false,
	}
	body := "data: {\"id\":\"chatcmpl-tool\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-5.6-sol\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"lookup\",\"arguments\":\"{}\"}}]},\"finish_reason\":\"tool_calls\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\n" +
		"data: [DONE]\n\n"

	usage, apiErr := OaiStreamHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 3, usage.PromptTokens)
	assert.Equal(t, 2, usage.CompletionTokens)
	assert.Equal(t, 5, usage.TotalTokens)
	assert.Contains(t, recorder.Body.String(), `"tool_calls"`)
	assert.Contains(t, recorder.Body.String(), `"name":"lookup"`)
	assert.Contains(t, recorder.Body.String(), `"finish_reason":"tool_calls"`)
	assert.Contains(t, recorder.Body.String(), "data: [DONE]")
}
