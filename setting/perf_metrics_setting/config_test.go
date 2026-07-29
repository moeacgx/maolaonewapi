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
		{ID: "status-400", Name: "忽略 400", Enabled: true, Field: FailureFilterFieldStatusCode, Mode: FailureFilterModeExact, Value: "400"},
		{ID: "policy-copy", Name: "内容政策文案", Enabled: true, Field: FailureFilterFieldMessage, Mode: FailureFilterModeContains, Value: "可能违反了OpenAI的内容政策"},
		{ID: "policy-regex", Name: "内容政策正则", Enabled: true, Field: FailureFilterFieldFullError, Mode: FailureFilterModeRegex, Value: `status_code=4\d\d, .*内容政策`},
	}

	got, err := ParseAndValidateFailureFilterRules(marshalFailureFilterRules(t, rules))
	require.NoError(t, err)
	require.Equal(t, rules, got)
}

func TestValidateFailureFilterRulesRejectsInvalidRules(t *testing.T) {
	base := FailureFilterRule{
		ID: "rule-1", Name: "规则", Enabled: true,
		Field: FailureFilterFieldMessage, Mode: FailureFilterModeContains, Value: "blocked",
	}
	tests := []struct {
		name  string
		rules []FailureFilterRule
	}{
		{name: "缺少 ID", rules: []FailureFilterRule{{Name: base.Name, Field: base.Field, Mode: base.Mode, Value: base.Value}}},
		{name: "ID 过长", rules: []FailureFilterRule{{ID: strings.Repeat("a", MaxFailureFilterRuleID+1), Name: base.Name, Field: base.Field, Mode: base.Mode, Value: base.Value}}},
		{name: "ID 包含中文", rules: []FailureFilterRule{{ID: "规则-1", Name: base.Name, Field: base.Field, Mode: base.Mode, Value: base.Value}}},
		{name: "ID 包含空格", rules: []FailureFilterRule{{ID: "rule 1", Name: base.Name, Field: base.Field, Mode: base.Mode, Value: base.Value}}},
		{name: "ID 包含斜杠", rules: []FailureFilterRule{{ID: "rule/1", Name: base.Name, Field: base.Field, Mode: base.Mode, Value: base.Value}}},
		{name: "重复 ID", rules: []FailureFilterRule{base, base}},
		{name: "缺少名称", rules: []FailureFilterRule{{ID: base.ID, Field: base.Field, Mode: base.Mode, Value: base.Value}}},
		{name: "名称过长", rules: []FailureFilterRule{{ID: base.ID, Name: strings.Repeat("名", MaxFailureFilterRuleName+1), Field: base.Field, Mode: base.Mode, Value: base.Value}}},
		{name: "匹配值过长", rules: []FailureFilterRule{{ID: base.ID, Name: base.Name, Field: base.Field, Mode: base.Mode, Value: strings.Repeat("值", MaxFailureFilterRuleValue+1)}}},
		{name: "空匹配值", rules: []FailureFilterRule{{ID: base.ID, Name: base.Name, Field: base.Field, Mode: base.Mode}}},
		{name: "未知字段", rules: []FailureFilterRule{{ID: base.ID, Name: base.Name, Field: "unknown", Mode: base.Mode, Value: base.Value}}},
		{name: "未知模式", rules: []FailureFilterRule{{ID: base.ID, Name: base.Name, Field: base.Field, Mode: "unknown", Value: base.Value}}},
		{name: "非法正则", rules: []FailureFilterRule{{ID: base.ID, Name: base.Name, Field: base.Field, Mode: FailureFilterModeRegex, Value: "["}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Error(t, ValidateFailureFilterRules(marshalFailureFilterRules(t, test.rules)))
		})
	}

	tooMany := make([]FailureFilterRule, MaxFailureFilterRules+1)
	for index := range tooMany {
		tooMany[index] = base
		tooMany[index].ID = fmt.Sprintf("rule-%d", index)
	}
	require.Error(t, ValidateFailureFilterRules(marshalFailureFilterRules(t, tooMany)))
	require.Error(t, ValidateFailureFilterRules("null"))
}

func TestValidateFailureFilterRulesAcceptsIDCharacterSetAndLengthBoundary(t *testing.T) {
	rules := []FailureFilterRule{{
		ID: strings.Repeat("a", MaxFailureFilterRuleID-4) + "._-9", Name: "合法 ID", Enabled: true,
		Field: FailureFilterFieldMessage, Mode: FailureFilterModeContains, Value: "blocked",
	}}
	require.NoError(t, ValidateFailureFilterRules(marshalFailureFilterRules(t, rules)))
}

func TestFailureFilterRulesRoundTripThroughGlobalConfig(t *testing.T) {
	original := perfMetricsSetting.FailureFilterRules
	t.Cleanup(func() { perfMetricsSetting.FailureFilterRules = original })
	rules := []FailureFilterRule{{
		ID: "round-trip", Name: "往返测试", Enabled: true,
		Field: FailureFilterFieldErrorCode, Mode: FailureFilterModeExact, Value: "policy_blocked",
	}}
	value := marshalFailureFilterRules(t, rules)
	require.NoError(t, config.UpdateConfigFromMap(&perfMetricsSetting, map[string]string{"failure_filter_rules": value}))
	require.Equal(t, rules, GetSetting().FailureFilterRules)
}
