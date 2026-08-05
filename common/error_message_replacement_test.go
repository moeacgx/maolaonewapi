package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrorMessageReplacementRulesUseFirstMatch(t *testing.T) {
	require.NoError(t, UpdateErrorMessageReplacementRules(`[
		{"match":"Insufficient balance","mode":"contains","replace":"渠道余额不足，请稍后重试"},
		{"match":"^status_code=403","mode":"regex","replace":"正则规则不应覆盖前一条"}
	]`))
	t.Cleanup(func() { require.NoError(t, UpdateErrorMessageReplacementRules(`[]`)) })

	replaced, matched := ReplaceClientErrorMessage("status_code=403, Insufficient balance")
	require.True(t, matched)
	require.Equal(t, "渠道余额不足，请稍后重试", replaced)
}

func TestErrorMessageReplacementRulesValidateModesAndRegex(t *testing.T) {
	require.Error(t, ValidateErrorMessageReplacementRules(`[{"match":"x","mode":"unknown","replace":"y"}]`))
	require.Error(t, ValidateErrorMessageReplacementRules(`[{"match":"[","mode":"regex","replace":"y"}]`))
	require.Error(t, ValidateErrorMessageReplacementRules(`[{"match":"x","mode":"exact","replace":""}]`))
}

func TestErrorMessageReplacementCandidateKeepsRuleOrder(t *testing.T) {
	require.NoError(t, UpdateErrorMessageReplacementRules(`[
		{"match":"stable client message","mode":"exact","replace":"first rule"},
		{"match":"raw upstream message","mode":"exact","replace":"second rule"}
	]`))
	t.Cleanup(func() { require.NoError(t, UpdateErrorMessageReplacementRules(`[]`)) })

	replaced, matched := ReplaceClientErrorMessageCandidates(
		"raw upstream message",
		"stable client message",
	)
	require.True(t, matched)
	require.Equal(t, "first rule", replaced)
}
