package middleware

import (
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func CORS() gin.HandlerFunc {
	config := cors.DefaultConfig()
	config.AllowCredentials = true
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
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

	parsedOrigin, err := url.Parse(normalizedOrigin)
	if err != nil || parsedOrigin.Scheme != "https" || parsedOrigin.Port() != "" {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(parsedOrigin.Hostname(), "."))
	return host == "maolaoapi.com" || strings.HasSuffix(host, ".maolaoapi.com")
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
