package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/require"
)

func TestUpdateCCSwitchAPIAddressNormalizesBeforePersisting(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Where(commonKeyCol+" = ?", "CCSwitchAPIAddress").Delete(&Option{}).Error)

	originalAddress := setting.GetCCSwitchAPIAddress()
	common.OptionMapRWMutex.Lock()
	optionMapWasNil := common.OptionMap == nil
	if optionMapWasNil {
		common.OptionMap = make(map[string]string)
	}
	originalMapValue, hadOriginalMapValue := common.OptionMap["CCSwitchAPIAddress"]
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		setting.SetCCSwitchAPIAddress(originalAddress)
		common.OptionMapRWMutex.Lock()
		if optionMapWasNil {
			common.OptionMap = nil
		} else if hadOriginalMapValue {
			common.OptionMap["CCSwitchAPIAddress"] = originalMapValue
		} else {
			delete(common.OptionMap, "CCSwitchAPIAddress")
		}
		common.OptionMapRWMutex.Unlock()
		require.NoError(t, DB.Where(commonKeyCol+" = ?", "CCSwitchAPIAddress").Delete(&Option{}).Error)
	})

	require.NoError(t, UpdateOption("CCSwitchAPIAddress", "  https://api.example.com/gateway///  "))

	var stored Option
	require.NoError(t, DB.Where(commonKeyCol+" = ?", "CCSwitchAPIAddress").First(&stored).Error)
	require.Equal(t, "https://api.example.com/gateway", stored.Value)
	require.Equal(t, stored.Value, setting.GetCCSwitchAPIAddress())
	common.OptionMapRWMutex.RLock()
	require.Equal(t, stored.Value, common.OptionMap["CCSwitchAPIAddress"])
	common.OptionMapRWMutex.RUnlock()

	require.Error(t, UpdateOption("CCSwitchAPIAddress", "file:///tmp/api"))
	require.NoError(t, DB.Where(commonKeyCol+" = ?", "CCSwitchAPIAddress").First(&stored).Error)
	require.Equal(t, "https://api.example.com/gateway", stored.Value)
	require.Equal(t, stored.Value, setting.GetCCSwitchAPIAddress())
}
