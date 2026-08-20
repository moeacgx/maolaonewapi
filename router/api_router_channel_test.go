package router

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSetApiRouterRegistersChannelRootWithAndWithoutTrailingSlash(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	routes := make(map[string]bool)
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = true
	}

	for _, route := range []string{
		"GET /api/channel",
		"GET /api/channel/",
		"POST /api/channel",
		"POST /api/channel/",
		"PUT /api/channel",
		"PUT /api/channel/",
	} {
		require.True(t, routes[route], route)
	}
}
