package openai

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIResponsesViaChatPolicyConvertsRequestAndURL(t *testing.T) {
	info := newOpenAIResponsesViaChatInfo(false)
	info.ChannelBaseUrl = "https://chat-only.example"
	info.SupportStreamOptions = true
	settingsJSON, err := common.Marshal(info.ChannelOtherSettings)
	require.NoError(t, err)
	assert.JSONEq(t, `{"responses_to_chat_enabled":true}`, string(settingsJSON))
	stream := true
	request := dto.OpenAIResponsesRequest{
		Model:                "gpt-test",
		Input:                mustOpenAIResponsesRaw(t, "hello"),
		Stream:               &stream,
		PromptCacheKey:       mustOpenAIResponsesRaw(t, "cache-key"),
		PromptCacheRetention: mustOpenAIResponsesRaw(t, "24h"),
		FrequencyPenalty:     mustOpenAIResponsesRaw(t, 0.25),
		PresencePenalty:      mustOpenAIResponsesRaw(t, -0.5),
		Reasoning:            &dto.Reasoning{Effort: "extra-high"},
		ClientMetadata:       mustOpenAIResponsesRaw(t, map[string]any{"thread_id": "private-thread"}),
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, request)
	require.NoError(t, err)
	chatRequest, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	assert.Equal(t, "hello", chatRequest.Messages[0].StringContent())
	assert.Equal(t, "cache-key", chatRequest.PromptCacheKey)
	assert.JSONEq(t, `"24h"`, string(chatRequest.PromptCacheRetention))
	require.NotNil(t, chatRequest.FrequencyPenalty)
	assert.Equal(t, 0.25, *chatRequest.FrequencyPenalty)
	require.NotNil(t, chatRequest.PresencePenalty)
	assert.Equal(t, -0.5, *chatRequest.PresencePenalty)
	assert.Equal(t, "xhigh", chatRequest.ReasoningEffort)
	assert.Equal(t, "xhigh", info.ReasoningEffort)
	require.NotNil(t, chatRequest.StreamOptions)
	assert.True(t, chatRequest.StreamOptions.IncludeUsage)
	assert.Equal(t, types.RelayFormatOpenAI, info.FinalRequestRelayFormat)

	requestURL, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://chat-only.example/v1/chat/completions", requestURL)
}

func TestOpenAIResponsesViaChatPolicyHonorsStreamSupportInFinalUpstreamBody(t *testing.T) {
	for _, tt := range []struct {
		name                 string
		supportStreamOptions bool
		wantStreamOptions    bool
	}{
		{name: "supported", supportStreamOptions: true, wantStreamOptions: true},
		{name: "unsupported", supportStreamOptions: false, wantStreamOptions: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stream := true
			info := newOpenAIResponsesViaChatInfo(true)
			info.ChannelType = constant.ChannelTypeAdvancedCustom
			info.SupportStreamOptions = tt.supportStreamOptions

			converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{
				Model:  "gpt-test",
				Input:  mustOpenAIResponsesRaw(t, "hello"),
				Stream: &stream,
			})
			require.NoError(t, err)

			upstreamJSON, err := common.Marshal(converted)
			require.NoError(t, err)
			var upstreamBody struct {
				StreamOptions *dto.StreamOptions `json:"stream_options"`
			}
			require.NoError(t, common.Unmarshal(upstreamJSON, &upstreamBody))
			if tt.wantStreamOptions {
				require.NotNil(t, upstreamBody.StreamOptions)
				assert.True(t, upstreamBody.StreamOptions.IncludeUsage)
				assert.Contains(t, string(upstreamJSON), `"stream_options":{"include_usage":true}`)
			} else {
				assert.Nil(t, upstreamBody.StreamOptions)
				assert.NotContains(t, string(upstreamJSON), `"stream_options"`)
			}
		})
	}
}

func TestOpenAIResponsesNormalPolicyKeepsResponsesRouteAndDropsClientMetadata(t *testing.T) {
	info := newOpenAIResponsesViaChatInfo(false)
	info.ChannelOtherSettings.ResponsesToChatEnabled = false
	info.ChannelBaseUrl = "https://responses.example"
	request := dto.OpenAIResponsesRequest{
		Model:          "gpt-test",
		Input:          mustOpenAIResponsesRaw(t, "hello"),
		ClientMetadata: mustOpenAIResponsesRaw(t, map[string]any{"thread_id": "private-thread"}),
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, request)
	require.NoError(t, err)
	responsesRequest, ok := converted.(dto.OpenAIResponsesRequest)
	require.True(t, ok)
	assert.Empty(t, responsesRequest.ClientMetadata)
	assert.NotEmpty(t, request.ClientMetadata)
	assert.Empty(t, info.FinalRequestRelayFormat)

	requestURL, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://responses.example/v1/responses", requestURL)
}

func TestOpenAIResponsesViaChatPolicyRejectsAzureRouting(t *testing.T) {
	info := newOpenAIResponsesViaChatInfo(false)
	info.ChannelType = constant.ChannelTypeAzure

	requestURL, err := (&Adaptor{}).GetRequestURL(info)

	assert.Empty(t, requestURL)
	require.EqualError(t, err, "responses_to_chat_enabled is not supported for Azure channels")
}

func TestOpenAIResponsesViaChatPolicyRoutesNonStreamResponse(t *testing.T) {
	c, recorder := newOpenAIResponsesViaChatContext(t)
	info := newOpenAIResponsesViaChatInfo(false)
	body := []byte(`{
		"id":"chatcmpl_upstream","object":"chat.completion","created":1710000000,"model":"gpt-test",
		"choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}
	}`)

	usageValue, relayErr := (&Adaptor{}).DoResponse(c, &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}, info)
	require.Nil(t, relayErr)
	usage, ok := usageValue.(*dto.Usage)
	require.True(t, ok)
	assert.Equal(t, 10, usage.PromptTokens)
	assert.Equal(t, 2, usage.CompletionTokens)
	assert.Equal(t, 12, usage.TotalTokens)

	var response dto.OpenAIResponsesResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "response", response.Object)
	require.Len(t, response.Output, 1)
	assert.Equal(t, "hello", response.Output[0].Content[0].Text)
	require.NotNil(t, response.Usage)
	assert.Equal(t, 12, response.Usage.TotalTokens)
	assert.NotContains(t, recorder.Body.String(), `"choices"`)
}

func TestOpenAIResponsesViaChatPolicyRoutesStreamResponseAndTerminalUsage(t *testing.T) {
	c, recorder := newOpenAIResponsesViaChatContext(t)
	info := newOpenAIResponsesViaChatInfo(true)
	info.SetEstimatePromptTokens(999)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	body := strings.Join([]string{
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[],"usage":{"prompt_tokens":20,"completion_tokens":7,"total_tokens":27,"prompt_tokens_details":{"cached_tokens":11},"completion_tokens_details":{"reasoning_tokens":4}}}`,
		`data: [DONE]`,
		``,
	}, "\n")

	usageValue, relayErr := (&Adaptor{}).DoResponse(c, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
	}, info)
	require.Nil(t, relayErr)
	usage, ok := usageValue.(*dto.Usage)
	require.True(t, ok)
	assert.Equal(t, 20, usage.PromptTokens)
	assert.Equal(t, 7, usage.CompletionTokens)
	assert.Equal(t, 27, usage.TotalTokens)
	require.NotNil(t, usage.InputTokensDetails)
	assert.Equal(t, 11, usage.InputTokensDetails.CachedTokens)
	assert.Equal(t, 4, usage.CompletionTokenDetails.ReasoningTokens)
	require.NotNil(t, usage.BillingUsage)
	assert.Equal(t, dto.BillingUsageSourceOAIChat, usage.BillingUsage.Source)
	assert.Equal(t, dto.BillingUsageSemanticOpenAI, usage.BillingUsage.Semantic)
	assert.False(t, usage.BillingUsage.Estimated)
	require.NotNil(t, usage.BillingUsage.OpenAIUsage)
	assert.Equal(t, 20, usage.BillingUsage.OpenAIUsage.PromptTokens)
	assert.Equal(t, 11, usage.BillingUsage.OpenAIUsage.PromptTokensDetails.CachedTokens)
	assert.Equal(t, 4, usage.BillingUsage.OpenAIUsage.CompletionTokenDetails.ReasoningTokens)

	output := recorder.Body.String()
	assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	requireOrderedSubstrings(t, output,
		"event: response.created",
		"event: response.output_item.added",
		"event: response.output_text.delta",
		"event: response.output_text.done",
		"event: response.completed",
	)
	assert.Contains(t, output, `"input_tokens":20`)
	assert.Contains(t, output, `"output_tokens":7`)
	assert.Contains(t, output, `"cached_tokens":11`)
	assert.Contains(t, output, `"reasoning_tokens":4`)
}

func newOpenAIResponsesViaChatContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(common.RequestIdKey, "responses-via-chat-test")
	return c, recorder
}

func newOpenAIResponsesViaChatInfo(stream bool) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		IsStream:        stream,
		RelayMode:       relayconstant.RelayModeResponses,
		RelayFormat:     types.RelayFormatOpenAIResponses,
		RequestURLPath:  "/v1/responses",
		OriginModelName: "gpt-test",
		DisablePing:     true,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			UpstreamModelName: "gpt-test",
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ResponsesToChatEnabled: true,
			},
		},
	}
}

func mustOpenAIResponsesRaw(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := common.Marshal(value)
	require.NoError(t, err)
	return raw
}
