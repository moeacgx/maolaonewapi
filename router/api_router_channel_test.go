package router

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
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

func TestSetApiRouterRegistersSelfUpdateStatusRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	routes := make(map[string]bool)
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = true
	}

	for _, route := range []string{
		"GET /api/status/github-latest-release",
		"POST /api/status/self-update",
	} {
		require.True(t, routes[route], route)
	}

	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/status/github-latest-release"},
		{http.MethodPost, "/api/status/self-update"},
	} {
		request := httptest.NewRequest(route.method, route.path, nil)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		require.Equal(t, http.StatusUnauthorized, response.Code, route.path)
	}
}
