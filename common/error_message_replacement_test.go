package common

import (
	"fmt"
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

func TestErrorMessageReplacementRuleMatchesSecondaryExactAndRegexValues(t *testing.T) {
	testCases := []struct {
		name    string
		mode    string
		matches string
		message string
	}{
		{
			name:    "exact",
			mode:    ErrorMessageReplacementModeExact,
			matches: `["first exact value","second exact value"]`,
			message: "second exact value",
		},
		{
			name:    "regex",
			mode:    ErrorMessageReplacementModeRegex,
			matches: `["^first-[0-9]+$","^second-[0-9]+$"]`,
			message: "second-42",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			rules := fmt.Sprintf(`[{"matches":%s,"mode":%q,"replace":"matched"}]`, testCase.matches, testCase.mode)
			require.NoError(t, UpdateErrorMessageReplacementRules(rules))
			t.Cleanup(func() { require.NoError(t, UpdateErrorMessageReplacementRules(`[]`)) })

			message, statusCode, matched := ReplaceClientErrorCandidates(502, testCase.message)
			require.True(t, matched)
			require.Equal(t, "matched", message)
			require.Equal(t, 502, statusCode)
		})
	}
}

func TestErrorMessageReplacementRuleLimitsMatchValues(t *testing.T) {
	matches := make([]string, maxErrorMessageMatchesPerRule)
	for index := range matches {
		matches[index] = fmt.Sprintf("match-%d", index)
	}
	rules := []ErrorMessageReplacementRule{{
		Match:   matches[0],
		Matches: matches,
		Mode:    ErrorMessageReplacementModeContains,
		Replace: "replacement",
	}}
	encoded, err := Marshal(rules)
	require.NoError(t, err)
	require.NoError(t, ValidateErrorMessageReplacementRules(string(encoded)))

	rules[0].Matches = append(rules[0].Matches, "too-many")
	encoded, err = Marshal(rules)
	require.NoError(t, err)
	require.Error(t, ValidateErrorMessageReplacementRules(string(encoded)))
}

func TestErrorMessageReplacementRuleMatchesStatusAndMessage(t *testing.T) {
	require.NoError(t, UpdateErrorMessageReplacementRules(`[
		{"match":"Insufficient balance","mode":"contains","status_code":403,"replace":"请求过多，请稍后重试","replace_status_code":429}
	]`))
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
	require.NoError(t, UpdateErrorMessageReplacementRules(`[
		{"match":"upstream error","mode":"exact","replace":"client error"}
	]`))
	t.Cleanup(func() { require.NoError(t, UpdateErrorMessageReplacementRules(`[]`)) })

	message, statusCode, matched := ReplaceClientErrorCandidates(502, "upstream error")
	require.True(t, matched)
	require.Equal(t, "client error", message)
	require.Equal(t, 502, statusCode)
}

func TestErrorMessageReplacementCandidateKeepsRuleOrder(t *testing.T) {
	require.NoError(t, UpdateErrorMessageReplacementRules(`[
		{"match":"stable client message","mode":"exact","replace":"first rule"},
		{"match":"raw upstream message","mode":"exact","replace":"second rule"}
	]`))
	t.Cleanup(func() { require.NoError(t, UpdateErrorMessageReplacementRules(`[]`)) })

	replaced, statusCode, matched := ReplaceClientErrorCandidates(
		500,
		"raw upstream message",
		"stable client message",
	)
	require.True(t, matched)
	require.Equal(t, "first rule", replaced)
	require.Equal(t, 500, statusCode)
}
