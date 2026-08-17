package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
)

func TestPaymentReturnURLUsesThemeAwareDashboardPath(t *testing.T) {
	previousAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://dashboard.example.com/"
	previousTheme := common.GetTheme()
	t.Cleanup(func() {
		system_setting.ServerAddress = previousAddress
		common.SetTheme(previousTheme)
	})

	common.SetTheme("default")
	assert.Equal(t, "https://dashboard.example.com/wallet?pay=success", PaymentReturnURL("/console/topup?pay=success"))
	assert.Equal(t, "https://dashboard.example.com/invoices?pay=pending", PaymentReturnURL("/console/invoice?pay=pending"))

	common.SetTheme("classic")
	assert.Equal(t, "https://dashboard.example.com/console/topup?show_history=true", PaymentReturnURL("/wallet?show_history=true"))
	assert.Equal(t, "https://dashboard.example.com/console/log?pay=pending", PaymentReturnURL("/usage-logs?pay=pending"))
}
