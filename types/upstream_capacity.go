package types

import (
	"net/http"
	"strings"
)

var upstreamCapacityErrorCodes = map[string]struct{}{
	"account_pool_capacity_exhausted": {},
	"capacity_exhausted":              {},
	"model_at_capacity":               {},
	"model_capacity_exhausted":        {},
	"model_overloaded":                {},
	"upstream_capacity_exhausted":     {},
}

// UpstreamCapacityClientMessage 是容量错误对外统一展示的描述。
const UpstreamCapacityClientMessage = "已触发OpenAI官方限流"

func IsUpstreamCapacityError(err *NewAPIError) bool {
	if err == nil {
		return false
	}
	code := strings.ToLower(strings.TrimSpace(string(err.GetErrorCode())))
	if _, ok := upstreamCapacityErrorCodes[code]; ok {
		return true
	}
	if !isUpstreamErrorType(err.GetErrorType()) {
		return false
	}

	message := strings.ToLower(strings.TrimSpace(err.Error()))
	for _, marker := range []string{
		"selected model is at capacity",
		"model is at capacity",
		"model capacity is exhausted",
		"model capacity has been exhausted",
		"account pool capacity exhausted",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func isUpstreamErrorType(errorType ErrorType) bool {
	switch errorType {
	case ErrorTypeOpenAIError, ErrorTypeClaudeError, ErrorTypeMidjourneyError,
		ErrorTypeGeminiError, ErrorTypeRerankError, ErrorTypeUpstreamError:
		return true
	default:
		return false
	}
}

func normalizeUpstreamCapacityStatus(err *NewAPIError) {
	if err == nil || !IsUpstreamCapacityError(err) {
		return
	}
	if err.StatusCode != http.StatusTooManyRequests {
		if err.OriginalStatusCode == 0 {
			err.OriginalStatusCode = err.StatusCode
		}
		err.StatusCode = http.StatusTooManyRequests
	}
	err.SetClientMessage(UpstreamCapacityClientMessage)
}
