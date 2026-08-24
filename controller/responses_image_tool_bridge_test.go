package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildResponsesImageToolBridgeRequestUsesImageModelAndPrompt(t *testing.T) {
	request := &dto.OpenAIResponsesRequest{
		Model:      "gpt-5.6-sol",
		Input:      json.RawMessage(`"draw a red fox"`),
		ToolChoice: json.RawMessage(`{"type":"image_generation"}`),
		Tools:      json.RawMessage(`[{"type":"image_generation"}]`),
	}

	imageRequest, err := buildResponsesImageToolBridgeRequest(request, "gpt-image-2")

	require.NoError(t, err)
	require.NotNil(t, imageRequest)
	assert.Equal(t, "gpt-image-2", imageRequest.Model)
	assert.Equal(t, "draw a red fox", imageRequest.Prompt)
	require.NotNil(t, imageRequest.N)
	assert.Equal(t, uint(1), *imageRequest.N)
	assert.Equal(t, "b64_json", imageRequest.ResponseFormat)
}

func TestBuildResponsesImageToolBridgeRequestRejectsImageInputs(t *testing.T) {
	request := &dto.OpenAIResponsesRequest{
		Model: "gpt-5.6-sol",
		Input: json.RawMessage(`[{"type":"message","content":[{"type":"input_image","image_url":"data:image/png;base64,abc"}]}]`),
		Tools: json.RawMessage(`[{"type":"image_generation"}]`),
	}

	_, err := buildResponsesImageToolBridgeRequest(request, "gpt-image-2")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "text-to-image generation only")
}

func TestBuildResponsesImageToolBridgeRequestCopiesSupportedImageOptions(t *testing.T) {
	request := &dto.OpenAIResponsesRequest{
		Model: "gpt-5.6-sol",
		Input: json.RawMessage(`"draw a red fox"`),
		Tools: json.RawMessage(`[{"type":"image_generation","model":"gpt-image-1","size":"1536x1024","quality":"high","background":"transparent","moderation":"low","output_format":"png","output_compression":80,"partial_images":0}]`),
	}

	imageRequest, err := buildResponsesImageToolBridgeRequest(request, "gpt-image-2")

	require.NoError(t, err)
	assert.Equal(t, "1536x1024", imageRequest.Size)
	assert.Equal(t, "high", imageRequest.Quality)
	assert.JSONEq(t, `"transparent"`, string(imageRequest.Background))
	assert.JSONEq(t, `"low"`, string(imageRequest.Moderation))
	assert.JSONEq(t, `"png"`, string(imageRequest.OutputFormat))
	assert.JSONEq(t, `80`, string(imageRequest.OutputCompression))
}

func TestBuildResponsesImageToolBridgeRequestRejectsUnsupportedImageOptions(t *testing.T) {
	tests := []struct {
		name  string
		tools string
		want  string
	}{
		{
			name:  "partial images",
			tools: `[{"type":"image_generation","partial_images":1}]`,
			want:  "partial_images 0",
		},
		{
			name:  "edit-only option",
			tools: `[{"type":"image_generation","input_fidelity":"high"}]`,
			want:  "input_fidelity",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := &dto.OpenAIResponsesRequest{
				Model: "gpt-5.6-sol",
				Input: json.RawMessage(`"draw a red fox"`),
				Tools: json.RawMessage(test.tools),
			}

			_, err := buildResponsesImageToolBridgeRequest(request, "gpt-image-2")

			require.Error(t, err)
			assert.Contains(t, err.Error(), test.want)
		})
	}
}

func TestSetResponsesImageToolBridgeModelOverridesImageToolModel(t *testing.T) {
	request := &dto.OpenAIResponsesRequest{
		Model: "gpt-5.6-sol",
		Tools: json.RawMessage(`[{"type":"image_generation","model":"gpt-image-1","size":"1024x1024"},{"type":"web_search"}]`),
	}

	err := setResponsesImageToolBridgeModel(request, "gpt-image-2")

	require.NoError(t, err)
	var tools []map[string]any
	require.NoError(t, common.Unmarshal(request.Tools, &tools))
	require.Len(t, tools, 2)
	assert.Equal(t, "gpt-image-2", tools[0]["model"])
	assert.Equal(t, "1024x1024", tools[0]["size"])
	assert.Equal(t, "web_search", tools[1]["type"])
}

func TestApplyResponsesImageToolBridgeBillingDefaultsPreConsumesOneImage(t *testing.T) {
	meta := &types.TokenCountMeta{}
	info := &relaycommon.RelayInfo{
		ResponsesImageToolBridge: &relaycommon.ResponsesImageToolBridge{},
	}

	applyResponsesImageToolBridgeBillingDefaults(info, meta)

	require.NotNil(t, meta.BillingRatios)
	assert.Equal(t, 1.0, meta.BillingRatios["n"])
}

func TestValidateResponsesImageToolBridgeTargetBillingRejectsTieredExpr(t *testing.T) {
	withTieredBillingConfig(t, map[string]string{
		"gpt-image-tiered": billing_setting.BillingModeTieredExpr,
	}, map[string]string{
		"gpt-image-tiered": `tier("base", p * 2 + c * 8)`,
	})

	err := validateResponsesImageToolBridgeTargetBilling("gpt-image-tiered")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot use tiered_expr billing")
	require.NoError(t, validateResponsesImageToolBridgeTargetBilling("gpt-image-2"))
}

func TestResolveResponsesImageToolBridgeTargetGroupAuthorization(t *testing.T) {
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalRatios := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","image":"Image"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"image":1}`))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatios))
	})

	newContext := func() (*gin.Context, *relaycommon.RelayInfo) {
		gin.SetMode(gin.TestMode)
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		context.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		common.SetContextKey(context, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(context, constant.ContextKeyUsingGroup, "default")
		common.SetContextKey(context, constant.ContextKeySelectedChannelGroup, "default")
		return context, &relaycommon.RelayInfo{UsingGroup: "default"}
	}

	t.Run("inherited token keeps current group", func(t *testing.T) {
		context, info := newContext()
		group, apiErr := resolveResponsesImageToolBridgeTargetGroup(context, info, dto.DynamicRoutingRule{TargetGroup: "default"})

		require.Nil(t, apiErr)
		assert.Equal(t, "default", group)
	})

	t.Run("empty target group inherits selected source group", func(t *testing.T) {
		context, info := newContext()
		common.SetContextKey(context, constant.ContextKeySelectedChannelGroup, "image")
		info.UsingGroup = "image"
		group, apiErr := resolveResponsesImageToolBridgeTargetGroup(context, info, dto.DynamicRoutingRule{})

		require.Nil(t, apiErr)
		assert.Equal(t, "image", group)
	})

	t.Run("inherited token cannot hop to a selectable group", func(t *testing.T) {
		context, info := newContext()
		_, apiErr := resolveResponsesImageToolBridgeTargetGroup(context, info, dto.DynamicRoutingRule{TargetGroup: "image"})

		require.NotNil(t, apiErr)
		assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
		assert.Equal(t, types.ErrorCodeAccessDenied, apiErr.GetErrorCode())
	})

	t.Run("explicit token can use its declared target group", func(t *testing.T) {
		context, info := newContext()
		common.SetContextKey(context, constant.ContextKeyTokenGroupMode, model.TokenGroupModeExplicit)
		common.SetContextKey(context, constant.ContextKeyTokenGroup, "default,image")
		common.SetContextKey(context, constant.ContextKeyTokenGroups, []string{"default", "image"})
		group, apiErr := resolveResponsesImageToolBridgeTargetGroup(context, info, dto.DynamicRoutingRule{TargetGroup: "image"})

		require.Nil(t, apiErr)
		assert.Equal(t, "image", group)
	})

	t.Run("explicit token rejects an undeclared target group", func(t *testing.T) {
		context, info := newContext()
		common.SetContextKey(context, constant.ContextKeyTokenGroupMode, model.TokenGroupModeExplicit)
		common.SetContextKey(context, constant.ContextKeyTokenGroup, "default")
		common.SetContextKey(context, constant.ContextKeyTokenGroups, []string{"default"})
		_, apiErr := resolveResponsesImageToolBridgeTargetGroup(context, info, dto.DynamicRoutingRule{TargetGroup: "image"})

		require.NotNil(t, apiErr)
		assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
		assert.Equal(t, types.ErrorCodeAccessDenied, apiErr.GetErrorCode())
	})

	t.Run("auto token can use its declared target group", func(t *testing.T) {
		context, info := newContext()
		common.SetContextKey(context, constant.ContextKeyTokenGroupMode, model.TokenGroupModeAuto)
		common.SetContextKey(context, constant.ContextKeyTokenGroup, model.TokenGroupModeAuto)
		common.SetContextKey(context, constant.ContextKeyTokenAutoGroups, []string{"image"})
		group, apiErr := resolveResponsesImageToolBridgeTargetGroup(context, info, dto.DynamicRoutingRule{TargetGroup: "image"})

		require.Nil(t, apiErr)
		assert.Equal(t, "image", group)
	})

	t.Run("auto token rejects an undeclared target group", func(t *testing.T) {
		context, info := newContext()
		common.SetContextKey(context, constant.ContextKeyTokenGroupMode, model.TokenGroupModeAuto)
		common.SetContextKey(context, constant.ContextKeyTokenGroup, model.TokenGroupModeAuto)
		common.SetContextKey(context, constant.ContextKeyTokenAutoGroups, []string{"default"})
		_, apiErr := resolveResponsesImageToolBridgeTargetGroup(context, info, dto.DynamicRoutingRule{TargetGroup: "image"})

		require.NotNil(t, apiErr)
		assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
		assert.Equal(t, types.ErrorCodeAccessDenied, apiErr.GetErrorCode())
	})
}

func TestSelectResponsesImageToolBridgeSpecificChannelChecksAdvancedCustomPath(t *testing.T) {
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMemoryCache := common.MemoryCacheEnabled
	db := setupModelListControllerTestDB(t)
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCache
	})

	const (
		group       = "image"
		targetModel = "gpt-image-2"
	)
	channel := &model.Channel{
		Name:   "advanced-custom-bridge",
		Key:    "advanced-custom-bridge-key",
		Type:   constant.ChannelTypeAdvancedCustom,
		Status: common.ChannelStatusEnabled,
		Group:  group,
		Models: targetModel,
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		AdvancedCustom: &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{
			IncomingPath: dto.DynamicRoutingResponsesPath,
			UpstreamPath: "/provider/responses",
			Models:       []string{targetModel},
		}}},
	})
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group: group, Model: targetModel, ChannelId: channel.Id, Enabled: true,
	}).Error)

	newContext := func() *gin.Context {
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		common.SetContextKey(context, constant.ContextKeyTokenSpecificChannelId, "1")
		common.SetContextKey(context, constant.ContextKeySelectedChannel, channel)
		return context
	}

	_, _, apiErr := selectResponsesImageToolBridgeChannel(
		newContext(),
		&relaycommon.RelayInfo{UsingGroup: group},
		targetModel,
		dto.DynamicRoutingImageGenerationPath,
		group,
	)
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.Equal(t, types.ErrorCodeModelNotFound, apiErr.GetErrorCode())

	channel.SetOtherSettings(dto.ChannelOtherSettings{
		AdvancedCustom: &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{
			IncomingPath: dto.DynamicRoutingImageGenerationPath,
			UpstreamPath: "/provider/images/generations",
			Models:       []string{targetModel},
		}}},
	})
	selected, selectedGroup, apiErr := selectResponsesImageToolBridgeChannel(
		newContext(),
		&relaycommon.RelayInfo{UsingGroup: group},
		targetModel,
		dto.DynamicRoutingImageGenerationPath,
		group,
	)
	require.Nil(t, apiErr)
	require.NotNil(t, selected)
	assert.Equal(t, channel.Id, selected.Id)
	assert.Equal(t, group, selectedGroup)
}

func TestResponsesImageToolBridgeRetryKeepsTargetGroup(t *testing.T) {
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMemoryCache := common.MemoryCacheEnabled
	db := setupModelListControllerTestDB(t)
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCache
	})

	const targetModel = "gpt-image-2"
	priority := int64(0)
	weight := uint(1)
	source := &model.Channel{
		Name: "bridge-source", Key: "bridge-source-key", Type: constant.ChannelTypeOpenAI,
		Status: common.ChannelStatusEnabled, Group: "codex", Models: "gpt-5.6-sol",
		Priority: &priority, Weight: &weight,
	}
	target := &model.Channel{
		Name: "bridge-target", Key: "bridge-target-key", Type: constant.ChannelTypeOpenAI,
		Status: common.ChannelStatusEnabled, Group: "image", Models: targetModel,
		Priority: &priority, Weight: &weight,
	}
	require.NoError(t, db.Create(source).Error)
	require.NoError(t, db.Create(target).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group: "codex", Model: "gpt-5.6-sol", ChannelId: source.Id, Enabled: true, Priority: &priority, Weight: weight,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group: "image", Model: targetModel, ChannelId: target.Id, Enabled: true, Priority: &priority, Weight: weight,
	}).Error)

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	common.SetContextKey(context, constant.ContextKeyUsingGroup, "codex")
	common.SetContextKey(context, constant.ContextKeySelectedChannelGroup, "codex")
	info := &relaycommon.RelayInfo{
		OriginModelName: targetModel,
		UsingGroup:      "codex",
		RetryIndex:      1,
		ResponsesImageToolBridge: &relaycommon.ResponsesImageToolBridge{
			TargetModel: targetModel,
			TargetPath:  dto.DynamicRoutingImageGenerationPath,
			TargetGroup: "image",
		},
	}
	retry := 1
	channel, apiErr := getChannel(context, info, &service.RetryParam{
		Ctx: context, TokenGroup: "image", ModelName: targetModel,
		RequestPath: dto.DynamicRoutingImageGenerationPath, Retry: &retry,
	})

	require.Nil(t, apiErr)
	require.NotNil(t, channel)
	assert.Equal(t, target.Id, channel.Id)
	assert.Equal(t, "image", info.UsingGroup)
	assert.Equal(t, "image", common.GetContextKeyString(context, constant.ContextKeyUsingGroup))
	assert.Equal(t, "image", common.GetContextKeyString(context, constant.ContextKeySelectedChannelGroup))
}

func TestResolveResponsesImageToolBridgeMatchInfoResolvesSpecificChannelSourceGroup(t *testing.T) {
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMemoryCache := common.MemoryCacheEnabled
	db := setupModelListControllerTestDB(t)
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCache
	})

	const sourceModel = "gpt-5.6-sol"
	channel := &model.Channel{
		Name: "bridge-specific-source", Key: "bridge-specific-source-key", Type: constant.ChannelTypeOpenAI,
		Status: common.ChannelStatusEnabled, Group: "codex", Models: sourceModel,
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group: "codex", Model: sourceModel, ChannelId: channel.Id, Enabled: true,
	}).Error)

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, dto.DynamicRoutingResponsesPath, nil)
	common.SetContextKey(context, constant.ContextKeyTokenSpecificChannelId, "1")
	common.SetContextKey(context, constant.ContextKeySelectedChannel, channel)
	common.SetContextKey(context, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(context, constant.ContextKeyTokenGroupMode, model.TokenGroupModeExplicit)
	common.SetContextKey(context, constant.ContextKeyTokenGroup, "codex,image")
	common.SetContextKey(context, constant.ContextKeyTokenGroups, []string{"codex", "image"})
	info := &relaycommon.RelayInfo{
		OriginModelName:        sourceModel,
		OriginalRequestURLPath: dto.DynamicRoutingResponsesPath,
		UsingGroup:             "codex,image",
	}

	matchInfo, group := resolveResponsesImageToolBridgeMatchInfo(context, info)

	require.NotNil(t, matchInfo)
	assert.Equal(t, "codex", group)
	assert.Equal(t, "codex", matchInfo.UsingGroup)
	assert.Equal(t, "codex,image", info.UsingGroup, "matching must not change normal fixed-channel behavior before a rule matches")
}
