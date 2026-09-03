package openai

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
)

func isResponsesStreamErrorType(eventType string) bool {
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "error", "response.error", "response.failed":
		return true
	default:
		return false
	}
}

func isProvisionalResponsesStreamEvent(event *dto.ResponsesStreamResponse) bool {
	if event == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(event.Type)) {
	case "response.created", "response.in_progress", "response.queued":
		return true
	case "response.output_item.added":
		if event.Item == nil {
			return false
		}
		itemType := strings.ToLower(strings.TrimSpace(event.Item.Type))
		return (itemType == "message" || itemType == "reasoning") && len(event.Item.Content) == 0
	case "response.content_part.added":
		return event.Part != nil &&
			strings.EqualFold(strings.TrimSpace(event.Part.Type), "output_text") &&
			strings.TrimSpace(event.Part.Text) == ""
	case "response.reasoning_summary_part.added":
		return event.Part != nil &&
			strings.EqualFold(strings.TrimSpace(event.Part.Type), "summary_text") &&
			strings.TrimSpace(event.Part.Text) == ""
	default:
		return false
	}
}

func responsesStreamAPIError(streamResp *dto.ResponsesStreamResponse, statusCode int) *types.NewAPIError {
	if streamResp == nil || !isResponsesStreamErrorType(streamResp.Type) {
		return nil
	}
	if statusCode < 100 || statusCode > 599 {
		statusCode = http.StatusInternalServerError
	}
	openAIError := streamResp.GetOpenAIError()
	var relayErr *types.NewAPIError
	if openAIError == nil {
		relayErr = types.NewOpenAIError(
			fmt.Errorf("responses stream error: %s", streamResp.Type),
			types.ErrorCodeBadResponse,
			statusCode,
		)
	} else {
		relayErr = types.WithOpenAIError(*openAIError, statusCode)
	}
	if relayErr.StatusCode >= http.StatusOK && relayErr.StatusCode < http.StatusMultipleChoices {
		relayErr.OriginalStatusCode = relayErr.StatusCode
		relayErr.StatusCode = http.StatusInternalServerError
	}
	return relayErr
}
