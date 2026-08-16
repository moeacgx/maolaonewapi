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

func TestCORSPreflightAllowsCanvasCredentialHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORS())
	router.POST("/canvas/v1/images/generations", func(c *gin.Context) { c.Status(http.StatusOK) })
	request := httptest.NewRequest(http.MethodOptions, "https://api.example.test/canvas/v1/images/generations?group=Image2", nil)
	request.Header.Set("Origin", "https://canvas.maolaoapi.com")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "anthropic-beta,anthropic-version,authorization,content-type,mj-api-secret,new-api-user,x-api-key,x-auth-session,x-goog-api-key,x-requested-with,x-security-proof")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.Equal(t, "true", recorder.Header().Get("Access-Control-Allow-Credentials"))
	require.Equal(t, "https://canvas.maolaoapi.com", recorder.Header().Get("Access-Control-Allow-Origin"))
	allowedHeaders := strings.ToLower(recorder.Header().Get("Access-Control-Allow-Headers"))
	for _, header := range []string{"anthropic-beta", "anthropic-version", "authorization", "content-type", "mj-api-secret", "new-api-user", "x-api-key", "x-auth-session", "x-goog-api-key", "x-requested-with", "x-security-proof"} {
		require.Contains(t, allowedHeaders, header)
	}
}

func TestCORSRestrictsMaolaoAPIDeploymentOrigins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, testCase := range []struct {
		name    string
		origin  string
		allowed bool
	}{
		{name: "root", origin: "https://maolaoapi.com", allowed: true},
		{name: "subdomain", origin: "https://canvas.maolaoapi.com", allowed: true},
		{name: "normalized default port", origin: "https://canvas.maolaoapi.com:443", allowed: true},
		{name: "wrong scheme", origin: "http://canvas.maolaoapi.com"},
		{name: "arbitrary port", origin: "https://canvas.maolaoapi.com:8443"},
		{name: "root suffix attack", origin: "https://maolaoapi.com.attacker.example"},
		{name: "prefix attack", origin: "https://evilmaolaoapi.com"},
		{name: "subdomain suffix attack", origin: "https://canvas.maolaoapi.com.evil.test"},
		{name: "path", origin: "https://maolaoapi.com/path"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			router := gin.New()
			router.Use(CORS())
			router.GET("/api", func(c *gin.Context) { c.Status(http.StatusOK) })
			request := httptest.NewRequest(http.MethodGet, "https://api.example.test/api", nil)
			request.Header.Set("Origin", testCase.origin)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if testCase.allowed {
				require.Equal(t, testCase.origin, recorder.Header().Get("Access-Control-Allow-Origin"))
			} else {
				require.Empty(t, recorder.Header().Get("Access-Control-Allow-Origin"))
			}
		})
	}
}

func TestCORSMatchesNormalizedRequestOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, testCase := range []struct {
		name           string
		target         string
		origin         string
		forwardedProto string
		allowed        bool
	}{
		{name: "exact HTTPS origin", target: "https://api.example.test/api", origin: "https://api.example.test", allowed: true},
		{name: "same host wrong scheme", target: "https://api.example.test/api", origin: "http://api.example.test"},
		{name: "same host wrong port", target: "https://api.example.test/api", origin: "https://api.example.test:8443"},
		{name: "HTTPS default port normalization", target: "https://api.example.test/api", origin: "https://api.example.test:443", allowed: true},
		{name: "HTTP default port normalization", target: "http://api.example.test/api", origin: "http://api.example.test:80", allowed: true},
		{name: "forwarded scheme is not authoritative", target: "http://api.example.test/api", origin: "https://api.example.test", forwardedProto: "https"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, testCase.target, nil)
			if testCase.forwardedProto != "" {
				request.Header.Set("X-Forwarded-Proto", testCase.forwardedProto)
			}
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = request
			require.Equal(t, testCase.allowed, isAllowedCredentialOrigin(context, testCase.origin))
		})
	}
}

func TestCORSAllowsOnlyExplicitLocalDevelopmentOriginPairs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, testCase := range []struct {
		name    string
		target  string
		origin  string
		allowed bool
	}{
		{name: "localhost Vite frontend", target: "http://localhost:3000/api", origin: "http://localhost:5173", allowed: true},
		{name: "IPv4 loopback Vite frontend", target: "http://127.0.0.1:3000/api", origin: "http://127.0.0.1:5173", allowed: true},
		{name: "IPv6 loopback Vite frontend", target: "http://[::1]:3000/api", origin: "http://[::1]:5173", allowed: true},
		{name: "localhost cannot call remote deployment", target: "https://api.example.test/api", origin: "http://localhost:5173"},
		{name: "unlisted localhost frontend port", target: "http://localhost:3000/api", origin: "http://localhost:4173"},
		{name: "mixed loopback hosts", target: "http://127.0.0.1:3000/api", origin: "http://localhost:5173"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			router := gin.New()
			router.Use(CORS())
			router.GET("/api", func(c *gin.Context) { c.Status(http.StatusOK) })
			request := httptest.NewRequest(http.MethodGet, testCase.target, nil)
			request.Header.Set("Origin", testCase.origin)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if testCase.allowed {
				require.Equal(t, testCase.origin, recorder.Header().Get("Access-Control-Allow-Origin"))
			} else {
				require.Empty(t, recorder.Header().Get("Access-Control-Allow-Origin"))
			}
		})
	}
}

func TestCORSAllowsExactNormalizedConfiguredTrustedOrigins(t *testing.T) {
	previousTrustedOrigins := common.SessionCookieTrustedURLs
	common.SessionCookieTrustedURLs = []string{"https://trusted.example.test:443", "https://admin.example.test:8443"}
	t.Cleanup(func() { common.SessionCookieTrustedURLs = previousTrustedOrigins })
	gin.SetMode(gin.TestMode)

	for _, testCase := range []struct {
		name    string
		origin  string
		allowed bool
	}{
		{name: "default port normalization", origin: "https://trusted.example.test", allowed: true},
		{name: "exact nondefault port", origin: "https://admin.example.test:8443", allowed: true},
		{name: "configured host wrong scheme", origin: "http://trusted.example.test"},
		{name: "configured host wrong port", origin: "https://admin.example.test"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			router := gin.New()
			router.Use(CORS())
			router.GET("/api", func(c *gin.Context) { c.Status(http.StatusOK) })
			request := httptest.NewRequest(http.MethodGet, "https://api.example.test/api", nil)
			request.Header.Set("Origin", testCase.origin)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if testCase.allowed {
				require.Equal(t, testCase.origin, recorder.Header().Get("Access-Control-Allow-Origin"))
			} else {
				require.Empty(t, recorder.Header().Get("Access-Control-Allow-Origin"))
			}
		})
	}
}

func TestCORSPreflightRejectsHeadersOutsideExplicitAllowlist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORS())
	router.POST("/v1/chat/completions", func(c *gin.Context) { c.Status(http.StatusOK) })

	request := httptest.NewRequest(http.MethodOptions, "https://api.example.test/v1/chat/completions", nil)
	request.Header.Set("Origin", "https://canvas.maolaoapi.com")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "x-not-allowed")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.Equal(t, "https://canvas.maolaoapi.com", recorder.Header().Get("Access-Control-Allow-Origin"))
	require.NotContains(t, strings.ToLower(recorder.Header().Get("Access-Control-Allow-Headers")), "x-not-allowed")
}
