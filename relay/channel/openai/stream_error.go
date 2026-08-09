package openai

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
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
			return true
		}
		itemType := strings.ToLower(strings.TrimSpace(event.Item.Type))
		return (itemType == "message" || itemType == "reasoning") && len(event.Item.Content) == 0
	case "response.content_part.added", "response.reasoning_summary_part.added":
		return event.Part == nil || strings.TrimSpace(event.Part.Text) == ""
	default:
		return false
	}
}

func newOpenAIStreamAPIError(
	openAIError *types.OpenAIError,
	statusCode int,
	fallback string,
) *types.NewAPIError {
	if statusCode < 100 || statusCode > 599 {
		statusCode = http.StatusInternalServerError
	}
	var err *types.NewAPIError
	if openAIError == nil {
		err = types.NewOpenAIError(errors.New(fallback), types.ErrorCodeBadResponse, statusCode)
	} else {
		err = types.WithOpenAIError(*openAIError, statusCode)
	}
	if err.StatusCode >= http.StatusOK && err.StatusCode < http.StatusMultipleChoices {
		if err.OriginalStatusCode == 0 {
			err.OriginalStatusCode = err.StatusCode
		}
		err.StatusCode = http.StatusInternalServerError
	}
	return err
}

func responsesStreamAPIError(
	streamResp *dto.ResponsesStreamResponse,
	statusCode int,
) *types.NewAPIError {
	if streamResp == nil || !isResponsesStreamErrorType(streamResp.Type) {
		return nil
	}
	if openAIError := streamResp.GetOpenAIError(); openAIError != nil {
		return newOpenAIStreamAPIError(openAIError, statusCode, "responses stream error")
	}
	return newOpenAIStreamAPIError(nil, statusCode,
		fmt.Sprintf("responses stream error: %s", streamResp.Type))
}

func chatCompletionsStreamAPIError(data string, statusCode int) *types.NewAPIError {
	var streamResp dto.ChatCompletionsStreamResponse
	if err := common.UnmarshalJsonStr(data, &streamResp); err != nil {
		return nil
	}
	if openAIError := streamResp.GetOpenAIError(); openAIError != nil {
		return newOpenAIStreamAPIError(openAIError, statusCode, "chat completions stream error")
	}
	return nil
}
