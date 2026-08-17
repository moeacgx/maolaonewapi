package perfmetrics

import (
	"encoding/json"
	"errors"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestRecordRelayFailureExcludesContentPolicyAndConfiguredRules(t *testing.T) {
	info := &relaycommon.RelayInfo{OriginModelName: "filter-test", UsingGroup: "default"}
	require.False(t, shouldRecordRelayFailure(info, types.NewError(errors.New("blocked"), hosttypes.ErrorCodeCyberPolicy)))

	relayErr := types.NewErrorWithStatusCode(errors.New("connection reset by peer"), types.ErrorCodeDoRequestFailed, 502)
	rules := []perf_metrics_setting.FailureFilterRule{{ID: "network", Name: "network", Enabled: true, Field: "message", Mode: "contains", Values: []string{"not present", "connection reset"}}}
	registeredSetting := config.GlobalConfig.Get("perf_metrics_setting")
	require.NotNil(t, registeredSetting)
	originalRules, err := json.Marshal(perf_metrics_setting.GetSetting().FailureFilterRules)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(registeredSetting, map[string]string{"failure_filter_rules": string(originalRules)}))
	})
	configuredRules, err := json.Marshal(rules)
	require.NoError(t, err)
	require.NoError(t, config.UpdateConfigFromMap(registeredSetting, map[string]string{"failure_filter_rules": string(configuredRules)}))
	require.True(t, matchesFailureFilterRule(relayErr, rules))
	require.False(t, shouldRecordRelayFailure(info, relayErr), "configured failure must be isolated from performance samples")
	require.True(t, shouldRecordRelayFailure(info, types.NewErrorWithStatusCode(errors.New("different failure"), types.ErrorCodeDoRequestFailed, 502)))
}

func TestMatchesFailureFilterRuleSupportsFieldsModesAndInvalidRegex(t *testing.T) {
	relayErr := types.NewErrorWithStatusCode(errors.New("upstream policy copy"), types.ErrorCodeBadResponseStatusCode, 400)
	tests := []perf_metrics_setting.FailureFilterRule{
		{Enabled: true, Field: " status_code ", Mode: " exact ", Value: "400"},
		{Enabled: true, Field: "error_code", Mode: "contains", Value: "bad_response"},
		{Enabled: true, Field: "message", Mode: "contains", Value: "policy copy"},
		{Enabled: true, Field: "full_error", Mode: "regex", Value: `status_code=400, .*policy`},
	}
	for _, rule := range tests {
		require.True(t, matchesFailureFilterRule(relayErr, []perf_metrics_setting.FailureFilterRule{rule}))
	}
	require.False(t, matchesFailureFilterRule(relayErr, []perf_metrics_setting.FailureFilterRule{{Enabled: true, Field: "message", Mode: "regex", Value: "["}}))
}

func TestFailureFilterRegexCacheStoresValidAndInvalidPatterns(t *testing.T) {
	failureFilterRegexCache.Lock()
	failureFilterRegexCache.entries = make(map[string]failureFilterRegexCacheEntry)
	failureFilterRegexCache.Unlock()
	first, valid := getFailureFilterRegex(`policy_\d+`)
	require.True(t, valid)
	second, valid := getFailureFilterRegex(`policy_\d+`)
	require.True(t, valid)
	require.Same(t, first, second)
	invalid, valid := getFailureFilterRegex("[")
	require.False(t, valid)
	require.Nil(t, invalid)
}
