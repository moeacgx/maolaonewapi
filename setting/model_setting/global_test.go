package model_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlobalSettingsCanvasDefaultGroupExportsAndUpdates(t *testing.T) {
	assert.Empty(t, defaultOpenaiSettings.CanvasDefaultGroup)
	assert.Empty(t, config.GlobalConfig.ExportAllConfigs()["global.canvas_default_group"])

	settings := GlobalSettings{}
	require.NoError(t, config.UpdateConfigFromMap(&settings, map[string]string{
		"canvas_default_group": "vip",
	}))
	assert.Equal(t, "vip", settings.CanvasDefaultGroup)
}
