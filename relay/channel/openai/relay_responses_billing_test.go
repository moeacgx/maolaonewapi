package openai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOaiResponsesHandlerCountsOutputCallsNotDeclarations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	operation_setting.SetToolPriceForTest("priced_fn", 5.0)
	operation_setting.SetToolPriceForTest("priced_custom", 7.0)
	t.Cleanup(func() {
		operation_setting.DeleteToolPriceForTest("priced_fn")
		operation_setting.DeleteToolPriceForTest("priced_custom")
	})

	body, err := common.Marshal(dto.OpenAIResponsesResponse{
		Tools: []map[string]any{
			{"type": "web_search_preview"},
			{"type": "file_search"},
		},
		Output: []dto.ResponsesOutput{
			{Type: dto.BuildInCallWebSearchCall},
			{Type: dto.BuildInCallWebSearchCall},
			{Type: dto.BuildInCallFunctionCall, Name: "priced_fn"},
			{Type: dto.BuildInCallFunctionCall, Name: "unpriced_fn"},
			{Type: "custom_tool_call", Name: "priced_custom", Input: json.RawMessage(`{"path":"README.md"}`)},
		},
		Usage: &dto.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.1",
		ResponsesUsageInfo: &relaycommon.ResponsesUsageInfo{
			BuiltInTools: map[string]*relaycommon.BuildInToolInfo{
				dto.BuildInToolWebSearchPreview: {ToolName: dto.BuildInToolWebSearchPreview, CallCount: 0},
				dto.BuildInToolFileSearch:       {ToolName: dto.BuildInToolFileSearch, CallCount: 0},
			},
		},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}

	usage, apiErr := OaiResponsesHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 2, info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview].CallCount)
	assert.Equal(t, 0, info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolFileSearch].CallCount)
	require.Contains(t, info.ResponsesUsageInfo.BuiltInTools, "priced_fn")
	assert.Equal(t, 1, info.ResponsesUsageInfo.BuiltInTools["priced_fn"].CallCount)
	assert.NotContains(t, info.ResponsesUsageInfo.BuiltInTools, "unpriced_fn")
	require.Contains(t, info.ResponsesUsageInfo.BuiltInTools, "priced_custom")
	assert.Equal(t, 1, info.ResponsesUsageInfo.BuiltInTools["priced_custom"].CallCount)
}

func TestOaiResponsesHandlerDeclaredToolsWithoutOutputCountZero(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body, err := common.Marshal(dto.OpenAIResponsesResponse{
		Tools: []map[string]any{
			{"type": "web_search_preview"},
			{"type": "file_search"},
		},
		Output: []dto.ResponsesOutput{
			{Type: "message", Role: "assistant"},
		},
		Usage: &dto.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.1",
		ResponsesUsageInfo: &relaycommon.ResponsesUsageInfo{
			BuiltInTools: map[string]*relaycommon.BuildInToolInfo{
				dto.BuildInToolWebSearchPreview: {ToolName: dto.BuildInToolWebSearchPreview, CallCount: 0},
				dto.BuildInToolFileSearch:       {ToolName: dto.BuildInToolFileSearch, CallCount: 0},
			},
		},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}

	_, apiErr := OaiResponsesHandler(c, info, resp)
	require.Nil(t, apiErr)
	assert.Equal(t, 0, info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview].CallCount)
	assert.Equal(t, 0, info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolFileSearch].CallCount)
}

func TestOaiResponsesStreamHandlerCountsTerminalOnlyToolCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitTokenEncoders()
	operation_setting.SetToolPriceForTest("priced_custom", 7.0)
	t.Cleanup(func() {
		operation_setting.DeleteToolPriceForTest("priced_custom")
	})
	tests := []struct {
		name      string
		events    []string
		toolName  string
		wantCount int
	}{
		{
			name: "terminal web search",
			events: []string{
				`{"type":"response.completed","response":{"status":"completed","output":[{"type":"web_search_call","id":"ws_1"}]}}`,
			},
			toolName:  dto.BuildInToolWebSearchPreview,
			wantCount: 1,
		},
		{
			name: "terminal custom tool",
			events: []string{
				`{"type":"response.completed","response":{"status":"completed","output":[{"type":"custom_tool_call","id":"custom_1","name":"priced_custom","input":"patch body"}]}}`,
			},
			toolName:  "priced_custom",
			wantCount: 1,
		},
		{
			name: "item done and terminal duplicate",
			events: []string{
				`{"type":"response.output_item.done","output_index":0,"item":{"type":"web_search_call","id":"ws_1"}}`,
				`{"type":"response.completed","response":{"status":"completed","output":[{"type":"web_search_call","id":"ws_1"}]}}`,
			},
			toolName:  dto.BuildInToolWebSearchPreview,
			wantCount: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var body strings.Builder
			for _, event := range test.events {
				body.WriteString("data: ")
				body.WriteString(event)
				body.WriteString("\n\n")
			}
			body.WriteString("data: [DONE]\n\n")

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			info := &relaycommon.RelayInfo{
				OriginModelName: "gpt-5.1",
				DisablePing:     true,
				ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.1"},
				PriceData: hosttypes.PriceData{
					ModelRatio:     1,
					GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1},
				},
			}

			usage, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body.String())),
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			})

			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			require.NotNil(t, info.ResponsesUsageInfo)
			require.Contains(t, info.ResponsesUsageInfo.BuiltInTools, test.toolName)
			assert.Equal(t, test.wantCount, info.ResponsesUsageInfo.BuiltInTools[test.toolName].CallCount)
			require.Nil(t, service.TextUsageError(c, info, usage))
		})
	}
}

func TestOaiResponsesHandlerFallsBackToLocalUsageWhenUpstreamUsageMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	body, err := common.Marshal(dto.OpenAIResponsesResponse{
		Output: []dto.ResponsesOutput{{
			Type:    "message",
			Content: []dto.ResponsesOutputContent{{Text: "response without usage"}},
		}},
	})
	require.NoError(t, err)

	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.1",
		PriceData: hosttypes.PriceData{
			ModelRatio:     1,
			GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1},
		},
	}
	usage, apiErr := OaiResponsesHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	})

	require.NotNil(t, usage)
	require.Nil(t, apiErr)
	assert.Greater(t, usage.CompletionTokens, 0, "visible response text must be counted locally when upstream usage is absent")
	assert.Greater(t, usage.TotalTokens, 0)
	assert.NotEmpty(t, recorder.Body.Bytes(), "a billable response must reach the client")
}

func TestOaiResponsesHandlerFillsOnlyMissingUsageFieldsLocally(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitTokenEncoders()
	const responseText = "partially reported usage"
	tests := []struct {
		name           string
		upstreamUsage  *dto.Usage
		estimatePrompt int
		wantPrompt     int
		wantCompletion int
	}{
		{
			name:           "missing output tokens",
			upstreamUsage:  &dto.Usage{InputTokens: 11, TotalTokens: 11},
			estimatePrompt: 13,
			wantPrompt:     11,
			wantCompletion: service.CountTextToken(responseText, "gpt-5.1"),
		},
		{
			name:           "missing input tokens",
			upstreamUsage:  &dto.Usage{OutputTokens: 5, TotalTokens: 5},
			estimatePrompt: 13,
			wantPrompt:     13,
			wantCompletion: 5,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			body, err := common.Marshal(dto.OpenAIResponsesResponse{
				Output: []dto.ResponsesOutput{{
					Type:    "message",
					Content: []dto.ResponsesOutputContent{{Text: responseText}},
				}},
				Usage: test.upstreamUsage,
			})
			require.NoError(t, err)
			info := &relaycommon.RelayInfo{
				OriginModelName: "gpt-5.1",
				PriceData: hosttypes.PriceData{
					ModelRatio:     1,
					GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1},
				},
			}
			info.SetEstimatePromptTokens(test.estimatePrompt)

			usage, apiErr := OaiResponsesHandler(c, info, &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			})

			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, test.wantPrompt, usage.PromptTokens)
			assert.Equal(t, test.wantCompletion, usage.CompletionTokens)
			assert.Equal(t, test.wantPrompt+test.wantCompletion, usage.TotalTokens)
			assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyLocalCountTokens))
			assert.NotEmpty(t, recorder.Body.Bytes())
		})
	}
}

func TestOaiResponsesHandlerRejectsTrulyEmptyUsageBeforeWritingResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	body, err := common.Marshal(dto.OpenAIResponsesResponse{Output: []dto.ResponsesOutput{}})
	require.NoError(t, err)

	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.1",
		PriceData: hosttypes.PriceData{
			ModelRatio:     1,
			GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1},
		},
	}
	usage, apiErr := OaiResponsesHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	})

	require.NotNil(t, usage)
	require.Error(t, apiErr)
	assert.Equal(t, types.ErrorCodeEmptyResponse, apiErr.GetErrorCode())
	assert.Empty(t, recorder.Body.Bytes(), "a truly empty response must be rejected before it reaches the client")
}

func TestOaiResponsesHandlerFallsBackToLocalUsageForFunctionCallOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	body, err := common.Marshal(dto.OpenAIResponsesResponse{
		Output: []dto.ResponsesOutput{{
			Type:      dto.BuildInCallFunctionCall,
			Name:      "lookup_weather",
			Arguments: json.RawMessage(`{"city":"Shanghai"}`),
		}},
	})
	require.NoError(t, err)

	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.1",
		PriceData: hosttypes.PriceData{
			ModelRatio:     1,
			GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1},
		},
	}
	usage, apiErr := OaiResponsesHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Greater(t, usage.CompletionTokens, 0)
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyLocalCountTokens))
	assert.NotEmpty(t, recorder.Body.Bytes())
}

func TestOaiResponsesHandlerFallsBackToLocalUsageForReasoningAndCustomToolOutput(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "reasoning summary",
			body: `{"output":[{"type":"reasoning","summary":[{"type":"summary_text","text":"visible reasoning"}]}]}`,
		},
		{
			name: "custom tool input",
			body: `{"output":[{"type":"custom_tool_call","input":"patch body"}]}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			info := &relaycommon.RelayInfo{
				OriginModelName: "gpt-5.1",
				PriceData: hosttypes.PriceData{
					ModelRatio:     1,
					GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1},
				},
			}

			usage, apiErr := OaiResponsesHandler(c, info, &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(test.body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			})

			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Greater(t, usage.CompletionTokens, 0)
			assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyLocalCountTokens))
			assert.NotEmpty(t, recorder.Body.Bytes())
		})
	}
}

func TestOaiResponsesHandlerFallsBackToLocalUsageForRefusal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitTokenEncoders()
	const refusal = "policy refusal"
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.1",
		PriceData: hosttypes.PriceData{
			ModelRatio:     1,
			GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1},
		},
	}
	body := `{"output":[{"type":"message","role":"assistant","content":[{"type":"refusal","refusal":"` + refusal + `"}]}]}`

	usage, apiErr := OaiResponsesHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, service.EstimateTokenByModel(info.OriginModelName, refusal), usage.CompletionTokens)
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyLocalCountTokens))
	assert.NotEmpty(t, recorder.Body.Bytes())
}

func TestResponsesLocalUsageTextDoesNotDuplicateReasoningContent(t *testing.T) {
	response := &dto.OpenAIResponsesResponse{Output: []dto.ResponsesOutput{{
		Type:    "reasoning",
		Content: []dto.ResponsesOutputContent{{Type: "summary_text", Text: "one summary"}},
		Summary: []dto.ResponsesReasoningSummaryPart{{Type: "summary_text", Text: "one summary"}},
	}}}

	assert.Equal(t, "one summary", responsesLocalUsageText(response))
}

func TestResponsesLocalUsageTextUsesProtocolSpecificToolPayload(t *testing.T) {
	tests := []struct {
		name     string
		output   dto.ResponsesOutput
		expected string
	}{
		{
			name: "function call uses arguments",
			output: dto.ResponsesOutput{
				Type:      dto.BuildInCallFunctionCall,
				Name:      "lookup",
				Arguments: json.RawMessage(`{"city":"Shanghai"}`),
				Input:     json.RawMessage(`{"duplicate":"must not count"}`),
			},
			expected: `lookup{"city":"Shanghai"}`,
		},
		{
			name: "custom tool uses input",
			output: dto.ResponsesOutput{
				Type:      "custom_tool_call",
				Name:      "patch",
				Arguments: json.RawMessage(`{"duplicate":"must not count"}`),
				Input:     json.RawMessage(`{"path":"README.md"}`),
			},
			expected: `patch{"path":"README.md"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := &dto.OpenAIResponsesResponse{Output: []dto.ResponsesOutput{test.output}}
			assert.Equal(t, test.expected, responsesLocalUsageText(response))
		})
	}
}

func TestOaiResponsesStreamHandlerFallsBackToCompletedOutputWhenUsageMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitTokenEncoders()
	var body strings.Builder
	body.WriteString("data: ")
	body.WriteString(`{"type":"response.completed","response":{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"locally counted response"}]}]}}`)
	body.WriteString("\n\n")
	body.WriteString("data: [DONE]\n\n")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.1",
		DisablePing:     true,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.1"},
	}

	usage, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body.String())),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Greater(t, usage.CompletionTokens, 0, "completed output must be counted locally when stream usage is absent")
	assert.Greater(t, usage.TotalTokens, 0)
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyLocalCountTokens))
	assert.NotEmpty(t, recorder.Body.Bytes(), "the completed response must reach the client")
}

func TestOaiResponsesStreamHandlerDoesNotDoubleCountDeltaAndCompletedOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitTokenEncoders()
	const outputText = "count this response exactly once"
	var body strings.Builder
	body.WriteString("data: ")
	body.WriteString(`{"type":"response.created","response":{"status":"in_progress"}}`)
	body.WriteString("\n\n")
	body.WriteString("data: ")
	body.WriteString(`{"type":"response.output_text.delta","delta":"` + outputText + `"}`)
	body.WriteString("\n\n")
	body.WriteString("data: ")
	body.WriteString(`{"type":"response.completed","response":{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"` + outputText + `"}]}]}}`)
	body.WriteString("\n\n")
	body.WriteString("data: [DONE]\n\n")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.1",
		DisablePing:     true,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.1"},
	}

	usage, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body.String())),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, service.CountTextToken(outputText, info.UpstreamModelName), usage.CompletionTokens)
	assert.Contains(t, recorder.Body.String(), "event: response.created")
	assert.Contains(t, recorder.Body.String(), "event: response.output_text.delta")
	assert.Less(t,
		strings.Index(recorder.Body.String(), "event: response.created"),
		strings.Index(recorder.Body.String(), "event: response.output_text.delta"),
	)
}

func TestOaiResponsesStreamHandlerLocallyCountsReasoningAndToolDeltas(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitTokenEncoders()
	const reasoning = "reasoning summary"
	const arguments = `{"path":"README.md"}`
	var body strings.Builder
	for _, event := range []string{
		`{"type":"response.reasoning_summary_text.delta","delta":"` + reasoning + `"}`,
		`{"type":"response.function_call_arguments.delta","delta":"{\"path\":\"README.md\"}"}`,
	} {
		body.WriteString("data: ")
		body.WriteString(event)
		body.WriteString("\n\n")
	}
	body.WriteString("data: [DONE]\n\n")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.1",
		DisablePing:     true,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.1"},
	}

	usage, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body.String())),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, service.CountTextToken(reasoning+arguments, info.UpstreamModelName), usage.CompletionTokens)
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyLocalCountTokens))
	assert.NotEmpty(t, recorder.Body.Bytes())
}

func TestOaiResponsesStreamHandlerUsesUsageFromIncompleteResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := "data: " +
		`{"type":"response.incomplete","response":{"status":"incomplete","usage":{"input_tokens":7,"output_tokens":3,"total_tokens":10}}}` +
		"\n\ndata: [DONE]\n\n"

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.1",
		DisablePing:     true,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.1"},
	}

	usage, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 7, usage.PromptTokens)
	assert.Equal(t, 3, usage.CompletionTokens)
	assert.Equal(t, 10, usage.TotalTokens)
	assert.NotEmpty(t, recorder.Body.Bytes())
}

func TestOaiResponsesStreamHandlerLocallyCountsOutputFromIncompleteResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitTokenEncoders()
	const outputText = "partial output before interruption"
	body := "data: " +
		`{"type":"response.incomplete","response":{"status":"incomplete","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"` + outputText + `"}]}]}}` +
		"\n\ndata: [DONE]\n\n"

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.1",
		DisablePing:     true,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.1"},
	}

	usage, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, service.CountTextToken(outputText, info.UpstreamModelName), usage.CompletionTokens)
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyLocalCountTokens))
	assert.NotEmpty(t, recorder.Body.Bytes())
}

func TestOaiResponsesStreamHandlerLocallyCountsFunctionCallOutputItemDone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitTokenEncoders()
	const toolName = "lookup_weather"
	const arguments = `{"city":"Shanghai"}`
	body := "data: " +
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","name":"` + toolName + `","arguments":` + arguments + `}}` +
		"\n\ndata: [DONE]\n\n"

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.1",
		DisablePing:     true,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.1"},
	}

	usage, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, service.CountTextToken(toolName+arguments, info.UpstreamModelName), usage.CompletionTokens)
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyLocalCountTokens))
	assert.NotEmpty(t, recorder.Body.Bytes())
}

func TestOaiResponsesStreamHandlerLocallyCountsReasoningPartAdded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitTokenEncoders()
	const reasoningText = "partial visible reasoning"
	body := "data: " +
		`{"type":"response.reasoning_summary_part.added","output_index":0,"part":{"type":"summary_text","text":"` + reasoningText + `"}}` +
		"\n\ndata: [DONE]\n\n"

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.1",
		DisablePing:     true,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.1"},
	}

	usage, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, service.CountTextToken(reasoningText, info.UpstreamModelName), usage.CompletionTokens)
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyLocalCountTokens))
	assert.NotEmpty(t, recorder.Body.Bytes())
}

func TestOaiResponsesStreamHandlerLocallyCountsRefusalPartAdded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitTokenEncoders()
	const refusal = "policy refusal"
	body := "data: " +
		`{"type":"response.content_part.added","output_index":0,"part":{"type":"refusal","refusal":"` + refusal + `"}}` +
		"\n\ndata: [DONE]\n\n"

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.1",
		DisablePing:     true,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.1"},
	}

	usage, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, service.CountTextToken(refusal, info.UpstreamModelName), usage.CompletionTokens)
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyLocalCountTokens))
	assert.NotEmpty(t, recorder.Body.Bytes())
}

func TestOaiResponsesStreamHandlerLocallyCountsContentPartDone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitTokenEncoders()
	tests := []struct {
		name  string
		event string
		text  string
	}{
		{
			name:  "content part done",
			event: `{"type":"response.content_part.done","output_index":0,"part":{"type":"output_text","text":"content done"}}`,
			text:  "content done",
		},
		{
			name:  "reasoning summary part done",
			event: `{"type":"response.reasoning_summary_part.done","output_index":0,"part":{"type":"summary_text","text":"reasoning done"}}`,
			text:  "reasoning done",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			info := &relaycommon.RelayInfo{
				OriginModelName: "gpt-5.1",
				DisablePing:     true,
				ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.1"},
			}
			body := "data: " + test.event + "\n\ndata: [DONE]\n\n"

			usage, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			})

			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, service.CountTextToken(test.text, info.UpstreamModelName), usage.CompletionTokens)
			assert.NotEmpty(t, recorder.Body.Bytes())
		})
	}
}

func TestOaiResponsesStreamHandlerLocallyCountsMeaningfulOutputItemAdded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitTokenEncoders()
	tests := []struct {
		name string
		item string
		text string
	}{
		{
			name: "function item",
			item: `{"type":"function_call","id":"fc_1","name":"lookup","arguments":{"city":"Shanghai"}}`,
			text: `lookup{"city":"Shanghai"}`,
		},
		{
			name: "message item",
			item: `{"type":"message","id":"msg_1","content":[{"type":"output_text","text":"message added"}]}`,
			text: "message added",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			info := &relaycommon.RelayInfo{
				OriginModelName: "gpt-5.1",
				DisablePing:     true,
				ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.1"},
			}
			body := "data: " + `{"type":"response.output_item.added","output_index":0,"item":` + test.item + `}` + "\n\ndata: [DONE]\n\n"

			usage, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			})

			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, service.CountTextToken(test.text, info.UpstreamModelName), usage.CompletionTokens)
			assert.NotEmpty(t, recorder.Body.Bytes())
		})
	}
}

func TestOaiResponsesStreamHandlerLocallyCountsRefusalAndDoneEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitTokenEncoders()
	tests := []struct {
		name  string
		event string
		text  string
	}{
		{
			name:  "refusal delta",
			event: `{"type":"response.refusal.delta","output_index":0,"delta":"policy refusal"}`,
			text:  "policy refusal",
		},
		{
			name:  "output text done",
			event: `{"type":"response.output_text.done","output_index":0,"text":"done output"}`,
			text:  "done output",
		},
		{
			name:  "reasoning summary delta alias",
			event: `{"type":"response.reasoning_summary.delta","output_index":0,"delta":"summary delta"}`,
			text:  "summary delta",
		},
		{
			name:  "reasoning summary done alias",
			event: `{"type":"response.reasoning_summary.done","output_index":0,"text":"summary done"}`,
			text:  "summary done",
		},
		{
			name:  "refusal done",
			event: `{"type":"response.refusal.done","output_index":0,"refusal":"done refusal"}`,
			text:  "done refusal",
		},
		{
			name:  "function arguments done",
			event: `{"type":"response.function_call_arguments.done","output_index":0,"arguments":"{\"city\":\"Shanghai\"}"}`,
			text:  `{"city":"Shanghai"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			info := &relaycommon.RelayInfo{
				OriginModelName: "gpt-5.1",
				DisablePing:     true,
				ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.1"},
			}
			body := "data: " + test.event + "\n\ndata: [DONE]\n\n"

			usage, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			})

			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, service.CountTextToken(test.text, info.UpstreamModelName), usage.CompletionTokens)
			assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyLocalCountTokens))
			assert.NotEmpty(t, recorder.Body.Bytes())
		})
	}
}

func TestOaiResponsesStreamHandlerDoesNotDoubleCountMixedOutputIndexes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitTokenEncoders()
	const outputText = "single output"
	var body strings.Builder
	for _, event := range []string{
		`{"type":"response.output_text.delta","output_index":0,"delta":"` + outputText + `"}`,
		`{"type":"response.output_item.done","item":{"type":"message","content":[{"type":"output_text","text":"` + outputText + `"}]}}`,
	} {
		body.WriteString("data: ")
		body.WriteString(event)
		body.WriteString("\n\n")
	}
	body.WriteString("data: [DONE]\n\n")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.1",
		DisablePing:     true,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.1"},
	}

	usage, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body.String())),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, service.CountTextToken(outputText, info.UpstreamModelName), usage.CompletionTokens)
}

func TestOaiResponsesStreamHandlerCountsDistinctUnindexedOutputItems(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitTokenEncoders()
	const firstText = "alpha output"
	const secondText = "beta output"
	var body strings.Builder
	for _, event := range []string{
		`{"type":"response.output_text.delta","output_index":0,"item_id":"msg_a","delta":"` + firstText + `"}`,
		`{"type":"response.output_item.done","item":{"type":"message","id":"msg_a","content":[{"type":"output_text","text":"` + firstText + `"}]}}`,
		`{"type":"response.output_item.done","item":{"type":"message","id":"msg_b","content":[{"type":"output_text","text":"` + secondText + `"}]}}`,
	} {
		body.WriteString("data: ")
		body.WriteString(event)
		body.WriteString("\n\n")
	}
	body.WriteString("data: [DONE]\n\n")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.1",
		DisablePing:     true,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.1"},
	}

	usage, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body.String())),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, service.CountTextToken(firstText+secondText, info.UpstreamModelName), usage.CompletionTokens)
}

func TestOaiResponsesStreamHandlerPreservesMissingIndexOutputIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitTokenEncoders()
	const firstText = "alpha output"
	const secondText = "beta output"
	tests := []struct {
		name   string
		events []string
		want   string
	}{
		{
			name: "indexed delta then different unindexed item",
			events: []string{
				`{"type":"response.output_text.delta","output_index":0,"item_id":"msg_a","delta":"` + firstText + `"}`,
				`{"type":"response.output_item.done","item":{"type":"message","id":"msg_b","content":[{"type":"output_text","text":"` + secondText + `"}]}}`,
			},
			want: firstText + secondText,
		},
		{
			name: "fully unindexed different items",
			events: []string{
				`{"type":"response.output_text.delta","item_id":"msg_a","delta":"` + firstText + `"}`,
				`{"type":"response.output_item.done","item":{"type":"message","id":"msg_b","content":[{"type":"output_text","text":"` + secondText + `"}]}}`,
			},
			want: firstText + secondText,
		},
		{
			name: "fully unindexed same item",
			events: []string{
				`{"type":"response.output_text.delta","item_id":"msg_a","delta":"` + firstText + `"}`,
				`{"type":"response.output_item.done","item":{"type":"message","id":"msg_a","content":[{"type":"output_text","text":"` + firstText + `"}]}}`,
			},
			want: firstText,
		},
		{
			name: "multiple unindexed deltas with one matching done",
			events: []string{
				`{"type":"response.output_text.delta","item_id":"msg_a","delta":"` + firstText + `"}`,
				`{"type":"response.output_text.delta","item_id":"msg_b","delta":"` + secondText + `"}`,
				`{"type":"response.output_item.done","item":{"type":"message","id":"msg_b","content":[{"type":"output_text","text":"` + secondText + `"}]}}`,
			},
			want: firstText + secondText,
		},
		{
			name: "unindexed delta then indexed done for same item",
			events: []string{
				`{"type":"response.output_text.delta","item_id":"msg_a","delta":"` + firstText + `"}`,
				`{"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"msg_a","content":[{"type":"output_text","text":"` + firstText + `"}]}}`,
			},
			want: firstText,
		},
		{
			name: "equivalent output and content done",
			events: []string{
				`{"type":"response.output_text.done","output_index":0,"text":"` + firstText + `"}`,
				`{"type":"response.content_part.done","output_index":0,"part":{"type":"output_text","text":"` + firstText + `"}}`,
			},
			want: firstText,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var body strings.Builder
			for _, event := range test.events {
				body.WriteString("data: ")
				body.WriteString(event)
				body.WriteString("\n\n")
			}
			body.WriteString("data: [DONE]\n\n")

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			info := &relaycommon.RelayInfo{
				OriginModelName: "gpt-5.1",
				DisablePing:     true,
				ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.1"},
			}

			usage, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body.String())),
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			})

			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, service.CountTextToken(test.want, info.UpstreamModelName), usage.CompletionTokens)
		})
	}
}

func TestOaiResponsesStreamHandlerReturnsUpstreamStreamErrorWithoutCommitting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		event       string
		wantMessage string
		wantCode    string
	}{
		{
			name:        "nested response failed",
			event:       `{"type":"response.failed","response":{"status":"failed","error":{"message":"invalid upstream input","type":"invalid_request_error","code":"invalid_prompt"},"output":[{"type":"message","content":[{"type":"output_text","text":"must not settle"}]}],"usage":{"input_tokens":7,"output_tokens":3,"total_tokens":10}}}`,
			wantMessage: "invalid upstream input",
			wantCode:    "invalid_prompt",
		},
		{
			name:        "top level response error",
			event:       `{"type":"response.error","error":{"message":"upstream overloaded","type":"server_error","code":"overloaded"}}`,
			wantMessage: "upstream overloaded",
			wantCode:    "overloaded",
		},
		{
			name:        "official direct error",
			event:       `{"type":"error","message":"direct stream failure","code":"direct_failure","param":"input"}`,
			wantMessage: "direct stream failure",
			wantCode:    "direct_failure",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			info := &relaycommon.RelayInfo{
				OriginModelName: "gpt-5.1",
				DisablePing:     true,
				ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.1"},
			}
			body := "data: " + test.event + "\n\ndata: [DONE]\n\n"

			usage, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			})

			require.Nil(t, usage)
			require.Error(t, apiErr)
			assert.Equal(t, test.wantCode, string(apiErr.GetErrorCode()))
			assert.Equal(t, test.wantMessage, apiErr.Error())
			assert.Empty(t, recorder.Body.Bytes(), "an uncommitted upstream error must remain retryable")
			assert.False(t, recorder.Flushed)
		})
	}
}

func TestSendCommittedResponsesStreamAPIErrorAppliesClientReplacement(t *testing.T) {
	require.NoError(t, common.UpdateErrorMessageReplacementRules(`[{"match":"upstream overloaded","mode":"exact","status_code":503,"replace":"client overloaded"}]`))
	t.Cleanup(func() { require.NoError(t, common.UpdateErrorMessageReplacementRules(`[]`)) })

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	_, err := c.Writer.WriteString("data: committed\n\n")
	require.NoError(t, err)

	relayErr := types.WithOpenAIError(types.OpenAIError{
		Type:    "server_error",
		Code:    "overloaded",
		Message: "upstream overloaded",
	}, http.StatusServiceUnavailable)
	require.NoError(t, sendCommittedResponsesStreamAPIError(c, relayErr, 7))
	require.Contains(t, recorder.Body.String(), `"message":"client overloaded"`)
	require.NotContains(t, recorder.Body.String(), `"message":"upstream overloaded"`)
}

func TestOaiResponsesStreamHandlerKeepsEmptyLifecycleUncommittedForRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body strings.Builder
	for _, event := range []string{
		`{"type":"response.created","response":{"status":"in_progress"}}`,
		`{"type":"response.in_progress","response":{"status":"in_progress"}}`,
		`{"type":"response.completed","response":{"status":"completed","output":[]}}`,
	} {
		body.WriteString("data: ")
		body.WriteString(event)
		body.WriteString("\n\n")
	}
	body.WriteString("data: [DONE]\n\n")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.1",
		DisablePing:     true,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.1"},
		PriceData: hosttypes.PriceData{
			ModelRatio:     1,
			GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1},
		},
	}

	usage, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body.String())),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Zero(t, usage.TotalTokens)
	assert.Empty(t, recorder.Body.Bytes(), "lifecycle-only SSE must stay uncommitted so the outer relay can retry")
	assert.False(t, recorder.Flushed)
	usageErr := service.TextUsageError(c, info, usage)
	require.Error(t, usageErr)
	assert.Equal(t, types.ErrorCodeEmptyResponse, usageErr.GetErrorCode())
}

func TestOaiResponsesStreamHandlerCommitsLifecycleOverflowAtBufferLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body strings.Builder
	for i := 0; i < maxProvisionalResponsesStreamEvents+1; i++ {
		body.WriteString("data: ")
		body.WriteString(`{"type":"response.in_progress","response":{"status":"in_progress"}}`)
		body.WriteString("\n\n")
	}
	body.WriteString("data: [DONE]\n\n")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.1",
		DisablePing:     true,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.1"},
	}

	usage, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body.String())),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Zero(t, usage.TotalTokens)
	assert.Equal(t, maxProvisionalResponsesStreamEvents+1,
		strings.Count(recorder.Body.String(), `"type":"response.in_progress"`))
	assert.True(t, recorder.Flushed)
}

func TestOaiResponsesStreamHandlerRetainsStructureAfterLifecycleOverflow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body strings.Builder
	for i := 0; i < maxProvisionalResponsesStreamEvents; i++ {
		body.WriteString("data: ")
		body.WriteString(`{"type":"response.in_progress","response":{"status":"in_progress"}}`)
		body.WriteString("\n\n")
	}
	for _, event := range []string{
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_1","content":[]}}`,
		`{"type":"response.content_part.added","output_index":0,"item_id":"msg_1","part":{"type":"output_text","text":""}}`,
		`{"type":"response.output_text.delta","output_index":0,"item_id":"msg_1","delta":"visible output"}`,
	} {
		body.WriteString("data: ")
		body.WriteString(event)
		body.WriteString("\n\n")
	}
	body.WriteString("data: [DONE]\n\n")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.1",
		DisablePing:     true,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.1"},
	}

	usage, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body.String())),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	responseBody := recorder.Body.String()
	itemAdded := strings.Index(responseBody, "event: response.output_item.added")
	partAdded := strings.Index(responseBody, "event: response.content_part.added")
	delta := strings.Index(responseBody, "event: response.output_text.delta")
	require.GreaterOrEqual(t, itemAdded, 0)
	require.Greater(t, partAdded, itemAdded)
	require.Greater(t, delta, partAdded)
}

func TestOaiResponsesStreamHandlerKeepsEmptyTextEventsUncommittedForRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body strings.Builder
	for _, event := range []string{
		`{"type":"response.created","response":{"status":"in_progress"}}`,
		`{"type":"response.output_text.delta","output_index":0,"delta":""}`,
		`{"type":"response.output_text.done","output_index":0,"text":""}`,
		`{"type":"response.completed","response":{"status":"completed","output":[]}}`,
	} {
		body.WriteString("data: ")
		body.WriteString(event)
		body.WriteString("\n\n")
	}
	body.WriteString("data: [DONE]\n\n")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.1",
		DisablePing:     true,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.1"},
	}

	usage, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body.String())),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Zero(t, usage.TotalTokens)
	assert.Empty(t, recorder.Body.Bytes())
	assert.False(t, recorder.Flushed)
}

func TestOaiResponsesStreamHandlerKeepsTotalOnlyUsageUncommittedForRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := "data: " +
		`{"type":"response.completed","response":{"status":"completed","output":[],"usage":{"total_tokens":10}}}` +
		"\n\ndata: [DONE]\n\n"

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.1",
		DisablePing:     true,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.1"},
	}

	usage, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Zero(t, usage.TotalTokens)
	assert.Empty(t, recorder.Body.Bytes())
	assert.False(t, recorder.Flushed)
}

func TestOaiResponsesHandlerCountsCompletedImageGenerationOutputs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body, err := common.Marshal(dto.OpenAIResponsesResponse{
		Status: []byte(`"completed"`),
		Output: []dto.ResponsesOutput{
			{
				Type:   dto.ResponsesOutputTypeImageGenerationCall,
				ID:     "img_1",
				Status: "completed",
				Result: "base64-a",
			},
			{
				Type:   dto.ResponsesOutputTypeImageGenerationCall,
				ID:     "img_2",
				Status: "completed",
				Result: "base64-b",
			},
			{
				Type:   dto.ResponsesOutputTypeImageGenerationCall,
				ID:     "img_empty",
				Status: "completed",
				Result: "",
			},
		},
		Usage: &dto.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{OriginModelName: "gpt-5.1"}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}

	_, apiErr := OaiResponsesHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.Contains(t, info.ResponsesUsageInfo.BuiltInTools, dto.BuildInToolImageGeneration)
	assert.Equal(t, 2, info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolImageGeneration].CallCount)
	assert.False(t, c.GetBool("image_generation_call"))
}

func TestOaiResponsesHandlerIncompleteStatusCommitsZeroImageGeneration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body, err := common.Marshal(dto.OpenAIResponsesResponse{
		Status: []byte(`"incomplete"`),
		Output: []dto.ResponsesOutput{
			{
				Type:   dto.ResponsesOutputTypeImageGenerationCall,
				ID:     "img_1",
				Status: "completed",
				Result: "base64-a",
			},
		},
		Usage: &dto.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.1",
		ResponsesUsageInfo: &relaycommon.ResponsesUsageInfo{
			BuiltInTools: map[string]*relaycommon.BuildInToolInfo{
				dto.BuildInToolImageGeneration: {ToolName: dto.BuildInToolImageGeneration, CallCount: 0},
			},
		},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}

	_, apiErr := OaiResponsesHandler(c, info, resp)
	require.Nil(t, apiErr)
	assert.Equal(t, 0, info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolImageGeneration].CallCount)
}

func TestOaiResponsesHandlerPreservesCanonicalCacheWriteAliases(t *testing.T) {
	tests := []struct {
		name        string
		usageJSON   string
		wantTokens  int
		wantPresent bool
	}{
		{
			name:        "detail alias precedence",
			usageJSON:   `{"input_tokens":100,"output_tokens":5,"cache_write_tokens":90,"input_tokens_details":{"cached_tokens":70,"cache_creation_tokens":30,"cached_creation_tokens":40}}`,
			wantTokens:  30,
			wantPresent: true,
		},
		{
			name:        "detail explicit zero",
			usageJSON:   `{"input_tokens":100,"output_tokens":5,"cache_write_tokens":90,"input_tokens_details":{"cached_tokens":70,"cache_write_tokens":0}}`,
			wantTokens:  0,
			wantPresent: true,
		},
		{
			name:        "absent",
			usageJSON:   `{"input_tokens":100,"output_tokens":5,"input_tokens_details":{"cached_tokens":70}}`,
			wantTokens:  0,
			wantPresent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			body := []byte(`{"id":"resp_cache","status":"completed","usage":` + tt.usageJSON + `}`)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}

			usage, apiErr := OaiResponsesHandler(c, &relaycommon.RelayInfo{}, resp)

			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			tokens, present := usage.GetCacheCreationTokensWithPresence()
			assert.Equal(t, tt.wantTokens, tokens)
			assert.Equal(t, tt.wantPresent, present)
			assert.Equal(t, 70, usage.PromptTokensDetails.CachedTokens)
		})
	}
}

func runResponsesImageBillingStream(t *testing.T, events ...string) *relaycommon.RelayInfo {
	t.Helper()
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		constant.StreamingTimeout = oldTimeout
	})

	var body strings.Builder
	for _, event := range events {
		body.WriteString("data: ")
		body.WriteString(event)
		body.WriteString("\n\n")
	}
	body.WriteString("data: [DONE]\n\n")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(common.RequestIdKey, "responses-image-billing-test")
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.1",
		DisablePing:     true,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5.1",
		},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body.String())),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}

	_, apiErr := OaiResponsesStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, info.ResponsesUsageInfo)
	require.Contains(t, info.ResponsesUsageInfo.BuiltInTools, dto.BuildInToolImageGeneration)
	return info
}

func TestOaiResponsesStreamHandlerDeduplicatesCompletedImageOutput(t *testing.T) {
	item := `{"type":"image_generation_call","id":"img_1","call_id":"call_1","status":"completed","result":"base64-a"}`
	info := runResponsesImageBillingStream(
		t,
		`{"type":"response.output_item.done","output_index":0,"item":`+item+`}`,
		`{"type":"response.completed","response":{"status":"completed","output":[`+item+`],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`,
	)

	assert.Equal(t, 1, info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolImageGeneration].CallCount)
}

func TestOaiResponsesStreamHandlerDiscardsImageOutputOnIncomplete(t *testing.T) {
	info := runResponsesImageBillingStream(
		t,
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"image_generation_call","id":"img_1","status":"completed","result":"base64-a"}}`,
		`{"type":"response.incomplete","response":{"status":"incomplete"}}`,
	)

	assert.Equal(t, 0, info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolImageGeneration].CallCount)
}

func TestOaiResponsesStreamHandlerDoesNotCountPartialImageEvent(t *testing.T) {
	info := runResponsesImageBillingStream(
		t,
		`{"type":"response.image_generation_call.partial_image","output_index":0,"partial_image_b64":"partial-bytes"}`,
		`{"type":"response.completed","response":{"status":"completed","output":[]}}`,
	)

	assert.Equal(t, 0, info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolImageGeneration].CallCount)
}
