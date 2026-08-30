package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useConfiguredStatusExchangeRate(t *testing.T, usdExchangeRate, price float64) {
	t.Helper()
	generalSetting := operation_setting.GetGeneralSetting()
	originalAutoExchangeRate := generalSetting.AutoUSDExchangeRate
	originalUSDExchangeRate := operation_setting.USDExchangeRate
	originalPrice := operation_setting.Price
	generalSetting.AutoUSDExchangeRate = false
	operation_setting.USDExchangeRate = usdExchangeRate
	operation_setting.Price = price
	t.Cleanup(func() {
		generalSetting.AutoUSDExchangeRate = originalAutoExchangeRate
		operation_setting.USDExchangeRate = originalUSDExchangeRate
		operation_setting.Price = originalPrice
	})
}

func TestGetStatusReturnsEffectiveOIDCDisplayName(t *testing.T) {
	useConfiguredStatusExchangeRate(t, 6.8, 6.9)
	settings := system_setting.GetOIDCSettings()
	originalDisplayName := settings.DisplayName
	originalOptionMap := common.OptionMap
	t.Cleanup(func() {
		settings.DisplayName = originalDisplayName
		common.OptionMap = originalOptionMap
	})
	common.OptionMap = map[string]string{}

	tests := []struct {
		name        string
		displayName string
		want        string
	}{
		{
			name:        "custom name is trimmed",
			displayName: "  Acme SSO  ",
			want:        "Acme SSO",
		},
		{
			name:        "whitespace-only name falls back",
			displayName: "   ",
			want:        "OIDC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings.DisplayName = tt.displayName
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			context.Request = httptest.NewRequest(http.MethodGet, "/api/status", nil)

			GetStatus(context)

			var payload struct {
				Success bool           `json:"success"`
				Data    map[string]any `json:"data"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
			require.True(t, payload.Success)
			assert.Equal(t, tt.want, payload.Data["oidc_display_name"])
		})
	}
}

func TestGetStatusReturnsV243ExchangeRateContract(t *testing.T) {
	useConfiguredStatusExchangeRate(t, 6.8, 6.9)
	originalOkpayRateSource := setting.OkpayRateSource
	originalOkpayExchangeRate := setting.OkpayExchangeRate
	originalOkpayUsdtCnyRate := setting.OkpayUsdtCnyRate
	setting.OkpayRateSource = "okx-alipay-rate-module"
	setting.OkpayExchangeRate = 8.88
	setting.OkpayUsdtCnyRate = 6.66
	originalOptionMap := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		setting.OkpayRateSource = originalOkpayRateSource
		setting.OkpayExchangeRate = originalOkpayExchangeRate
		setting.OkpayUsdtCnyRate = originalOkpayUsdtCnyRate
		common.OptionMap = originalOptionMap
	})

	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/status", nil)

	GetStatus(context)

	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			USDExchangeRate              float64 `json:"usd_exchange_rate"`
			USDExchangeRateSource        string  `json:"usd_exchange_rate_source"`
			USDExchangeRateLastUpdatedAt int64   `json:"usd_exchange_rate_last_updated_at"`
			USDExchangeRateIsFallback    bool    `json:"usd_exchange_rate_is_fallback"`
			AutoUSDExchangeRate          bool    `json:"auto_usd_exchange_rate"`
			Price                        float64 `json:"price"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	assert.InDelta(t, 6.8, payload.Data.USDExchangeRate, 0.000001)
	assert.Equal(t, "configured", payload.Data.USDExchangeRateSource)
	assert.Equal(t, int64(0), payload.Data.USDExchangeRateLastUpdatedAt)
	assert.False(t, payload.Data.USDExchangeRateIsFallback)
	assert.False(t, payload.Data.AutoUSDExchangeRate)
	assert.InDelta(t, 6.9, payload.Data.Price, 0.000001)
}
