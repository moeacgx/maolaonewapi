package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAnonymousPaymentNotifyGETRoutesRejectOversizedBodies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalLimitKB := constant.AnonymousRequestBodyLimitKB
	constant.AnonymousRequestBodyLimitKB = 1
	t.Cleanup(func() { constant.AnonymousRequestBodyLimitKB = originalLimitKB })

	engine := gin.New()
	SetApiRouter(engine)

	paths := []string{
		"/api/bepusdt/notify",
		"/api/okpay/notify",
		"/api/invoice/bepusdt/notify",
		"/api/invoice/epay/notify",
		"/api/invoice/okpay/notify",
	}
	for index, path := range paths {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, strings.NewReader(strings.Repeat("x", 1025)))
			request.RemoteAddr = "192.0.2." + string(rune('1'+index)) + ":12345"
			recorder := httptest.NewRecorder()

			engine.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
		})
	}
}
