package router

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestIsRealStaticWebAssetRequest(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatalf("create assets directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "app.js"), []byte("app"), 0o600); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("index"), 0o600); err != nil {
		t.Fatalf("write index: %v", err)
	}
	frontendFS := static.LocalFile(root, false)

	tests := []struct {
		name   string
		method string
		path   string
		want   bool
	}{
		{name: "existing GET asset", method: http.MethodGet, path: "/assets/app.js", want: true},
		{name: "existing HEAD asset", method: http.MethodHead, path: "/assets/app.js", want: true},
		{name: "missing asset", method: http.MethodGet, path: "/assets/missing.js"},
		{name: "asset write", method: http.MethodPost, path: "/assets/app.js"},
		{name: "html", method: http.MethodGet, path: "/index.html"},
		{name: "spa route", method: http.MethodGet, path: "/dashboard"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			if got := isRealStaticWebAssetRequest(request, frontendFS); got != test.want {
				t.Fatalf("isRealStaticWebAssetRequest() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestSetWebRouterStatsBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	type observation struct {
		activeConnections int64
		routeTag          string
	}
	observations := make(map[string]observation)
	recordObservation := func(c *gin.Context) {
		routeTag, _ := c.Get(middleware.RouteTagKey)
		routeTagString, _ := routeTag.(string)
		observations[c.Request.URL.Path] = observation{
			activeConnections: middleware.GetStats().ActiveConnections,
			routeTag:          routeTagString,
		}
	}
	countedHandler := func(c *gin.Context) {
		recordObservation(c)
		c.Status(http.StatusNoContent)
	}

	relayRouter := engine.Group("/relay")
	relayRouter.Use(middleware.StatsMiddleware(), middleware.RouteTag("relay"))
	relayRouter.GET("/stats", countedHandler)

	videoRouter := engine.Group("/video")
	videoRouter.Use(middleware.StatsMiddleware(), middleware.RouteTag("relay"))
	videoRouter.GET("/stats", countedHandler)

	setWebRouter(engine, static.LocalFile(t.TempDir(), false), []byte("index"))
	engine.Use(func(c *gin.Context) {
		c.Next()
		if c.Request.URL.Path == "/web-stats" {
			recordObservation(c)
		}
	})

	baseline := middleware.GetStats().ActiveConnections
	tests := []struct {
		name       string
		path       string
		statusCode int
		routeTag   string
	}{
		{name: "web", path: "/web-stats", statusCode: http.StatusOK, routeTag: "web"},
		{name: "relay", path: "/relay/stats", statusCode: http.StatusNoContent, routeTag: "relay"},
		{name: "video", path: "/video/stats", statusCode: http.StatusNoContent, routeTag: "relay"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))

			require.Equal(t, test.statusCode, recorder.Code)
			observed, ok := observations[test.path]
			require.True(t, ok, "route handler did not record an observation")
			require.Equal(t, baseline+1, observed.activeConnections)
			require.Equal(t, test.routeTag, observed.routeTag)
			require.Equal(t, baseline, middleware.GetStats().ActiveConnections)
		})
	}
}
