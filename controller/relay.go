package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/relay"
	atlascloudrelay "github.com/QuantumNous/new-api/relay/channel/atlascloud"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func relayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		err = relay.ImageHelper(c, info)
	case relayconstant.RelayModeAudioSpeech:
		fallthrough
	case relayconstant.RelayModeAudioTranslation:
		fallthrough
	case relayconstant.RelayModeAudioTranscription:
		err = relay.AudioHelper(c, info)
	case relayconstant.RelayModeRerank:
		err = relay.RerankHelper(c, info)
	case relayconstant.RelayModeEmbeddings:
		err = relay.EmbeddingHelper(c, info)
	case relayconstant.RelayModeResponses, relayconstant.RelayModeResponsesCompact:
		err = relay.ResponsesHelper(c, info)
	case relayconstant.RelayModeAlphaSearch:
		err = relay.AlphaSearchHelper(c, info)
	default:
		err = relay.TextHelper(c, info)
	}
	return err
}

func geminiRelayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	if strings.Contains(c.Request.URL.Path, "embed") {
		err = relay.GeminiEmbeddingHandler(c, info)
	} else {
		err = relay.GeminiHelper(c, info)
	}
	return err
}

func Relay(c *gin.Context, relayFormat types.RelayFormat) {
	service.BeginChannelMetricRequest(c)
	if common.GetContextKeyBool(c, constant.ContextKeyCanvasTrusted) {
		service.MarkChannelMetricCanvasRequest(c)
	} else if c.Request != nil && c.Request.URL != nil && strings.HasPrefix(c.Request.URL.Path, "/pg/") {
		service.MarkChannelMetricPlaygroundRequest(c)
	}

	requestId := c.GetString(common.RequestIdKey)
	//group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	//originalModel := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)

	var (
		newAPIError *types.NewAPIError
		ws          *websocket.Conn
		relayInfo   *relaycommon.RelayInfo
	)
	defer func() {
		service.FinishChannelMetricRequest(c, relayInfo, newAPIError)
	}()

	if relayFormat == types.RelayFormatOpenAIRealtime {
		if auditedWs, ok := common.GetContextKeyType[*websocket.Conn](c, constant.ContextKeyPromptAuditRealtimeClientWs); ok && auditedWs != nil {
			ws = auditedWs
		} else {
			var err error
			ws, err = upgrader.Upgrade(c.Writer, c.Request, nil)
			if err != nil {
				clientError, _ := clientOpenAIError(types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry()), "")
				helper.WssError(c, ws, clientError)
				return
			}
		}
		defer ws.Close()
	}

	defer func() {
		writeRelayErrorResponse(c, ws, relayFormat, newAPIError, requestId)
	}()

	request, err := helper.GetAndValidateRequest(c, relayFormat)
	if err != nil {
		// Map "request body too large" to 413 so clients can handle it correctly
		if common.IsRequestBodyTooLargeError(err) || errors.Is(err, common.ErrRequestBodyTooLarge) {
			newAPIError = types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
		} else {
			newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest)
		}
		return
	}

	var originalRequestBody []byte
	if relayFormat != types.RelayFormatOpenAIRealtime {
		bodyStorage, bodyErr := common.GetBodyStorage(c)
		if bodyErr != nil {
			if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
				newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
			} else {
				newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
			return
		}
		originalRequestBody, bodyErr = bodyStorage.Bytes()
		if bodyErr != nil {
			newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			return
		}
	}

	relayInfo, err = relaycommon.GenRelayInfo(c, relayFormat, request, ws)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeGenRelayInfoFailed)
		return
	}
	applyImageTaskAsyncPreConsume(c, relayInfo)
	service.BindChannelMetricRelayInfo(c, relayInfo)
	if err := applyAtlasCloudImageDefaultsForPricing(c, relayInfo, relayFormat, request); err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
		return
	}

	needCountToken := constant.CountToken
	var meta *types.TokenCountMeta
	if needCountToken {
		meta = request.GetTokenCountMeta()
	} else {
		meta = fastTokenCountMetaForPricing(request)
	}
	applyAtlasCloudImageBillingDefaultsForPricing(c, relayInfo, relayFormat, request, meta)

	tokens, err := service.EstimateRequestToken(c, meta, relayInfo)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeCountTokenFailed)
		return
	}

	relayInfo.SetEstimatePromptTokens(tokens)

	priceData, err := helper.ModelPriceHelper(c, relayInfo, tokens, meta)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithStatusCode(http.StatusBadRequest))
		return
	}

	// common.SetContextKey(c, constant.ContextKeyTokenCountMeta, meta)

	if priceData.FreeModel {
		logger.LogInfo(c, fmt.Sprintf("模型 %s 免费，跳过预扣费", relayInfo.OriginModelName))
	} else {
		newAPIError = service.PreConsumeBilling(c, priceData.QuotaToPreConsume, relayInfo)
		if newAPIError != nil {
			return
		}
	}

	defer func() {
		// Only return quota if downstream failed and quota was actually pre-consumed
		if newAPIError != nil {
			newAPIError = service.NormalizeViolationFeeError(newAPIError)
			if relayInfo.Billing != nil {
				relayInfo.Billing.Refund(c)
			}
			service.ChargeViolationFeeIfNeeded(c, relayInfo, newAPIError)
		}
	}()

	retryParam := &service.RetryParam{
		Ctx: c, TokenGroup: relayInfo.TokenGroup, ModelName: relayInfo.OriginModelName,
		RequestPath: c.Request.URL.Path, Retry: common.GetPointer(0),
	}
	relayInfo.RetryIndex = 0
	relayInfo.LastError = nil
	maxRetries := service.RelayMaxRetries(retryParam)

	for attemptIndex := 0; attemptIndex <= maxRetries; attemptIndex++ {
		relayInfo.RetryIndex = attemptIndex
		channel, channelErr := getChannel(c, relayInfo, retryParam)
		if channelErr != nil {
			logger.LogError(c, channelErr.Error())
			newAPIError = channelErr
			break
		}
		addUsedChannel(c, channel.Id)
		attemptRequest, policyErr := prepareSelectedRouteRequest(c, relayFormat, originalRequestBody)
		if policyErr != nil {
			newAPIError = policyErr
			break
		}
		if attemptRequest != nil {
			relayInfo.Request = attemptRequest
		}
		if billingErr := service.PrepareTieredBillingForSelectedGroup(c, relayInfo); billingErr != nil {
			newAPIError = billingErr
			break
		}
		service.BeginChannelMetricAttempt(c, relayInfo, channel.Id, channel.Name, channel.Type)

		switch relayFormat {
		case types.RelayFormatOpenAIRealtime:
			newAPIError = relay.WssHelper(c, relayInfo)
		case types.RelayFormatClaude:
			newAPIError = relay.ClaudeHelper(c, relayInfo)
		case types.RelayFormatGemini:
			newAPIError = geminiRelayHandler(c, relayInfo)
		default:
			newAPIError = relayHandler(c, relayInfo)
		}

		if newAPIError == nil {
			relayInfo.LastError = nil
			service.FinishChannelMetricAttempt(c, relayInfo, nil, false, "")
			return
		}

		service.RecordUpstreamPolicyError(c, newAPIError, "response")

		newAPIError = service.NormalizeViolationFeeError(newAPIError)
		relayInfo.LastError = newAPIError
		processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError)

		retryPlanned := shouldRetry(c, newAPIError, maxRetries-attemptIndex)
		service.FinishChannelMetricAttempt(c, relayInfo, newAPIError, retryPlanned, string(newAPIError.GetErrorCode()))
		if !retryPlanned {
			break
		}
		// Retryable regular-group failures are not retried on the same channel
		// until all untried candidates are exhausted; ordered groups are hard
		// one-attempt boundaries and never relax this exclusion.
		retryParam.ExcludeChannelID(channel.Id, true)
		retryParam.IncreaseRetry()
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}
	if newAPIError != nil {
		gopool.Go(func() {
			perfmetrics.RecordRelayFailure(relayInfo, newAPIError)
		})
	}
}

func applyAtlasCloudImageDefaultsForPricing(c *gin.Context, info *relaycommon.RelayInfo, relayFormat types.RelayFormat, request dto.Request) error {
	if !isAtlasCloudImageRelay(c, relayFormat) || info == nil {
		return nil
	}
	imageRequest, ok := request.(*dto.ImageRequest)
	if !ok {
		return nil
	}
	if err := helper.ModelMappedHelper(c, info, request); err != nil {
		return err
	}
	atlascloudrelay.ApplyImageRequestDefaults(
		c,
		imageRequest,
		info.UpstreamModelName,
		info.RelayMode == relayconstant.RelayModeImagesEdits,
	)
	return nil
}

func applyAtlasCloudImageBillingDefaultsForPricing(c *gin.Context, info *relaycommon.RelayInfo, relayFormat types.RelayFormat, request dto.Request, meta *types.TokenCountMeta) {
	if !isAtlasCloudImageRelay(c, relayFormat) || info == nil {
		return
	}
	imageRequest, ok := request.(*dto.ImageRequest)
	if !ok {
		return
	}
	atlascloudrelay.ApplyImageBillingDefaults(
		meta,
		imageRequest,
		info.UpstreamModelName,
		info.RelayMode == relayconstant.RelayModeImagesEdits,
	)
	atlascloudrelay.ApplyImageFormulaBillingInputs(
		c,
		info,
		meta,
		imageRequest,
		info.UpstreamModelName,
		info.RelayMode == relayconstant.RelayModeImagesEdits,
	)
}

func isAtlasCloudImageRelay(c *gin.Context, relayFormat types.RelayFormat) bool {
	if c == nil {
		return false
	}
	return relayFormat == types.RelayFormatOpenAIImage &&
		common.GetContextKeyInt(c, constant.ContextKeyChannelType) == constant.ChannelTypeAtlasCloud
}

func RelayTask(c *gin.Context) {
	service.BeginChannelMetricRequest(c)
	if common.GetContextKeyBool(c, constant.ContextKeyCanvasTrusted) {
		service.MarkChannelMetricCanvasRequest(c)
	} else {
		service.MarkChannelMetricTaskRequest(c)
	}
	var relayInfo *relaycommon.RelayInfo
	var metricError *types.NewAPIError
	defer func() {
		service.FinishChannelMetricRequest(c, relayInfo, metricError)
	}()

	relayInfo, originRoutePrepared := common.GetContextKeyType[*relaycommon.RelayInfo](c, resolvedOriginTaskRelayInfoContextKey)
	if !originRoutePrepared || relayInfo == nil {
		var err error
		relayInfo, err = relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
		if err != nil {
			metricError = types.NewError(err, types.ErrorCodeGenRelayInfoFailed)
			c.JSON(http.StatusInternalServerError, &taskdto.TaskError{
				Code:       "gen_relay_info_failed",
				Message:    err.Error(),
				StatusCode: http.StatusInternalServerError,
			})
			return
		}
		if taskErr := relay.ResolveOriginTask(c, relayInfo); taskErr != nil {
			metricError = taskErrorToMetricError(taskErr)
			respondTaskError(c, taskErr)
			return
		}
	}
	service.BindChannelMetricRelayInfo(c, relayInfo)

	if lockedChannel, ok := relayInfo.LockedChannel.(*model.Channel); ok && lockedChannel != nil && !originRoutePrepared {
		if setupErr := middleware.SetupContextForSelectedChannel(c, lockedChannel, relayInfo.OriginModelName); setupErr != nil {
			taskErr := service.TaskErrorWrapperLocal(setupErr.Err, "setup_locked_channel_failed", http.StatusInternalServerError)
			metricError = taskErrorToMetricError(taskErr)
			respondTaskError(c, taskErr)
			return
		}
		setSelectedSecurityAuditRoute(c, lockedChannel, relayInfo.UsingGroup)
	}

	bodyStorage, bodyErr := common.GetBodyStorage(c)
	if bodyErr != nil {
		statusCode := http.StatusBadRequest
		if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
			statusCode = http.StatusRequestEntityTooLarge
		}
		taskErr := service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", statusCode)
		metricError = taskErrorToMetricError(taskErr)
		respondTaskError(c, taskErr)
		return
	}
	originalRequestBody, bodyErr := bodyStorage.Bytes()
	if bodyErr != nil {
		taskErr := service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusBadRequest)
		metricError = taskErrorToMetricError(taskErr)
		respondTaskError(c, taskErr)
		return
	}
	originalRequestBody = append([]byte(nil), originalRequestBody...)

	var result *relay.TaskSubmitResult
	var taskErr *taskdto.TaskError
	defer func() {
		if taskErr != nil && relayInfo.Billing != nil {
			relayInfo.Billing.Refund(c)
		}
	}()

	retryParam := &service.RetryParam{
		Ctx: c, TokenGroup: relayInfo.TokenGroup, ModelName: relayInfo.OriginModelName,
		RequestPath: c.Request.URL.Path, Retry: common.GetPointer(0),
	}
	maxRetries := service.RelayMaxRetries(retryParam)
	if relayInfo.LockedChannel != nil {
		maxRetries = common.RetryTimes
	}

	for attemptIndex := 0; attemptIndex <= maxRetries; attemptIndex++ {
		relayInfo.RetryIndex = attemptIndex
		var channel *model.Channel

		if lockedCh, ok := relayInfo.LockedChannel.(*model.Channel); ok && lockedCh != nil {
			channel = lockedCh
			if retryParam.GetRetry() > 0 {
				if setupErr := middleware.SetupContextForSelectedChannel(c, channel, relayInfo.OriginModelName); setupErr != nil {
					taskErr = service.TaskErrorWrapperLocal(setupErr.Err, "setup_locked_channel_failed", http.StatusInternalServerError)
					break
				}
			}
		} else {
			var channelErr *types.NewAPIError
			channel, channelErr = getChannel(c, relayInfo, retryParam)
			if channelErr != nil {
				logger.LogError(c, channelErr.Error())
				taskErr = service.TaskErrorWrapperLocal(channelErr.Err, "get_channel_failed", http.StatusInternalServerError)
				break
			}
		}

		setSelectedSecurityAuditRoute(c, channel, common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
		if taskErr = prepareSelectedRouteTaskRequest(c, types.RelayFormatTask, originalRequestBody); taskErr != nil {
			break
		}
		addUsedChannel(c, channel.Id)
		service.BeginChannelMetricAttempt(c, relayInfo, channel.Id, channel.Name, channel.Type)

		result, taskErr = relay.RelayTaskSubmit(c, relayInfo)
		if taskErr == nil {
			metricError = nil
			break
		}

		if !taskErr.LocalError {
			processChannelError(c,
				*types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey,
					common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()),
				types.NewOpenAIError(taskErr.Error, types.ErrorCodeBadResponseStatusCode, taskErr.StatusCode))
		}

		metricError = taskErrorToMetricError(taskErr)
		retryPlanned := shouldRetryTaskRelay(c, channel.Id, taskErr, maxRetries-attemptIndex)
		service.FinishChannelMetricAttempt(c, relayInfo, metricError, retryPlanned, taskErr.Code)
		if !retryPlanned {
			break
		}
		if relayInfo.LockedChannel == nil {
			retryParam.ExcludeChannelID(channel.Id, true)
		}
		retryParam.IncreaseRetry()
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}

	// ── 成功：结算 + 日志 + 插入任务 ──
	if taskErr == nil {
		settleErr := service.SettleBilling(c, relayInfo, result.Quota)
		service.AttachChannelMetricUsageAfterSettlement(c, service.ChannelMetricUsage{}, result.Quota, settleErr)
		service.FinishChannelMetricAttempt(c, relayInfo, nil, false, "")
		if settleErr != nil {
			common.SysError("settle task billing error: " + settleErr.Error())
		}
		service.LogTaskConsumption(c, relayInfo)

		task := model.InitTask(result.Platform, relayInfo)
		task.PrivateData.UpstreamTaskID = result.UpstreamTaskID
		task.PrivateData.BillingSource = relayInfo.BillingSource
		task.PrivateData.SubscriptionId = relayInfo.SubscriptionId
		task.PrivateData.TokenId = relayInfo.TokenId
		task.PrivateData.NodeName = common.NodeName
		task.PrivateData.BillingContext = &model.TaskBillingContext{
			ModelPrice:      relayInfo.PriceData.ModelPrice,
			GroupRatio:      relayInfo.PriceData.GroupRatioInfo.GroupRatio,
			ModelRatio:      relayInfo.PriceData.ModelRatio,
			OtherRatios:     relayInfo.PriceData.OtherRatios(),
			OriginModelName: relayInfo.OriginModelName,
			PerCallBilling:  common.StringsContains(constant.TaskPricePatches, relayInfo.OriginModelName) || relayInfo.PriceData.UsePrice,
		}
		task.Quota = result.Quota
		task.Data = result.TaskData
		task.Action = relayInfo.Action
		if insertErr := task.Insert(); insertErr != nil {
			common.SysError("insert task error: " + insertErr.Error())
		}
	}

	if taskErr != nil && metricError == nil {
		metricError = taskErrorToMetricError(taskErr)
	}
	if taskErr != nil {
		service.RecordUpstreamPolicyCode(c, taskErr.Code, "task_response")
		respondTaskError(c, taskErr)
	}
}

func writeRelayErrorResponse(c *gin.Context, ws *websocket.Conn, relayFormat types.RelayFormat, relayErr *types.NewAPIError, requestID string) {
	if relayErr == nil {
		return
	}
	if reason := requestContextErrorReason(c, relayErr); reason != "" {
		logger.LogInfo(c, "relay stopped after request context ended: "+reason)
		return
	}
	logger.LogError(c, fmt.Sprintf("relay error: %s", common.LocalLogPreview(relayErr.Error())))
	if relayFormat != types.RelayFormatOpenAIRealtime && c != nil && c.Writer != nil && c.Writer.Written() {
		logger.LogInfo(c, "relay response already started; skip writing a second error response")
		return
	}
	switch relayFormat {
	case types.RelayFormatOpenAIRealtime:
		clientError, _ := clientOpenAIError(relayErr, requestID)
		helper.WssError(c, ws, clientError)
	case types.RelayFormatClaude:
		clientError, statusCode := clientClaudeError(relayErr, requestID)
		c.JSON(statusCode, gin.H{"type": "error", "error": clientError})
	default:
		openAIError := relayErr.ToOpenAIError()
		if relayErr.GetErrorCode() == types.ErrorCodeSensitiveWordsDetected {
			openAIError = service.SensitiveFilterClientOpenAIError(relayErr)
		}
		message := openAIError.Message
		statusCode := relayErr.StatusCode
		if types.IsUpstreamReturnedError(relayErr) {
			message, statusCode, _ = common.ReplaceClientErrorCandidates(relayErr.StatusCode, relayErr.Error(), openAIError.Message)
		}
		openAIError.Message = common.MessageWithRequestId(message, requestID)
		c.JSON(statusCode, gin.H{"error": openAIError})
	}

}

var upgrader = websocket.Upgrader{
	Subprotocols: []string{"realtime"}, // WS 握手支持的协议，如果有使用 Sec-WebSocket-Protocol，则必须在此声明对应的 Protocol TODO add other protocol
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许跨域
	},
}

func addUsedChannel(c *gin.Context, channelId int) {
	useChannel := c.GetStringSlice("use_channel")
	useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
	c.Set("use_channel", useChannel)
}

func fastTokenCountMetaForPricing(request dto.Request) *types.TokenCountMeta {
	if request == nil {
		return &types.TokenCountMeta{}
	}
	meta := &types.TokenCountMeta{
		TokenType: types.TokenTypeTokenizer,
	}
	switch r := request.(type) {
	case *dto.GeneralOpenAIRequest:
		maxCompletionTokens := lo.FromPtrOr(r.MaxCompletionTokens, uint(0))
		maxTokens := lo.FromPtrOr(r.MaxTokens, uint(0))
		if maxCompletionTokens > maxTokens {
			meta.MaxTokens = int(maxCompletionTokens)
		} else {
			meta.MaxTokens = int(maxTokens)
		}
	case *dto.OpenAIResponsesRequest:
		meta.MaxTokens = int(lo.FromPtrOr(r.MaxOutputTokens, uint(0)))
	case *dto.ClaudeRequest:
		meta.MaxTokens = int(lo.FromPtr(r.MaxTokens))
	case *dto.ImageRequest:
		// Pricing for image requests depends on ImagePriceRatio; safe to compute even when CountToken is disabled.
		return r.GetTokenCountMeta()
	default:
		// Best-effort: leave CombineText empty to avoid large allocations.
	}
	return meta
}

// prepareSelectedRouteRequest rebuilds the attempt from the replayable client
// body, then applies policy for the final channel and group. Re-parsing after a
// mask keeps the typed request serialized by adaptors in sync with BodyStorage.
func prepareSelectedRouteRequest(c *gin.Context, relayFormat types.RelayFormat, originalBody []byte) (dto.Request, *types.NewAPIError) {
	if relayFormat == types.RelayFormatOpenAIRealtime {
		return nil, nil
	}
	if err := resetRelayRequestBody(c, originalBody); err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	filterResult, err := service.ApplySensitiveFilterToRequestBody(c, relayFormat)
	if err != nil {
		if common.IsRequestBodyTooLargeError(err) || errors.Is(err, common.ErrRequestBodyTooLarge) {
			return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
		}
		return nil, types.NewError(err, types.ErrorCodeInvalidRequest)
	}
	if filterResult.Blocked {
		logger.LogWarn(c, fmt.Sprintf("user sensitive request blocked: %s", service.FormatSensitiveFilterMatches(filterResult.Matches)))
		return nil, service.NewSensitiveFilterAPIError(nil)
	}

	request, err := helper.GetAndValidateRequest(c, relayFormat)
	if err != nil {
		if common.IsRequestBodyTooLargeError(err) || errors.Is(err, common.ErrRequestBodyTooLarge) {
			return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
		}
		return nil, types.NewError(err, types.ErrorCodeInvalidRequest)
	}
	return request, nil
}

func resetRelayRequestBody(c *gin.Context, originalBody []byte) error {
	storage, err := common.CreateBodyStorage(originalBody)
	if err != nil {
		return err
	}
	if current, ok := c.Get(common.KeyBodyStorage); ok {
		if currentStorage, ok := current.(common.BodyStorage); ok && currentStorage != nil {
			_ = currentStorage.Close()
		}
	}
	c.Set(common.KeyBodyStorage, storage)
	c.Request.Body = io.NopCloser(storage)
	c.Request.ContentLength = int64(len(originalBody))
	return nil
}

func getChannel(c *gin.Context, info *relaycommon.RelayInfo, retryParam *service.RetryParam) (*model.Channel, *types.NewAPIError) {
	if info.ChannelMeta == nil {
		channel, ok := common.GetContextKeyType[*model.Channel](c, constant.ContextKeySelectedChannel)
		if !ok || channel == nil {
			return nil, types.NewError(errors.New("selected channel is unavailable"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
		}
		selectedGroup := common.GetContextKeyString(c, constant.ContextKeySelectedChannelGroup)
		if selectedGroup == "" {
			selectedGroup = info.UsingGroup
		}
		setSelectedSecurityAuditRoute(c, channel, selectedGroup)
		return channel, nil
	}
	channel, selectedGroup, err := service.CacheGetRandomSatisfiedChannel(retryParam)
	if err != nil {
		return nil, types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的可用渠道失败（retry）: %s", selectedGroup, info.OriginModelName, err.Error()), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if channel == nil {
		return nil, types.NewError(fmt.Errorf("分组 %s 下模型 %s 的可用渠道不存在（retry）", selectedGroup, info.OriginModelName), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if selectedGroup != "" {
		info.UsingGroup = selectedGroup
		common.SetContextKey(c, constant.ContextKeyUsingGroup, selectedGroup)
		common.SetContextKey(c, constant.ContextKeySelectedChannelGroup, selectedGroup)
	}
	info.PriceData.GroupRatioInfo = helper.HandleGroupRatio(c, info)
	newAPIError := middleware.SetupContextForSelectedChannel(c, channel, info.OriginModelName)
	if newAPIError != nil {
		return nil, newAPIError
	}
	setSelectedSecurityAuditRoute(c, channel, selectedGroup)
	return channel, nil
}

func setSelectedSecurityAuditRoute(c *gin.Context, channel *model.Channel, groupCode string) {
	if c == nil || channel == nil {
		return
	}
	common.SetContextKey(c, constant.ContextKeySelectedChannel, channel)
	if groupCode = strings.TrimSpace(groupCode); groupCode != "" {
		common.SetContextKey(c, constant.ContextKeySelectedChannelGroup, groupCode)
	}
}

func shouldRetry(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
	if openaiErr == nil {
		return false
	}
	if c == nil || c.Request == nil || c.Request.Context().Err() != nil {
		return false
	}
	if c.Writer != nil && c.Writer.Written() {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	if types.IsSkipRetryError(openaiErr) {
		return false
	}

	code := openaiErr.StatusCode
	if operation_setting.IsAlwaysSkipRetryCode(openaiErr.GetErrorCode()) {
		return false
	}
	configuredRetry := (code < 100 || code > 599) || operation_setting.ShouldRetryByStatusCode(code)
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if types.IsChannelError(openaiErr) {
		return true
	}
	if code >= 200 && code < 300 {
		return false
	}
	return configuredRetry
}

func processChannelError(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError) {
	if reason := requestContextErrorReason(c, err); reason != "" {
		logger.LogInfo(c, fmt.Sprintf("channel request stopped after request context ended: %s", reason))
		return
	}
	logger.LogError(c, fmt.Sprintf("channel error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, common.LocalLogPreview(err.Error())))
	// 不要使用context获取渠道信息，异步处理时可能会出现渠道信息不一致的情况
	// do not use context to get channel info, there may be inconsistent channel info when processing asynchronously
	if service.ShouldDisableChannel(err) && channelError.AutoBan {
		gopool.Go(func() {
			service.DisableChannelWithError(channelError, err)
		})
	}

	if constant.ErrorLogEnabled && types.IsRecordErrorLog(err) {
		// 保存错误日志到mysql中
		userId := c.GetInt("id")
		tokenName := c.GetString("token_name")
		modelName := c.GetString("original_model")
		tokenId := c.GetInt("token_id")
		userGroup := c.GetString("group")
		channelId := c.GetInt("channel_id")
		other := make(map[string]interface{})
		if c.Request != nil && c.Request.URL != nil {
			other["request_path"] = c.Request.URL.Path
		}
		other["error_type"] = err.GetErrorType()
		other["error_code"] = err.GetErrorCode()
		other["status_code"] = err.StatusCode
		other["channel_id"] = channelId
		other["channel_name"] = c.GetString("channel_name")
		other["channel_type"] = c.GetInt("channel_type")
		adminInfo := make(map[string]interface{})
		adminInfo["use_channel"] = c.GetStringSlice("use_channel")
		isMultiKey := common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey)
		if isMultiKey {
			adminInfo["is_multi_key"] = true
			adminInfo["multi_key_index"] = common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex)
		}
		service.AppendChannelAffinityAdminInfo(c, adminInfo)
		other["admin_info"] = adminInfo
		startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
		if startTime.IsZero() {
			startTime = time.Now()
		}
		useTimeSeconds := int(time.Since(startTime).Seconds())
		model.RecordErrorLog(c, userId, channelId, modelName, tokenName, err.MaskSensitiveErrorWithStatusCode(), tokenId, useTimeSeconds, common.GetContextKeyBool(c, constant.ContextKeyIsStream), userGroup, other)
	}

}

func RelayMidjourney(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatMjProxy, nil, nil)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"description": fmt.Sprintf("failed to generate relay info: %s", err.Error()),
			"type":        "upstream_error",
			"code":        4,
		})
		return
	}

	var mjErr *taskdto.MidjourneyResponse
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeMidjourneyNotify:
		mjErr = relay.RelayMidjourneyNotify(c)
	case relayconstant.RelayModeMidjourneyTaskFetch, relayconstant.RelayModeMidjourneyTaskFetchByCondition:
		mjErr = relay.RelayMidjourneyTask(c, relayInfo.RelayMode)
	case relayconstant.RelayModeMidjourneyTaskImageSeed:
		mjErr = relay.RelayMidjourneyTaskImageSeed(c)
	case relayconstant.RelayModeSwapFace:
		bodyStorage, bodyErr := common.GetBodyStorage(c)
		if bodyErr != nil {
			mjErr = service.MidjourneyErrorWrapper(constant.MjRequestError, "read_request_body_failed")
			break
		}
		originalBody, bodyErr := bodyStorage.Bytes()
		if bodyErr != nil {
			mjErr = service.MidjourneyErrorWrapper(constant.MjRequestError, "read_request_body_failed")
			break
		}
		if policyErr := prepareSelectedRouteTaskRequest(c, types.RelayFormatMjProxy, append([]byte(nil), originalBody...)); policyErr != nil {
			mjErr = service.MidjourneyErrorWrapper(constant.MjRequestError, policyErr.Message)
			break
		}
		mjErr = relay.RelaySwapFace(c, relayInfo)
	default:
		mjErr = relay.RelayMidjourneySubmit(c, relayInfo)
	}
	//err = relayMidjourneySubmit(c, relayMode)
	log.Println(mjErr)
	if mjErr != nil {
		statusCode := http.StatusBadRequest
		if mjErr.Code == 30 {
			mjErr.Result = "当前分组负载已饱和，请稍后再试，或升级账户以提升服务质量。"
			statusCode = http.StatusTooManyRequests
		}
		c.JSON(statusCode, gin.H{
			"description": fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result),
			"type":        "upstream_error",
			"code":        mjErr.Code,
		})
		channelId := c.GetInt("channel_id")
		logger.LogError(c, fmt.Sprintf("relay error (channel #%d, status code %d): %s", channelId, statusCode, fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result)))
	}
}

func RelayNotImplemented(c *gin.Context) {
	err := types.OpenAIError{
		Message: "API not implemented",
		Type:    "new_api_error",
		Param:   "",
		Code:    "api_not_implemented",
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": err,
	})
}

func RelayNotFound(c *gin.Context) {
	err := types.OpenAIError{
		Message: fmt.Sprintf("Invalid URL (%s %s)", c.Request.Method, c.Request.URL.Path),
		Type:    "invalid_request_error",
		Param:   "",
		Code:    "",
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error": err,
	})
}

const resolvedOriginTaskRelayInfoContextKey = "resolved_origin_task_relay_info"

// ResolveRemixOriginTask resolves and installs the persisted remix route before
// PromptAudit. The remix router intentionally omits Distribute so attempt zero
// receives exactly one complete selected-channel setup.
func ResolveRemixOriginTask() gin.HandlerFunc {
	return func(c *gin.Context) {
		relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
		if err != nil {
			respondTaskError(c, service.TaskErrorWrapperLocal(err, "gen_relay_info_failed", http.StatusInternalServerError))
			c.Abort()
			return
		}
		if taskErr := relay.ResolveOriginTask(c, relayInfo); taskErr != nil {
			respondTaskError(c, taskErr)
			c.Abort()
			return
		}
		channel, ok := relayInfo.LockedChannel.(*model.Channel)
		if !ok || channel == nil {
			respondTaskError(c, service.TaskErrorWrapperLocal(errors.New("remix origin channel is unavailable"), "channel_not_found", http.StatusBadRequest))
			c.Abort()
			return
		}
		if setupErr := middleware.SetupContextForSelectedChannel(c, channel, relayInfo.OriginModelName); setupErr != nil {
			respondTaskError(c, service.TaskErrorWrapperLocal(setupErr.Err, "setup_locked_channel_failed", http.StatusInternalServerError))
			c.Abort()
			return
		}
		service.RecordSystemInstanceRequestStart()
		defer service.RecordSystemInstanceRequestEnd()
		defer middleware.ReleaseChannelConcurrencyForContext(c)
		setSelectedSecurityAuditRoute(c, channel, relayInfo.UsingGroup)
		common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
		c.Set(resolvedOriginTaskRelayInfoContextKey, relayInfo)
		c.Next()
	}
}

func prepareSelectedRouteTaskRequest(c *gin.Context, relayFormat types.RelayFormat, originalBody []byte) *taskdto.TaskError {
	if err := resetRelayRequestBody(c, originalBody); err != nil {
		return service.TaskErrorWrapperLocal(err, "read_request_body_failed", http.StatusBadRequest)
	}
	filterResult, err := service.ApplySensitiveFilterToRequestBody(c, relayFormat)
	if err != nil {
		statusCode := http.StatusBadRequest
		if common.IsRequestBodyTooLargeError(err) || errors.Is(err, common.ErrRequestBodyTooLarge) {
			statusCode = http.StatusRequestEntityTooLarge
		}
		return service.TaskErrorWrapperLocal(err, "invalid_request", statusCode)
	}
	if !filterResult.Blocked {
		return nil
	}
	logger.LogWarn(c, fmt.Sprintf("user sensitive request blocked: %s", service.FormatSensitiveFilterMatches(filterResult.Matches)))
	apiErr := service.NewSensitiveFilterAPIError(c)
	clientErr, statusCode := service.SensitiveFilterFinalClientView(c, apiErr)
	return service.TaskErrorWrapperLocal(errors.New(clientErr.Message), string(types.ErrorCodeSensitiveWordsDetected), statusCode)
}

func RelayTaskFetch(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, &taskdto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}
	if taskErr := relay.RelayTaskFetch(c, relayInfo.RelayMode); taskErr != nil {
		respondTaskError(c, taskErr)
	}
}

func applyImageTaskAsyncPreConsume(c *gin.Context, relayInfo *relaycommon.RelayInfo) {
	if c == nil || relayInfo == nil || !c.GetBool(imageTaskAsyncContextKey) {
		return
	}
	relayInfo.ForcePreConsume = true
	// The replay boundary normalizes this typed capability from the persisted
	// task platform before relay generation. Copying it here keeps async Canvas
	// funding fully reserved without turning the request into Playground traffic.
	relayInfo.TokenQuotaExempt = common.GetContextKeyBool(c, constant.ContextKeyTokenQuotaExempt)
}

func taskErrorToMetricError(taskErr *taskdto.TaskError) *types.NewAPIError {
	if taskErr == nil {
		return nil
	}
	return types.NewOpenAIError(taskErr.Error, types.ErrorCodeBadResponseStatusCode, taskErr.StatusCode)
}

func requestContextErrorReason(c *gin.Context, err error) string {
	if err == nil || c == nil || c.Request == nil {
		return ""
	}
	contextErr := c.Request.Context().Err()
	if contextErr == nil || !errors.Is(err, contextErr) {
		return ""
	}
	switch {
	case errors.Is(contextErr, context.Canceled):
		return "request_context_canceled"
	case errors.Is(contextErr, context.DeadlineExceeded):
		return "request_context_deadline_exceeded"
	default:
		return "request_context_done"
	}
}

// respondTaskError 统一输出 Task 错误响应（含 429 限流提示改写）
func respondTaskError(c *gin.Context, taskErr *taskdto.TaskError) {
	if taskErr.StatusCode == http.StatusTooManyRequests {
		taskErr.Message = "当前分组上游负载已饱和，请稍后再试"
	}
	c.JSON(taskErr.StatusCode, taskErr)
}

func shouldRetryTaskRelay(c *gin.Context, channelId int, taskErr *taskdto.TaskError, retryTimes int) bool {
	if taskErr == nil || c == nil || c.Request == nil || c.Request.Context().Err() != nil {
		return false
	}
	if c.Writer != nil && c.Writer.Written() {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	status := taskErr.StatusCode
	configuredRetry := status == http.StatusTooManyRequests || status == http.StatusTemporaryRedirect ||
		(status/100 == 5 && !operation_setting.IsAlwaysSkipRetryStatusCode(status))
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if configuredRetry {
		return true
	}
	if status == http.StatusBadRequest || status == http.StatusRequestTimeout || taskErr.LocalError || status/100 == 2 {
		return false
	}
	return true
}
