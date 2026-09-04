package openai

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func responsesLocalUsageText(response *dto.OpenAIResponsesResponse) string {
	if response == nil {
		return ""
	}
	var text strings.Builder
	for i := range response.Output {
		output := &response.Output[i]
		switch output.Type {
		case "message":
			for _, content := range output.Content {
				if content.Type == "refusal" {
					if content.Refusal != "" {
						text.WriteString(content.Refusal)
					} else {
						text.WriteString(content.Text)
					}
					continue
				}
				if content.Text != "" {
					text.WriteString(content.Text)
				} else {
					text.WriteString(content.Refusal)
				}
			}
		case "reasoning":
			if len(output.Summary) > 0 {
				for _, summary := range output.Summary {
					text.WriteString(summary.Text)
				}
			} else {
				for _, content := range output.Content {
					text.WriteString(content.Text)
				}
			}
		case dto.BuildInCallFunctionCall:
			text.WriteString(output.Name)
			payload := output.ArgumentsString()
			if payload == "" {
				payload = output.InputString()
			}
			text.WriteString(payload)
		case "custom_tool_call", dto.BuildInCallToolUse:
			text.WriteString(output.Name)
			payload := output.InputString()
			if payload == "" {
				payload = output.ArgumentsString()
			}
			text.WriteString(payload)
		}
	}
	return text.String()
}

const (
	maxProvisionalResponsesStreamEvents = 16
	maxProvisionalResponsesStreamBytes  = 1 << 20
)

func responsesOutputIsMeaningful(response *dto.OpenAIResponsesResponse) bool {
	if response == nil {
		return false
	}
	if strings.TrimSpace(responsesLocalUsageText(response)) != "" {
		return true
	}
	for i := range response.Output {
		output := &response.Output[i]
		switch output.Type {
		case "message", "reasoning":
			continue
		case dto.ResponsesOutputTypeImageGenerationCall:
			if strings.TrimSpace(output.Result) != "" {
				return true
			}
		default:
			// 工具调用和未知输出项会原样发给 Responses 客户端。
			return true
		}
	}
	return false
}

func responsesUsageIsPresent(usage *dto.Usage) bool {
	return usage != nil && (usage.InputTokens != 0 || usage.OutputTokens != 0)
}

func responsesTextsReferToSameOutput(left string, right string) bool {
	if left == "" || right == "" {
		return false
	}
	return left == right || strings.HasPrefix(left, right) || strings.HasPrefix(right, left)
}

func mergeResponsesOutputText(current string, candidate string) string {
	if current == "" {
		return candidate
	}
	if candidate == "" {
		return current
	}
	if responsesTextsReferToSameOutput(current, candidate) {
		if len(candidate) > len(current) {
			return candidate
		}
		return current
	}
	return current + candidate
}

func responsesBillableToolCall(output *dto.ResponsesOutput) (string, string, string, bool) {
	if output == nil {
		return "", "", "", false
	}
	itemType := output.Type
	functionName := output.Name
	switch output.Type {
	case dto.BuildInCallWebSearchCall, dto.BuildInCallFileSearchCall, dto.BuildInCallFunctionCall:
	case "custom_tool_call", dto.BuildInCallToolUse:
		itemType = dto.BuildInCallToolUse
	default:
		return "", "", "", false
	}
	identity := output.ID
	if identity == "" {
		identity = output.CallId
	}
	if identity != "" {
		identity = "id:" + identity
	} else {
		identity = strings.Join([]string{
			itemType,
			functionName,
			output.ArgumentsString(),
			output.InputString(),
		}, "\x00")
	}
	return itemType, functionName, identity, true
}

func shouldBufferResponsesStreamEvent(event *dto.ResponsesStreamResponse) bool {
	if event == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(event.Type)) {
	case "response.created", "response.in_progress", "response.queued":
		return true
	case "response.output_item.added", dto.ResponsesOutputTypeItemDone:
		if event.Item == nil {
			return false
		}
		return !responsesOutputIsMeaningful(&dto.OpenAIResponsesResponse{Output: []dto.ResponsesOutput{*event.Item}})
	case "response.content_part.added", "response.content_part.done":
		return event.Part != nil &&
			strings.EqualFold(strings.TrimSpace(event.Part.Type), "output_text") &&
			strings.TrimSpace(event.Part.Text) == ""
	case "response.reasoning_summary_part.added", "response.reasoning_summary_part.done":
		return event.Part != nil &&
			strings.EqualFold(strings.TrimSpace(event.Part.Type), "summary_text") &&
			strings.TrimSpace(event.Part.Text) == ""
	case "response.output_text.delta", "response.refusal.delta",
		"response.reasoning_summary_text.delta", "response.reasoning_summary.delta", "response.reasoning_text.delta",
		"response.function_call_arguments.delta", "response.custom_tool_call_input.delta":
		return event.Delta == ""
	case "response.output_text.done", "response.reasoning_summary_text.done", "response.reasoning_summary.done", "response.reasoning_text.done":
		return event.Text == ""
	case "response.refusal.done":
		return event.Refusal == ""
	case "response.function_call_arguments.done":
		return event.ArgumentsString() == ""
	case "response.custom_tool_call_input.done":
		return event.InputString() == ""
	case "response.completed", "response.done", "response.failed", "response.incomplete",
		"response.cancelled", "response.canceled", "response.error", "error":
		return event.Response == nil ||
			(!responsesUsageIsPresent(event.Response.Usage) && !responsesOutputIsMeaningful(event.Response))
	default:
		return false
	}
}

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
	info.SetUpstreamResponseModelName(responsesResponse.Model)
	if oaiError := responsesResponse.GetOpenAIError(); oaiError != nil {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
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
	if usage.PromptTokens == 0 || usage.CompletionTokens == 0 {
		if responseText := responsesLocalUsageText(&responsesResponse); strings.TrimSpace(responseText) != "" {
			modelName := info.GetUpstreamModelName()
			if modelName == "" {
				modelName = info.OriginModelName
			}
			localUsage := service.ResponseText2Usage(c, responseText, modelName, info.GetEstimatePromptTokens())
			if usage.PromptTokens == 0 {
				usage.PromptTokens = localUsage.PromptTokens
			}
			if usage.CompletionTokens == 0 {
				usage.CompletionTokens = localUsage.CompletionTokens
			}
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		}
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	// Count actual tool invocations from Output (not tool declarations).
	for i := range responsesResponse.Output {
		if itemType, functionName, _, ok := responsesBillableToolCall(&responsesResponse.Output[i]); ok {
			info.CountBillableToolCall(itemType, functionName)
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
	if usageError := service.TextUsageError(c, info, &usage); usageError != nil {
		return &usage, usageError
	}

	// 只有用量有效时才把响应写给客户端，否则外层 Relay 仍可切换渠道重试。
	service.IOCopyBytesGracefully(c, resp, responseBody)

	return &usage, nil
}

func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	var usage = &dto.Usage{}
	var responseTextBuilder strings.Builder
	responseTextByOutputIndex := make(map[int]*strings.Builder)
	responseTextByItemID := make(map[string]*strings.Builder)
	outputItemTextByIndex := make(map[int]string)
	outputItemTextByItemID := make(map[string]string)
	outputItemIDByIndex := make(map[int]string)
	outputDoneTextByIndex := make(map[int]*strings.Builder)
	outputDoneTextByItemID := make(map[string]*strings.Builder)
	unindexedItemOrder := make([]string, 0)
	seenUnindexedItemID := make(map[string]struct{})
	boundItemIDs := make(map[string]struct{})
	var unindexedOutputItemText strings.Builder
	var unindexedOutputDoneText strings.Builder
	terminalResponseText := ""
	lastOutputIndex := 0
	hasLastOutputIndex := false
	trackUnindexedItemID := func(itemID string) {
		if itemID == "" {
			return
		}
		if _, exists := seenUnindexedItemID[itemID]; exists {
			return
		}
		seenUnindexedItemID[itemID] = struct{}{}
		unindexedItemOrder = append(unindexedItemOrder, itemID)
	}
	bindItemIDToOutputIndex := func(itemID string, outputIndex int) {
		if itemID == "" {
			return
		}
		boundItemIDs[itemID] = struct{}{}
		if outputItemIDByIndex[outputIndex] == "" {
			outputItemIDByIndex[outputIndex] = itemID
		}
		if source := responseTextByItemID[itemID]; source != nil {
			target := responseTextByOutputIndex[outputIndex]
			if target == nil {
				target = &strings.Builder{}
				responseTextByOutputIndex[outputIndex] = target
			}
			merged := mergeResponsesOutputText(target.String(), source.String())
			target.Reset()
			target.WriteString(merged)
			delete(responseTextByItemID, itemID)
		}
		if source := outputDoneTextByItemID[itemID]; source != nil {
			target := outputDoneTextByIndex[outputIndex]
			if target == nil {
				target = &strings.Builder{}
				outputDoneTextByIndex[outputIndex] = target
			}
			merged := mergeResponsesOutputText(target.String(), source.String())
			target.Reset()
			target.WriteString(merged)
			delete(outputDoneTextByItemID, itemID)
		}
		if source := outputItemTextByItemID[itemID]; source != "" {
			outputItemTextByIndex[outputIndex] = mergeResponsesOutputText(outputItemTextByIndex[outputIndex], source)
			delete(outputItemTextByItemID, itemID)
		}
	}
	appendResponseText := func(text string, outputIndex *int, itemID string) {
		if text == "" {
			return
		}
		if outputIndex == nil {
			if itemID != "" {
				trackUnindexedItemID(itemID)
				builder := responseTextByItemID[itemID]
				if builder == nil {
					builder = &strings.Builder{}
					responseTextByItemID[itemID] = builder
				}
				builder.WriteString(text)
				return
			}
			responseTextBuilder.WriteString(text)
			return
		}
		builder := responseTextByOutputIndex[*outputIndex]
		if builder == nil {
			builder = &strings.Builder{}
			responseTextByOutputIndex[*outputIndex] = builder
		}
		builder.WriteString(text)
	}
	recordDoneText := func(text string, outputIndex *int, itemID string) {
		if text == "" {
			return
		}
		if outputIndex == nil {
			if itemID != "" {
				trackUnindexedItemID(itemID)
				builder := outputDoneTextByItemID[itemID]
				if builder == nil {
					builder = &strings.Builder{}
					outputDoneTextByItemID[itemID] = builder
				}
				merged := mergeResponsesOutputText(builder.String(), text)
				builder.Reset()
				builder.WriteString(merged)
				return
			}
			merged := mergeResponsesOutputText(unindexedOutputDoneText.String(), text)
			unindexedOutputDoneText.Reset()
			unindexedOutputDoneText.WriteString(merged)
			return
		}
		builder := outputDoneTextByIndex[*outputIndex]
		if builder == nil {
			builder = &strings.Builder{}
			outputDoneTextByIndex[*outputIndex] = builder
		}
		merged := mergeResponsesOutputText(builder.String(), text)
		builder.Reset()
		builder.WriteString(merged)
	}
	imageCounter := &relaycommon.ImageGenerationCallCounter{}
	imageCommitted := false
	var nextSequenceNumber int64
	countedToolCalls := make(map[string]int)
	countToolCall := func(output *dto.ResponsesOutput) {
		itemType, functionName, identity, ok := responsesBillableToolCall(output)
		if !ok {
			return
		}
		info.CountBillableToolCall(itemType, functionName)
		countedToolCalls[identity]++
	}
	countTerminalToolCalls := func(outputs []dto.ResponsesOutput) {
		terminalOccurrences := make(map[string]int)
		for i := range outputs {
			itemType, functionName, identity, ok := responsesBillableToolCall(&outputs[i])
			if !ok {
				continue
			}
			terminalOccurrences[identity]++
			if terminalOccurrences[identity] <= countedToolCalls[identity] {
				continue
			}
			info.CountBillableToolCall(itemType, functionName)
			countedToolCalls[identity]++
		}
	}
	var streamErr *types.NewAPIError
	provisionalEvents := make([]responsesStreamDataItem, 0, 2)
	provisionalBytes := 0
	holdingProvisionalEvents := true
	flushProvisionalEvents := func() error {
		if err := sendResponsesStreamDataBatch(c, provisionalEvents); err != nil {
			return err
		}
		provisionalEvents = nil
		provisionalBytes = 0
		return nil
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(streamErr)
			return
		}

		// 检查当前数据是否包含 completed 状态和 usage 信息
		var streamResponse dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			logger.LogError(c, "failed to unmarshal stream response: "+err.Error())
			sr.Error(err)
			return
		}
		if streamResponse.Response != nil {
			info.SetUpstreamResponseModelName(streamResponse.Response.Model)
		}
		if streamResponse.SequenceNumber != nil && *streamResponse.SequenceNumber >= nextSequenceNumber {
			nextSequenceNumber = *streamResponse.SequenceNumber + 1
		}
		resolvedOutputIndex := streamResponse.OutputIndex
		if resolvedOutputIndex != nil {
			lastOutputIndex = *resolvedOutputIndex
			hasLastOutputIndex = true
		} else if hasLastOutputIndex {
			existingID := outputItemIDByIndex[lastOutputIndex]
			if streamResponse.ItemID == "" || existingID == "" || streamResponse.ItemID == existingID {
				resolvedOutputIndex = &lastOutputIndex
			}
		}
		if resolvedOutputIndex != nil && streamResponse.ItemID != "" {
			bindItemIDToOutputIndex(streamResponse.ItemID, *resolvedOutputIndex)
		}
		if streamErr = responsesStreamAPIError(&streamResponse, resp.StatusCode); streamErr != nil {
			errorSequenceNumber := nextSequenceNumber
			if streamResponse.SequenceNumber != nil {
				errorSequenceNumber = *streamResponse.SequenceNumber
			}
			if !imageCommitted {
				imageCounter.Reset()
				imageCounter.Commit(info)
				imageCommitted = true
			}
			if c.Writer != nil && c.Writer.Written() {
				if holdingProvisionalEvents {
					holdingProvisionalEvents = false
					if err := flushProvisionalEvents(); err != nil {
						sr.Error(err)
					}
				}
				if err := sendCommittedResponsesStreamAPIError(c, streamErr, errorSequenceNumber); err != nil {
					sr.Error(err)
				}
			}
			sr.Stop(streamErr)
			return
		}
		switch streamResponse.Type {
		case "response.completed", "response.done", "response.incomplete",
			"response.cancelled", "response.canceled":
			if streamResponse.Response != nil {
				countTerminalToolCalls(streamResponse.Response.Output)
				if completedText := responsesLocalUsageText(streamResponse.Response); strings.TrimSpace(completedText) != "" {
					terminalResponseText = completedText
				}
				if streamResponse.Response.Usage != nil {
					usage.PromptTokens = streamResponse.Response.Usage.InputTokens
					usage.CompletionTokens = streamResponse.Response.Usage.OutputTokens
					usage.TotalTokens = streamResponse.Response.Usage.TotalTokens
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
		case "response.output_text.delta",
			"response.refusal.delta",
			"response.reasoning_summary_text.delta",
			"response.reasoning_summary.delta",
			"response.reasoning_text.delta",
			"response.function_call_arguments.delta",
			"response.custom_tool_call_input.delta":
			// 收集所有会出现在客户端的文本、推理与工具参数，用于缺失 usage 时本地估算。
			appendResponseText(streamResponse.Delta, resolvedOutputIndex, streamResponse.ItemID)
		case "response.output_text.done", "response.reasoning_summary_text.done", "response.reasoning_summary.done", "response.reasoning_text.done",
			"response.refusal.done", "response.function_call_arguments.done", "response.custom_tool_call_input.done":
			doneText := streamResponse.Text
			switch streamResponse.Type {
			case "response.refusal.done":
				doneText = streamResponse.Refusal
			case "response.function_call_arguments.done":
				doneText = streamResponse.ArgumentsString()
			case "response.custom_tool_call_input.done":
				doneText = streamResponse.InputString()
			}
			recordDoneText(doneText, resolvedOutputIndex, streamResponse.ItemID)
		case "response.content_part.added", "response.reasoning_summary_part.added":
			appendResponseText(streamResponse.PartText(), resolvedOutputIndex, streamResponse.ItemID)
		case "response.content_part.done", "response.reasoning_summary_part.done":
			recordDoneText(streamResponse.PartText(), resolvedOutputIndex, streamResponse.ItemID)
		case dto.ResponsesOutputTypeItemAdded, dto.ResponsesOutputTypeItemDone:
			if streamResponse.Item != nil {
				itemText := responsesLocalUsageText(&dto.OpenAIResponsesResponse{
					Output: []dto.ResponsesOutput{*streamResponse.Item},
				})
				itemOutputIndex := streamResponse.OutputIndex
				if itemOutputIndex == nil && hasLastOutputIndex {
					candidateIndex := lastOutputIndex
					existingText := outputItemTextByIndex[candidateIndex]
					if existingText == "" {
						if builder := outputDoneTextByIndex[candidateIndex]; builder != nil {
							existingText = builder.String()
						}
					}
					if existingText == "" {
						if builder := responseTextByOutputIndex[candidateIndex]; builder != nil {
							existingText = builder.String()
						}
					}
					existingID := outputItemIDByIndex[candidateIndex]
					currentID := streamResponse.Item.ID
					hasExistingOutput := existingText != "" || existingID != ""
					idsConflict := existingID != "" && currentID != "" && existingID != currentID
					sameItem := existingID != "" && currentID != "" && existingID == currentID
					if !idsConflict && responsesTextsReferToSameOutput(existingText, itemText) {
						sameItem = true
					}
					if !hasExistingOutput || sameItem {
						itemOutputIndex = &lastOutputIndex
					}
				}
				if itemOutputIndex != nil && streamResponse.Item.ID != "" {
					bindItemIDToOutputIndex(streamResponse.Item.ID, *itemOutputIndex)
				}
				if itemText != "" {
					if itemOutputIndex == nil {
						if streamResponse.Item.ID != "" {
							trackUnindexedItemID(streamResponse.Item.ID)
							outputItemTextByItemID[streamResponse.Item.ID] = itemText
						} else {
							unindexedOutputItemText.WriteString(itemText)
						}
					} else {
						outputItemTextByIndex[*itemOutputIndex] = itemText
						if streamResponse.Item.ID != "" {
							outputItemIDByIndex[*itemOutputIndex] = streamResponse.Item.ID
						}
					}
				}
				if streamResponse.Type == dto.ResponsesOutputTypeItemDone {
					countToolCall(streamResponse.Item)
					if streamResponse.Item.Type == dto.ResponsesOutputTypeImageGenerationCall && !imageCommitted {
						imageCounter.Observe(streamResponse.Item, itemOutputIndex)
					}
				}
			}
		}

		if holdingProvisionalEvents && shouldBufferResponsesStreamEvent(&streamResponse) {
			eventBytes := len(data)
			if len(provisionalEvents) < maxProvisionalResponsesStreamEvents &&
				eventBytes <= maxProvisionalResponsesStreamBytes &&
				provisionalBytes <= maxProvisionalResponsesStreamBytes-eventBytes {
				provisionalEvents = append(provisionalEvents, responsesStreamDataItem{
					response: streamResponse,
					data:     data,
				})
				provisionalBytes += eventBytes
				return
			}
		}

		holdingProvisionalEvents = false
		batch := make([]responsesStreamDataItem, 0, len(provisionalEvents)+1)
		batch = append(batch, provisionalEvents...)
		batch = append(batch, responsesStreamDataItem{response: streamResponse, data: data})
		provisionalEvents = nil
		provisionalBytes = 0
		if err := sendResponsesStreamDataBatch(c, batch); err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			sr.Stop(streamErr)
		}
	})
	if streamErr != nil {
		return nil, streamErr
	}

	if usage.CompletionTokens == 0 {
		// 计算输出文本的 token 数量
		tempStr := terminalResponseText
		if tempStr == "" {
			indices := make(map[int]struct{}, len(responseTextByOutputIndex)+len(outputItemTextByIndex)+len(outputDoneTextByIndex))
			for index := range responseTextByOutputIndex {
				indices[index] = struct{}{}
			}
			for index := range outputItemTextByIndex {
				indices[index] = struct{}{}
			}
			for index := range outputDoneTextByIndex {
				indices[index] = struct{}{}
			}
			orderedIndices := make([]int, 0, len(indices))
			for index := range indices {
				orderedIndices = append(orderedIndices, index)
			}
			sort.Ints(orderedIndices)

			var mergedText strings.Builder
			for _, index := range orderedIndices {
				if itemText := outputItemTextByIndex[index]; itemText != "" {
					mergedText.WriteString(itemText)
				} else if builder := outputDoneTextByIndex[index]; builder != nil && builder.Len() > 0 {
					mergedText.WriteString(builder.String())
				} else if builder := responseTextByOutputIndex[index]; builder != nil {
					mergedText.WriteString(builder.String())
				}
			}
			for _, itemID := range unindexedItemOrder {
				if _, bound := boundItemIDs[itemID]; bound {
					continue
				}
				if itemText := outputItemTextByItemID[itemID]; itemText != "" {
					mergedText.WriteString(itemText)
				} else if builder := outputDoneTextByItemID[itemID]; builder != nil && builder.Len() > 0 {
					mergedText.WriteString(builder.String())
				} else if builder := responseTextByItemID[itemID]; builder != nil {
					mergedText.WriteString(builder.String())
				}
			}
			unindexedText := responseTextBuilder.String()
			unindexedText = mergeResponsesOutputText(unindexedText, unindexedOutputDoneText.String())
			unindexedText = mergeResponsesOutputText(unindexedText, unindexedOutputItemText.String())
			mergedText.WriteString(unindexedText)
			tempStr = mergedText.String()
		}
		if len(tempStr) > 0 {
			// 非正常结束，使用输出文本的 token 数量
			common.SetContextKey(c, constant.ContextKeyLocalCountTokens, true)
			completionTokens := service.CountTextToken(tempStr, info.UpstreamModelName)
			usage.CompletionTokens = completionTokens
		}
	}

	if usage.PromptTokens == 0 && usage.CompletionTokens != 0 {
		common.SetContextKey(c, constant.ContextKeyLocalCountTokens, true)
		usage.PromptTokens = info.GetEstimatePromptTokens()
	}

	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens

	// 生命周期事件本身不是模型输出。若直到结束仍只收到这些事件，直接丢弃，
	// 让外层 empty_response 502 可以在响应未提交时安全切换渠道。
	if holdingProvisionalEvents {
		provisionalEvents = nil
	}

	return usage, nil
}
