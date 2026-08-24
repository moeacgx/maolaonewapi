package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"

	"github.com/gin-gonic/gin"
)

// prepareResponsesImageToolBridge resolves a channel/global bridge rule while
// the source Responses channel is still available, then switches the real
// relay selection, pricing and billing model to the configured target model.
// The target path and group are part of the rule, so the same public model can
// use either a native Responses image model or an Images API upstream.
func prepareResponsesImageToolBridge(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	request dto.Request,
) (dto.Request, *types.NewAPIError) {
	responsesRequest, ok := request.(*dto.OpenAIResponsesRequest)
	if !ok || responsesRequest == nil || info == nil {
		return request, nil
	}

	// Channel rules have priority over global rules, so resolve while the
	// distributor-selected source channel is still installed in the context.
	info.InitChannelMeta(c)
	matchInfo, resolvedSourceGroup := resolveResponsesImageToolBridgeMatchInfo(c, info)
	rule, matched := helper.ResolveDynamicResponsesImageToolBridge(matchInfo, responsesRequest)
	if !matched {
		// This eager initialization is only needed for matching. Preserve the
		// normal relay path exactly when no bridge rule applies.
		info.ChannelMeta = nil
		return request, nil
	}
	if resolvedSourceGroup != "" {
		info.UsingGroup = resolvedSourceGroup
		common.SetContextKey(c, constant.ContextKeyUsingGroup, resolvedSourceGroup)
		common.SetContextKey(c, constant.ContextKeySelectedChannelGroup, resolvedSourceGroup)
	}
	targetPath := dto.EffectiveDynamicRoutingTargetPath(rule)
	if !dto.IsSupportedDynamicRoutingImageTargetPath(targetPath) {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("unsupported responses image tool bridge target path %s", targetPath),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if err := validateResponsesImageToolBridgeTargetBilling(rule.TargetModel); err != nil {
		return nil, types.NewErrorWithStatusCode(
			err,
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}

	targetGroup, groupErr := resolveResponsesImageToolBridgeTargetGroup(c, info, rule)
	if groupErr != nil {
		return nil, groupErr
	}

	sourceModel := info.OriginModelName
	targetChannel, selectedGroup, selectErr := selectResponsesImageToolBridgeChannel(c, info, rule.TargetModel, targetPath, targetGroup)
	if selectErr != nil {
		return nil, selectErr
	}
	if selectedGroup != "" {
		info.UsingGroup = selectedGroup
		common.SetContextKey(c, constant.ContextKeyUsingGroup, selectedGroup)
		common.SetContextKey(c, constant.ContextKeySelectedChannelGroup, selectedGroup)
	}
	if c.Keys != nil {
		delete(c.Keys, string(constant.ContextKeyChannelPreferredMultiKeyIndex))
	}
	if setupErr := middleware.SetupContextForSelectedChannel(c, targetChannel, rule.TargetModel); setupErr != nil {
		return nil, setupErr
	}
	setSelectedSecurityAuditRoute(c, targetChannel, selectedGroup)

	info.OriginModelName = strings.TrimSpace(rule.TargetModel)
	info.RequestURLPath = targetPath
	info.ResponsesImageToolBridge = &relaycommon.ResponsesImageToolBridge{
		RuleID:           rule.ID,
		SourceModel:      sourceModel,
		TargetModel:      info.OriginModelName,
		TargetPath:       targetPath,
		TargetGroup:      selectedGroup,
		UseImagesAPI:     targetPath == dto.DynamicRoutingImageGenerationPath,
		DownstreamStream: responsesRequest.IsStream(c.Request),
	}
	info.ForceRequestConversion = true
	if targetPath == dto.DynamicRoutingResponsesPath {
		if err := setResponsesImageToolBridgeModel(responsesRequest, info.OriginModelName); err != nil {
			return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		responsesRequest.SetModelName(info.OriginModelName)
		info.Request = responsesRequest
		info.RelayMode = relayconstant.RelayModeResponses
		info.FinalRequestRelayFormat = types.RelayFormatOpenAIResponses
		info.ChannelMeta = nil
		return responsesRequest, nil
	}

	imageRequest, err := buildResponsesImageToolBridgeRequest(responsesRequest, info.OriginModelName)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	info.Request = imageRequest
	info.RelayMode = relayconstant.RelayModeImagesGenerations
	info.FinalRequestRelayFormat = types.RelayFormatOpenAIImage
	// The upstream Images call is buffered so it can be wrapped into complete
	// Responses events only after success. This also preserves the normal retry
	// path until an actual image result is available.
	info.IsStream = false
	// A downstream Responses SSE request commonly advertises text/event-stream.
	// This bridge deliberately requests a buffered Images JSON response and
	// emits Responses SSE itself after the image result is complete.
	c.Request.Header.Set("Accept", "application/json")
	info.ChannelMeta = nil
	return imageRequest, nil
}

// resolveResponsesImageToolBridgeMatchInfo gives scoped bridge rules a real
// source group for an administrator-selected channel. Unlike normal channel
// distribution, fixed-channel requests do not resolve a multi-group or Auto
// token to one group before the relay starts. The first authorized group that
// actually enables the fixed source channel mirrors ordered group selection.
// No context is changed unless a bridge rule later matches.
func resolveResponsesImageToolBridgeMatchInfo(
	c *gin.Context,
	info *relaycommon.RelayInfo,
) (*relaycommon.RelayInfo, string) {
	if info == nil {
		return info, ""
	}
	if sourceGroup := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeySelectedChannelGroup)); isSingleDynamicRoutingGroup(sourceGroup) {
		if sourceGroup == info.UsingGroup {
			return info, ""
		}
		matchInfo := *info
		matchInfo.UsingGroup = sourceGroup
		return &matchInfo, sourceGroup
	}
	if isSingleDynamicRoutingGroup(info.UsingGroup) {
		return info, ""
	}
	if _, fixedChannel := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId); !fixedChannel {
		return info, ""
	}

	selectedChannel, ok := common.GetContextKeyType[*model.Channel](c, constant.ContextKeySelectedChannel)
	if !ok || selectedChannel == nil {
		return info, ""
	}
	requestPath := dynamicRoutingBridgeSourceRequestPath(c, info)
	if !middleware.ChannelSupportsRequestPath(selectedChannel, requestPath, info.OriginModelName) {
		return info, ""
	}
	for _, group := range responsesImageToolBridgeAuthorizedGroups(c) {
		if !model.IsChannelEnabledForGroupModel(group, info.OriginModelName, selectedChannel.Id) {
			continue
		}
		matchInfo := *info
		matchInfo.UsingGroup = group
		return &matchInfo, group
	}
	return info, ""
}

func dynamicRoutingBridgeSourceRequestPath(c *gin.Context, info *relaycommon.RelayInfo) string {
	path := ""
	if info != nil {
		path = info.OriginalRequestURLPath
		if path == "" {
			path = info.RequestURLPath
		}
	}
	if path == "" && c != nil && c.Request != nil && c.Request.URL != nil {
		path = c.Request.URL.Path
	}
	if queryIndex := strings.IndexByte(path, '?'); queryIndex >= 0 {
		return path[:queryIndex]
	}
	return path
}

func isSingleDynamicRoutingGroup(group string) bool {
	group = strings.TrimSpace(group)
	return group != "" && !strings.EqualFold(group, model.TokenGroupModeAuto) && !strings.Contains(group, ",")
}

func responsesImageToolBridgeAuthorizedGroups(c *gin.Context) []string {
	if c == nil {
		return nil
	}
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	tokenGroup := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyTokenGroup))
	switch responsesImageToolBridgeTokenGroupMode(c, tokenGroup) {
	case model.TokenGroupModeAuto:
		return service.GetRequestAutoGroups(c, userGroup)
	case model.TokenGroupModeExplicit:
		return service.GetRequestTokenGroups(c, tokenGroup)
	default:
		if isSingleDynamicRoutingGroup(userGroup) {
			return []string{userGroup}
		}
		return nil
	}
}

func responsesImageToolBridgeTokenGroupMode(c *gin.Context, tokenGroup string) string {
	mode := strings.ToLower(strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyTokenGroupMode)))
	if mode != "" {
		return mode
	}
	switch {
	case strings.EqualFold(tokenGroup, model.TokenGroupModeAuto):
		return model.TokenGroupModeAuto
	case tokenGroup != "":
		return model.TokenGroupModeExplicit
	default:
		return model.TokenGroupModeInherit
	}
}

func resolveResponsesImageToolBridgeTargetGroup(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	rule dto.DynamicRoutingRule,
) (string, *types.NewAPIError) {
	selectedGroup := common.GetContextKeyString(c, constant.ContextKeySelectedChannelGroup)
	if selectedGroup == "" && info != nil {
		selectedGroup = info.UsingGroup
	}
	configuredGroup := strings.TrimSpace(rule.TargetGroup)
	if configuredGroup == "" {
		if !isSingleDynamicRoutingGroup(selectedGroup) {
			return "", types.NewErrorWithStatusCode(
				fmt.Errorf("dynamic routing image bridge requires target_group when the source group is unresolved"),
				types.ErrorCodeInvalidRequest,
				http.StatusBadRequest,
				types.ErrOptionWithSkipRetry(),
			)
		}
		return selectedGroup, nil
	}
	if !service.IsUserSelectableGroup(common.GetContextKeyString(c, constant.ContextKeyUserGroup), configuredGroup) {
		return "", types.NewErrorWithStatusCode(
			fmt.Errorf("dynamic routing target group %s is not available to this user", configuredGroup),
			types.ErrorCodeAccessDenied,
			http.StatusForbidden,
			types.ErrOptionWithSkipRetry(),
		)
	}

	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	tokenGroup := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyTokenGroup))
	mode := responsesImageToolBridgeTokenGroupMode(c, tokenGroup)

	switch mode {
	case model.TokenGroupModeAuto:
		for _, group := range service.GetRequestAutoGroups(c, common.GetContextKeyString(c, constant.ContextKeyUserGroup)) {
			if strings.EqualFold(group, configuredGroup) {
				return configuredGroup, nil
			}
		}
	case model.TokenGroupModeExplicit:
		for _, group := range service.GetRequestTokenGroups(c, configuredGroup) {
			if strings.EqualFold(group, configuredGroup) {
				return configuredGroup, nil
			}
		}
	case model.TokenGroupModeInherit:
		// 未声明分组的令牌与常规选渠行为一致，只能留在当前实际分组。
		if strings.EqualFold(selectedGroup, configuredGroup) ||
			(selectedGroup == "" && strings.EqualFold(userGroup, configuredGroup)) {
			return configuredGroup, nil
		}
	}

	return "", types.NewErrorWithStatusCode(
		fmt.Errorf("dynamic routing target group %s is not authorized for this token", configuredGroup),
		types.ErrorCodeAccessDenied,
		http.StatusForbidden,
		types.ErrOptionWithSkipRetry(),
	)
}

func buildResponsesImageToolBridgeRequest(
	request *dto.OpenAIResponsesRequest,
	targetModel string,
) (*dto.ImageRequest, error) {
	if request == nil {
		return nil, fmt.Errorf("responses image tool bridge requires a request")
	}
	if strings.TrimSpace(targetModel) == "" {
		return nil, fmt.Errorf("responses image tool bridge target model is empty")
	}

	imageTool, err := findResponsesImageGenerationTool(request)
	if err != nil {
		return nil, err
	}
	if err := validateResponsesImageToolBridgeImageOptions(imageTool); err != nil {
		return nil, err
	}
	action := strings.ToLower(strings.TrimSpace(common.Interface2String(imageTool["action"])))
	if action != "" && action != "generate" && action != "auto" {
		return nil, fmt.Errorf("responses image tool bridge currently supports only image_generation action generate")
	}

	promptParts := make([]string, 0)
	for _, input := range request.ParseInput() {
		switch input.Type {
		case "input_text":
			if text := strings.TrimSpace(input.Text); text != "" {
				promptParts = append(promptParts, text)
			}
		case "input_image", "input_file":
			return nil, fmt.Errorf("responses image tool bridge currently supports text-to-image generation only; use a dedicated edit action for image inputs")
		}
	}
	if len(promptParts) == 0 {
		return nil, fmt.Errorf("responses image tool bridge requires text input")
	}

	imageRequest := &dto.ImageRequest{
		Model:          strings.TrimSpace(targetModel),
		Prompt:         strings.Join(promptParts, "\n"),
		N:              common.GetPointer(uint(1)),
		ResponseFormat: "b64_json",
	}
	if err := copyResponsesImageToolBridgeImageOptions(imageTool, imageRequest); err != nil {
		return nil, err
	}
	return imageRequest, nil
}

func findResponsesImageGenerationTool(request *dto.OpenAIResponsesRequest) (map[string]any, error) {
	if request == nil || len(request.Tools) == 0 {
		return nil, fmt.Errorf("responses image tool bridge requires an image_generation tool definition")
	}
	var tools []map[string]any
	if err := common.Unmarshal(request.Tools, &tools); err != nil {
		return nil, fmt.Errorf("responses image tool bridge cannot parse tools: %w", err)
	}
	for _, tool := range tools {
		if strings.EqualFold(strings.TrimSpace(common.Interface2String(tool["type"])), dto.BuildInToolImageGeneration) {
			return tool, nil
		}
	}
	return nil, fmt.Errorf("responses image tool bridge requires an image_generation tool definition")
}

func setResponsesImageToolBridgeModel(request *dto.OpenAIResponsesRequest, targetModel string) error {
	if strings.TrimSpace(targetModel) == "" {
		return fmt.Errorf("responses image tool bridge target model is empty")
	}
	if _, err := findResponsesImageGenerationTool(request); err != nil {
		return err
	}

	var tools []map[string]any
	if err := common.Unmarshal(request.Tools, &tools); err != nil {
		return fmt.Errorf("responses image tool bridge cannot parse tools: %w", err)
	}
	for _, tool := range tools {
		if strings.EqualFold(strings.TrimSpace(common.Interface2String(tool["type"])), dto.BuildInToolImageGeneration) {
			tool["model"] = strings.TrimSpace(targetModel)
		}
	}
	encoded, err := common.Marshal(tools)
	if err != nil {
		return fmt.Errorf("responses image tool bridge cannot encode tools: %w", err)
	}
	request.Tools = encoded
	return nil
}

func validateResponsesImageToolBridgeImageOptions(tool map[string]any) error {
	for key := range tool {
		switch key {
		case "type", "action", "model", "size", "quality", "background", "moderation", "output_format", "output_compression", "partial_images":
		default:
			return fmt.Errorf("responses image tool bridge does not support image_generation option %q with /v1/images/generations", key)
		}
	}
	if partialImages, ok := tool["partial_images"]; ok && partialImages != nil {
		if value, ok := partialImages.(float64); !ok || value != 0 {
			return fmt.Errorf("responses image tool bridge supports only partial_images 0 with /v1/images/generations")
		}
	}
	return nil
}

func copyResponsesImageToolBridgeImageOptions(tool map[string]any, imageRequest *dto.ImageRequest) error {
	if imageRequest == nil {
		return fmt.Errorf("responses image tool bridge image request is unavailable")
	}
	if err := copyResponsesImageToolBridgeStringOption(tool, "size", &imageRequest.Size); err != nil {
		return err
	}
	if err := copyResponsesImageToolBridgeStringOption(tool, "quality", &imageRequest.Quality); err != nil {
		return err
	}
	if err := copyResponsesImageToolBridgeRawOption(tool, "background", &imageRequest.Background); err != nil {
		return err
	}
	if err := copyResponsesImageToolBridgeRawOption(tool, "moderation", &imageRequest.Moderation); err != nil {
		return err
	}
	if err := copyResponsesImageToolBridgeRawOption(tool, "output_format", &imageRequest.OutputFormat); err != nil {
		return err
	}
	return copyResponsesImageToolBridgeRawOption(tool, "output_compression", &imageRequest.OutputCompression)
}

func copyResponsesImageToolBridgeStringOption(tool map[string]any, name string, destination *string) error {
	value, ok := tool[name]
	if !ok || value == nil {
		return nil
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return fmt.Errorf("responses image tool bridge option %s must be a non-empty string", name)
	}
	*destination = strings.TrimSpace(text)
	return nil
}

func copyResponsesImageToolBridgeRawOption(tool map[string]any, name string, destination *json.RawMessage) error {
	value, ok := tool[name]
	if !ok || value == nil {
		return nil
	}
	encoded, err := common.Marshal(value)
	if err != nil {
		return fmt.Errorf("responses image tool bridge cannot encode option %s: %w", name, err)
	}
	*destination = encoded
	return nil
}

func applyResponsesImageToolBridgeBillingDefaults(info *relaycommon.RelayInfo, meta *types.TokenCountMeta) {
	if info == nil || info.ResponsesImageToolBridge == nil || meta == nil {
		return
	}
	if meta.BillingRatios == nil {
		meta.BillingRatios = make(map[string]float64)
	}
	meta.BillingRatios["n"] = 1
}

func validateResponsesImageToolBridgeTargetBilling(targetModel string) error {
	if billing_setting.GetBillingMode(strings.TrimSpace(targetModel)) == billing_setting.BillingModeTieredExpr {
		return fmt.Errorf("responses image tool bridge target model %s cannot use tiered_expr billing", targetModel)
	}
	return nil
}

func selectResponsesImageToolBridgeChannel(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	targetModel string,
	targetPath string,
	targetGroup string,
) (*model.Channel, string, *types.NewAPIError) {
	selectedGroup := strings.TrimSpace(targetGroup)
	if selectedGroup == "" {
		selectedGroup = common.GetContextKeyString(c, constant.ContextKeySelectedChannelGroup)
		if selectedGroup == "" && info != nil {
			selectedGroup = info.UsingGroup
		}
	}

	if _, hasSpecificChannel := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId); hasSpecificChannel {
		channel, ok := common.GetContextKeyType[*model.Channel](c, constant.ContextKeySelectedChannel)
		if !ok || channel == nil ||
			!model.IsChannelEnabledForGroupModel(selectedGroup, targetModel, channel.Id) ||
			!middleware.ChannelSupportsRequestPath(channel, targetPath, targetModel) {
			return nil, "", types.NewErrorWithStatusCode(
				fmt.Errorf("the selected channel does not support image bridge target model %s in group %s", targetModel, selectedGroup),
				types.ErrorCodeModelNotFound,
				http.StatusBadRequest,
				types.ErrOptionWithSkipRetry(),
			)
		}
		return channel, selectedGroup, nil
	}

	channel, group, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
		Ctx:         c,
		TokenGroup:  selectedGroup,
		ModelName:   targetModel,
		RequestPath: targetPath,
		Retry:       common.GetPointer(0),
	})
	if err != nil {
		return nil, "", types.NewError(
			fmt.Errorf("failed to select image bridge target model %s in group %s: %w", targetModel, selectedGroup, err),
			types.ErrorCodeGetChannelFailed,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if channel == nil {
		return nil, "", types.NewErrorWithStatusCode(
			fmt.Errorf("no channel supports image bridge target model %s in group %s", targetModel, selectedGroup),
			types.ErrorCodeModelNotFound,
			http.StatusServiceUnavailable,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if group == "" {
		group = selectedGroup
	}
	return channel, group, nil
}
