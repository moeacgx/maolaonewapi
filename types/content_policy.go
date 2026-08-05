package types

import (
	"fmt"
	"strings"
)

const (
	ErrorCodeCyberPolicy        ErrorCode = "cyber_policy"
	ErrorCodePromptGuardBlocked ErrorCode = "prompt_guard_blocked"
)

// IsContentPolicyErrorCode 仅识别稳定的内容策略错误码，避免把普通 4xx
// 请求错误一并排除在连接质量统计之外。
func IsContentPolicyErrorCode(code ErrorCode) bool {
	switch ErrorCode(strings.ToLower(strings.TrimSpace(string(code)))) {
	case ErrorCodeSensitiveWordsDetected, ErrorCodePromptBlocked, ErrorCodePromptGuardBlocked, ErrorCodeCyberPolicy:
		return true
	default:
		return false
	}
}

// IsContentPolicyRejection 同时检查网关错误码和结构化上游错误码。
// 部分适配器会把上游 cyber_policy 保存在 RelayError.Code 中，而外层错误码
// 仍是通用的 bad_response_status_code。
func IsContentPolicyRejection(err *NewAPIError) bool {
	if err == nil {
		return false
	}
	if IsContentPolicyErrorCode(err.GetErrorCode()) {
		return true
	}
	switch relayErr := err.RelayError.(type) {
	case OpenAIError:
		return IsContentPolicyErrorCode(ErrorCode(fmt.Sprint(relayErr.Code)))
	case *OpenAIError:
		return relayErr != nil && IsContentPolicyErrorCode(ErrorCode(fmt.Sprint(relayErr.Code)))
	case ClaudeError:
		return IsContentPolicyErrorCode(ErrorCode(relayErr.Type))
	case *ClaudeError:
		return relayErr != nil && IsContentPolicyErrorCode(ErrorCode(relayErr.Type))
	default:
		return false
	}
}
