package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetStatusIncludesCCSwitchAPIAddress(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const expected = "https://api.example.com/gateway"

	originalAddress := setting.GetCCSwitchAPIAddress()
	setting.SetCCSwitchAPIAddress(expected)
	t.Cleanup(func() {
		setting.SetCCSwitchAPIAddress(originalAddress)
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/status", nil)
	GetStatus(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			CCSwitchAPIAddress string `json:"cc_switch_api_address"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Equal(t, expected, response.Data.CCSwitchAPIAddress)
}
