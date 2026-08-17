package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
)

func TestPaymentReturnPathUsesThemeAwareDashboardRoutes(t *testing.T) {
	previousAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://dashboard.example.com/"
	previousTheme := common.GetTheme()
	t.Cleanup(func() {
		system_setting.ServerAddress = previousAddress
		common.SetTheme(previousTheme)
	})

	common.SetTheme("default")
	defaultCases := map[string]string{
		"/console/invoice?pay=pending":            "https://dashboard.example.com/invoices?pay=pending",
		"/console/topup?pay=success":              "https://dashboard.example.com/wallet?pay=success",
		"/console/log/export?range=7d#latest":     "https://dashboard.example.com/usage-logs/export?range=7d#latest",
		"/console/personal/security?tab=passkeys": "https://dashboard.example.com/profile/security?tab=passkeys",
		"/console/unknown?keep=true":              "https://dashboard.example.com/console/unknown?keep=true",
	}
	for suffix, want := range defaultCases {
		assert.Equal(t, want, paymentReturnPath(suffix))
	}

	common.SetTheme("classic")
	classicCases := map[string]string{
		"/invoices?pay=pending":            "https://dashboard.example.com/console/invoice?pay=pending",
		"/wallet?show_history=true":        "https://dashboard.example.com/console/topup?show_history=true",
		"/usage-logs/export?range=7d#tail": "https://dashboard.example.com/console/log/export?range=7d#tail",
		"/profile/security?tab=passkeys":   "https://dashboard.example.com/console/personal/security?tab=passkeys",
		"/docs?keep=true":                  "https://dashboard.example.com/docs?keep=true",
	}
	for suffix, want := range classicCases {
		assert.Equal(t, want, paymentReturnPath(suffix))
	}
}
