package helper

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	settingconfig "github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHasForcedResponsesImageGenerationToolRequiresExplicitChoice(t *testing.T) {
	tests := []struct {
		name       string
		toolChoice string
		want       bool
	}{
		{name: "object choice", toolChoice: `{"type":"image_generation"}`, want: true},
		{name: "string choice", toolChoice: `"image_generation"`, want: true},
		{name: "auto choice", toolChoice: `"auto"`, want: false},
		{name: "missing choice", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &dto.OpenAIResponsesRequest{
				Model: "gpt-5.6-sol",
				Tools: json.RawMessage(`[{"type":"image_generation"}]`),
			}
			if tt.toolChoice != "" {
				request.ToolChoice = json.RawMessage(tt.toolChoice)
			}
			assert.Equal(t, tt.want, hasForcedResponsesImageGenerationTool(request))
		})
	}

	t.Run("ordinary tools do not match", func(t *testing.T) {
		request := &dto.OpenAIResponsesRequest{
			Model:      "gpt-5.6-sol",
			Tools:      json.RawMessage(`[{"type":"web_search_preview"}]`),
			ToolChoice: json.RawMessage(`{"type":"image_generation"}`),
		}
		assert.False(t, hasForcedResponsesImageGenerationTool(request))
	})
}

func dynamicRoutingRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		OriginModelName:        "gemini-3.7-flash",
		OriginalRequestURLPath: "/v1/chat/completions?trace=true",
		ReasoningEffort:        "high",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: 1,
		},
	}
}

func dynamicRoutingRule(id string, target string) dto.DynamicRoutingRule {
	return dto.DynamicRoutingRule{
		ID:          id,
		Enabled:     true,
		SourceModel: "gemini-3.7-flash",
		TargetModel: target,
	}
}

func TestResolveDynamicModelRouteSelectsMatchingRuleByPriority(t *testing.T) {
	info := dynamicRoutingRelayInfo()
	request := &dto.GeneralOpenAIRequest{Model: info.OriginModelName}
	lowPriority := dynamicRoutingRule("fallback-high", "gemini-3.7-flash-high-a")
	lowPriority.Conditions = []dto.DynamicRoutingCondition{{
		Field: dto.DynamicRoutingConditionReasoningEffort,
		Value: "high",
	}}
	lowPriority.Priority = 10
	highPriority := dynamicRoutingRule("preferred-high", "gemini-3.7-flash-high-b")
	highPriority.Conditions = []dto.DynamicRoutingCondition{{
		Field: dto.DynamicRoutingConditionReasoningEffort,
		Value: "high",
	}}
	highPriority.Priority = 20

	rule, matched := resolveDynamicModelRoute(info, request, dto.DynamicRoutingConfig{
		Enabled: true,
		Rules:   []dto.DynamicRoutingRule{lowPriority, highPriority},
	})

	require.True(t, matched)
	assert.Equal(t, "preferred-high", rule.ID)
	assert.Equal(t, "gemini-3.7-flash-high-b", rule.TargetModel)
}

func TestResolveDynamicModelRouteUsesRequestReasoningEffortWhenInfoCleared(t *testing.T) {
	info := dynamicRoutingRelayInfo()
	info.OriginalRequestURLPath = "/v1/responses"
	info.ReasoningEffort = ""
	request := &dto.OpenAIResponsesRequest{
		Model:     info.OriginModelName,
		Reasoning: &dto.Reasoning{Effort: "High"},
	}
	rule := dynamicRoutingRule("responses-high", "gemini-3.7-flash-high")
	rule.RequestPaths = []string{"/v1/responses"}
	rule.Conditions = []dto.DynamicRoutingCondition{{
		Field: dto.DynamicRoutingConditionReasoningEffort,
		Value: "HIGH",
	}}

	matchedRule, matched := resolveDynamicModelRoute(info, request, dto.DynamicRoutingConfig{
		Enabled: true,
		Rules:   []dto.DynamicRoutingRule{rule},
	})

	require.True(t, matched)
	assert.Equal(t, "responses-high", matchedRule.ID)
}

func TestResolveDynamicModelRouteMatchesPathAndRequestCondition(t *testing.T) {
	info := dynamicRoutingRelayInfo()
	request := &dto.GeneralOpenAIRequest{
		Model:          info.OriginModelName,
		ResponseFormat: &dto.ResponseFormat{Type: "json_object"},
	}
	rule := dynamicRoutingRule("json-chat", "gemini-3.7-flash-json")
	rule.ChannelTypes = []int{1}
	rule.RequestPaths = []string{"/v1/chat/completions"}
	rule.Conditions = []dto.DynamicRoutingCondition{{
		Field: dto.DynamicRoutingConditionRequestPrefix + "response_format.type",
		Value: "json_object",
	}}

	matchedRule, matched := resolveDynamicModelRoute(info, request, dto.DynamicRoutingConfig{
		Enabled: true,
		Rules:   []dto.DynamicRoutingRule{rule},
	})

	require.True(t, matched)
	assert.Equal(t, "json-chat", matchedRule.ID)
}

func TestResolveDynamicModelRouteMatchesSelectedSourceGroup(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{Model: "gemini-3.7-flash"}
	rule := dynamicRoutingRule("image-group", "gemini-3.7-flash-image")
	rule.SourceGroups = []string{"image"}

	matchedInfo := dynamicRoutingRelayInfo()
	matchedInfo.UsingGroup = "image"
	matchedRule, matched := resolveDynamicModelRoute(matchedInfo, request, dto.DynamicRoutingConfig{
		Enabled: true,
		Rules:   []dto.DynamicRoutingRule{rule},
	})
	require.True(t, matched)
	assert.Equal(t, "image-group", matchedRule.ID)

	unmatchedInfo := dynamicRoutingRelayInfo()
	unmatchedInfo.UsingGroup = "default"
	_, matched = resolveDynamicModelRoute(unmatchedInfo, request, dto.DynamicRoutingConfig{
		Enabled: true,
		Rules:   []dto.DynamicRoutingRule{rule},
	})
	assert.False(t, matched)
}

func TestResolveDynamicModelRouteChannelOverrideAndFallback(t *testing.T) {
	globalRule := dynamicRoutingRule("global", "gemini-3.7-flash-global")
	globalConfig := dto.DynamicRoutingConfig{Enabled: true, Rules: []dto.DynamicRoutingRule{globalRule}}
	request := &dto.GeneralOpenAIRequest{Model: "gemini-3.7-flash"}

	t.Run("channel rule wins for the same source model", func(t *testing.T) {
		info := dynamicRoutingRelayInfo()
		channelRule := dynamicRoutingRule("channel", "gemini-3.7-flash-channel")
		info.ChannelSetting.DynamicRouting = &dto.DynamicRoutingChannelConfig{
			Rules: []dto.DynamicRoutingRule{channelRule},
		}

		rule, matched := resolveDynamicModelRoute(info, request, globalConfig)
		require.True(t, matched)
		assert.Equal(t, "channel", rule.ID)
	})

	t.Run("no channel rule for model falls back to global", func(t *testing.T) {
		info := dynamicRoutingRelayInfo()
		channelRule := dynamicRoutingRule("other-model", "gemini-3.7-flash-channel")
		channelRule.SourceModel = "other-public-model"
		info.ChannelSetting.DynamicRouting = &dto.DynamicRoutingChannelConfig{
			Rules: []dto.DynamicRoutingRule{channelRule},
		}

		rule, matched := resolveDynamicModelRoute(info, request, globalConfig)
		require.True(t, matched)
		assert.Equal(t, "global", rule.ID)
	})

	t.Run("matching channel scope suppresses global when condition does not match", func(t *testing.T) {
		info := dynamicRoutingRelayInfo()
		channelRule := dynamicRoutingRule("channel-low-only", "gemini-3.7-flash-low")
		channelRule.Conditions = []dto.DynamicRoutingCondition{{
			Field: dto.DynamicRoutingConditionReasoningEffort,
			Value: "low",
		}}
		info.ChannelSetting.DynamicRouting = &dto.DynamicRoutingChannelConfig{
			Rules: []dto.DynamicRoutingRule{channelRule},
		}

		_, matched := resolveDynamicModelRoute(info, request, globalConfig)
		assert.False(t, matched)
	})

	t.Run("explicit channel disable blocks global routing", func(t *testing.T) {
		info := dynamicRoutingRelayInfo()
		disabled := false
		info.ChannelSetting.DynamicRouting = &dto.DynamicRoutingChannelConfig{Enabled: &disabled}

		_, matched := resolveDynamicModelRoute(info, request, globalConfig)
		assert.False(t, matched)
	})
}

func TestModelMappedHelperDynamicRoutePrecedesStaticMappingAndFallsBack(t *testing.T) {
	registered, ok := settingconfig.GlobalConfig.Get("dynamic_routing").(*dto.DynamicRoutingConfig)
	require.True(t, ok, "dynamic routing settings must be registered")
	previous := *registered
	previous.Rules = append([]dto.DynamicRoutingRule(nil), previous.Rules...)
	t.Cleanup(func() {
		*registered = previous
	})

	dynamicRule := dynamicRoutingRule("dynamic", "gemini-3.7-flash-high")
	*registered = dto.DynamicRoutingConfig{
		Enabled: true,
		Rules:   []dto.DynamicRoutingRule{dynamicRule},
	}

	gin.SetMode(gin.TestMode)
	newContext := func() *gin.Context {
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		context.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		context.Set("model_mapping", `{"gemini-3.7-flash":"static-target"}`)
		return context
	}

	dynamicInfo := dynamicRoutingRelayInfo()
	dynamicRequest := &dto.GeneralOpenAIRequest{Model: dynamicInfo.OriginModelName}
	require.NoError(t, ModelMappedHelper(newContext(), dynamicInfo, dynamicRequest))
	assert.Equal(t, "gemini-3.7-flash-high", dynamicInfo.UpstreamModelName)
	assert.True(t, dynamicInfo.IsDynamicModelRouted)
	assert.True(t, dynamicInfo.ForceRequestConversion)
	assert.Equal(t, "dynamic", dynamicInfo.DynamicRoutingRuleID)
	assert.Equal(t, "gemini-3.7-flash-high", dynamicRequest.Model)

	*registered = dto.DynamicRoutingConfig{Enabled: false, Rules: []dto.DynamicRoutingRule{dynamicRule}}
	staticInfo := dynamicRoutingRelayInfo()
	staticRequest := &dto.GeneralOpenAIRequest{Model: staticInfo.OriginModelName}
	require.NoError(t, ModelMappedHelper(newContext(), staticInfo, staticRequest))
	assert.Equal(t, "static-target", staticInfo.UpstreamModelName)
	assert.False(t, staticInfo.IsDynamicModelRouted)
	assert.True(t, staticInfo.ForceRequestConversion)
	assert.Equal(t, "static-target", staticRequest.Model)
}
