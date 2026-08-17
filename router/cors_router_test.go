package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
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

	t.Run("dashboard uses strict credential policy", func(t *testing.T) {
		engine := newCORSTestEngine()
		SetDashboardRouter(engine)
		allowed := servePreflight(engine, "/dashboard/billing/usage", "https://console.example.test", http.MethodGet, "authorization")
		rejected := servePreflight(engine, "/dashboard/billing/usage", "https://third-party.example", http.MethodGet, "authorization")

		require.Equal(t, http.StatusNoContent, allowed.Code)
		require.Equal(t, "https://console.example.test", allowed.Header().Get("Access-Control-Allow-Origin"))
		require.Equal(t, "true", allowed.Header().Get("Access-Control-Allow-Credentials"))
		require.Equal(t, http.StatusForbidden, rejected.Code)
		require.Empty(t, rejected.Header().Get("Access-Control-Allow-Origin"))
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
