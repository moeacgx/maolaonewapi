package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRouterCORSBoundaries(t *testing.T) {
	previousTrustedOrigins := common.SessionCookieTrustedURLs
	common.SessionCookieTrustedURLs = []string{"https://console.example.test"}
	t.Cleanup(func() { common.SessionCookieTrustedURLs = previousTrustedOrigins })
	gin.SetMode(gin.TestMode)

	t.Run("API rejects untrusted browser origin", func(t *testing.T) {
		engine := newCORSTestEngine()
		SetApiRouter(engine)
		recorder := servePreflight(engine, "/api/status", "https://third-party.example", http.MethodGet, "content-type")

		require.Equal(t, http.StatusForbidden, recorder.Code)
		require.Empty(t, recorder.Header().Get("Access-Control-Allow-Origin"))
		require.Empty(t, recorder.Header().Get("Access-Control-Allow-Credentials"))
	})

	t.Run("API allows exact configured credential origin", func(t *testing.T) {
		engine := newCORSTestEngine()
		SetApiRouter(engine)
		recorder := servePreflight(engine, "/api/status", "https://console.example.test", http.MethodGet, "content-type")

		require.Equal(t, http.StatusNoContent, recorder.Code)
		require.Equal(t, "https://console.example.test", recorder.Header().Get("Access-Control-Allow-Origin"))
		require.Equal(t, "true", recorder.Header().Get("Access-Control-Allow-Credentials"))
		require.NotEqual(t, "*", recorder.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("adjacent dashboard preflight remains strict", func(t *testing.T) {
		engine := newCORSTestEngine()
		recorder := servePreflight(engine, "/v1/dashboard/profile", "https://third-party.example", http.MethodGet, "authorization")

		require.Equal(t, http.StatusForbidden, recorder.Code)
		require.Empty(t, recorder.Header().Get("Access-Control-Allow-Origin"))
		require.Empty(t, recorder.Header().Get("Access-Control-Allow-Credentials"))
	})

	t.Run("bearer browser endpoints use relay policy", func(t *testing.T) {
		bearerPaths := []string{
			"/dashboard/billing/subscription",
			"/dashboard/billing/usage",
			"/v1/dashboard/billing/subscription",
			"/v1/dashboard/billing/usage",
			"/api/usage/token/",
			"/api/log/token",
		}
		for _, path := range bearerPaths {
			require.True(t, middleware.IsBearerBrowserPath(path), "missing bearer inventory entry: %s", path)
			engine := newCORSTestEngine()
			if strings.HasPrefix(path, "/api/") {
				SetApiRouter(engine)
			} else {
				SetDashboardRouter(engine)
			}
			preflight := servePreflight(engine, path, "https://third-party.example", http.MethodGet, "authorization,content-type,openai-project,x-stainless-runtime")
			require.Equal(t, http.StatusNoContent, preflight.Code, path)
			require.Equal(t, "*", preflight.Header().Get("Access-Control-Allow-Origin"), path)
			require.Empty(t, preflight.Header().Get("Access-Control-Allow-Credentials"), path)

			actualEngine := gin.New()
			if strings.HasPrefix(path, "/api/") {
				actualEngine.Use(middleware.APIPathCORS())
			} else {
				actualEngine.Use(middleware.RelayCORS())
			}
			actualEngine.GET(path, func(c *gin.Context) { c.Status(http.StatusNoContent) })
			request := httptest.NewRequest(http.MethodGet, "https://api.example.test"+path, nil)
			request.Header.Set("Origin", "https://third-party.example")
			recorder := httptest.NewRecorder()
			actualEngine.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusNoContent, recorder.Code, path)
			require.Equal(t, "*", recorder.Header().Get("Access-Control-Allow-Origin"), path)
			require.Empty(t, recorder.Header().Get("Access-Control-Allow-Credentials"), path)
		}
		for _, path := range []string{"/api/status", "/pg/chat/completions", "/v1/dashboard/profile"} {
			require.False(t, middleware.IsBearerBrowserPath(path), "cookie route entered bearer inventory: %s", path)
		}
	})

	t.Run("cookie playground keeps strict credential policy", func(t *testing.T) {
		engine := newCORSTestEngine()
		SetRelayRouter(engine)
		allowed := servePreflight(engine, "/pg/chat/completions", "https://console.example.test", http.MethodPost, "authorization,content-type")
		rejected := servePreflight(engine, "/pg/chat/completions", "https://third-party.example", http.MethodPost, "authorization,content-type")

		require.Equal(t, http.StatusNoContent, allowed.Code)
		require.Equal(t, "https://console.example.test", allowed.Header().Get("Access-Control-Allow-Origin"))
		require.Equal(t, "true", allowed.Header().Get("Access-Control-Allow-Credentials"))
		require.Equal(t, http.StatusForbidden, rejected.Code)
		require.Empty(t, rejected.Header().Get("Access-Control-Allow-Origin"))
	})

	for _, path := range []string{
		"/v1/chat/completions",
		"/v1beta/models/gemini-2.5-pro:generateContent",
	} {
		t.Run("relay allows third-party bearer browser for "+path, func(t *testing.T) {
			engine := newCORSTestEngine()
			SetRelayRouter(engine)
			recorder := servePreflight(engine, path, "https://third-party.example", http.MethodPost, "authorization,content-type,openai-project,x-goog-api-key,x-stainless-runtime")

			require.Equal(t, http.StatusNoContent, recorder.Code)
			require.Equal(t, "*", recorder.Header().Get("Access-Control-Allow-Origin"))
			require.Empty(t, recorder.Header().Get("Access-Control-Allow-Credentials"))
		})
	}
}

func TestLocalWebNoRouteUsesPathAwareCORS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := newCORSTestEngine()
	setWebRouter(engine, static.LocalFile(t.TempDir(), false), []byte("index"))

	t.Run("unmatched relay is wildcard without credentials", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/v1/missing", nil)
		request.Header.Set("Origin", "https://third-party.example")
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusNotFound, recorder.Code)
		require.Equal(t, "*", recorder.Header().Get("Access-Control-Allow-Origin"))
		require.Empty(t, recorder.Header().Get("Access-Control-Allow-Credentials"))
	})

	t.Run("unmatched strict API is rejected", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/api/missing", nil)
		request.Header.Set("Origin", "https://third-party.example")
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusForbidden, recorder.Code)
		require.Empty(t, recorder.Header().Get("Access-Control-Allow-Origin"))
		require.Empty(t, recorder.Header().Get("Access-Control-Allow-Credentials"))
	})
}

func TestRedirectNoRouteUsesPathAwareCORS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousMasterNode := common.IsMasterNode
	common.IsMasterNode = false
	t.Cleanup(func() { common.IsMasterNode = previousMasterNode })
	t.Setenv("FRONTEND_BASE_URL", "https://frontend.example.test")
	engine := gin.New()
	SetRouter(engine, ThemeAssets{})

	t.Run("unmatched relay redirects with wildcard without credentials", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/v1/missing", nil)
		request.Header.Set("Origin", "https://third-party.example")
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusMovedPermanently, recorder.Code)
		require.Equal(t, "https://frontend.example.test/v1/missing", recorder.Header().Get("Location"))
		require.Equal(t, "*", recorder.Header().Get("Access-Control-Allow-Origin"))
		require.Empty(t, recorder.Header().Get("Access-Control-Allow-Credentials"))
	})

	t.Run("strict path remains strict", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/api/missing", nil)
		request.Header.Set("Origin", "https://third-party.example")
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusForbidden, recorder.Code)
		require.Empty(t, recorder.Header().Get("Location"))
		require.Empty(t, recorder.Header().Get("Access-Control-Allow-Origin"))
		require.Empty(t, recorder.Header().Get("Access-Control-Allow-Credentials"))
	})
}

func TestPlaygroundActualRequestDoesNotInheritRelayCORS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := newCORSTestEngine()
	SetRelayRouter(engine)

	request := httptest.NewRequest(http.MethodPost, "https://api.example.test/pg/chat/completions", strings.NewReader(`{"message":"test"}`))
	request.Header.Set("Origin", "https://third-party.example")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Empty(t, recorder.Header().Get("Access-Control-Allow-Origin"))
	require.Empty(t, recorder.Header().Get("Access-Control-Allow-Credentials"))
}

func TestVideoRouterKeepsRelayTransportMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := newCORSTestEngine()
	SetVideoRouter(engine)

	request := httptest.NewRequest(http.MethodPost, "https://api.example.test/v1/video/generations", strings.NewReader("not-gzip"))
	request.Header.Set("Origin", "https://third-party.example")
	request.Header.Set("Content-Encoding", "gzip")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Equal(t, "*", recorder.Header().Get("Access-Control-Allow-Origin"))
}

func TestImageTaskRouteKeepsRelayTransportMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := newCORSTestEngine()
	SetRelayRouter(engine)

	request := httptest.NewRequest(http.MethodPost, "https://api.example.test/v1/images/tasks?action=generations", strings.NewReader("not-gzip"))
	request.Header.Set("Origin", "https://third-party.example")
	request.Header.Set("Content-Encoding", "gzip")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Equal(t, "*", recorder.Header().Get("Access-Control-Allow-Origin"))
}

func newCORSTestEngine() *gin.Engine {
	engine := gin.New()
	engine.Use(corsPreflightBoundary())
	return engine
}

func servePreflight(engine *gin.Engine, path, origin, method, headers string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodOptions, "https://api.example.test"+path, nil)
	request.Header.Set("Origin", origin)
	request.Header.Set("Access-Control-Request-Method", method)
	request.Header.Set("Access-Control-Request-Headers", headers)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder
}
