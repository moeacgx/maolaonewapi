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

const UpstreamCapacityClientMessage = "已触发 OpenAI 官方限流，请重试"

func IsUpstreamCapacityError(err *NewAPIError) bool {
	if err == nil || !err.upstreamCapacityClassification {
		return false
	}
	code := strings.ToLower(strings.TrimSpace(string(err.GetErrorCode())))
	if _, ok := upstreamCapacityErrorCodes[code]; ok {
		return true
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	for _, marker := range []string{
		"selected model is at capacity",
		"model is at capacity",
		"model capacity is exhausted",
		"model capacity has been exhausted",
		"account pool capacity exhausted",
		"rate limit exceeded",
		"rate limited",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
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
	err.clientMessage = UpstreamCapacityClientMessage
}
