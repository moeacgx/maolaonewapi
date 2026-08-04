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
