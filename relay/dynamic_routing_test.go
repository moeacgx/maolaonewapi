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
	settingconfig "github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeHelperDynamicRouteKeepsTargetThinkingSuffix(t *testing.T) {
	registered, ok := settingconfig.GlobalConfig.Get("dynamic_routing").(*dto.DynamicRoutingConfig)
	require.True(t, ok, "dynamic routing settings must be registered")
	previous := *registered
	previous.Rules = append([]dto.DynamicRoutingRule(nil), previous.Rules...)
	t.Cleanup(func() {
		*registered = previous
	})

	*registered = dto.DynamicRoutingConfig{
		Enabled: true,
		Rules: []dto.DynamicRoutingRule{{
			ID:          "claude-high",
			Enabled:     true,
			SourceModel: "public-claude",
			TargetModel: "claude-opus-4-7-high",
			Conditions: []dto.DynamicRoutingCondition{{
				Field: dto.DynamicRoutingConditionReasoningEffort,
				Value: "high",
			}},
		}},
	}

	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	capturedBody := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		capturedBody <- body
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"captured"}}`))
	}))
	t.Cleanup(server.Close)

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	context.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(context, constant.ContextKeyChannelType, constant.ChannelTypeAnthropic)
	common.SetContextKey(context, constant.ContextKeyChannelBaseUrl, server.URL)
	common.SetContextKey(context, constant.ContextKeyChannelKey, "test-key")
	common.SetContextKey(context, constant.ContextKeyOriginalModel, "public-claude")

	maxTokens := uint(1280)
	request := &dto.ClaudeRequest{
		Model:        "public-claude",
		MaxTokens:    &maxTokens,
		OutputConfig: json.RawMessage(`{"effort":"high"}`),
		Messages:     []dto.ClaudeMessage{{Role: "user", Content: "hello"}},
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "public-claude",
		Request:         request,
	}

	apiErr := ClaudeHelper(context, info)
	require.NotNil(t, apiErr)
	require.True(t, info.HasDynamicModelRoute())
	assert.Equal(t, "claude-opus-4-7-high", info.UpstreamModelName)

	var outbound map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(<-capturedBody, &outbound))
	require.JSONEq(t, `"claude-opus-4-7-high"`, string(outbound["model"]))
	assert.NotContains(t, outbound, "thinking")
	require.Contains(t, outbound, "output_config")
	require.JSONEq(t, `{"effort":"high"}`, string(outbound["output_config"]))
}
