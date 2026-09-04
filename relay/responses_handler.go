package relay

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"

	"github.com/gin-gonic/gin"
)

func ResponsesHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)
	if info.RelayMode == relayconstant.RelayModeResponsesCompact &&
		!common.SupportsResponsesCompact(info.ChannelType, info.ApiType) {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("unsupported endpoint %q for api type %d", "/v1/responses/compact", info.ApiType),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}

	var responsesReq *dto.OpenAIResponsesRequest
	switch req := info.Request.(type) {
	case *dto.OpenAIResponsesRequest:
		responsesReq = req
	case *dto.OpenAIResponsesCompactionRequest:
		// Only fields documented for POST /v1/responses/compact are forwarded:
		// model, input, instructions, previous_response_id, prompt_cache_key,
		// prompt_cache_options, prompt_cache_retention, service_tier.
		// Undocumented Codex-parity fields (tools, reasoning, text) are parsed
		// for client compatibility but intentionally not sent upstream.
		responsesReq = &dto.OpenAIResponsesRequest{
			Model:                req.Model,
			Input:                req.Input,
			Instructions:         req.Instructions,
			PreviousResponseID:   req.PreviousResponseID,
			ParallelToolCalls:    req.ParallelToolCalls,
			ServiceTier:          req.ServiceTier,
			PromptCacheKey:       req.PromptCacheKey,
			PromptCacheOptions:   req.PromptCacheOptions,
			PromptCacheRetention: req.PromptCacheRetention,
		}
	default:
		return types.NewErrorWithStatusCode(
			fmt.Errorf("invalid request type, expected dto.OpenAIResponsesRequest or dto.OpenAIResponsesCompactionRequest, got %T", info.Request),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}

	request, err := common.DeepCopy(responsesReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to GeneralOpenAIRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}
	stripHTTPResponsesContinuation(request)
	request.Input, err = normalizeHTTPResponsesInput(request.Input)
	if err != nil {
		return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)
	var requestBody io.Reader
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return types.NewError(err, types.ErrorCodeReadRequestBodyFailed, types.ErrOptionWithSkipRetry())
		}
		var bodyCloser io.Closer
		requestBody, bodyCloser, err = newSanitizedHTTPResponsesBody(storage)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		if bodyCloser != nil {
			defer bodyCloser.Close()
		}
	} else {
		convertedRequest, err := adaptor.ConvertOpenAIResponsesRequest(c, info, *request)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)
		jsonData, err := common.Marshal(convertedRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		// remove disabled fields for OpenAI Responses API
		jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		// apply param override
		if len(info.ParamOverride) > 0 {
			jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
			if err != nil {
				return newAPIErrorFromParamOverride(err)
			}
		}
		jsonData, err = sanitizeHTTPResponsesBody(jsonData)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		logger.LogDebug(c, "requestBody: %s", jsonData)
		body, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		defer closer.Close()
		jsonData = nil
		requestBody = body
	}

	var httpResp *http.Response
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	if resp != nil {
		httpResp = resp.(*http.Response)

		if httpResp.StatusCode != http.StatusOK {
			newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			// reset status code 重置状态码
			service.ResetStatusCode(newAPIError, statusCodeMappingStr)
			return newAPIError
		}
	}

	usage, newAPIError := adaptor.DoResponse(c, httpResp, info)
	if newAPIError != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}

	usageDto := usage.(*dto.Usage)
	if info.RelayMode == relayconstant.RelayModeResponsesCompact {
		originModelName := info.OriginModelName
		originPriceData := info.PriceData

		_, err := helper.ModelPriceHelper(c, info, info.GetEstimatePromptTokens(), &types.TokenCountMeta{})
		if err != nil {
			info.OriginModelName = originModelName
			info.PriceData = originPriceData
			return types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithSkipRetry(), types.ErrOptionWithStatusCode(http.StatusBadRequest))
		}
		quotaErr := service.PostTextConsumeQuota(c, info, usageDto, nil)

		info.OriginModelName = originModelName
		info.PriceData = originPriceData
		return quotaErr
	}

	if strings.HasPrefix(info.OriginModelName, "gpt-4o-audio") {
		return service.PostAudioConsumeQuota(c, info, usageDto, "")
	} else {
		return service.PostTextConsumeQuota(c, info, usageDto, nil)
	}
}

// stripHTTPResponsesContinuation 删除 HTTP/SSE 上游不支持的 WebSocket 续传 ID。
func stripHTTPResponsesContinuation(request *dto.OpenAIResponsesRequest) {
	if request == nil {
		return
	}
	request.PreviousResponseID = ""
}

func newSanitizedHTTPResponsesBody(storage common.BodyStorage) (io.Reader, io.Closer, error) {
	if storage == nil {
		return nil, nil, fmt.Errorf("request body storage is nil")
	}
	raw, err := storage.Bytes()
	if err != nil {
		return nil, nil, err
	}
	if common.GetJsonType(raw) != "object" {
		return common.NewReplayableBodyReader(storage), nil, nil
	}

	cleaned, err := sanitizeHTTPResponsesBody(raw)
	if err != nil {
		return nil, nil, err
	}
	if string(cleaned) == string(raw) {
		return common.NewReplayableBodyReader(storage), nil, nil
	}
	cleanedBody, closer, err := relaycommon.NewOutboundJSONBody(cleaned)
	if err != nil {
		return nil, nil, err
	}
	return cleanedBody, closer, nil
}

func sanitizeHTTPResponsesBody(raw []byte) ([]byte, error) {
	if common.GetJsonType(raw) != "object" {
		return raw, nil
	}

	var body map[string]json.RawMessage
	if err := common.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("unmarshal Responses request body: %w", err)
	}
	changed := false
	if _, exists := body["previous_response_id"]; exists {
		delete(body, "previous_response_id")
		changed = true
	}
	if input, exists := body["input"]; exists {
		normalized, err := normalizeHTTPResponsesInput(input)
		if err != nil {
			return nil, err
		}
		if string(normalized) != string(input) {
			body["input"] = normalized
			changed = true
		}
	}
	if !changed {
		return raw, nil
	}
	cleaned, err := common.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal Responses request body: %w", err)
	}
	return cleaned, nil
}

func normalizeHTTPResponsesInput(input json.RawMessage) (json.RawMessage, error) {
	if common.GetJsonType(input) != "array" {
		return input, nil
	}

	var items []json.RawMessage
	if err := common.Unmarshal(input, &items); err != nil {
		return nil, fmt.Errorf("unmarshal Responses input: %w", err)
	}
	changed := false
	for index, rawItem := range items {
		if common.GetJsonType(rawItem) != "object" {
			continue
		}
		var item map[string]any
		if err := common.Unmarshal(rawItem, &item); err != nil {
			return nil, fmt.Errorf("unmarshal Responses input item %d: %w", index, err)
		}
		itemType := strings.ToLower(strings.TrimSpace(fmt.Sprint(item["type"])))
		callID, hasCallID := item["call_id"]
		if !strings.HasSuffix(itemType, "_call_output") {
			continue
		}
		if hasCallID && callID != nil && strings.TrimSpace(fmt.Sprint(callID)) != "" {
			continue
		}
		output := item["output"]
		var outputText string
		if output == nil {
			outputText = ""
		} else if text, ok := output.(string); ok {
			outputText = text
		} else if encoded, err := common.Marshal(output); err == nil {
			outputText = string(encoded)
		} else {
			outputText = fmt.Sprint(output)
		}
		normalizedItem, err := common.Marshal(map[string]any{
			"role":    "user",
			"content": fmt.Sprintf("[tool_output_missing_call_id] %s", outputText),
		})
		if err != nil {
			return nil, fmt.Errorf("marshal normalized Responses input item %d: %w", index, err)
		}
		items[index] = normalizedItem
		changed = true
	}
	if !changed {
		return input, nil
	}
	normalized, err := common.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("marshal normalized Responses input: %w", err)
	}
	return normalized, nil
}
