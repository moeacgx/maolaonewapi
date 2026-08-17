package system_setting

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

const (
	FrontendThemeDefault = "default"
	FrontendThemeClassic = "classic"
)

type ThemeSettings struct {
	Frontend string `json:"frontend"`
}

var themeSettings = ThemeSettings{
	Frontend: FrontendThemeClassic,
}

func init() {
	config.GlobalConfig.Register("theme", &themeSettings)
	syncThemeToCommon()
}

func NormalizeFrontendTheme(theme string) string {
	if theme == FrontendThemeDefault || theme == FrontendThemeClassic {
		return theme
	}
	return FrontendThemeClassic
}

func syncThemeToCommon() {
	themeSettings.Frontend = NormalizeFrontendTheme(themeSettings.Frontend)
	common.SetTheme(themeSettings.Frontend)
}

func GetThemeSettings() *ThemeSettings {
	return &themeSettings
}

func UpdateAndSyncTheme() {
	syncThemeToCommon()
}
