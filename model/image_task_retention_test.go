package model

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageTaskRetentionOptionValidationAndRuntimeSync(t *testing.T) {
	const key = "performance_setting.image_task_data_retention_hours"
	for _, value := range []string{"0", "1", strconv.Itoa(common.MaxImageTaskDataRetentionHours)} {
		require.NoError(t, validateOptionValue(key, value))
	}
	for _, value := range []string{"-1", "1.5", strconv.Itoa(common.MaxImageTaskDataRetentionHours + 1)} {
		assert.Error(t, validateOptionValue(key, value))
	}

	require.NoError(t, DB.AutoMigrate(&Option{}))
	truncateTables(t)
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	previous := common.GetImageTaskDataRetentionHours()
	t.Cleanup(func() {
		common.SetImageTaskDataRetentionHours(previous)
		_ = DB.Where("key = ?", key).Delete(&Option{}).Error
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})
	require.NoError(t, UpdateOption(key, "2"))
	assert.Equal(t, 2, common.GetImageTaskDataRetentionHours())
	require.Error(t, UpdateOption(key, "invalid"))
	assert.Equal(t, 2, common.GetImageTaskDataRetentionHours())
}
