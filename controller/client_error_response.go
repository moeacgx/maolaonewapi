package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// clientOpenAIError serializes a final client view without changing the internal error.
func clientOpenAIError(apiErr *types.NewAPIError, requestID string) (types.OpenAIError, int) {
	if apiErr == nil {
		return types.OpenAIError{}, 0
	}
	result := apiErr.ToOpenAIError()
	message, statusCode, _ := common.ReplaceClientErrorCandidates(apiErr.StatusCode, apiErr.Error(), result.Message)
	result.Message = common.MessageWithRequestId(message, requestID)
	return result, statusCode
}

func clientClaudeError(apiErr *types.NewAPIError, requestID string) (types.ClaudeError, int) {
	if apiErr == nil {
		return types.ClaudeError{}, 0
	}
	result := apiErr.ToClaudeError()
	message, statusCode, _ := common.ReplaceClientErrorCandidates(apiErr.StatusCode, apiErr.Error(), result.Message)
	result.Message = common.MessageWithRequestId(message, requestID)
	return result, statusCode
}
