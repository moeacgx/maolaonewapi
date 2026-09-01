package router

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestMarketingDeleteRoutesKeepStaticPathsAheadOfIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)
	routes := make(map[string]struct{})
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	for _, path := range []string{
		"/api/redemption/batch", "/api/redemption/invalid", "/api/redemption/:id",
		"/api/promo_code/batch", "/api/promo_code/invalid", "/api/promo_code/:id",
		"/api/benefit/admin/activities/batch", "/api/benefit/admin/activities/:id",
		"/api/benefit/vouchers/:id/ledger", "/api/benefit/admin/vouchers/batch-void",
	} {
		method := http.MethodDelete
		if path == "/api/benefit/vouchers/:id/ledger" {
			method = http.MethodGet
		}
		if path == "/api/benefit/admin/vouchers/batch-void" {
			method = http.MethodPost
		}
		_, ok := routes[method+" "+path]
		assert.True(t, ok, "missing marketing route %s %s", method, path)
	}
}
