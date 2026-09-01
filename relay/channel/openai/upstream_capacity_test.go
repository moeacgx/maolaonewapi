package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesStreamCapacityErrorAfterPreludeRemainsRetryable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test"}}`,
		`data: {"type":"error","code":"server_error","message":"Selected model is at capacity. Please try a different model."}`,
		"",
	}, "\n")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-test",
		IsStream:        true,
		DisablePing:     true,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}

	usage, relayErr := OaiResponsesStreamHandler(ctx, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, relayErr)
	assert.True(t, types.IsUpstreamCapacityError(relayErr))
	assert.Equal(t, http.StatusTooManyRequests, relayErr.StatusCode)
	assert.False(t, ctx.Writer.Written())
	assert.Empty(t, recorder.Body.String())
}

func TestResponsesStreamCapacityErrorAfterOutputIsReportedToClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := strings.Join([]string{
		`data: {"type":"response.created","sequence_number":0,"response":{"id":"resp_1","model":"gpt-test"}}`,
		`data: {"type":"response.output_text.delta","sequence_number":1,"delta":"partial"}`,
		`data: {"type":"error","sequence_number":2,"code":"server_error","message":"Selected model is at capacity. Please try a different model."}`,
		"",
	}, "\n")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-test",
		IsStream:        true,
		DisablePing:     true,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}

	usage, relayErr := OaiResponsesStreamHandler(ctx, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, relayErr)
	assert.True(t, ctx.Writer.Written())
	assert.Contains(t, recorder.Body.String(), "partial")
	assert.Contains(t, recorder.Body.String(), types.UpstreamCapacityClientMessage)
	assert.Contains(t, recorder.Body.String(), `"sequence_number":2`)
	assert.Contains(t, recorder.Body.String(), `"param":""`)
	assert.NotContains(t, recorder.Body.String(), "Selected model is at capacity")
}

func TestResponsesStreamRefusalPartPreventsTransparentRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test"}}`,
		`data: {"type":"response.content_part.added","part":{"type":"refusal","refusal":"blocked"}}`,
		`data: {"type":"error","code":"server_error","message":"Selected model is at capacity. Please try a different model."}`,
		"",
	}, "\n")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-test",
		IsStream:        true,
		DisablePing:     true,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}

	usage, relayErr := OaiResponsesStreamHandler(ctx, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, relayErr)
	assert.True(t, ctx.Writer.Written())
	assert.Contains(t, recorder.Body.String(), `"refusal":"blocked"`)
	assert.Contains(t, recorder.Body.String(), types.UpstreamCapacityClientMessage)
}

func TestResponsesStreamKnownEmptyPartPreludeRemainsRetryable(t *testing.T) {
	tests := []struct {
		name  string
		event string
	}{
		{
			name:  "output text",
			event: `{"type":"response.content_part.added","part":{"type":"output_text","text":""}}`,
		},
		{
			name:  "reasoning summary",
			event: `{"type":"response.reasoning_summary_part.added","part":{"type":"summary_text","text":""}}`,
		},
		{
			name:  "output text done",
			event: `{"type":"response.content_part.done","part":{"type":"output_text","text":""}}`,
		},
		{
			name:  "reasoning summary done",
			event: `{"type":"response.reasoning_summary_part.done","part":{"type":"summary_text","text":""}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder, ctx, relayErr := runResponsesCapacityStream(t,
				`{"type":"response.created","response":{"id":"resp_1","model":"gpt-test"}}`,
				test.event,
				`{"type":"error","code":"server_error","message":"Selected model is at capacity. Please try a different model."}`,
			)

			require.NotNil(t, relayErr)
			assert.True(t, types.IsUpstreamCapacityError(relayErr))
			assert.False(t, ctx.Writer.Written())
			assert.Empty(t, recorder.Body.String())
		})
	}
}

func TestResponsesStreamUnknownOrMissingPartPreventsTransparentRetry(t *testing.T) {
	tests := []struct {
		name  string
		event string
	}{
		{
			name:  "missing part",
			event: `{"type":"response.content_part.added"}`,
		},
		{
			name:  "unknown part",
			event: `{"type":"response.content_part.added","part":{"type":"future_output","text":""}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder, ctx, relayErr := runResponsesCapacityStream(t,
				test.event,
				`{"type":"error","code":"server_error","message":"Selected model is at capacity. Please try a different model."}`,
			)

			require.NotNil(t, relayErr)
			assert.True(t, ctx.Writer.Written())
			assert.Contains(t, recorder.Body.String(), test.event)
			assert.Contains(t, recorder.Body.String(), types.UpstreamCapacityClientMessage)
		})
	}
}

func TestResponsesJSONCapacityErrorWithoutTypeBecomesRateLimit(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{OriginModelName: "gpt-test"}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(
			`{"error":{"code":"server_error","message":"Selected model is at capacity. Please try a different model."}}`,
		)),
		Header: http.Header{"Content-Type": []string{"application/json"}},
	}

	usage, relayErr := OaiResponsesHandler(ctx, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, relayErr)
	assert.Equal(t, http.StatusTooManyRequests, relayErr.StatusCode)
	assert.Equal(t, types.UpstreamCapacityClientMessage, relayErr.ToOpenAIError().Message)
	assert.False(t, ctx.Writer.Written())
}

func TestResponsesFailedNestedCapacityErrorRemainsRetryable(t *testing.T) {
	recorder, ctx, relayErr := runResponsesCapacityStream(t,
		`{"type":"response.failed","response":{"status":"failed","error":{"code":"model_at_capacity","message":"temporary upstream failure"}}}`,
	)

	require.NotNil(t, relayErr)
	assert.True(t, types.IsUpstreamCapacityError(relayErr))
	assert.Equal(t, http.StatusTooManyRequests, relayErr.StatusCode)
	assert.False(t, ctx.Writer.Written())
	assert.Empty(t, recorder.Body.String())
}

func TestResponsesStreamProvisionalEventLimitCommitsBeforeCapacityError(t *testing.T) {
	events := make([]string, 0, 18)
	for range 17 {
		events = append(events, `{"type":"response.in_progress","response":{"id":"resp_1","model":"gpt-test"}}`)
	}
	events = append(events, `{"type":"error","code":"server_error","message":"Selected model is at capacity. Please try a different model."}`)

	recorder, ctx, relayErr := runResponsesCapacityStream(t, events...)

	require.NotNil(t, relayErr)
	assert.True(t, ctx.Writer.Written())
	assert.Equal(t, 17, strings.Count(recorder.Body.String(), `"type":"response.in_progress"`))
	assert.Contains(t, recorder.Body.String(), types.UpstreamCapacityClientMessage)
}

func TestResponsesStreamProvisionalEventLimitKeeps16EventsRetryable(t *testing.T) {
	events := make([]string, 0, 17)
	for range 16 {
		events = append(events, `{"type":"response.in_progress","response":{"id":"resp_1","model":"gpt-test"}}`)
	}
	events = append(events, `{"type":"error","code":"server_error","message":"Selected model is at capacity. Please try a different model."}`)

	recorder, ctx, relayErr := runResponsesCapacityStream(t, events...)

	require.NotNil(t, relayErr)
	assert.True(t, types.IsUpstreamCapacityError(relayErr))
	assert.False(t, ctx.Writer.Written())
	assert.Empty(t, recorder.Body.String())
}

func TestResponsesStreamProvisionalByteLimitCommitsBeforeCapacityError(t *testing.T) {
	largeID := strings.Repeat("x", (1<<20)+1)
	events := []string{
		`{"type":"response.created","response":{"id":"` + largeID + `","model":"gpt-test"}}`,
		`{"type":"error","code":"server_error","message":"Selected model is at capacity. Please try a different model."}`,
	}

	recorder, ctx, relayErr := runResponsesCapacityStream(t, events...)

	require.NotNil(t, relayErr)
	assert.True(t, ctx.Writer.Written())
	assert.Contains(t, recorder.Body.String(), `"type":"response.created"`)
	assert.Contains(t, recorder.Body.String(), types.UpstreamCapacityClientMessage)
}

func TestResponsesStreamCumulativeByteLimitCommitsBeforeCapacityError(t *testing.T) {
	largeID := strings.Repeat("x", 1<<19)
	events := []string{
		`{"type":"response.created","response":{"id":"` + largeID + `-1","model":"gpt-test"}}`,
		`{"type":"response.created","response":{"id":"` + largeID + `-2","model":"gpt-test"}}`,
		`{"type":"error","code":"server_error","message":"Selected model is at capacity. Please try a different model."}`,
	}

	recorder, ctx, relayErr := runResponsesCapacityStream(t, events...)

	require.NotNil(t, relayErr)
	assert.True(t, ctx.Writer.Written())
	assert.Equal(t, 2, strings.Count(recorder.Body.String(), `"type":"response.created"`))
	assert.Contains(t, recorder.Body.String(), types.UpstreamCapacityClientMessage)
}

func runResponsesCapacityStream(t *testing.T, events ...string) (*httptest.ResponseRecorder, *gin.Context, *types.NewAPIError) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	var body strings.Builder
	for _, event := range events {
		body.WriteString("data: ")
		body.WriteString(event)
		body.WriteString("\n\n")
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-test",
		IsStream:        true,
		DisablePing:     true,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body.String())),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}

	usage, relayErr := OaiResponsesStreamHandler(ctx, info, resp)
	require.Nil(t, usage)
	return recorder, ctx, relayErr
}
