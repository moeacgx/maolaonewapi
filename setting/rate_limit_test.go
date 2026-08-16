package setting

import (
	"strings"
	"testing"
)

func TestUpdateModelRequestRateLimitOptionsPublishesOneCompleteGeneration(t *testing.T) {
	original := GetModelRequestRateLimitSnapshot()
	t.Cleanup(func() { modelRequestRateLimitSnapshot.Store(original) })

	err := UpdateModelRequestRateLimitOptions(map[string]string{
		"ModelRequestRateLimitEnabled":         "true",
		"ModelRequestRateLimitDurationMinutes": "7",
		"ModelRequestRateLimitCount":           "11",
		"ModelRequestRateLimitSuccessCount":    "13",
		"ModelRequestRateLimitGroup":           `{"codex":[17,19]}`,
		"ModelRequestRateLimitUserGroup":       `{"vip":{"global":[23,29],"groups":{"codex":[31,37]}}}`,
	})
	if err != nil {
		t.Fatalf("valid full update failed: %v", err)
	}
	published := GetModelRequestRateLimitSnapshot()
	if !published.Enabled() || published.DurationMinutes() != 7 {
		t.Fatalf("unexpected scalar snapshot: enabled=%v duration=%d", published.Enabled(), published.DurationMinutes())
	}
	if total, success := published.GlobalRateLimit(); total != 11 || success != 13 {
		t.Fatalf("unexpected global snapshot: [%d,%d]", total, success)
	}
	if total, success, found := published.GetUserGroupRateLimit("vip", "codex"); !found || total != 31 || success != 37 {
		t.Fatalf("unexpected most-specific rule: [%d,%d], found=%v", total, success, found)
	}

	err = UpdateModelRequestRateLimitOptions(map[string]string{
		"ModelRequestRateLimitEnabled": "false",
		"ModelRequestRateLimitGroup":   `{"broken":[-1,0]}`,
	})
	if err == nil {
		t.Fatal("invalid mixed update succeeded")
	}
	if current := GetModelRequestRateLimitSnapshot(); current != published {
		t.Fatal("invalid mixed update published a partial generation")
	}
}

func TestCapturedModelRequestRateLimitSnapshotRemainsStable(t *testing.T) {
	original := GetModelRequestRateLimitSnapshot()
	t.Cleanup(func() { modelRequestRateLimitSnapshot.Store(original) })

	if err := UpdateModelRequestRateLimitOptions(map[string]string{
		"ModelRequestRateLimitCount": "41",
		"ModelRequestRateLimitGroup": `{"codex":[43,47]}`,
	}); err != nil {
		t.Fatalf("publish first generation: %v", err)
	}
	first := GetModelRequestRateLimitSnapshot()
	if err := UpdateModelRequestRateLimitOptions(map[string]string{
		"ModelRequestRateLimitCount": "53",
		"ModelRequestRateLimitGroup": `{"codex":[59,61]}`,
	}); err != nil {
		t.Fatalf("publish second generation: %v", err)
	}
	if total, _ := first.GlobalRateLimit(); total != 41 {
		t.Fatalf("captured global value changed to %d", total)
	}
	if total, success, found := first.GetGroupRateLimit("codex"); !found || total != 43 || success != 47 {
		t.Fatalf("captured map changed: [%d,%d], found=%v", total, success, found)
	}
}

func TestModelRequestRateLimitUserGroupPrecedenceInputs(t *testing.T) {
	original := GetModelRequestRateLimitSnapshot()
	t.Cleanup(func() { modelRequestRateLimitSnapshot.Store(original) })

	raw := `{"vip":{"global":[0,2000],"groups":{"codex":[0,5000],"default":[100,1000]}}}`
	if err := UpdateModelRequestRateLimitUserGroupByJSONString(raw); err != nil {
		t.Fatalf("valid user-group update failed: %v", err)
	}
	if total, success, found := GetUserGroupGlobalRateLimit("vip"); !found || total != 0 || success != 2000 {
		t.Fatalf("unexpected vip global limit: [%d,%d], found=%v", total, success, found)
	}
	if total, success, found := GetUserGroupRateLimit("vip", "codex"); !found || total != 0 || success != 5000 {
		t.Fatalf("unexpected vip/codex limit: [%d,%d], found=%v", total, success, found)
	}
}

func TestCheckModelRequestRateLimitUserGroupRejectsInvalidLimits(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{`{"":{"global":[0,100]}}`, "user group is empty"},
		{`{"vip":{"global":[-1,100]}}`, "negative rate limit"},
		{`{"vip":{"groups":{"codex":[0,0]}}}`, "success"},
		{`{"vip":{"groups":{"":[0,100]}}}`, "request group is empty"},
	}
	for _, test := range tests {
		err := CheckModelRequestRateLimitUserGroup(test.raw)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("CheckModelRequestRateLimitUserGroup(%q) error = %v, want %q", test.raw, err, test.want)
		}
	}
}
