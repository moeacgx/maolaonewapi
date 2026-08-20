package model

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestUpdateLogRetentionDaysOptionUpdatesRuntimeValue(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	previous := common.GetLogRetentionDays()
	common.OptionMapRWMutex.Lock()
	previousOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		_ = updateOptionMap("LogRetentionDays", strconv.Itoa(previous))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
		_ = DB.Where("key = ?", "LogRetentionDays").Delete(&Option{}).Error
	})

	require.NoError(t, UpdateOption("LogRetentionDays", "30"))
	require.Equal(t, 30, common.GetLogRetentionDays())

	var option Option
	require.NoError(t, DB.Where("key = ?", "LogRetentionDays").First(&option).Error)
	require.Equal(t, "30", option.Value)

	require.Error(t, UpdateOption("LogRetentionDays", "invalid"))
	require.Error(t, UpdateOption("LogRetentionDays", "-1"))
	require.Error(t, UpdateOption("LogRetentionDays", strconv.Itoa(common.MaxLogRetentionDays+1)))
	require.NoError(t, DB.Where("key = ?", "LogRetentionDays").First(&option).Error)
	require.Equal(t, "30", option.Value, "invalid retention values must not overwrite the stored valid value")
	require.Equal(t, 30, common.GetLogRetentionDays())
}
