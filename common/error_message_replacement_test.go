package common

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrorMessageReplacementRulesUseFirstMatch(t *testing.T) {
	require.NoError(t, UpdateErrorMessageReplacementRules(`[
		{"match":"Insufficient balance","mode":"contains","replace":"渠道余额不足，请稍后重试"},
		{"match":"^status_code=403","mode":"regex","replace":"正则规则不应覆盖前一条"}
	]`))
	t.Cleanup(func() { require.NoError(t, UpdateErrorMessageReplacementRules(`[]`)) })

	replaced, statusCode, matched := ReplaceClientErrorCandidates(403, "status_code=403, Insufficient balance")
	require.True(t, matched)
	require.Equal(t, "渠道余额不足，请稍后重试", replaced)
	require.Equal(t, 403, statusCode)
}

func TestErrorMessageReplacementRulesValidateModesAndRegex(t *testing.T) {
	require.Error(t, ValidateErrorMessageReplacementRules(`[{"match":"x","mode":"unknown","replace":"y"}]`))
	require.Error(t, ValidateErrorMessageReplacementRules(`[{"match":"[","mode":"regex","replace":"y"}]`))
	require.Error(t, ValidateErrorMessageReplacementRules(`[{"matches":["valid","["],"mode":"regex","replace":"y"}]`))
	require.Error(t, ValidateErrorMessageReplacementRules(`[{"matches":[],"mode":"exact","replace":"y"}]`))
	require.Error(t, ValidateErrorMessageReplacementRules(`[{"matches":["x"," "],"mode":"exact","replace":"y"}]`))
	require.Error(t, ValidateErrorMessageReplacementRules(`[{"match":"x","matches":["y"],"mode":"exact","replace":"z"}]`))
	require.Error(t, ValidateErrorMessageReplacementRules(`[{"match":"x","mode":"exact","replace":""}]`))
	require.Error(t, ValidateErrorMessageReplacementRules(`[{"match":"x","mode":"exact","status_code":99,"replace":"y"}]`))
	require.Error(t, ValidateErrorMessageReplacementRules(`[{"match":"x","mode":"exact","replace":"y","replace_status_code":399}]`))
	require.Error(t, ValidateErrorMessageReplacementRules(`[{"match":"x","mode":"exact","replace":"y","replace_status_code":600}]`))
	require.NoError(t, ValidateErrorMessageReplacementRules(`[{"match":"x","mode":"exact","status_code":100,"replace":"y","replace_status_code":400}]`))
}

func TestErrorMessageReplacementRuleMatchesAnyConfiguredValue(t *testing.T) {
	require.NoError(t, UpdateErrorMessageReplacementRules(`[
		{"match":"Insufficient balance","matches":["Insufficient balance","account balance is insufficient"],"mode":"contains","status_code":403,"replace":"请求过多，请稍后重试","replace_status_code":429}
	]`))
	t.Cleanup(func() { require.NoError(t, UpdateErrorMessageReplacementRules(`[]`)) })

	message, statusCode, matched := ReplaceClientErrorCandidates(403, "upstream: account balance is insufficient")
	require.True(t, matched)
	require.Equal(t, "请求过多，请稍后重试", message)
	require.Equal(t, 429, statusCode)
}

func TestErrorMessageReplacementContainsReplacesWholeMessage(t *testing.T) {
	require.NoError(t, UpdateErrorMessageReplacementRules(`[{"match":"balance","mode":"contains","replace":"client message"}]`))
	t.Cleanup(func() { require.NoError(t, UpdateErrorMessageReplacementRules(`[]`)) })

	message, statusCode, matched := ReplaceClientErrorCandidates(502, "provider: balance is insufficient")
	require.True(t, matched)
	require.Equal(t, "client message", message)
	require.Equal(t, 502, statusCode)
}

func TestErrorMessageReplacementExactReplacesAllLiteralOccurrences(t *testing.T) {
	require.NoError(t, UpdateErrorMessageReplacementRules(`[{"match":"balance","mode":"exact","replace":"额度 $1"}]`))
	t.Cleanup(func() { require.NoError(t, UpdateErrorMessageReplacementRules(`[]`)) })

	message, statusCode, matched := ReplaceClientErrorCandidates(502, "provider: BALANCE is insufficient; balance is required")
	require.True(t, matched)
	require.Equal(t, "provider: 额度 $1 is insufficient; 额度 $1 is required", message)
	require.Equal(t, 502, statusCode)
}

func TestErrorMessageReplacementRegexReplacesAllMatchesWithCaptureGroups(t *testing.T) {
	require.NoError(t, UpdateErrorMessageReplacementRules(`[{"match":"balance=([0-9]+)","mode":"regex","replace":"额度=$1"}]`))
	t.Cleanup(func() { require.NoError(t, UpdateErrorMessageReplacementRules(`[]`)) })

	message, statusCode, matched := ReplaceClientErrorCandidates(502, "balance=12; balance=34")
	require.True(t, matched)
	require.Equal(t, "额度=12; 额度=34", message)
	require.Equal(t, 502, statusCode)
}

func TestErrorMessageReplacementExactAndRegexDoNotMatchWithoutTheirTargetText(t *testing.T) {
	t.Run("exact", func(t *testing.T) {
		require.NoError(t, UpdateErrorMessageReplacementRules(`[{"match":"balance","mode":"exact","replace":"额度"}]`))
		t.Cleanup(func() { require.NoError(t, UpdateErrorMessageReplacementRules(`[]`)) })
		message, _, matched := ReplaceClientErrorCandidates(502, "provider: quota is insufficient")
		require.False(t, matched)
		require.Equal(t, "provider: quota is insufficient", message)
	})

	t.Run("regex", func(t *testing.T) {
		require.NoError(t, UpdateErrorMessageReplacementRules(`[{"match":"balance=([0-9]+)","mode":"regex","replace":"额度=$1"}]`))
		t.Cleanup(func() { require.NoError(t, UpdateErrorMessageReplacementRules(`[]`)) })
		message, _, matched := ReplaceClientErrorCandidates(502, "quota is insufficient")
		require.False(t, matched)
		require.Equal(t, "quota is insufficient", message)
	})
}

func TestErrorMessageReplacementRuleMatchesSecondaryExactAndRegexValues(t *testing.T) {
	testCases := []struct{ name, mode, matches, message, want string }{
		{"exact", ErrorMessageReplacementModeExact, `["first exact value","second exact value"]`, "prefix second exact value suffix", "prefix matched suffix"},
		{"regex", ErrorMessageReplacementModeRegex, `["^first-[0-9]+$","second-[0-9]+"]`, "prefix second-42 suffix", "prefix matched suffix"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			rules := fmt.Sprintf(`[{"matches":%s,"mode":%q,"replace":"matched"}]`, testCase.matches, testCase.mode)
			require.NoError(t, UpdateErrorMessageReplacementRules(rules))
			t.Cleanup(func() { require.NoError(t, UpdateErrorMessageReplacementRules(`[]`)) })
			message, statusCode, matched := ReplaceClientErrorCandidates(502, testCase.message)
			require.True(t, matched)
			require.Equal(t, testCase.want, message)
			require.Equal(t, 502, statusCode)
		})
	}
}

func TestErrorMessageReplacementRuleLimitsMatchValues(t *testing.T) {
	matches := make([]string, maxErrorMessageMatchesPerRule)
	for index := range matches {
		matches[index] = fmt.Sprintf("match-%d", index)
	}
	rules := []ErrorMessageReplacementRule{{Match: matches[0], Matches: matches, Mode: ErrorMessageReplacementModeContains, Replace: "replacement"}}
	encoded, err := Marshal(rules)
	require.NoError(t, err)
	require.NoError(t, ValidateErrorMessageReplacementRules(string(encoded)))
	rules[0].Matches = append(rules[0].Matches, "too-many")
	encoded, err = Marshal(rules)
	require.NoError(t, err)
	require.Error(t, ValidateErrorMessageReplacementRules(string(encoded)))
}

func TestErrorMessageReplacementRuleMatchesStatusAndMessage(t *testing.T) {
	require.NoError(t, UpdateErrorMessageReplacementRules(`[{"match":"Insufficient balance","mode":"contains","status_code":403,"replace":"请求过多，请稍后重试","replace_status_code":429}]`))
	t.Cleanup(func() { require.NoError(t, UpdateErrorMessageReplacementRules(`[]`)) })
	message, statusCode, matched := ReplaceClientErrorCandidates(403, "upstream: Insufficient balance")
	require.True(t, matched)
	require.Equal(t, "请求过多，请稍后重试", message)
	require.Equal(t, 429, statusCode)
	message, statusCode, matched = ReplaceClientErrorCandidates(400, "upstream: Insufficient balance")
	require.False(t, matched)
	require.Equal(t, "upstream: Insufficient balance", message)
	require.Equal(t, 400, statusCode)
}

func TestErrorMessageReplacementRuleWithoutStatusKeepsOriginalStatus(t *testing.T) {
	require.NoError(t, UpdateErrorMessageReplacementRules(`[{"match":"upstream error","mode":"exact","replace":"client error"}]`))
	t.Cleanup(func() { require.NoError(t, UpdateErrorMessageReplacementRules(`[]`)) })
	message, statusCode, matched := ReplaceClientErrorCandidates(502, "upstream error")
	require.True(t, matched)
	require.Equal(t, "client error", message)
	require.Equal(t, 502, statusCode)
}

func TestErrorMessageReplacementCandidateKeepsRuleOrder(t *testing.T) {
	require.NoError(t, UpdateErrorMessageReplacementRules(`[{"match":"stable client message","mode":"exact","replace":"first rule"},{"match":"raw upstream message","mode":"exact","replace":"second rule"}]`))
	t.Cleanup(func() { require.NoError(t, UpdateErrorMessageReplacementRules(`[]`)) })
	replaced, statusCode, matched := ReplaceClientErrorCandidates(500, "raw upstream message", "stable client message")
	require.True(t, matched)
	require.Equal(t, "first rule", replaced)
	require.Equal(t, 500, statusCode)
}

func TestErrorMessageReplacementRuleMatchesContainsAndExactCaseInsensitively(t *testing.T) {
	require.NoError(t, UpdateErrorMessageReplacementRules(`[{"match":"Insufficient Balance","mode":"contains","replace":"contains matched"},{"match":"UPSTREAM EXACT ERROR","mode":"exact","replace":"exact matched"}]`))
	t.Cleanup(func() { require.NoError(t, UpdateErrorMessageReplacementRules(`[]`)) })
	message, statusCode, matched := ReplaceClientErrorCandidates(502, "provider: insufficient balance")
	require.True(t, matched)
	require.Equal(t, "contains matched", message)
	require.Equal(t, 502, statusCode)
	message, statusCode, matched = ReplaceClientErrorCandidates(502, "upstream exact error")
	require.True(t, matched)
	require.Equal(t, "exact matched", message)
	require.Equal(t, 502, statusCode)
}

func TestErrorMessageReplacementRuleNoMatchKeepsOriginalCandidateCase(t *testing.T) {
	require.NoError(t, UpdateErrorMessageReplacementRules(`[{"match":"Insufficient Balance","mode":"contains","replace":"contains matched"}]`))
	t.Cleanup(func() { require.NoError(t, UpdateErrorMessageReplacementRules(`[]`)) })
	message, statusCode, matched := ReplaceClientErrorCandidates(502, "Original Upstream Error")
	require.False(t, matched)
	require.Equal(t, "Original Upstream Error", message)
	require.Equal(t, 502, statusCode)
}

func TestErrorMessageReplacementRuleBounds(t *testing.T) {
	tooManyRules := make([]ErrorMessageReplacementRule, maxErrorMessageReplacementRules+1)
	encoded, err := Marshal(tooManyRules)
	require.NoError(t, err)
	require.Error(t, ValidateErrorMessageReplacementRules(string(encoded)))

	longMatch := strings.Repeat("x", maxErrorMessageMatchLength+1)
	encoded, err = Marshal([]ErrorMessageReplacementRule{{Match: longMatch, Mode: ErrorMessageReplacementModeExact, Replace: "client"}})
	require.NoError(t, err)
	require.Error(t, ValidateErrorMessageReplacementRules(string(encoded)))

	longReplacement := strings.Repeat("界", maxErrorMessageReplaceLength+1)
	encoded, err = Marshal([]ErrorMessageReplacementRule{{Match: "upstream", Mode: ErrorMessageReplacementModeExact, Replace: longReplacement}})
	require.NoError(t, err)
	require.Error(t, ValidateErrorMessageReplacementRules(string(encoded)))
}
