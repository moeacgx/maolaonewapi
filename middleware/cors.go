package middleware

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORS protects cookie-authenticated dashboard and API routes. Cross-origin
// credentials are accepted only from an exact deployment or operator-trusted
// origin; bearer-key relay routes must use RelayCORS instead.
func CORS() gin.HandlerFunc {
	config := cors.DefaultConfig()
	config.AllowCredentials = true
	config.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	config.AllowHeaders = []string{
		"Accept", "Authorization", "Cache-Control", "Content-Length", "Content-Type",
		"Anthropic-Beta", "Anthropic-Version", "Mj-Api-Secret", "New-API-User", "Origin",
		"X-API-Key", "X-Auth-Session", "X-Goog-Api-Key", "X-Requested-With", "X-Security-Proof",
	}
	config.AllowOriginWithContextFunc = func(c *gin.Context, origin string) bool {
		return isAllowedCredentialOrigin(c, origin)
	}
	return cors.New(config)
}

// RelayCORS permits browser SDKs to call bearer-key relay endpoints from any
// origin without ever enabling cookies or other browser credentials.
func RelayCORS() gin.HandlerFunc {
	config := cors.Config{
		AllowAllOrigins: true,
		AllowMethods:    []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{
			"Accept", "Accept-Encoding", "Anthropic-Beta", "Anthropic-Dangerous-Direct-Browser-Access",
			"Anthropic-Version", "Authorization", "Cache-Control", "Content-Encoding", "Content-Length",
			"Content-Type", "Idempotency-Key", "Mj-Api-Secret", "New-API-User", "OpenAI-Beta",
			"OpenAI-Organization", "OpenAI-Project", "Origin", "Sec-WebSocket-Protocol", "X-API-Key",
			"X-Auth-Session", "X-Goog-Api-Client", "X-Goog-Api-Key", "X-Requested-With", "X-Security-Proof",
			"X-Stainless-Arch", "X-Stainless-Async", "X-Stainless-Custom-Poll-Interval",
			"X-Stainless-Event-Id", "X-Stainless-Helper-Method", "X-Stainless-Lang", "X-Stainless-OS",
			"X-Stainless-Package-Version", "X-Stainless-Poll-Helper", "X-Stainless-Raw-Response",
			"X-Stainless-Read-Timeout", "X-Stainless-Retry-Count", "X-Stainless-Runtime",
			"X-Stainless-Runtime-Version", "X-Stainless-Timeout",
		},
		ExposeHeaders: []string{
			"Content-Encoding", "Content-Length", "Content-Type", "OpenAI-Organization",
			"OpenAI-Processing-Ms", "OpenAI-Version", "Request-Id", "X-New-Api-Version", "X-Request-Id",
		},
	}
	return cors.New(config)
}

func isAllowedCredentialOrigin(c *gin.Context, origin string) bool {
	if origin == "" {
		return true
	}
	normalizedOrigin, err := common.NormalizeOrigin(origin)
	if err != nil {
		return false
	}

	requestOrigin, hasRequestOrigin := credentialRequestOrigin(c)
	if hasRequestOrigin && normalizedOrigin == requestOrigin {
		return true
	}
	for _, configuredOrigin := range common.SessionCookieTrustedURLs {
		normalizedConfiguredOrigin, err := common.NormalizeOrigin(configuredOrigin)
		if err == nil && normalizedOrigin == normalizedConfiguredOrigin {
			return true
		}
	}
	if hasRequestOrigin && isAllowedLocalDevelopmentOrigin(normalizedOrigin, requestOrigin) {
		return true
	}
	return false
}

func credentialRequestOrigin(c *gin.Context) (string, bool) {
	if c == nil || c.Request == nil || c.Request.Host == "" {
		return "", false
	}
	requestScheme := "http"
	if c.Request.TLS != nil {
		requestScheme = "https"
	}
	requestOrigin, err := common.NormalizeOrigin(requestScheme + "://" + c.Request.Host)
	return requestOrigin, err == nil
}

func isAllowedLocalDevelopmentOrigin(origin, requestOrigin string) bool {
	switch requestOrigin {
	case "http://localhost:3000":
		return origin == "http://localhost:5173"
	case "http://127.0.0.1:3000":
		return origin == "http://127.0.0.1:5173"
	case "http://[::1]:3000":
		return origin == "http://[::1]:5173"
	default:
		return false
	}
}

func Version() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-New-Api-Version", common.Version)
		c.Next()
	}
}
