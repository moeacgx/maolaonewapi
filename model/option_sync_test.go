package model

import (
	"runtime"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/require"
)

func TestUpdateOptionMapReleasesOptionMapLockBeforeInvalidatingPricing(t *testing.T) {
	oldMap := common.OptionMap
	oldMain := common.MainDatabaseType()
	oldModes := billing_setting.GetBillingModeCopy()
	oldExprs := billing_setting.GetBillingExprCopy()
	t.Cleanup(func() {
		common.OptionMap = oldMap
		common.SetMainDatabaseType(oldMain)
		_ = config.UpdateConfigFromMap(config.GlobalConfig.Get("billing_setting"), map[string]string{
			"billing_mode": func() string {
				value, _ := common.Marshal(oldModes)
				return string(value)
			}(),
			"billing_expr": func() string {
				value, _ := common.Marshal(oldExprs)
				return string(value)
			}(),
		})
	})
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.OptionMap = map[string]string{}

	updatePricingLock.Lock()
	done := make(chan error, 1)
	go func() {
		done <- updateOptionMap("billing_setting.billing_mode", `{"lock-test-model":"ratio"}`)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		common.OptionMapRWMutex.RLock()
		updated := common.OptionMap["billing_setting.billing_mode"] != ""
		common.OptionMapRWMutex.RUnlock()
		if updated {
			break
		}
		if time.Now().After(deadline) {
			updatePricingLock.Unlock()
			t.Fatal("option update did not publish its value")
		}
		runtime.Gosched()
	}

	updatePricingLock.Unlock()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("option update deadlocked while invalidating pricing")
	}
}
