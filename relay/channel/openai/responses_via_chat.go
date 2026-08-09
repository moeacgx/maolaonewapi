package openai

import (
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/openaicompat"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func shouldUseChatCompletionsForResponses(info *relaycommon.RelayInfo) bool {
	return info != nil && info.ChannelMeta != nil && info.ChannelOtherSettings.ResponsesToChatEnabled
}

func OaiChatToResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	var chatResp dto.OpenAITextResponse
	if err := common.Unmarshal(body, &chatResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := chatResp.GetOpenAIError(); oaiError != nil {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	responseID := helper.GetResponseID(c)
	responsesResp, usage, err := service.ChatCompletionsResponseToResponsesResponse(&chatResp, responseID)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if usage == nil || usage.TotalTokens == 0 {
		text := service.ExtractOutputTextFromResponses(responsesResp)
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		responsesResp.Usage = openaicompat.UsageFromChatUsage(usage)
	}

	responseBody, err := common.Marshal(responsesResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}
	service.IOCopyBytesGracefully(c, resp, responseBody)
	return usage, nil
}

func OaiChatToResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	state := openaicompat.NewChatToResponsesStreamState(helper.GetResponseID(c), info.UpstreamModelName)
	var streamErr *types.NewAPIError
	provisionalEvents := make([]responsesStreamDataItem, 0, 2)
	provisionalBytes := 0
	holdingProvisionalEvents := true
	convertEvents := func(events []openaicompat.ChatToResponsesStreamEvent) ([]responsesStreamDataItem, error) {
		items := make([]responsesStreamDataItem, 0, len(events))
		for _, event := range events {
			data, err := common.Marshal(event.Payload)
			if err != nil {
				return nil, err
			}
			items = append(items, responsesStreamDataItem{
				response: event.Payload,
				data:     string(data),
			})
		}
		return items, nil
	}
	sendEvents := func(events []responsesStreamDataItem) bool {
		if err := sendResponsesStreamDataBatch(c, events); err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			return false
		}
		return true
	}
	flushProvisionalEvents := func() bool {
		if !sendEvents(provisionalEvents) {
			return false
		}
		provisionalEvents = nil
		provisionalBytes = 0
		return true
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, result *helper.StreamResult) {
		if streamErr != nil {
			result.Stop(streamErr)
			return
		}
		if upstreamErr := chatCompletionsStreamAPIError(data, resp.StatusCode); upstreamErr != nil {
			if c.Writer != nil && c.Writer.Written() {
				if holdingProvisionalEvents {
					holdingProvisionalEvents = false
					if !flushProvisionalEvents() {
						result.Stop(streamErr)
						return
					}
				}
				if err := sendCommittedResponsesStreamAPIError(c, upstreamErr); err != nil {
					result.Error(err)
				}
			}
			streamErr = upstreamErr
			result.Stop(streamErr)
			return
		}

		var chunk dto.ChatCompletionsStreamResponse
		if err := common.UnmarshalJsonStr(data, &chunk); err != nil {
			logger.LogError(c, "failed to unmarshal chat stream response: "+err.Error())
			result.Error(err)
			return
		}
		events, err := openaicompat.ChatCompletionsStreamChunkToResponsesEvents(&chunk, state)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			result.Stop(streamErr)
			return
		}
		items, err := convertEvents(events)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			result.Stop(streamErr)
			return
		}
		if len(items) == 0 {
			return
		}
		if holdingProvisionalEvents {
			allProvisional := true
			eventBytes := 0
			for _, item := range items {
				eventBytes += len(item.data)
				if !isProvisionalResponsesStreamEvent(&item.response) {
					allProvisional = false
				}
			}
			if allProvisional && len(provisionalEvents)+len(items) <= maxProvisionalResponsesStreamEvents &&
				provisionalBytes <= maxProvisionalResponsesStreamBytes-eventBytes {
				provisionalEvents = append(provisionalEvents, items...)
				provisionalBytes += eventBytes
				return
			}
		}
		holdingProvisionalEvents = false
		batch := make([]responsesStreamDataItem, 0, len(provisionalEvents)+len(items))
		batch = append(batch, provisionalEvents...)
		batch = append(batch, items...)
		provisionalEvents = nil
		provisionalBytes = 0
		if !sendEvents(batch) {
			result.Stop(streamErr)
		}
	})
	if streamErr != nil {
		return nil, streamErr
	}

	usage := state.GetUsage()
	if usage == nil || usage.TotalTokens == 0 {
		usage = service.ResponseText2Usage(c, state.UsageText(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		state.SetUsage(usage)
	}
	if !flushProvisionalEvents() {
		return nil, streamErr
	}
	finalEvents, err := convertEvents(openaicompat.FinalizeChatCompletionsStreamToResponses(state))
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}
	if !sendEvents(finalEvents) {
		return nil, streamErr
	}
	return usage, nil
}
