package openai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

// writeResponsesImageToolBridgeResponse converts a successful Images API
// response into the Responses image_generation_call shape that Codex expects.
// The image request was already billed as its actual target model by
// ImageHelper; this function only owns the downstream protocol translation.
func writeResponsesImageToolBridgeResponse(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	responseBody []byte,
	usage *dto.Usage,
) *types.NewAPIError {
	if info == nil || info.ResponsesImageToolBridge == nil {
		return types.NewOpenAIError(fmt.Errorf("responses image tool bridge metadata is unavailable"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	var imageResponse dto.ImageResponse
	if err := common.Unmarshal(responseBody, &imageResponse); err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if len(imageResponse.Data) == 0 {
		return types.NewOpenAIError(fmt.Errorf("images API returned no images"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}

	responseID := fmt.Sprintf("resp_%s", info.RequestId)
	createdAt := imageResponse.Created
	if createdAt == 0 {
		createdAt = time.Now().Unix()
	}
	output := make([]dto.ResponsesOutput, 0, len(imageResponse.Data))
	for index, image := range imageResponse.Data {
		if image.B64Json == "" {
			return types.NewOpenAIError(
				fmt.Errorf("images API response item %d is missing b64_json", index),
				types.ErrorCodeBadResponseBody,
				http.StatusBadGateway,
			)
		}
		callID := fmt.Sprintf("ig_%s_%d", info.RequestId, index)
		output = append(output, dto.ResponsesOutput{
			Type:   dto.ResponsesOutputTypeImageGenerationCall,
			ID:     callID,
			Status: "completed",
			Result: image.B64Json,
		})
	}

	response := &dto.OpenAIResponsesResponse{
		ID:        responseID,
		Object:    "response",
		CreatedAt: int(createdAt),
		Status:    json.RawMessage(`"completed"`),
		Model:     info.ResponsesImageToolBridge.SourceModel,
		Output:    output,
		Usage:     usage,
	}

	if info.ResponsesImageToolBridge.DownstreamStream {
		if bridgeErr := writeResponsesImageToolBridgeStream(c, info, response); bridgeErr != nil {
			return bridgeErr
		}
	} else {
		info.SetFirstResponseTime()
		c.JSON(http.StatusOK, response)
	}
	return nil
}

func writeResponsesImageToolBridgeStream(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	completed *dto.OpenAIResponsesResponse,
) *types.NewAPIError {
	if completed == nil {
		return types.NewOpenAIError(fmt.Errorf("responses image tool bridge response is unavailable"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	helper.SetEventStreamHeaders(c)
	created := *completed
	created.Status = json.RawMessage(`"in_progress"`)
	created.Output = nil
	created.Usage = nil
	if err := writeResponsesImageToolBridgeEvent(c, dto.ResponsesStreamResponse{
		Type:     "response.created",
		Response: &created,
	}); err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	for index := range completed.Output {
		outputIndex := index
		inProgress := completed.Output[index]
		inProgress.Status = "in_progress"
		inProgress.Result = ""
		if err := writeResponsesImageToolBridgeEvent(c, dto.ResponsesStreamResponse{
			Type:        dto.ResponsesOutputTypeItemAdded,
			OutputIndex: &outputIndex,
			Item:        &inProgress,
		}); err != nil {
			return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		item := completed.Output[index]
		if err := writeResponsesImageToolBridgeEvent(c, dto.ResponsesStreamResponse{
			Type:        dto.ResponsesOutputTypeItemDone,
			OutputIndex: &outputIndex,
			Item:        &item,
		}); err != nil {
			return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
	}

	if err := writeResponsesImageToolBridgeEvent(c, dto.ResponsesStreamResponse{
		Type:     "response.completed",
		Response: completed,
	}); err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if err := helper.StringData(c, "[DONE]"); err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	info.SetFirstResponseTime()
	return nil
}

func writeResponsesImageToolBridgeEvent(c *gin.Context, event dto.ResponsesStreamResponse) error {
	payload, err := common.Marshal(event)
	if err != nil {
		return err
	}
	return helper.ResponseChunkData(c, event, string(payload))
}
