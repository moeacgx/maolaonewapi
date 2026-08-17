package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCORSActualRequestsAllowOnlyStrictCredentialOrigins(t *testing.T) {
	previousTrustedOrigins := common.SessionCookieTrustedURLs
	common.SessionCookieTrustedURLs = []string{
		"https://trusted.example.test:443",
		"https://admin.example.test:8443",
	}
	t.Cleanup(func() { common.SessionCookieTrustedURLs = previousTrustedOrigins })
	gin.SetMode(gin.TestMode)

	for _, testCase := range []struct {
		name        string
		target      string
		origin      string
		allowed     bool
		corsHeaders bool
	}{
		{name: "same deployment", target: "https://api.example.test/api/status", origin: "https://api.example.test", allowed: true},
		{name: "same deployment default port", target: "https://api.example.test/api/status", origin: "https://api.example.test:443", allowed: true, corsHeaders: true},
		{name: "configured origin normalized", target: "https://api.example.test/api/status", origin: "https://trusted.example.test", allowed: true, corsHeaders: true},
		{name: "configured nondefault port", target: "https://api.example.test/api/status", origin: "https://admin.example.test:8443", allowed: true, corsHeaders: true},
		{name: "localhost development pair", target: "http://localhost:3000/api/status", origin: "http://localhost:5173", allowed: true, corsHeaders: true},
		{name: "IPv4 development pair", target: "http://127.0.0.1:3000/api/status", origin: "http://127.0.0.1:5173", allowed: true, corsHeaders: true},
		{name: "IPv6 development pair", target: "http://[::1]:3000/api/status", origin: "http://[::1]:5173", allowed: true, corsHeaders: true},
		{name: "compiled-in root is rejected", target: "https://api.example.test/api/status", origin: "https://maolaoapi.com"},
		{name: "compiled-in subdomain is rejected", target: "https://api.example.test/api/status", origin: "https://canvas.maolaoapi.com"},
		{name: "deployment suffix confusion", target: "https://api.example.test/api/status", origin: "https://api.example.test.attacker.invalid"},
		{name: "configured suffix confusion", target: "https://api.example.test/api/status", origin: "https://trusted.example.test.attacker.invalid"},
		{name: "configured prefix confusion", target: "https://api.example.test/api/status", origin: "https://eviltrusted.example.test"},
		{name: "configured userinfo confusion", target: "https://api.example.test/api/status", origin: "https://trusted.example.test@attacker.invalid"},
		{name: "wrong configured scheme", target: "https://api.example.test/api/status", origin: "http://trusted.example.test"},
		{name: "wrong configured port", target: "https://api.example.test/api/status", origin: "https://admin.example.test"},
		{name: "localhost cannot call deployment", target: "https://api.example.test/api/status", origin: "http://localhost:5173"},
		{name: "unlisted localhost frontend port", target: "http://localhost:3000/api/status", origin: "http://localhost:4173"},
		{name: "mixed loopback hosts", target: "http://127.0.0.1:3000/api/status", origin: "http://localhost:5173"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			router := gin.New()
			router.Use(CORS())
			router.GET("/api/status", func(c *gin.Context) { c.Status(http.StatusOK) })
			request := httptest.NewRequest(http.MethodGet, testCase.target, nil)
			request.Header.Set("Origin", testCase.origin)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, request)

			if testCase.allowed {
				require.Equal(t, http.StatusOK, recorder.Code)
				if testCase.corsHeaders {
					require.Equal(t, testCase.origin, recorder.Header().Get("Access-Control-Allow-Origin"))
					require.Equal(t, "true", recorder.Header().Get("Access-Control-Allow-Credentials"))
				} else {
					require.Empty(t, recorder.Header().Get("Access-Control-Allow-Origin"))
				}
			} else {
				require.Equal(t, http.StatusForbidden, recorder.Code)
				require.Empty(t, recorder.Header().Get("Access-Control-Allow-Origin"))
			}
		})
	}
}

func TestCORSPreflightUsesExactConfiguredOrigin(t *testing.T) {
	previousTrustedOrigins := common.SessionCookieTrustedURLs
	common.SessionCookieTrustedURLs = []string{"https://console.example.test"}
	t.Cleanup(func() { common.SessionCookieTrustedURLs = previousTrustedOrigins })
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORS())
	router.POST("/api/option", func(c *gin.Context) { c.Status(http.StatusOK) })
	request := httptest.NewRequest(http.MethodOptions, "https://api.example.test/api/option", nil)
	request.Header.Set("Origin", "https://console.example.test")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "authorization,content-type,x-auth-session,x-security-proof")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.Equal(t, "https://console.example.test", recorder.Header().Get("Access-Control-Allow-Origin"))
	require.NotEqual(t, "*", recorder.Header().Get("Access-Control-Allow-Origin"))
	require.Equal(t, "true", recorder.Header().Get("Access-Control-Allow-Credentials"))
}

func TestRelayCORSPreflightAllowsThirdPartyBrowserSDKHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RelayCORS())
	router.POST("/v1/chat/completions", func(c *gin.Context) { c.Status(http.StatusOK) })
	requiredHeaders := []string{
		"anthropic-beta",
		"anthropic-dangerous-direct-browser-access",
		"anthropic-version",
		"authorization",
		"content-encoding",
		"content-type",
		"idempotency-key",
		"openai-beta",
		"openai-organization",
		"openai-project",
		"x-api-key",
		"x-goog-api-client",
		"x-goog-api-key",
		"x-stainless-arch",
		"x-stainless-async",
		"x-stainless-helper-method",
		"x-stainless-lang",
		"x-stainless-custom-poll-interval",
		"x-stainless-event-id",
		"x-stainless-os",
		"x-stainless-package-version",
		"x-stainless-poll-helper",
		"x-stainless-read-timeout",
		"x-stainless-retry-count",
		"x-stainless-runtime",
		"x-stainless-runtime-version",
		"x-stainless-raw-response",
		"x-stainless-timeout",
	}
	request := httptest.NewRequest(http.MethodOptions, "https://api.example.test/v1/chat/completions", nil)
	request.Header.Set("Origin", "https://third-party.example")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", strings.Join(requiredHeaders, ","))
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.Equal(t, "*", recorder.Header().Get("Access-Control-Allow-Origin"))
	require.Empty(t, recorder.Header().Get("Access-Control-Allow-Credentials"))
	allowedHeaders := strings.ToLower(recorder.Header().Get("Access-Control-Allow-Headers"))
	for _, header := range requiredHeaders {
		require.Contains(t, allowedHeaders, header)
	}
}

func TestRelayCORSActualRequestNeverApprovesBrowserCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RelayCORS())
	handlerCalled := false
	router.GET("/v1/models", func(c *gin.Context) {
		handlerCalled = true
		c.Status(http.StatusOK)
	})
	request := httptest.NewRequest(http.MethodGet, "https://api.example.test/v1/models", nil)
	request.Header.Set("Origin", "https://third-party.example")
	request.Header.Set("Authorization", "Bearer relay-key")
	request.Header.Set("Cookie", "session=must-not-be-approved-by-cors")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	require.True(t, handlerCalled)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "*", recorder.Header().Get("Access-Control-Allow-Origin"))
	require.Empty(t, recorder.Header().Get("Access-Control-Allow-Credentials"))
}

func TestRelayCORSPreflightDoesNotAllowCookieHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RelayCORS())
	router.POST("/v1/messages", func(c *gin.Context) { c.Status(http.StatusOK) })
	request := httptest.NewRequest(http.MethodOptions, "https://api.example.test/v1/messages", nil)
	request.Header.Set("Origin", "https://third-party.example")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "cookie")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.Equal(t, "*", recorder.Header().Get("Access-Control-Allow-Origin"))
	require.Empty(t, recorder.Header().Get("Access-Control-Allow-Credentials"))
	require.NotContains(t, strings.ToLower(recorder.Header().Get("Access-Control-Allow-Headers")), "cookie")
}
