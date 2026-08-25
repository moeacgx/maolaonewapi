package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	hosttypes "github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const responsesImageFunctionName = "newapi_image_generation"

const (
	responsesImageFunctionBridgeMarker = "<newapi-image-generation>"
	responsesImageFunctionBridgeText   = responsesImageFunctionBridgeMarker + "\nWhen the user asks for image generation or editing, call the newapi_image_generation function included in this request. Do not describe or simulate the image result yourself.\n</newapi-image-generation>"
)

// prepareResponsesImageFunctionBridge resolves a two-stage image rule while
// the source text channel is still selected. It only mutates the typed
// Responses request by adding a private function tool; target channel
// selection and image billing happen after the source model calls the tool.
func prepareResponsesImageFunctionBridge(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	request dto.Request,
) (dto.Request, *types.NewAPIError) {
	responsesRequest, ok := request.(*dto.OpenAIResponsesRequest)
	if !ok || responsesRequest == nil || info == nil {
		return request, nil
	}
	info.InitChannelMeta(c)
	matchInfo, resolvedSourceGroup := resolveResponsesImageToolBridgeMatchInfo(c, info)
	rule, matched := helper.ResolveDynamicResponsesImageFunctionBridge(matchInfo, responsesRequest)
	if !matched {
		info.ChannelMeta = nil
		return request, nil
	}
	if resolvedSourceGroup != "" {
		info.UsingGroup = resolvedSourceGroup
		common.SetContextKey(c, constant.ContextKeyUsingGroup, resolvedSourceGroup)
		common.SetContextKey(c, constant.ContextKeySelectedChannelGroup, resolvedSourceGroup)
	}

	targetPath := dto.EffectiveDynamicRoutingTargetPath(rule)
	if targetPath != dto.DynamicRoutingImageGenerationPath {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("responses image function bridge target path must be %s", dto.DynamicRoutingImageGenerationPath),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if err := validateResponsesImageToolBridgeTargetBilling(rule.TargetModel); err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	targetGroup, groupErr := resolveResponsesImageToolBridgeTargetGroup(c, info, rule)
	if groupErr != nil {
		return nil, groupErr
	}
	injected, err := injectResponsesImageFunctionTool(responsesRequest)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if !injected {
		// A client-selected tool choice, tool_choice=none, or an existing native
		// image_generation tool is outside this private-function bridge. Leave
		// the request on its ordinary Responses path so the native bridge (if
		// configured) or the upstream can handle it.
		info.ChannelMeta = nil
		return request, nil
	}

	info.ResponsesImageFunctionBridge = &relaycommon.ResponsesImageFunctionBridge{
		RuleID:       rule.ID,
		SourceModel:  info.OriginModelName,
		TargetModel:  strings.TrimSpace(rule.TargetModel),
		TargetPath:   targetPath,
		TargetGroup:  targetGroup,
		FunctionName: responsesImageFunctionName,
	}
	// The injected tool is not present in the replayed original body. Force the
	// adaptor path so a channel's pass-through setting cannot drop it.
	info.ForceRequestConversion = true
	info.ChannelMeta = nil
	info.Request = responsesRequest
	return responsesRequest, nil
}

// injectResponsesImageFunctionTool appends the private function definition in
// the native Responses API shape. It is idempotent so retry reconstruction can
// safely call it for every source attempt.
func injectResponsesImageFunctionTool(request *dto.OpenAIResponsesRequest) (bool, error) {
	if request == nil {
		return false, fmt.Errorf("responses image function bridge requires a request")
	}

	var tools []map[string]any
	if len(bytes.TrimSpace(request.Tools)) > 0 && string(bytes.TrimSpace(request.Tools)) != "null" {
		if err := common.Unmarshal(request.Tools, &tools); err != nil {
			return false, fmt.Errorf("responses image function bridge cannot parse tools: %w", err)
		}
	}
	for _, tool := range tools {
		toolType := strings.ToLower(strings.TrimSpace(common.Interface2String(tool["type"])))
		if toolType == dto.BuildInToolImageGeneration {
			return false, nil
		}
		if toolType == "function" &&
			strings.EqualFold(strings.TrimSpace(common.Interface2String(tool["name"])), responsesImageFunctionName) {
			return false, fmt.Errorf("responses image function bridge reserves function name %q", responsesImageFunctionName)
		}
	}

	shouldInject, err := responsesImageFunctionToolChoiceAllowsInjection(request.ToolChoice)
	if err != nil {
		return false, err
	}
	if !shouldInject {
		return false, nil
	}

	tool := map[string]any{
		"type":        "function",
		"name":        responsesImageFunctionName,
		"description": "Generate an image when the user asks for image creation or editing.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{"type": "string"},
				// Strict Responses function schemas require every declared property
				// to be listed in required. Nullable values preserve optional Images
				// parameters without forcing the model to invent a value.
				"size":          map[string]any{"type": []string{"string", "null"}},
				"quality":       map[string]any{"type": []string{"string", "null"}},
				"output_format": map[string]any{"type": []string{"string", "null"}},
			},
			"required":             []string{"prompt", "size", "quality", "output_format"},
			"additionalProperties": false,
		},
		"strict": true,
	}
	tools = append(tools, tool)
	encoded, err := common.Marshal(tools)
	if err != nil {
		return false, fmt.Errorf("responses image function bridge cannot encode tools: %w", err)
	}
	request.Tools = encoded
	if len(bytes.TrimSpace(request.ToolChoice)) == 0 || bytes.Equal(bytes.TrimSpace(request.ToolChoice), []byte("null")) {
		request.ToolChoice = json.RawMessage(`"auto"`)
	}
	if err := appendResponsesImageFunctionBridgeInstructions(request); err != nil {
		return false, err
	}
	return true, nil
}

// responsesImageFunctionToolChoiceAllowsInjection mirrors the conservative
// Sub2API injection policy: auto or an omitted choice permits the private
// function; none, a different function, or a native image choice leaves the
// client-controlled selection untouched.
func responsesImageFunctionToolChoiceAllowsInjection(choice json.RawMessage) (bool, error) {
	raw := bytes.TrimSpace(choice)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return true, nil
	}
	var stringChoice string
	if common.Unmarshal(raw, &stringChoice) == nil {
		switch strings.ToLower(strings.TrimSpace(stringChoice)) {
		case "", "auto":
			return true, nil
		default:
			return false, nil
		}
	}
	var objectChoice map[string]json.RawMessage
	if err := common.Unmarshal(raw, &objectChoice); err != nil {
		return false, fmt.Errorf("responses image function bridge tool_choice is invalid: %w", err)
	}
	var choiceType string
	_ = common.Unmarshal(objectChoice["type"], &choiceType)
	choiceType = strings.ToLower(strings.TrimSpace(choiceType))
	switch choiceType {
	case "auto", "":
		return true, nil
	case "none", dto.BuildInToolImageGeneration:
		return false, nil
	case "function":
		var functionName string
		_ = common.Unmarshal(objectChoice["name"], &functionName)
		if strings.EqualFold(strings.TrimSpace(functionName), responsesImageFunctionName) {
			return false, fmt.Errorf("responses image function bridge reserves function name %q", responsesImageFunctionName)
		}
		return false, nil
	default:
		return false, nil
	}
}

func appendResponsesImageFunctionBridgeInstructions(request *dto.OpenAIResponsesRequest) error {
	if request == nil {
		return fmt.Errorf("responses image function bridge requires a request")
	}
	raw := bytes.TrimSpace(request.Instructions)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		encoded, err := common.Marshal(responsesImageFunctionBridgeText)
		if err != nil {
			return fmt.Errorf("responses image function bridge cannot encode instructions: %w", err)
		}
		request.Instructions = encoded
		return nil
	}
	var instructions string
	if common.Unmarshal(raw, &instructions) != nil {
		// Responses also accepts structured instructions. Preserve those bytes;
		// the function description remains authoritative for this bridge.
		return nil
	}
	if strings.Contains(instructions, responsesImageFunctionBridgeMarker) {
		return nil
	}
	instructions = strings.TrimRight(instructions, " \t\r\n")
	if instructions != "" {
		instructions += "\n\n"
	}
	instructions += responsesImageFunctionBridgeText
	encoded, err := common.Marshal(instructions)
	if err != nil {
		return fmt.Errorf("responses image function bridge cannot encode instructions: %w", err)
	}
	request.Instructions = encoded
	return nil
}

// imageRequestFromResponsesFunctionArguments converts the model-provided
// function arguments into the canonical Images API request. Responses
// function_call.arguments is usually a JSON string, but a few compatible
// upstreams return an object; both forms are accepted.
func imageRequestFromResponsesFunctionArguments(arguments json.RawMessage, targetModel string) (*dto.ImageRequest, error) {
	if strings.TrimSpace(targetModel) == "" {
		return nil, fmt.Errorf("responses image function bridge target model is empty")
	}
	raw := bytes.TrimSpace(arguments)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, fmt.Errorf("responses image function bridge returned empty arguments")
	}
	if raw[0] == '"' {
		var encoded string
		if err := common.Unmarshal(raw, &encoded); err != nil {
			return nil, fmt.Errorf("responses image function bridge arguments string is invalid: %w", err)
		}
		raw = bytes.TrimSpace([]byte(encoded))
	}
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("responses image function bridge arguments must be an object: %w", err)
	}
	for key := range fields {
		switch key {
		case "prompt", "size", "quality", "output_format":
		default:
			return nil, fmt.Errorf("responses image function bridge does not support argument %q", key)
		}
	}
	request := &dto.ImageRequest{
		Model:          strings.TrimSpace(targetModel),
		N:              common.GetPointer(uint(1)),
		ResponseFormat: "b64_json",
	}
	if err := decodeRequiredImageFunctionString(fields, "prompt", &request.Prompt); err != nil {
		return nil, err
	}
	if err := decodeOptionalImageFunctionString(fields, "size", &request.Size); err != nil {
		return nil, err
	}
	if err := decodeOptionalImageFunctionString(fields, "quality", &request.Quality); err != nil {
		return nil, err
	}
	var outputFormat string
	if err := decodeOptionalImageFunctionString(fields, "output_format", &outputFormat); err != nil {
		return nil, err
	}
	if outputFormat != "" {
		request.OutputFormat = json.RawMessage([]byte(fmt.Sprintf("%q", outputFormat)))
	}
	return request, nil
}

func decodeRequiredImageFunctionString(fields map[string]json.RawMessage, name string, destination *string) error {
	if _, ok := fields[name]; !ok {
		return fmt.Errorf("responses image function bridge requires argument %s", name)
	}
	if err := decodeOptionalImageFunctionString(fields, name, destination); err != nil {
		return err
	}
	if strings.TrimSpace(*destination) == "" {
		return fmt.Errorf("responses image function bridge argument %s must be a non-empty string", name)
	}
	return nil
}

func decodeOptionalImageFunctionString(fields map[string]json.RawMessage, name string, destination *string) error {
	raw, ok := fields[name]
	if !ok {
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	var value string
	if err := common.Unmarshal(raw, &value); err != nil || strings.TrimSpace(value) == "" {
		return fmt.Errorf("responses image function bridge argument %s must be a non-empty string", name)
	}
	*destination = strings.TrimSpace(value)
	return nil
}

// executeResponsesImageFunctionBridge performs the second, independent Images
// API stage after the source text attempt has already been settled by Relay.
// Only this target stage is retried; the successful source function call is
// never replayed and its channel is never disabled for a target failure.
func executeResponsesImageFunctionBridge(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	bridge := info.ResponsesImageFunctionBridge
	if bridge == nil || !bridge.Triggered {
		return nil
	}

	imageRequest, err := imageRequestFromResponsesFunctionArguments(bridge.Arguments, bridge.TargetModel)
	if err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	targetInfo := *info
	targetInfo.RequestId = common.NewRequestId()
	targetInfo.StartTime = time.Now()
	targetInfo.FirstResponseTime = time.Time{}
	targetInfo.ResetFirstResponseTime()
	targetInfo.SendResponseCount = 0
	targetInfo.ReceivedResponseCount = 0
	targetInfo.RetryIndex = 0
	targetInfo.LastError = nil
	targetInfo.StreamStatus = nil
	targetInfo.ResponsesImageFunctionBridge = nil
	targetInfo.ResponsesImageToolBridge = &relaycommon.ResponsesImageToolBridge{
		RuleID:           bridge.RuleID,
		SourceModel:      bridge.SourceModel,
		TargetModel:      bridge.TargetModel,
		TargetPath:       bridge.TargetPath,
		TargetGroup:      bridge.TargetGroup,
		UseImagesAPI:     true,
		DownstreamStream: info.IsStream,
	}
	targetInfo.OriginModelName = bridge.TargetModel
	targetInfo.UsingGroup = bridge.TargetGroup
	targetInfo.RequestURLPath = bridge.TargetPath
	targetInfo.RelayMode = relayconstant.RelayModeImagesGenerations
	targetInfo.RelayFormat = types.RelayFormatOpenAIImage
	targetInfo.FinalRequestRelayFormat = types.RelayFormatOpenAIImage
	targetInfo.IsStream = false
	targetInfo.ForceRequestConversion = true
	targetInfo.Request = imageRequest
	targetInfo.Billing = nil
	targetInfo.ChannelMeta = nil
	targetInfo.PriceData = hosttypes.PriceData{}
	targetInfo.RequestConversionChain = nil
	targetInfo.TieredBillingSnapshot = nil
	// 目标请求体不同于来源 /v1/responses 请求。阶梯图片定价可能读取 size、
	// quality 等字段，因此表达式输入必须使用生成的 Images 请求，不能回退到
	// Gin 中的原始文本请求体。
	billingRequestInput, billingInputErr := helper.BuildBillingExprRequestInputFromRequest(
		imageRequest,
		targetInfo.RequestHeaders,
	)
	if billingInputErr != nil {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("responses image function bridge cannot prepare target billing input: %w", billingInputErr),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	targetInfo.BillingRequestInput = &billingRequestInput
	targetInfo.QuotaClamp = nil

	retryParam := &service.RetryParam{
		Ctx:         c,
		TokenGroup:  bridge.TargetGroup,
		ModelName:   bridge.TargetModel,
		RequestPath: bridge.TargetPath,
		Retry:       common.GetPointer(0),
	}
	maxRetries := service.RelayMaxRetries(retryParam)
	var lastErr *types.NewAPIError

	for attemptIndex := 0; attemptIndex <= maxRetries; attemptIndex++ {
		targetInfo.RetryIndex = attemptIndex
		targetInfo.RequestId = common.NewRequestId()
		targetInfo.Billing = nil
		targetInfo.FinalPreConsumedQuota = 0
		targetInfo.BillingSource = ""
		targetInfo.SubscriptionId = 0
		targetInfo.SubscriptionPreConsumed = 0
		targetInfo.SubscriptionPostDelta = 0
		targetInfo.SubscriptionAmountTotal = 0
		targetInfo.SubscriptionAmountUsedAfterPreConsume = 0
		targetInfo.PriceData = hosttypes.PriceData{}
		targetInfo.ChannelMeta = nil
		targetInfo.RequestConversionChain = nil
		targetInfo.FinalRequestRelayFormat = types.RelayFormatOpenAIImage
		targetInfo.QuotaClamp = nil

		targetChannel, selectedGroup, selectErr := selectResponsesImageToolBridgeChannelWithRetry(
			c, &targetInfo, bridge.TargetModel, bridge.TargetPath, bridge.TargetGroup, retryParam,
		)
		if selectErr != nil {
			lastErr = responsesImageFunctionBridgeSkipRetry(selectErr)
			break
		}
		if selectedGroup != "" {
			targetInfo.UsingGroup = selectedGroup
			common.SetContextKey(c, constant.ContextKeyUsingGroup, selectedGroup)
			common.SetContextKey(c, constant.ContextKeySelectedChannelGroup, selectedGroup)
			targetInfo.ResponsesImageToolBridge.TargetGroup = selectedGroup
		}
		if c.Keys != nil {
			delete(c.Keys, string(constant.ContextKeyChannelPreferredMultiKeyIndex))
		}
		if setupErr := middleware.SetupContextForSelectedChannel(c, targetChannel, bridge.TargetModel); setupErr != nil {
			lastErr = responsesImageFunctionBridgeSkipRetry(setupErr)
			break
		}
		setSelectedSecurityAuditRoute(c, targetChannel, selectedGroup)
		targetInfo.InitChannelMeta(c)

		meta := imageRequest.GetTokenCountMeta()
		applyResponsesImageToolBridgeBillingDefaults(&targetInfo, meta)
		tokens, estimateErr := service.EstimateRequestToken(c, meta, &targetInfo)
		if estimateErr != nil {
			lastErr = responsesImageFunctionBridgeSkipRetry(types.NewError(estimateErr, types.ErrorCodeCountTokenFailed, types.ErrOptionWithSkipRetry()))
			break
		}
		targetInfo.SetEstimatePromptTokens(tokens)
		priceData, priceErr := helper.ModelPriceHelper(c, &targetInfo, tokens, meta)
		if priceErr != nil {
			lastErr = responsesImageFunctionBridgeSkipRetry(types.NewErrorWithStatusCode(priceErr, types.ErrorCodeModelPriceError, http.StatusBadRequest, types.ErrOptionWithSkipRetry()))
			break
		}
		if !priceData.FreeModel {
			if billingErr := service.PreConsumeBilling(c, priceData.QuotaToPreConsume, &targetInfo); billingErr != nil {
				lastErr = responsesImageFunctionBridgeSkipRetry(billingErr)
				break
			}
		}

		service.BeginChannelMetricAttempt(c, &targetInfo, targetChannel.Id, targetChannel.Name, targetChannel.Type)
		lastErr = relay.ImageHelper(c, &targetInfo)
		if lastErr == nil {
			service.FinishChannelMetricAttempt(c, &targetInfo, nil, false, "")
			return nil
		}

		lastErr = service.NormalizeViolationFeeError(lastErr)
		targetInfo.LastError = lastErr
		service.RecordUpstreamPolicyError(c, lastErr, "image")
		processChannelError(c, *types.NewChannelError(
			targetChannel.Id,
			targetChannel.Type,
			targetChannel.Name,
			targetChannel.ChannelInfo.IsMultiKey,
			common.GetContextKeyString(c, constant.ContextKeyChannelKey),
			targetChannel.GetAutoBan(),
		), lastErr)
		retryPlanned := shouldRetry(c, lastErr, maxRetries-attemptIndex)
		service.FinishChannelMetricAttempt(c, &targetInfo, lastErr, retryPlanned, string(lastErr.GetErrorCode()))
		if targetInfo.Billing != nil {
			targetInfo.Billing.Refund(c)
			targetInfo.Billing = nil
		}
		if !retryPlanned {
			break
		}
		retryParam.ExcludeChannelID(targetChannel.Id, true)
		retryParam.IncreaseRetry()
	}

	return responsesImageFunctionBridgeSkipRetry(lastErr)
}

// responsesImageFunctionBridgeSkipRetry marks every post-source-stage failure
// terminal for the outer source relay loop. Retrying there would repeat the
// source model request after its successful function_call has been settled.
func responsesImageFunctionBridgeSkipRetry(apiErr *types.NewAPIError) *types.NewAPIError {
	if apiErr == nil || types.IsSkipRetryError(apiErr) {
		return apiErr
	}
	return types.NewError(apiErr, apiErr.GetErrorCode(), types.ErrOptionWithSkipRetry())
}
