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
	"github.com/QuantumNous/new-api/setting/model_setting"
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
	claudeSettings := model_setting.GetClaudeSettings()
	originalAdapterEnabled := claudeSettings.ThinkingAdapterEnabled
	originalBudgetPercentage := claudeSettings.ThinkingAdapterBudgetTokensPercentage
	claudeSettings.ThinkingAdapterEnabled = true
	claudeSettings.ThinkingAdapterBudgetTokensPercentage = 0.8
	globalSettings := model_setting.GetGlobalSettings()
	originalBlacklist := append([]string(nil), globalSettings.ThinkingModelBlacklist...)
	t.Cleanup(func() {
		gin.SetMode(oldMode)
		claudeSettings.ThinkingAdapterEnabled = originalAdapterEnabled
		claudeSettings.ThinkingAdapterBudgetTokensPercentage = originalBudgetPercentage
		globalSettings.ThinkingModelBlacklist = originalBlacklist
	})

	tests := []struct {
		name           string
		model          string
		originModel    string
		blacklistModel string
		wantModel      string
		wantType       string
		wantThinking   bool
		wantBudget     bool
		wantEffort     string
		preserve       bool
	}{
		{name: "Sonnet 4.6 exact", model: "claude-sonnet-4-6-thinking", wantModel: "claude-sonnet-4-6", wantType: "enabled", wantThinking: true, wantBudget: true},
		{name: "Sonnet 4.6 dated alias", model: "claude-sonnet-4-6-20260217-thinking", wantModel: "claude-sonnet-4-6-20260217", wantType: "enabled", wantThinking: true, wantBudget: true},
		{name: "Sonnet 4.5 exact", model: "claude-sonnet-4-5-thinking", wantModel: "claude-sonnet-4-5", wantType: "enabled", wantThinking: true, wantBudget: true},
		{name: "Sonnet 4.5 dated alias", model: "claude-sonnet-4-5-20250929-thinking", wantModel: "claude-sonnet-4-5-20250929", wantType: "enabled", wantThinking: true, wantBudget: true},
		{name: "Claude 3.7 Sonnet exact", model: "claude-3-7-sonnet-thinking", wantModel: "claude-3-7-sonnet", wantType: "enabled", wantThinking: true, wantBudget: true},
		{name: "Claude 3.7 Sonnet dated alias", model: "claude-3-7-sonnet-20250219-thinking", wantModel: "claude-3-7-sonnet-20250219", wantType: "enabled", wantThinking: true, wantBudget: true},
		{name: "Opus 4.6 exact", model: "claude-opus-4-6-thinking", wantModel: "claude-opus-4-6", wantType: "enabled", wantThinking: true, wantBudget: true},
		{name: "Opus 4.6 dated alias", model: "claude-opus-4-6-20260201-thinking", wantModel: "claude-opus-4-6-20260201", wantType: "enabled", wantThinking: true, wantBudget: true},
		{name: "Opus 4.7 exact", model: "claude-opus-4-7-thinking", wantModel: "claude-opus-4-7", wantType: "adaptive", wantThinking: true, wantEffort: "high"},
		{name: "Opus 4.7 dated alias", model: "claude-opus-4-7-20260217-thinking", wantModel: "claude-opus-4-7-20260217", wantType: "adaptive", wantThinking: true, wantEffort: "high"},
		{name: "Opus 4.8 exact", model: "claude-opus-4-8-thinking", wantModel: "claude-opus-4-8", wantType: "adaptive", wantThinking: true, wantEffort: "high"},
		{name: "Opus 4.8 dated alias", model: "claude-opus-4-8-20260801-thinking", wantModel: "claude-opus-4-8-20260801", wantType: "adaptive", wantThinking: true, wantEffort: "high"},
		{name: "preserve enabled-budget suffix", model: "claude-sonnet-4-6-thinking", wantModel: "claude-sonnet-4-6-thinking", wantType: "enabled", wantThinking: true, wantBudget: true, preserve: true},
		{name: "preserve adaptive suffix", model: "claude-opus-4-7-20260217-thinking", wantModel: "claude-opus-4-7-20260217-thinking", wantType: "adaptive", wantThinking: true, wantEffort: "high", preserve: true},
		{name: "preserve mapped origin thinking alias", model: "claude-sonnet-4-6-thinking", originModel: "provider/sonnet-preserved-thinking", blacklistModel: "provider/sonnet-preserved-thinking", wantModel: "claude-sonnet-4-6-thinking", wantType: "enabled", wantThinking: true, wantBudget: true, preserve: true},
		{name: "preserve canonical effort suffix", model: "claude-opus-4-7-20260217-high", wantModel: "claude-opus-4-7-20260217-high", wantType: "adaptive", wantThinking: true, wantEffort: "high", preserve: true},
		{name: "preserve mapped origin effort alias", model: "claude-opus-4-7-20260217-high", originModel: "provider/opus-preserved-high", blacklistModel: "provider/opus-preserved-high", wantModel: "claude-opus-4-7-20260217-high", wantType: "adaptive", wantThinking: true, wantEffort: "high", preserve: true},
		{name: "Opus 4.7 effort suffix", model: "claude-opus-4-7-20260217-high", wantModel: "claude-opus-4-7-20260217", wantType: "adaptive", wantThinking: true, wantEffort: "high"},
		{name: "custom Opus effort collision", model: "claude-opus-4-7-enterprise-high", wantModel: "claude-opus-4-7-enterprise-high"},
		{name: "Opus 4.60 collision", model: "claude-opus-4-60-thinking", wantModel: "claude-opus-4-60-thinking"},
		{name: "Sonnet 4.60 collision", model: "claude-sonnet-4-60-thinking", wantModel: "claude-sonnet-4-60-thinking"},
		{name: "custom family suffix", model: "claude-sonnet-4-6-enterprise-thinking", wantModel: "claude-sonnet-4-6-enterprise-thinking"},
		{name: "dated alias extra segment", model: "claude-opus-4-7-20260217-custom-thinking", wantModel: "claude-opus-4-7-20260217-custom-thinking"},
		{name: "malformed dated alias", model: "claude-3-7-sonnet-2025beta-thinking", wantModel: "claude-3-7-sonnet-2025beta-thinking"},
		{name: "unrelated slash Claude collision", model: "claude/custom-thinking", wantModel: "claude/custom-thinking"},
		{name: "unrelated Claude collision", model: "claude-custom-thinking", wantModel: "claude-custom-thinking"},
		{name: "publisher-prefixed collision", model: "anthropic/claude-opus-4-8-thinking", wantModel: "anthropic/claude-opus-4-8-thinking"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globalSettings.ThinkingModelBlacklist = nil
			if tt.preserve {
				blacklistModel := tt.blacklistModel
				if blacklistModel == "" {
					blacklistModel = tt.model
				}
				globalSettings.ThinkingModelBlacklist = []string{blacklistModel}
			}
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
			originModel := tt.model
			if tt.originModel != "" {
				originModel = tt.originModel
				c.Set("model_mapping", "{\""+tt.originModel+"\":\""+tt.model+"\"}")
			}
			common.SetContextKey(c, constant.ContextKeyOriginalModel, originModel)
			maxTokens := uint(8192)
			request := &dto.ClaudeRequest{
				Model:     tt.model,
				MaxTokens: &maxTokens,
				Messages:  []dto.ClaudeMessage{{Role: "user", Content: "hello"}},
			}
			info := &relaycommon.RelayInfo{
				OriginModelName: originModel,
				Request:         request,
				ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: originModel,
				},
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
				var thinking struct {
					Type         string `json:"type"`
					BudgetTokens *int   `json:"budget_tokens"`
				}
				require.NoError(t, json.Unmarshal(outbound["thinking"], &thinking))
				assert.Equal(t, tt.wantType, thinking.Type)
				if tt.wantBudget {
					require.NotNil(t, thinking.BudgetTokens)
					assert.Equal(t, 6553, *thinking.BudgetTokens)
				} else {
					assert.Nil(t, thinking.BudgetTokens)
				}
			} else {
				assert.NotContains(t, outbound, "thinking")
			}
		})
	}
}
func TestShouldPreserveClaudeSuffixUsesOriginAndInputFallback(t *testing.T) {
	globalSettings := model_setting.GetGlobalSettings()
	originalBlacklist := append([]string(nil), globalSettings.ThinkingModelBlacklist...)
	t.Cleanup(func() { globalSettings.ThinkingModelBlacklist = originalBlacklist })

	globalSettings.ThinkingModelBlacklist = []string{"provider/origin-high"}
	assert.True(t, shouldPreserveClaudeSuffix(&relaycommon.RelayInfo{OriginModelName: "provider/origin-high"}, "claude-opus-4-7-high"))

	globalSettings.ThinkingModelBlacklist = []string{"claude-opus-4-7-high"}
	assert.True(t, shouldPreserveClaudeSuffix(&relaycommon.RelayInfo{}, "claude-opus-4-7-high"))
	assert.True(t, shouldPreserveClaudeSuffix(nil, "claude-opus-4-7-high"))
}
