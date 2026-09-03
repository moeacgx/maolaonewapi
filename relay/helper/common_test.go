package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestEventStreamHeadersCanBeRestoredAfterRetryReset(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	SetEventStreamHeaders(ctx)
	ResetEventStreamHeadersForRetry(ctx)
	SetEventStreamHeaders(ctx)

	assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", recorder.Header().Get("Cache-Control"))
	assert.Equal(t, "keep-alive", recorder.Header().Get("Connection"))
	assert.Equal(t, "no", recorder.Header().Get("X-Accel-Buffering"))
}

func TestRetryResetDropsAttemptScopedCodexHeaders(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	firstResponse := &http.Response{Header: http.Header{
		"X-Reasoning-Included": []string{"first"},
		"X-Codex-Turn-State":   []string{"turn-a"},
	}}
	secondResponse := &http.Response{Header: http.Header{
		"X-Reasoning-Included": []string{"second"},
		"X-Codex-Turn-State":   []string{"turn-b"},
	}}

	copyCodexSSEHeaders(ctx, firstResponse)
	ResetEventStreamHeadersForRetry(ctx)
	copyCodexSSEHeaders(ctx, secondResponse)

	assert.Equal(t, []string{"second"}, recorder.Header().Values("X-Reasoning-Included"))
	assert.Equal(t, []string{"turn-b"}, recorder.Header().Values("X-Codex-Turn-State"))
}
