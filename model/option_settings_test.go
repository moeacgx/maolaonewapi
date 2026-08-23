package model

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
)

func TestNormalizeOwnedOptionValues(t *testing.T) {
	got, err := normalizeOptionValue("CCSwitchAPIAddress", " https://api.example.com/root/// ")
	if err != nil {
		t.Fatalf("normalize CC Switch option: %v", err)
	}
	if got != "https://api.example.com/root" {
		t.Fatalf("normalized value = %q", got)
	}
	if _, err := normalizeOptionValue("CCSwitchAPIAddress", "/relative"); err == nil {
		t.Fatal("relative CC Switch address should be rejected before persistence")
	}
}

func TestValidateOwnedStructuredOptionValues(t *testing.T) {
	if err := validateOptionValue("ModelRequestRateLimitUserGroup", `{"vip":{"groups":{"codex":[0,100]}}}`); err != nil {
		t.Fatalf("valid user-group limiter rejected: %v", err)
	}
	if err := validateOptionValue("ModelRequestRateLimitUserGroup", `{"vip":{"groups":{"codex":[0,0]}}}`); err == nil {
		t.Fatal("invalid user-group limiter should be rejected before persistence")
	}
	if err := validateOptionValue("ModelRequestRateLimitDurationMinutes", "not-an-integer"); err == nil {
		t.Fatal("invalid limiter duration should be rejected before persistence")
	}
	if err := validateOptionValue("ModelRequestRateLimitSuccessCount", "0"); err == nil {
		t.Fatal("zero limiter success count should be rejected before persistence")
	}
	if err := validateOptionValue("perf_metrics_setting.failure_filter_rules", `[{"id":"policy","name":"policy","enabled":true,"field":"message","mode":"contains","value":"blocked"}]`); err != nil {
		t.Fatalf("valid performance failure filter rejected: %v", err)
	}
	if err := validateOptionValue("perf_metrics_setting.failure_filter_rules", `null`); err == nil {
		t.Fatal("null performance failure filter should be rejected before persistence")
	}
	if err := validateOptionValue("dynamic_routing.rules", `[{"id":"route","enabled":true,"source_model":"public","target_model":"upstream"}]`); err != nil {
		t.Fatalf("valid dynamic routing rules rejected: %v", err)
	}
	if err := validateOptionValue("dynamic_routing.rules", `[{"id":"route","enabled":true,"source_model":"public"}]`); err == nil {
		t.Fatal("invalid dynamic routing rules should be rejected before persistence")
	}
	if err := validateOptionValue("dynamic_routing.rules", "null"); err == nil {
		t.Fatal("null dynamic routing rules should be rejected before persistence")
	}
	if err := validateOptionValue("dynamic_routing.enabled", "true"); err != nil {
		t.Fatalf("valid dynamic routing switch rejected: %v", err)
	}
	if err := validateOptionValue("dynamic_routing.enabled", "maybe"); err == nil {
		t.Fatal("invalid dynamic routing switch should be rejected before persistence")
	}
}

func TestPublishModelRequestRateLimitOptionsRetainsGenerationOnInvalidBatch(t *testing.T) {
	common.OptionMapRWMutex.RLock()
	originalOptionMapWasNil := common.OptionMap == nil
	originalOptionMap := make(map[string]string, len(common.OptionMap))
	for key, value := range common.OptionMap {
		originalOptionMap[key] = value
	}
	common.OptionMapRWMutex.RUnlock()
	original := setting.GetModelRequestRateLimitSnapshot()
	t.Cleanup(func() {
		total, success := original.GlobalRateLimit()
		if err := setting.UpdateModelRequestRateLimitOptions(map[string]string{
			"ModelRequestRateLimitEnabled":         strconv.FormatBool(original.Enabled()),
			"ModelRequestRateLimitDurationMinutes": strconv.Itoa(original.DurationMinutes()),
			"ModelRequestRateLimitCount":           strconv.Itoa(total),
			"ModelRequestRateLimitSuccessCount":    strconv.Itoa(success),
			"ModelRequestRateLimitGroup":           original.GroupJSONString(),
			"ModelRequestRateLimitUserGroup":       original.UserGroupJSONString(),
		}); err != nil {
			t.Fatalf("restore model limiter snapshot: %v", err)
		}
		common.OptionMapRWMutex.Lock()
		if originalOptionMapWasNil {
			common.OptionMap = nil
		} else {
			common.OptionMap = originalOptionMap
		}
		common.OptionMapRWMutex.Unlock()
	})

	if err := publishModelRequestRateLimitOptions(map[string]string{
		"ModelRequestRateLimitEnabled":      "true",
		"ModelRequestRateLimitCount":        "101",
		"ModelRequestRateLimitSuccessCount": "103",
		"ModelRequestRateLimitGroup":        `{"codex":[107,109]}`,
	}); err != nil {
		t.Fatalf("publish valid batch: %v", err)
	}
	published := setting.GetModelRequestRateLimitSnapshot()
	common.OptionMapRWMutex.RLock()
	publishedOptionValues := map[string]string{
		"enabled": common.OptionMap["ModelRequestRateLimitEnabled"],
		"total":   common.OptionMap["ModelRequestRateLimitCount"],
		"success": common.OptionMap["ModelRequestRateLimitSuccessCount"],
		"groups":  common.OptionMap["ModelRequestRateLimitGroup"],
	}
	common.OptionMapRWMutex.RUnlock()
	if publishedOptionValues["enabled"] != "true" || publishedOptionValues["total"] != "101" || publishedOptionValues["success"] != "103" || publishedOptionValues["groups"] != `{"codex":[107,109]}` {
		t.Fatalf("option map did not receive one coherent generation: %#v", publishedOptionValues)
	}
	if err := publishModelRequestRateLimitOptions(map[string]string{
		"ModelRequestRateLimitEnabled": "false",
		"ModelRequestRateLimitCount":   "127",
		"ModelRequestRateLimitGroup":   `{"broken":[0,0]}`,
	}); err == nil {
		t.Fatal("invalid option batch succeeded")
	}
	if current := setting.GetModelRequestRateLimitSnapshot(); current != published {
		t.Fatal("invalid option batch replaced the complete published generation")
	}
	if total, success := published.GlobalRateLimit(); total != 101 || success != 103 {
		t.Fatalf("valid generation changed after rejection: [%d,%d]", total, success)
	}
	common.OptionMapRWMutex.RLock()
	retainedEnabled := common.OptionMap["ModelRequestRateLimitEnabled"]
	retainedTotal := common.OptionMap["ModelRequestRateLimitCount"]
	retainedGroups := common.OptionMap["ModelRequestRateLimitGroup"]
	common.OptionMapRWMutex.RUnlock()
	if retainedEnabled != "true" || retainedTotal != "101" || retainedGroups != `{"codex":[107,109]}` {
		t.Fatalf("invalid option batch changed published option values: enabled=%q total=%q groups=%q", retainedEnabled, retainedTotal, retainedGroups)
	}
}
