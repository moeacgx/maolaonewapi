package perf_metrics_setting

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/require"
)

func marshalFailureFilterRules(t *testing.T, rules []FailureFilterRule) string {
	t.Helper()
	raw, err := common.Marshal(rules)
	require.NoError(t, err)
	return string(raw)
}

func TestParseAndValidateFailureFilterRules(t *testing.T) {
	rules := []FailureFilterRule{
		{ID: "status-400", Name: "ignore 400", Enabled: true, Field: FailureFilterFieldStatusCode, Mode: FailureFilterModeExact, Value: "400"},
		{ID: "policy-copy", Name: "policy copy", Enabled: true, Field: FailureFilterFieldMessage, Mode: FailureFilterModeContains, Values: []string{"first", "second\nline"}},
		{ID: "policy-regex", Name: "policy regex", Enabled: true, Field: FailureFilterFieldFullError, Mode: FailureFilterModeRegex, Value: `status_code=4\d\d, .*policy`},
	}
	got, err := ParseAndValidateFailureFilterRules(marshalFailureFilterRules(t, rules))
	require.NoError(t, err)
	require.Equal(t, rules, got)
}

func TestValidateFailureFilterRulesRejectsInvalidRules(t *testing.T) {
	base := FailureFilterRule{ID: "rule-1", Name: "rule", Enabled: true, Field: FailureFilterFieldMessage, Mode: FailureFilterModeContains, Value: "blocked"}
	tests := [][]FailureFilterRule{
		{{Name: base.Name, Field: base.Field, Mode: base.Mode, Value: base.Value}},
		{{ID: "rule 1", Name: base.Name, Field: base.Field, Mode: base.Mode, Value: base.Value}},
		{base, base},
		{{ID: base.ID, Field: base.Field, Mode: base.Mode, Value: base.Value}},
		{{ID: base.ID, Name: base.Name, Field: "unknown", Mode: base.Mode, Value: base.Value}},
		{{ID: base.ID, Name: base.Name, Field: base.Field, Mode: "unknown", Value: base.Value}},
		{{ID: base.ID, Name: base.Name, Field: base.Field, Mode: FailureFilterModeRegex, Value: "["}},
		{{ID: base.ID, Name: base.Name, Field: base.Field, Mode: base.Mode, Values: []string{" \n "}}},
	}
	for index, rules := range tests {
		t.Run(fmt.Sprintf("case-%d", index), func(t *testing.T) {
			require.Error(t, ValidateFailureFilterRules(marshalFailureFilterRules(t, rules)))
		})
	}
	require.Error(t, ValidateFailureFilterRules("null"))
	tooMany := make([]FailureFilterRule, MaxFailureFilterRules+1)
	for index := range tooMany {
		tooMany[index] = base
		tooMany[index].ID = fmt.Sprintf("rule-%d", index)
	}
	require.Error(t, ValidateFailureFilterRules(marshalFailureFilterRules(t, tooMany)))
	require.Error(t, ValidateFailureFilterRules(marshalFailureFilterRules(t, []FailureFilterRule{{ID: base.ID, Name: strings.Repeat("n", MaxFailureFilterRuleName+1), Field: base.Field, Mode: base.Mode, Value: base.Value}})))
}

func TestFailureFilterRulesRoundTripThroughGlobalConfig(t *testing.T) {
	original := perfMetricsSetting.FailureFilterRules
	t.Cleanup(func() { perfMetricsSetting.FailureFilterRules = original })
	rules := []FailureFilterRule{{ID: "round-trip", Name: "round trip", Enabled: true, Field: FailureFilterFieldErrorCode, Mode: FailureFilterModeExact, Values: []string{"policy_blocked", "cyber_policy"}}}
	require.NoError(t, config.UpdateConfigFromMap(&perfMetricsSetting, map[string]string{"failure_filter_rules": marshalFailureFilterRules(t, rules)}))
	require.Equal(t, rules, GetSetting().FailureFilterRules)
}
