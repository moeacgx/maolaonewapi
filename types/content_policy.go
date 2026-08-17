package types

import (
	"fmt"
	"strings"

	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
)

const (
	ErrorCodeCyberPolicy               relaytypes.ErrorCode = "cyber_policy"
	ErrorCodeCyberPolicySessionBlocked relaytypes.ErrorCode = "session_blocked_by_cyber_policy"
	ErrorCodePromptGuardBlocked        relaytypes.ErrorCode = "prompt_guard_blocked"
)

// IsContentPolicyErrorCode 仅识别稳定的内容策略错误码，避免把普通 4xx
// 请求错误一并排除在连接质量统计之外。
func IsContentPolicyErrorCode(code relaytypes.ErrorCode) bool {
	switch relaytypes.ErrorCode(strings.ToLower(strings.TrimSpace(string(code)))) {
	case relaytypes.ErrorCodeSensitiveWordsDetected, relaytypes.ErrorCodePromptBlocked, ErrorCodePromptGuardBlocked, ErrorCodeCyberPolicy, ErrorCodeCyberPolicySessionBlocked:
		return true
	default:
		return false
	}
}

// IsContentPolicyRejection 同时检查网关错误码和结构化上游错误码。
// 部分适配器会把上游 cyber_policy 保存在 RelayError.Code 中，而外层错误码
// 仍是通用的 bad_response_status_code。
func IsContentPolicyRejection(err *relaytypes.NewAPIError) bool {
	if err == nil {
		return false
	}
	if IsContentPolicyErrorCode(err.GetErrorCode()) {
		return true
	}
	switch relayErr := err.RelayError.(type) {
	case relaytypes.OpenAIError:
		return IsContentPolicyErrorCode(relaytypes.ErrorCode(fmt.Sprint(relayErr.Code)))
	case *relaytypes.OpenAIError:
		return relayErr != nil && IsContentPolicyErrorCode(relaytypes.ErrorCode(fmt.Sprint(relayErr.Code)))
	case relaytypes.ClaudeError:
		return IsContentPolicyErrorCode(relaytypes.ErrorCode(relayErr.Type))
	case *relaytypes.ClaudeError:
		return relayErr != nil && IsContentPolicyErrorCode(relaytypes.ErrorCode(relayErr.Type))
	default:
		return false
	}
}
