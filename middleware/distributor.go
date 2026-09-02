package middleware

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type ModelRequest struct {
	Model string `json:"model"`
	Group string `json:"group,omitempty"`
}

const channelConcurrencyContextKey = "channel_concurrency_lease"

const groupUserConcurrencyContextKey = string(constant.ContextKeyGroupUserConcurrency)

type groupUserConcurrencyContextValue struct {
	lease     *model.GroupUserConcurrencyLease
	groupID   int
	isBenefit bool
}

type channelConcurrencyContextValue struct {
	lease         *model.ChannelConcurrencyLease
	stopHeartbeat context.CancelFunc
}

func applyRequestedGroup(c *gin.Context, inheritedGroup, requestedGroup string) (string, error) {
	requestedGroup = strings.TrimSpace(requestedGroup)
	if requestedGroup == "" {
		return inheritedGroup, nil
	}
	tokenGroup := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyTokenGroup))
	if requestedGroup == "auto" {
		if tokenGroup != "" && tokenGroup != "auto" {
			return "", errors.New("自动分组未绑定到令牌")
		}
	} else {
		authenticatedUserGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
		if !service.IsUserSelectableGroup(authenticatedUserGroup, requestedGroup) {
			return "", errors.New("请求分组不可用")
		}
		if tokenGroup != "" && tokenGroup != "auto" && len(service.GetRequestTokenGroups(c, requestedGroup)) == 0 {
			return "", errors.New("请求分组未绑定到令牌")
		}
	}
	common.SetContextKey(c, constant.ContextKeyUsingGroup, requestedGroup)
	common.SetContextKey(c, constant.ContextKeyBenefitGroupExplicit, requestedGroup != "auto")
	return requestedGroup, nil
}

func applyPlaygroundRequestedGroup(c *gin.Context, inheritedGroup, requestedGroup string) (string, error) {
	requestedGroup = strings.TrimSpace(requestedGroup)
	if requestedGroup == "" {
		return inheritedGroup, nil
	}
	authenticatedUserGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	if requestedGroup != "auto" {
		if !service.IsUserSelectableGroup(authenticatedUserGroup, requestedGroup) {
			return "", errors.New("请求分组不可用")
		}
	} else {
		if _, ok := service.GetUserUsableGroups(authenticatedUserGroup)["auto"]; !ok {
			return "", errors.New("自动分组不可用")
		}
	}
	common.SetContextKey(c, constant.ContextKeyUsingGroup, requestedGroup)
	common.SetContextKey(c, constant.ContextKeyBenefitGroupExplicit, requestedGroup != "auto")
	return requestedGroup, nil
}

func displayDistributorGroupIdentifier(group string, groupNames map[string]string) string {
	group = strings.TrimSpace(group)
	if group == "" {
		return ""
	}
	if name := strings.TrimSpace(groupNames[group]); name != "" {
		return name
	}
	return group
}

func displayDistributorGroupList(groups string, groupNames map[string]string) string {
	parts := strings.Split(groups, ",")
	for index, group := range parts {
		parts[index] = displayDistributorGroupIdentifier(group, groupNames)
	}
	return strings.Join(parts, ",")
}

func formatDistributorGroupForMessage(usingGroup, selectGroup string, groupNames map[string]string) string {
	using := strings.TrimSpace(usingGroup)
	selected := strings.TrimSpace(selectGroup)
	if using == "auto" {
		if selected == "" || selected == "auto" {
			return using
		}
		return fmt.Sprintf("auto(%s)", displayDistributorGroupList(selected, groupNames))
	}
	if strings.Contains(using, ",") {
		if selected == "" || selected == using || strings.Contains(selected, ",") {
			selected = using
		}
		return fmt.Sprintf("multi(%s)", displayDistributorGroupList(selected, groupNames))
	}
	return displayDistributorGroupIdentifier(using, groupNames)
}

func distributorGroupForMessage(usingGroup, selectGroup string) string {
	groupNames := map[string]string(nil)
	if model.DB != nil {
		if names, err := model.GetGroupDisplayNameMap(); err == nil {
			groupNames = names
		}
	}
	return formatDistributorGroupForMessage(usingGroup, selectGroup, groupNames)
}

func Distribute() func(c *gin.Context) {
	return func(c *gin.Context) {
		if IsResponsesWebsocketHandshake(c) {
			c.Next()
			return
		}
		var channel *model.Channel
		var selectGroup string
		usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
		channelId, ok := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId)
		modelRequest, shouldSelectChannel, err := getModelRequest(c)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, i18n.T(c, i18n.MsgDistributorInvalidRequest, map[string]any{"Error": err.Error()}))
			return
		}
		if modelRequest.Group != "" {
			if strings.HasPrefix(c.Request.URL.Path, "/pg/chat/completions") {
				usingGroup, err = applyPlaygroundRequestedGroup(c, usingGroup, modelRequest.Group)
			} else {
				usingGroup, err = applyRequestedGroup(c, usingGroup, modelRequest.Group)
			}
			if err != nil {
				abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgDistributorGroupAccessDenied))
				return
			}
		}
		if ok {
			id, err := strconv.Atoi(channelId.(string))
			if err != nil {
				abortWithOpenAiMessage(c, http.StatusBadRequest, i18n.T(c, i18n.MsgDistributorInvalidChannelId))
				return
			}
			channel, err = model.GetChannelById(id, true)
			if err != nil {
				abortWithOpenAiMessage(c, http.StatusBadRequest, i18n.T(c, i18n.MsgDistributorInvalidChannelId))
				return
			}
			if channel.Status != common.ChannelStatusEnabled {
				abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgDistributorChannelDisabled))
				return
			}
		} else {
			// Select a channel for the user
			// check token model mapping
			modelLimitEnable := common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled)
			if modelLimitEnable {
				s, ok := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
				if !ok {
					// token model limit is empty, all models are not allowed
					abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgDistributorTokenNoModelAccess))
					return
				}
				var tokenModelLimit map[string]bool
				tokenModelLimit, ok = s.(map[string]bool)
				if !ok {
					tokenModelLimit = map[string]bool{}
				}
				matchName := ratio_setting.FormatMatchingModelName(modelRequest.Model) // match gpts & thinking-*
				if _, ok := tokenModelLimit[matchName]; !ok {
					abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgDistributorTokenModelForbidden, map[string]any{"Model": modelRequest.Model}))
					return
				}
			}

			if shouldSelectChannel {
				if modelRequest.Model == "" {
					abortWithOpenAiMessage(c, http.StatusBadRequest, i18n.T(c, i18n.MsgDistributorModelNameRequired))
					return
				}
				if binding, found := service.GetPreferredChannelAffinityBinding(c, modelRequest.Model, usingGroup); found {
					affinityUsable := false
					bindingKeyUsable := true
					preferred, lookupErr := model.CacheGetChannel(binding.ChannelID)
					if binding.BindMultiKey && lookupErr == nil && preferred != nil {
						if _, _, keyErr := preferred.GetKeyByIndex(binding.MultiKeyIndex); keyErr != nil {
							bindingKeyUsable = false
						}
					}
					if bindingKeyUsable && lookupErr == nil && preferred != nil && preferred.Status == common.ChannelStatusEnabled && channelSupportsRequestPath(preferred, c.Request.URL.Path, modelRequest.Model) {
						if usingGroup == "auto" {
							userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
							for index, group := range service.GetRequestAutoGroups(c, userGroup) {
								if model.IsChannelEnabledForGroupModel(group, modelRequest.Model, preferred.Id) {
									selectGroup, channel, affinityUsable = group, preferred, true
									common.SetContextKey(c, constant.ContextKeyAutoGroup, group)
									setAffinityOrderedGroupRetryState(c, index)
									break
								}
							}
						} else {
							groups := service.GetRequestTokenGroups(c, usingGroup)
							if len(groups) == 0 {
								groups = []string{usingGroup}
							}
							for index, group := range groups {
								if model.IsChannelEnabledForGroupModel(group, modelRequest.Model, preferred.Id) {
									selectGroup, channel, affinityUsable = group, preferred, true
									if len(groups) > 1 {
										setAffinityOrderedGroupRetryState(c, index)
									}
									break
								}
							}
						}
						if affinityUsable && binding.BindMultiKey {
							common.SetContextKey(c, constant.ContextKeyChannelPreferredMultiKeyIndex, binding.MultiKeyIndex)
						}
						if affinityUsable {
							service.MarkChannelAffinityUsed(c, selectGroup, preferred.Id)
						}
					}
					if !affinityUsable && !service.ShouldKeepChannelAffinityOnChannelDisabled() {
						service.ClearCurrentChannelAffinityCache(c)
					}
				}

				if channel == nil {
					channel, selectGroup, err = service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
						Ctx: c, ModelName: modelRequest.Model, TokenGroup: usingGroup,
						RequestPath: c.Request.URL.Path, Retry: common.GetPointer(0),
					})
					if err != nil {
						showGroup := distributorGroupForMessage(usingGroup, selectGroup)
						message := i18n.T(c, i18n.MsgDistributorGetChannelFailed, map[string]any{"Group": showGroup, "Model": modelRequest.Model, "Error": channelSelectionErrorMessage(c, err)})
						errorCode := types.ErrorCodeModelNotFound
						if errors.Is(err, model.ErrChannelConcurrencyLimitReached) {
							errorCode = types.ErrorCodeChannelConcurrencyLimit
						}
						abortWithOpenAiMessage(c, http.StatusServiceUnavailable, message, errorCode)
						return
					}
					if channel == nil {
						abortWithOpenAiMessage(c, http.StatusServiceUnavailable, i18n.T(c, i18n.MsgDistributorNoAvailableChannel, map[string]any{"Group": distributorGroupForMessage(usingGroup, selectGroup), "Model": modelRequest.Model}), types.ErrorCodeModelNotFound)
						return
					}
				}
				if selectGroup != "" {
					common.SetContextKey(c, constant.ContextKeyUsingGroup, selectGroup)
					common.SetContextKey(c, constant.ContextKeySelectedChannelGroup, selectGroup)
				}

			}
		}
		common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
		newAPIError := SetupContextForSelectedChannel(c, channel, modelRequest.Model)
		if newAPIError != nil && !ok && shouldSelectChannel && !errors.Is(newAPIError, model.ErrGroupUserConcurrencyLimitReached) {
			releaseChannelConcurrencyForContext(c)
			releaseGroupUserConcurrencyForContext(c)
			exclusions := map[int]struct{}{channel.Id: {}}
			for attempt := 0; attempt < common.RetryTimes; attempt++ {
				channel, selectGroup, err = service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
					Ctx: c, ModelName: modelRequest.Model, TokenGroup: usingGroup,
					RequestPath: c.Request.URL.Path, Retry: common.GetPointer(0),
					ExcludedChannelIDs: exclusions,
				})
				if err != nil {
					if errors.Is(err, model.ErrChannelConcurrencyLimitReached) {
						newAPIError = types.NewError(err, types.ErrorCodeChannelConcurrencyLimit, types.ErrOptionWithSkipRetry())
					}
					break
				}
				if channel == nil {
					break
				}
				exclusions[channel.Id] = struct{}{}
				newAPIError = SetupContextForSelectedChannel(c, channel, modelRequest.Model)
				if newAPIError == nil {
					break
				}
				releaseChannelConcurrencyForContext(c)
			}
		}
		if newAPIError != nil {
			releaseChannelConcurrencyForContext(c)
			releaseGroupUserConcurrencyForContext(c)
			statusCode := types.ErrorCodeModelNotFound
			if errors.Is(newAPIError, model.ErrChannelConcurrencyLimitReached) {
				statusCode = types.ErrorCodeChannelConcurrencyLimit
			}
			httpStatus := http.StatusServiceUnavailable
			if errors.Is(newAPIError, model.ErrGroupUserConcurrencyLimitReached) {
				httpStatus = http.StatusTooManyRequests
				statusCode = types.ErrorCodeChannelConcurrencyLimit
			}
			abortWithOpenAiMessage(c, httpStatus, channelSelectionErrorMessage(c, newAPIError), statusCode)
			return
		}
		service.RecordSystemInstanceRequestStart()
		defer service.RecordSystemInstanceRequestEnd()
		defer releaseChannelConcurrencyForContext(c)
		defer releaseGroupUserConcurrencyForContext(c)
		c.Next()
		if channel != nil && c.Writer != nil && c.Writer.Status() < http.StatusBadRequest {
			service.RecordChannelAffinity(c, channel.Id)
		}
	}
}

// IsResponsesWebsocketHandshake identifies the GET handshake that must pass
// through authentication and admission middleware without attempting to parse
// a request body. Each WebSocket turn runs the normal POST middleware chain on
// its isolated synthetic request.
func IsResponsesWebsocketHandshake(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	return c.Request.Method == http.MethodGet &&
		c.Request.URL.Path == "/v1/responses" &&
		strings.EqualFold(c.Request.Header.Get("Upgrade"), "websocket")
}

func channelSelectionErrorMessage(c *gin.Context, err error) string {
	if errors.Is(err, model.ErrGroupUserConcurrencyLimitReached) {
		if benefit, ok := common.GetContextKey(c, constant.ContextKeyGroupUserConcurrencyBenefit); ok && benefit == true {
			return "福利分组限制，你请求太快啦！"
		}
		return "Too Many Requests"
	}
	if errors.Is(err, model.ErrChannelConcurrencyLimitReached) {
		return i18n.T(c, i18n.MsgDistributorChannelConcurrencyLimit)
	}
	return err.Error()
}

func setAffinityOrderedGroupRetryState(c *gin.Context, groupIndex int) {
	common.SetContextKey(c, constant.ContextKeyAutoGroupIndex, groupIndex)
	// The affinity-bound channel is attempt zero, but its actual priority tier
	// is unknown. Anchor the first relay retry (global retry one) at local tier
	// zero so an untried higher-priority channel cannot be skipped.
	common.SetContextKey(c, constant.ContextKeyAutoGroupRetryIndex, 1)
}

// channelSupportsRequestPath reports whether a channel can serve the request path.
// Only Advanced Custom (type 58) channels are path-checked; all other channel types
// always pass. A type-58 channel is usable only when one of its routes matches.
func channelSupportsRequestPath(channel *model.Channel, requestPath string, requestModel string) bool {
	if channel == nil {
		return false
	}
	if channel.Type != constant.ChannelTypeAdvancedCustom {
		return true
	}
	config := channel.GetOtherSettings().AdvancedCustom
	return config != nil && config.SupportsPathForModel(requestPath, requestModel)
}

// getModelFromRequest 从请求中读取模型信息
// 根据 Content-Type 自动处理：
// - application/json
// - application/x-www-form-urlencoded
// - multipart/form-data
func getModelFromRequest(c *gin.Context) (*ModelRequest, error) {
	if strings.HasPrefix(c.Request.Header.Get("Content-Type"), "application/json") {
		modelRequest, err := getModelFromJSONBody(c)
		if err != nil {
			return nil, errors.New(i18n.T(c, i18n.MsgDistributorInvalidRequest, map[string]any{"Error": err.Error()}))
		}
		return modelRequest, nil
	}

	var modelRequest ModelRequest
	err := common.UnmarshalBodyReusable(c, &modelRequest)
	if err != nil {
		return nil, errors.New(i18n.T(c, i18n.MsgDistributorInvalidRequest, map[string]any{"Error": err.Error()}))
	}
	return &modelRequest, nil
}

func getModelFromJSONBody(c *gin.Context) (*ModelRequest, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, err
	}
	requestBody, err := storage.Bytes()
	if err != nil {
		return nil, err
	}
	if !gjson.ValidBytes(requestBody) {
		return nil, errors.New("invalid JSON request body")
	}

	values := gjson.GetManyBytes(requestBody, "model", "group")
	model, err := getJSONStringValue(values[0], "model")
	if err != nil {
		return nil, err
	}
	group, err := getJSONStringValue(values[1], "group")
	if err != nil {
		return nil, err
	}

	if _, seekErr := storage.Seek(0, io.SeekStart); seekErr != nil {
		return nil, seekErr
	}
	c.Request.Body = io.NopCloser(storage)

	return &ModelRequest{
		Model: model,
		Group: group,
	}, nil
}

func getJSONStringValue(result gjson.Result, field string) (string, error) {
	if !result.Exists() || result.Type == gjson.Null {
		return "", nil
	}
	if result.Type != gjson.String {
		return "", fmt.Errorf("field %s must be a string", field)
	}
	return result.String(), nil
}

func getModelRequest(c *gin.Context) (*ModelRequest, bool, error) {
	var modelRequest ModelRequest
	shouldSelectChannel := true
	var err error
	if strings.Contains(c.Request.URL.Path, "/mj/") {
		relayMode := relayconstant.Path2RelayModeMidjourney(c.Request.URL.Path)
		if relayMode == relayconstant.RelayModeMidjourneyTaskFetch ||
			relayMode == relayconstant.RelayModeMidjourneyTaskFetchByCondition ||
			relayMode == relayconstant.RelayModeMidjourneyNotify ||
			relayMode == relayconstant.RelayModeMidjourneyTaskImageSeed {
			shouldSelectChannel = false
		} else {
			midjourneyRequest := taskdto.MidjourneyRequest{}
			err = common.UnmarshalBodyReusable(c, &midjourneyRequest)
			if err != nil {
				return nil, false, errors.New(i18n.T(c, i18n.MsgDistributorInvalidMidjourney, map[string]any{"Error": err.Error()}))
			}
			midjourneyModel, mjErr, success := service.GetMjRequestModel(relayMode, &midjourneyRequest)
			if mjErr != nil {
				return nil, false, fmt.Errorf("%s", mjErr.Description)
			}
			if midjourneyModel == "" {
				if !success {
					return nil, false, fmt.Errorf("%s", i18n.T(c, i18n.MsgDistributorInvalidParseModel))
				} else {
					// task fetch, task fetch by condition, notify
					shouldSelectChannel = false
				}
			}
			modelRequest.Model = midjourneyModel
		}
		c.Set("relay_mode", relayMode)
	} else if strings.Contains(c.Request.URL.Path, "/suno/") {
		relayMode := relayconstant.Path2RelaySuno(c.Request.Method, c.Request.URL.Path)
		if relayMode == relayconstant.RelayModeSunoFetch ||
			relayMode == relayconstant.RelayModeSunoFetchByID {
			shouldSelectChannel = false
		} else {
			modelName := service.CoverTaskActionToModelName(constant.TaskPlatformSuno, c.Param("action"))
			modelRequest.Model = modelName
		}
		c.Set("platform", string(constant.TaskPlatformSuno))
		c.Set("relay_mode", relayMode)
	} else if strings.Contains(c.Request.URL.Path, "/v1/videos/") && strings.HasSuffix(c.Request.URL.Path, "/remix") {
		relayMode := relayconstant.RelayModeVideoSubmit
		c.Set("relay_mode", relayMode)
		shouldSelectChannel = false
	} else if strings.Contains(c.Request.URL.Path, "/v1/videos") {
		//curl https://api.openai.com/v1/videos \
		//  -H "Authorization: Bearer $OPENAI_API_KEY" \
		//  -F "model=sora-2" \
		//  -F "prompt=A calico cat playing a piano on stage"
		//	-F input_reference="@image.jpg"
		relayMode := relayconstant.RelayModeUnknown
		if c.Request.Method == http.MethodPost {
			relayMode = relayconstant.RelayModeVideoSubmit
			req, err := getModelFromRequest(c)
			if err != nil {
				return nil, false, err
			}
			if req != nil {
				modelRequest.Model = req.Model
			}
		} else if c.Request.Method == http.MethodGet {
			relayMode = relayconstant.RelayModeVideoFetchByID
			shouldSelectChannel = false
			modelRequest.Model = getTaskOriginModelName(c)
		}
		c.Set("relay_mode", relayMode)
	} else if strings.Contains(c.Request.URL.Path, "/v1/video/generations") {
		relayMode := relayconstant.RelayModeUnknown
		if c.Request.Method == http.MethodPost {
			req, err := getModelFromRequest(c)
			if err != nil {
				return nil, false, err
			}
			modelRequest.Model = req.Model
			relayMode = relayconstant.RelayModeVideoSubmit
		} else if c.Request.Method == http.MethodGet {
			relayMode = relayconstant.RelayModeVideoFetchByID
			shouldSelectChannel = false
			modelRequest.Model = getTaskOriginModelName(c)
		}
		if _, ok := c.Get("relay_mode"); !ok {
			c.Set("relay_mode", relayMode)
		}
	} else if strings.HasPrefix(c.Request.URL.Path, "/v1beta/models/") || strings.HasPrefix(c.Request.URL.Path, "/v1/models/") {
		// Gemini API 路径处理: /v1beta/models/gemini-2.0-flash:generateContent
		relayMode := relayconstant.RelayModeGemini
		modelName := extractModelNameFromGeminiPath(c.Request.URL.Path)
		if modelName != "" {
			modelRequest.Model = modelName
		}
		c.Set("relay_mode", relayMode)
	} else if !strings.HasPrefix(c.Request.URL.Path, "/v1/audio/transcriptions") && !strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
		req, err := getModelFromRequest(c)
		if err != nil {
			return nil, false, err
		}
		modelRequest.Model = req.Model
		modelRequest.Group = req.Group
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/realtime") {
		//wss://api.openai.com/v1/realtime?model=gpt-4o-realtime-preview-2024-10-01
		modelRequest.Model = c.Query("model")
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/moderations") {
		if modelRequest.Model == "" {
			modelRequest.Model = "text-moderation-stable"
		}
	}
	if strings.HasSuffix(c.Request.URL.Path, "embeddings") {
		if modelRequest.Model == "" {
			modelRequest.Model = c.Param("model")
		}
	}
	if isImageGenerationPath(c.Request.URL.Path) {
		modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "dall-e")
	} else if isImageEditPath(c.Request.URL.Path) {
		//modelRequest.Model = common.GetStringIfEmpty(c.PostForm("model"), "gpt-image-1")
		contentType := c.ContentType()
		if slices.Contains([]string{gin.MIMEPOSTForm, gin.MIMEMultipartPOSTForm}, contentType) {
			req, err := getModelFromRequest(c)
			if err == nil && req.Model != "" {
				modelRequest.Model = req.Model
			}
		}
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/audio") {
		relayMode := relayconstant.RelayModeAudioSpeech
		if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/speech") {

			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "tts-1")
		} else if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/translations") {
			// 先尝试从请求读取
			if req, err := getModelFromRequest(c); err == nil && req.Model != "" {
				modelRequest.Model = req.Model
			}
			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "whisper-1")
			relayMode = relayconstant.RelayModeAudioTranslation
		} else if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/transcriptions") {
			// 先尝试从请求读取
			if req, err := getModelFromRequest(c); err == nil && req.Model != "" {
				modelRequest.Model = req.Model
			}
			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "whisper-1")
			relayMode = relayconstant.RelayModeAudioTranscription
		}
		c.Set("relay_mode", relayMode)
	}
	return &modelRequest, shouldSelectChannel, nil
}

func isImageGenerationPath(path string) bool {
	return strings.HasPrefix(path, "/v1/images/generations") ||
		strings.HasPrefix(path, "/canvas/v1/images/generations")
}

func isImageEditPath(path string) bool {
	return strings.HasPrefix(path, "/v1/images/edits") ||
		strings.HasPrefix(path, "/canvas/v1/images/edits")
}

// 修复 #4834: GET /v1/video/generations/:task_id && /v1/video/:task_id 此前不解析 model，
// 当 token 启用「可用模型限制」时，下游 modelLimitEnable 校验会因
// modelRequest.Model 为空而误报 "This token has no access to model"。
// 从已存储的任务记录中回填 OriginModelName 即可让校验走在正确的模型上。
func getTaskOriginModelName(c *gin.Context) string {
	if !common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled) {
		return ""
	}

	taskId := c.Param("task_id")
	if taskId == "" {
		// jimeng adapter
		taskId = c.GetString("task_id")
	}
	if taskId == "" {
		return ""
	}

	userId := c.GetInt("id")
	if task, exist, err := model.GetByTaskId(userId, taskId); err == nil && exist && task != nil {
		return task.Properties.OriginModelName
	}
	return ""
}

func SetupContextForSelectedChannel(c *gin.Context, channel *model.Channel, modelName string) (newAPIError *types.NewAPIError) {
	c.Set("original_model", modelName)
	if channel == nil {
		return types.NewError(errors.New("channel is nil"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if !tryAcquireChannelConcurrencyForContext(c, channel) {
		return types.NewError(model.ErrChannelConcurrencyLimitReached, types.ErrorCodeChannelConcurrencyLimit, types.ErrOptionWithSkipRetry())
	}
	defer func() {
		// setup errors must not retain a slot for a request that never entered relay.
		if newAPIError != nil {
			releaseChannelConcurrencyForContext(c)
		}
	}()
	common.SetContextKey(c, constant.ContextKeyChannelId, channel.Id)
	common.SetContextKey(c, constant.ContextKeyChannelName, channel.Name)
	common.SetContextKey(c, constant.ContextKeyChannelType, channel.Type)
	common.SetContextKey(c, constant.ContextKeyChannelCreateTime, channel.CreatedTime)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, channel.GetSetting())
	common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, channel.GetOtherSettings())
	paramOverride := channel.GetParamOverride()
	headerOverride := channel.GetHeaderOverride()
	if mergedParam, applied := service.ApplyChannelAffinityOverrideTemplate(c, paramOverride); applied {
		paramOverride = mergedParam
	}
	common.SetContextKey(c, constant.ContextKeyChannelParamOverride, paramOverride)
	common.SetContextKey(c, constant.ContextKeyChannelHeaderOverride, headerOverride)
	if channel.OpenAIOrganization != nil && *channel.OpenAIOrganization != "" {
		common.SetContextKey(c, constant.ContextKeyChannelOrganization, *channel.OpenAIOrganization)
	}
	common.SetContextKey(c, constant.ContextKeyChannelAutoBan, channel.GetAutoBan())
	common.SetContextKey(c, constant.ContextKeyChannelModelMapping, channel.GetModelMapping())
	common.SetContextKey(c, constant.ContextKeyChannelStatusCodeMapping, channel.GetStatusCodeMapping())
	common.SetContextKey(c, constant.ContextKeySelectedChannel, channel)
	selectedGroup := common.GetContextKeyString(c, constant.ContextKeySelectedChannelGroup)
	if selectedGroup == "" {
		selectedGroup = common.GetContextKeyString(c, constant.ContextKeyAutoGroup)
	}
	if selectedGroup == "" {
		selectedGroup = common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	}
	common.SetContextKey(c, constant.ContextKeySelectedChannelGroup, selectedGroup)
	if selectedGroup != "" {
		if group, err := model.GetGroupByCodeOrAlias(selectedGroup); err == nil && group != nil {
			if !tryAcquireGroupUserConcurrencyForContext(c, group) {
				return types.NewError(model.ErrGroupUserConcurrencyLimitReached, types.ErrorCodeChannelConcurrencyLimit, types.ErrOptionWithSkipRetry())
			}
		}
	}

	var key string
	var index int
	preferred, hasPreferred := common.GetContextKey(c, constant.ContextKeyChannelPreferredMultiKeyIndex)
	if c.Keys != nil {
		delete(c.Keys, string(constant.ContextKeyChannelPreferredMultiKeyIndex))
	}
	if hasPreferred {
		if preferredIndex, valid := preferred.(int); valid {
			key, index, newAPIError = channel.GetKeyByIndex(preferredIndex)
		}
	}
	if key == "" {
		key, index, newAPIError = channel.GetNextEnabledKey()
	}
	if newAPIError != nil {
		return newAPIError
	}
	if channel.ChannelInfo.IsMultiKey {
		common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, true)
		common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, index)
	} else {
		common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, false)
	}
	common.SetContextKey(c, constant.ContextKeyChannelKey, key)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, channel.GetBaseURL())
	common.SetContextKey(c, constant.ContextKeySystemPromptOverride, false)

	// TODO: api_version统一
	switch channel.Type {
	case constant.ChannelTypeAzure:
		c.Set("api_version", channel.Other)
	case constant.ChannelTypeVertexAi:
		c.Set("region", channel.Other)
	case constant.ChannelTypeXunfei:
		c.Set("api_version", channel.Other)
	case constant.ChannelTypeGemini:
		c.Set("api_version", channel.Other)
	case constant.ChannelTypeAli:
		c.Set("plugin", channel.Other)
	case constant.ChannelCloudflare:
		c.Set("api_version", channel.Other)
	case constant.ChannelTypeMokaAI:
		c.Set("api_version", channel.Other)
	case constant.ChannelTypeCoze:
		c.Set("bot_id", channel.Other)
	}
	return nil
}

func tryAcquireGroupUserConcurrencyForContext(c *gin.Context, group *model.Group) bool {
	if c == nil || group == nil {
		return true
	}
	if acquired, ok := common.GetContextKey(c, constant.ContextKeyGroupUserConcurrency); ok {
		if value, valid := acquired.(groupUserConcurrencyContextValue); valid && value.lease != nil && value.groupID == group.Id {
			return true
		}
		releaseGroupUserConcurrencyForContext(c)
	}
	lease, acquired := model.TryAcquireGroupUserConcurrency(c.GetInt("id"), group.Id, group.SingleUserConcurrencyLimit)
	if !acquired {
		common.SetContextKey(c, constant.ContextKeyGroupUserConcurrencyBenefit, model.HasActiveBenefitActivityForGroup(group.Id))
		return false
	}
	if lease == nil {
		return true
	}
	common.SetContextKey(c, constant.ContextKeyGroupUserConcurrencyBenefit, model.HasActiveBenefitActivityForGroup(group.Id))
	common.SetContextKey(c, constant.ContextKeyGroupUserConcurrency, groupUserConcurrencyContextValue{
		lease: lease, groupID: group.Id,
		isBenefit: common.GetContextKeyBool(c, constant.ContextKeyGroupUserConcurrencyBenefit),
	})
	return true
}

func releaseGroupUserConcurrencyForContext(c *gin.Context) {
	if c == nil {
		return
	}
	acquired, ok := common.GetContextKey(c, constant.ContextKeyGroupUserConcurrency)
	if c.Keys != nil {
		delete(c.Keys, groupUserConcurrencyContextKey)
		delete(c.Keys, string(constant.ContextKeyGroupUserConcurrencyBenefit))
	}
	if !ok {
		return
	}
	value, ok := acquired.(groupUserConcurrencyContextValue)
	if ok && value.lease != nil {
		value.lease.Release()
	}
}

func ReleaseGroupUserConcurrencyForContext(c *gin.Context) {
	releaseGroupUserConcurrencyForContext(c)
}

func tryAcquireChannelConcurrencyForContext(c *gin.Context, channel *model.Channel) bool {
	if acquired, ok := common.GetContextKey(c, channelConcurrencyContextKey); ok {
		if value, ok := acquired.(channelConcurrencyContextValue); ok && value.lease != nil && value.lease.ChannelID == channel.Id {
			return true
		}
	}
	lease, acquired := model.TryAcquireChannelConcurrencyLease(channel)
	if !acquired {
		return false
	}
	releaseChannelConcurrencyForContext(c)
	if lease != nil && common.RedisEnabled {
		heartbeatContext := context.Background()
		if c.Request != nil {
			heartbeatContext = c.Request.Context()
		}
		heartbeatContext, stopHeartbeat := context.WithCancel(heartbeatContext)
		common.SetContextKey(c, channelConcurrencyContextKey, channelConcurrencyContextValue{
			lease:         lease,
			stopHeartbeat: stopHeartbeat,
		})
		go renewChannelConcurrencyLease(heartbeatContext, lease)
	} else if lease != nil {
		common.SetContextKey(c, channelConcurrencyContextKey, channelConcurrencyContextValue{lease: lease})
	}
	return true
}

func releaseChannelConcurrencyForContext(c *gin.Context) {
	acquired, ok := common.GetContextKey(c, channelConcurrencyContextKey)
	if !ok {
		return
	}
	if c.Keys != nil {
		delete(c.Keys, channelConcurrencyContextKey)
	}
	value, ok := acquired.(channelConcurrencyContextValue)
	if !ok || value.lease == nil {
		return
	}
	if value.stopHeartbeat != nil {
		value.stopHeartbeat()
	}
	model.ReleaseChannelConcurrencyLease(value.lease)
}

func renewChannelConcurrencyLease(ctx context.Context, lease *model.ChannelConcurrencyLease) {
	ticker := time.NewTicker(model.ChannelConcurrencyLeaseRenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !model.RenewChannelConcurrencyLease(lease) {
				common.SysError(fmt.Sprintf("channel concurrency lease renewal lost for channel #%d", lease.ChannelID))
			}
		}
	}
}

// ReleaseChannelConcurrencyForContext 释放 c 所拥有的渠道并发槽位。
// Distribute 之外自行选渠的调用方必须按所有权释放，避免 setup 失败或重试
// 清理时误释放后续请求占用的槽位。
func ReleaseChannelConcurrencyForContext(c *gin.Context) {
	releaseChannelConcurrencyForContext(c)
}

// extractModelNameFromGeminiPath 从 Gemini API URL 路径中提取模型名
// 输入格式: /v1beta/models/gemini-2.0-flash:generateContent
// 输出: gemini-2.0-flash
func extractModelNameFromGeminiPath(path string) string {
	// 查找 "/models/" 的位置
	modelsPrefix := "/models/"
	modelsIndex := strings.Index(path, modelsPrefix)
	if modelsIndex == -1 {
		return ""
	}

	// 从 "/models/" 之后开始提取
	startIndex := modelsIndex + len(modelsPrefix)
	if startIndex >= len(path) {
		return ""
	}

	// 查找 ":" 的位置，模型名在 ":" 之前
	colonIndex := strings.Index(path[startIndex:], ":")
	if colonIndex == -1 {
		// 如果没有找到 ":"，返回从 "/models/" 到路径结尾的部分
		return path[startIndex:]
	}

	// 返回模型名部分
	return path[startIndex : startIndex+colonIndex]
}
