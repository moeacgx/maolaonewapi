package openai

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/sjson"
)

const responsesImageFunctionBridgeName = "newapi_image_generation"

func OaiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	// read response body
	var responsesResponse dto.OpenAIResponsesResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	err = common.Unmarshal(responseBody, &responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := responsesResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}
	if sourceModel := responsesImageToolBridgeSourceModel(info); sourceModel != "" {
		responsesResponse.Model = sourceModel
		responseBody, err = sjson.SetBytes(responseBody, "model", sourceModel)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
	}

	// compute usage
	usage := dto.Usage{}
	if responsesResponse.Usage != nil {
		usage.PromptTokens = responsesResponse.Usage.InputTokens
		usage.CompletionTokens = responsesResponse.Usage.OutputTokens
		usage.TotalTokens = responsesResponse.Usage.TotalTokens
		if responsesResponse.Usage.InputTokensDetails != nil {
			usage.PromptTokensDetails.CachedTokens = responsesResponse.Usage.InputTokensDetails.CachedTokens
		}
		usage.CopyCacheCreationTokensFrom(responsesResponse.Usage)
	}
	// Count actual tool invocations from Output (not tool declarations).
	for _, output := range responsesResponse.Output {
		switch output.Type {
		case dto.BuildInCallWebSearchCall:
			info.CountBillableToolCall(dto.BuildInCallWebSearchCall, "")
		case dto.BuildInCallFileSearchCall:
			info.CountBillableToolCall(dto.BuildInCallFileSearchCall, "")
		case dto.BuildInCallFunctionCall:
			if info.ResponsesImageFunctionBridge == nil ||
				!strings.EqualFold(strings.TrimSpace(output.Name), strings.TrimSpace(info.ResponsesImageFunctionBridge.FunctionName)) {
				info.CountBillableToolCall(dto.BuildInCallFunctionCall, output.Name)
			}
		}
	}

	imageCounter := &relaycommon.ImageGenerationCallCounter{}
	if !relaycommon.IsNonBillableResponsesStatus(responsesResponse.Status) {
		for i := range responsesResponse.Output {
			idx := i
			imageCounter.Observe(&responsesResponse.Output[i], &idx)
		}
	}
	imageCounter.Commit(info)

	if captureResponsesImageFunctionBridgeCall(info, responsesResponse.Output, &usage) {
		// The controller will issue the second Images API request and write its
		// wrapped Responses result. Do not leak the intermediary function_call
		// response to the downstream client.
		return &usage, nil
	}

	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)

	return &usage, nil
}

// captureResponsesImageFunctionBridgeCall records the first invocation of the
// relay-injected function. The bridge intentionally supports one image call
// per source response in its first version; the target Images request uses
// n=1, matching the function schema and its fixed per-call billing.
func captureResponsesImageFunctionBridgeCall(
	info *relaycommon.RelayInfo,
	output []dto.ResponsesOutput,
	usage *dto.Usage,
) bool {
	if info == nil || info.ResponsesImageFunctionBridge == nil {
		return false
	}
	bridge := info.ResponsesImageFunctionBridge
	for index := range output {
		item := &output[index]
		if item.Type != dto.BuildInCallFunctionCall ||
			!strings.EqualFold(strings.TrimSpace(item.Name), strings.TrimSpace(bridge.FunctionName)) {
			continue
		}
		bridge.Triggered = true
		bridge.Arguments = append(bridge.Arguments[:0], item.Arguments...)
		if usage == nil {
			bridge.SourceUsage = &dto.Usage{}
		} else {
			copied := *usage
			bridge.SourceUsage = &copied
		}
		return true
	}
	return false
}

func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	var usage = &dto.Usage{}
	var responseTextBuilder strings.Builder
	imageCounter := &relaycommon.ImageGenerationCallCounter{}
	imageCommitted := false
	imageFunctionBridge := info != nil && info.ResponsesImageFunctionBridge != nil
	var bufferedEvents []string
	var imageFunctionCapture responsesImageFunctionStreamCapture
	previousDisablePing := false
	if imageFunctionBridge {
		// A source event must never leak before the controller can replace the
		// response with the target Images result. Disable periodic pings while
		// the source is buffered; the target bridge emits its own SSE envelope.
		previousDisablePing = info.DisablePing
		info.DisablePing = true
		defer func() {
			info.DisablePing = previousDisablePing
		}()
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {

		// 检查当前数据是否包含 completed 状态和 usage 信息
		var streamResponse dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			logger.LogError(c, "failed to unmarshal stream response: "+err.Error())
			sr.Error(err)
			return
		}
		if sourceModel := responsesImageToolBridgeSourceModel(info); sourceModel != "" && streamResponse.Response != nil {
			streamResponse.Response.Model = sourceModel
			patched, patchErr := sjson.Set(data, "response.model", sourceModel)
			if patchErr != nil {
				sr.Error(patchErr)
				return
			}
			data = patched
		}
		if imageFunctionBridge {
			bufferedEvents = append(bufferedEvents, data)
		} else {
			sendResponsesStreamData(c, streamResponse, data)
		}
		imageFunctionCapture.observe(streamResponse)
		switch streamResponse.Type {
		case "response.completed", "response.done":
			if streamResponse.Response != nil {
				if streamResponse.Response.Usage != nil {
					if streamResponse.Response.Usage.InputTokens != 0 {
						usage.PromptTokens = streamResponse.Response.Usage.InputTokens
					}
					if streamResponse.Response.Usage.OutputTokens != 0 {
						usage.CompletionTokens = streamResponse.Response.Usage.OutputTokens
					}
					if streamResponse.Response.Usage.TotalTokens != 0 {
						usage.TotalTokens = streamResponse.Response.Usage.TotalTokens
					}
					if streamResponse.Response.Usage.InputTokensDetails != nil {
						usage.PromptTokensDetails.CachedTokens = streamResponse.Response.Usage.InputTokensDetails.CachedTokens
					}
					usage.CopyCacheCreationTokensFrom(streamResponse.Response.Usage)
				}
				if !imageCommitted {
					if relaycommon.IsNonBillableResponsesStatus(streamResponse.Response.Status) {
						imageCounter.Reset()
						imageCounter.Commit(info)
						imageCommitted = true
					} else {
						for i := range streamResponse.Response.Output {
							idx := i
							imageCounter.Observe(&streamResponse.Response.Output[i], &idx)
						}
						imageCounter.Commit(info)
						imageCommitted = true
					}
				}
			} else if !imageCommitted {
				imageCounter.Commit(info)
				imageCommitted = true
			}
		case "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
			if !imageCommitted {
				imageCounter.Reset()
				imageCounter.Commit(info)
				imageCommitted = true
			}
		case "response.output_text.delta":
			// 处理输出文本
			responseTextBuilder.WriteString(streamResponse.Delta)
		case dto.ResponsesOutputTypeItemDone:
			if streamResponse.Item != nil {
				switch streamResponse.Item.Type {
				case dto.BuildInCallWebSearchCall:
					info.CountBillableToolCall(dto.BuildInCallWebSearchCall, "")
				case dto.BuildInCallFileSearchCall:
					info.CountBillableToolCall(dto.BuildInCallFileSearchCall, "")
				case dto.BuildInCallFunctionCall:
					if info.ResponsesImageFunctionBridge == nil ||
						!strings.EqualFold(strings.TrimSpace(streamResponse.Item.Name), strings.TrimSpace(info.ResponsesImageFunctionBridge.FunctionName)) {
						info.CountBillableToolCall(dto.BuildInCallFunctionCall, streamResponse.Item.Name)
					}
				case dto.ResponsesOutputTypeImageGenerationCall:
					if !imageCommitted {
						imageCounter.Observe(streamResponse.Item, streamResponse.OutputIndex)
					}
				}
			}
		}
	})
	if imageFunctionBridge {
		info.DisablePing = previousDisablePing
		imageFunctionCapture.commit(info, usage)
		if !imageFunctionCapture.triggered {
			for _, data := range bufferedEvents {
				var streamResponse dto.ResponsesStreamResponse
				if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
					logger.LogError(c, "failed to replay buffered response stream: "+err.Error())
					continue
				}
				sendResponsesStreamData(c, streamResponse, data)
			}
		}
	}

	if usage.CompletionTokens == 0 {
		// 计算输出文本的 token 数量
		tempStr := responseTextBuilder.String()
		if len(tempStr) > 0 {
			// 非正常结束，使用输出文本的 token 数量
			completionTokens := service.CountTextToken(tempStr, info.UpstreamModelName)
			usage.CompletionTokens = completionTokens
		}
	}

	if usage.PromptTokens == 0 && usage.CompletionTokens != 0 {
		usage.PromptTokens = info.GetEstimatePromptTokens()
	}

	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens

	return usage, nil
}

// responsesImageFunctionStreamCapture collects the function call that is
// split across Responses SSE events. Full arguments from output_item.done (or
// response.completed) take precedence over delta fragments.
type responsesImageFunctionStreamCapture struct {
	seen      bool
	outputIdx *int
	itemID    string
	fullArgs  []byte
	deltaArgs strings.Builder
	triggered bool
}

func (capture *responsesImageFunctionStreamCapture) observe(event dto.ResponsesStreamResponse) {
	if capture == nil {
		return
	}
	if event.Item != nil && event.Item.Type == dto.BuildInCallFunctionCall &&
		strings.EqualFold(strings.TrimSpace(event.Item.Name), responsesImageFunctionBridgeName) {
		capture.seen = true
		capture.itemID = strings.TrimSpace(event.Item.ID)
		if event.OutputIndex != nil {
			value := *event.OutputIndex
			capture.outputIdx = &value
		}
		capture.setFullArguments(event.Item.Arguments)
	}

	switch event.Type {
	case "response.function_call_arguments.delta":
		if capture.matches(event) {
			capture.deltaArgs.WriteString(event.Delta)
		}
	case "response.function_call_arguments.done":
		if capture.matches(event) {
			capture.setFullArguments(event.Arguments)
		}
	case "response.completed", "response.done":
		if event.Response == nil {
			return
		}
		for index := range event.Response.Output {
			item := &event.Response.Output[index]
			if item.Type == dto.BuildInCallFunctionCall &&
				strings.EqualFold(strings.TrimSpace(item.Name), responsesImageFunctionBridgeName) {
				capture.seen = true
				capture.setFullArguments(item.Arguments)
			}
		}
	}
}

func (capture *responsesImageFunctionStreamCapture) matches(event dto.ResponsesStreamResponse) bool {
	if capture == nil || !capture.seen {
		return false
	}
	if event.ItemID != "" && capture.itemID != "" {
		return event.ItemID == capture.itemID
	}
	if event.OutputIndex != nil && capture.outputIdx != nil {
		return *event.OutputIndex == *capture.outputIdx
	}
	return true
}

func (capture *responsesImageFunctionStreamCapture) setFullArguments(arguments []byte) {
	trimmed := strings.TrimSpace(string(arguments))
	if trimmed == "" || trimmed == "null" || trimmed == `""` {
		return
	}
	capture.fullArgs = append(capture.fullArgs[:0], arguments...)
}

func (capture *responsesImageFunctionStreamCapture) commit(info *relaycommon.RelayInfo, usage *dto.Usage) {
	if capture == nil || info == nil || info.ResponsesImageFunctionBridge == nil || !capture.seen {
		return
	}
	arguments := capture.fullArgs
	if len(arguments) == 0 && capture.deltaArgs.Len() > 0 {
		arguments = []byte(capture.deltaArgs.String())
	}
	if len(strings.TrimSpace(string(arguments))) == 0 {
		return
	}
	bridge := info.ResponsesImageFunctionBridge
	bridge.Triggered = true
	bridge.Arguments = append(bridge.Arguments[:0], arguments...)
	if usage == nil {
		bridge.SourceUsage = &dto.Usage{}
	} else {
		copied := *usage
		bridge.SourceUsage = &copied
	}
	capture.triggered = true
}

// responsesImageToolBridgeSourceModel keeps bridge implementation details out
// of the client-visible Responses model field.
func responsesImageToolBridgeSourceModel(info *relaycommon.RelayInfo) string {
	if info == nil || info.ResponsesImageToolBridge == nil {
		return ""
	}
	return strings.TrimSpace(info.ResponsesImageToolBridge.SourceModel)
}
