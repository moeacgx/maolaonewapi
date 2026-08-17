package relay

import (
	"encoding/json"
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
)

func TestClaudeHelperNormalizesSamplingAfterModelMapping(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	tests := []struct {
		name           string
		originModel    string
		upstreamModel  string
		expectSampling bool
	}{
		{
			name:           "mapped to Sonnet 4.6 preserves sampling",
			originModel:    "claude-opus-4-7-client-sonnet",
			upstreamModel:  "claude-sonnet-4-6",
			expectSampling: true,
		},
		{
			name:           "mapped to Opus 4.6 preserves sampling",
			originModel:    "claude-opus-4-7-client-opus",
			upstreamModel:  "claude-opus-4-6",
			expectSampling: true,
		},
		{
			name:           "mapped to Opus 4.7 strips sampling",
			originModel:    "claude-opus-4-6-client-47",
			upstreamModel:  "claude-opus-4-7",
			expectSampling: false,
		},
		{
			name:           "mapped to Opus 4.8 strips sampling",
			originModel:    "claude-opus-4-6-client-48",
			upstreamModel:  "claude-opus-4-8-20260801",
			expectSampling: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capturedBody := make(chan []byte, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				capturedBody <- body
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"captured"}}`))
			}))
			t.Cleanup(server.Close)

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("model_mapping", `{"`+tt.originModel+`":"`+tt.upstreamModel+`"}`)
			common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeAnthropic)
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL)
			common.SetContextKey(c, constant.ContextKeyChannelKey, "test-key")
			common.SetContextKey(c, constant.ContextKeyOriginalModel, tt.originModel)

			zeroFloat := 0.0
			zeroInt := 0
			maxTokens := uint(64)
			request := &dto.ClaudeRequest{
				Model:       tt.originModel,
				MaxTokens:   &maxTokens,
				Temperature: &zeroFloat,
				TopP:        &zeroFloat,
				TopK:        &zeroInt,
				Messages:    []dto.ClaudeMessage{{Role: "user", Content: "hello"}},
			}
			info := &relaycommon.RelayInfo{
				OriginModelName: tt.originModel,
				RelayFormat:     types.RelayFormatClaude,
				Request:         request,
			}

			apiErr := ClaudeHelper(c, info)
			require.NotNil(t, apiErr)

			var outbound map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(<-capturedBody, &outbound))
			require.JSONEq(t, `"`+tt.upstreamModel+`"`, string(outbound["model"]))
			assert.Equal(t, tt.upstreamModel, info.UpstreamModelName)
			for _, field := range []string{"temperature", "top_p", "top_k"} {
				value, present := outbound[field]
				if tt.expectSampling {
					assert.True(t, present, "%s should be preserved", field)
					assert.JSONEq(t, "0", string(value))
				} else {
					assert.False(t, present, "%s should be stripped", field)
				}
			}
		})
	}
}

func TestClaudeHelperReasoningSuffixOnlyRewritesSupportedFamilies(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	tests := []struct {
		name         string
		model        string
		wantModel    string
		wantType     string
		wantThinking bool
		wantEffort   string
	}{
		{
			name:         "opus 4.7 effort suffix",
			model:        "claude-opus-4-7-20260217-high",
			wantModel:    "claude-opus-4-7-20260217",
			wantType:     "adaptive",
			wantThinking: true,
			wantEffort:   "high",
		},
		{
			name:         "opus 4.6 thinking",
			model:        "claude-opus-4-6-thinking",
			wantModel:    "claude-opus-4-6",
			wantType:     "enabled",
			wantThinking: true,
		},
		{
			name:         "opus 4.7 dated alias thinking",
			model:        "claude-opus-4-7-20260217-thinking",
			wantModel:    "claude-opus-4-7-20260217",
			wantType:     "adaptive",
			wantThinking: true,
			wantEffort:   "high",
		},
		{
			name:      "opus 4.60 effort collision",
			model:     "claude-opus-4-60-high",
			wantModel: "claude-opus-4-60-high",
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
			capturedBody := make(chan []byte, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				capturedBody <- body
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"captured"}}`))
			}))
			t.Cleanup(server.Close)

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			c.Request.Header.Set("Content-Type", "application/json")
			common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeAnthropic)
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL)
			common.SetContextKey(c, constant.ContextKeyChannelKey, "test-key")
			common.SetContextKey(c, constant.ContextKeyOriginalModel, tt.model)

			maxTokens := uint(8192)
			request := &dto.ClaudeRequest{
				Model:     tt.model,
				MaxTokens: &maxTokens,
				Messages:  []dto.ClaudeMessage{{Role: "user", Content: "hello"}},
			}
			info := &relaycommon.RelayInfo{
				OriginModelName:   tt.model,
				UpstreamModelName: tt.model,
				RelayFormat:       types.RelayFormatClaude,
				Request:           request,
			}

			apiErr := ClaudeHelper(c, info)
			require.NotNil(t, apiErr)

			var outbound map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(<-capturedBody, &outbound))
			require.JSONEq(t, `"`+tt.wantModel+`"`, string(outbound["model"]))
			assert.Equal(t, tt.wantModel, info.UpstreamModelName)
			assert.Equal(t, tt.wantEffort, info.ReasoningEffort)
			if tt.wantEffort != "" {
				assert.JSONEq(t, `{"effort":"`+tt.wantEffort+`"}`, string(outbound["output_config"]))
			} else {
				assert.NotContains(t, outbound, "output_config")
			}
			if tt.wantThinking {
				var thinking map[string]any
				require.NoError(t, json.Unmarshal(outbound["thinking"], &thinking))
				assert.Equal(t, tt.wantType, thinking["type"])
			} else {
				assert.NotContains(t, outbound, "thinking")
			}
		})
	}
}
