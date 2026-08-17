package router

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetRouter(router *gin.Engine, assets WebAssets) {
	router.Use(corsPreflightBoundary())
	SetApiRouter(router)
	SetDashboardRouter(router)
	SetRelayRouter(router)
	SetVideoRouter(router)
	frontendBaseUrl := os.Getenv("FRONTEND_BASE_URL")
	if common.IsMasterNode && frontendBaseUrl != "" {
		frontendBaseUrl = ""
		common.SysLog("FRONTEND_BASE_URL is ignored on master node")
	}
	if frontendBaseUrl == "" {
		SetWebRouter(router, assets)
	} else {
		frontendBaseUrl = strings.TrimSuffix(frontendBaseUrl, "/")
		router.NoRoute(func(c *gin.Context) {
			c.Set(middleware.RouteTagKey, "web")
			c.Redirect(http.StatusMovedPermanently, fmt.Sprintf("%s%s", frontendBaseUrl, c.Request.RequestURI))
		})
	}
}

func corsPreflightBoundary() gin.HandlerFunc {
	strictCORS := middleware.CORS()
	relayCORS := middleware.RelayCORS()
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodOptions {
			c.Next()
			return
		}
		path := c.Request.URL.Path
		switch {
		case isStrictCORSPath(path):
			strictCORS(c)
		case isRelayCORSPath(path):
			relayCORS(c)
		default:
			c.Next()
		}
	}
}

func isStrictCORSPath(path string) bool {
	return path == "/api" || strings.HasPrefix(path, "/api/") ||
		path == "/dashboard" || strings.HasPrefix(path, "/dashboard/") ||
		path == "/v1/dashboard" || strings.HasPrefix(path, "/v1/dashboard/") ||
		path == "/pg" || strings.HasPrefix(path, "/pg/")
}

func isRelayCORSPath(path string) bool {
	if path == "/v1" || strings.HasPrefix(path, "/v1/") ||
		path == "/v1beta" || strings.HasPrefix(path, "/v1beta/") ||
		path == "/mj" || strings.HasPrefix(path, "/mj/") ||
		path == "/suno" || strings.HasPrefix(path, "/suno/") ||
		path == "/kling/v1" || strings.HasPrefix(path, "/kling/v1/") ||
		path == "/jimeng" || strings.HasPrefix(path, "/jimeng/") {
		return true
	}
	segments := strings.Split(strings.Trim(path, "/"), "/")
	return len(segments) >= 2 && segments[1] == "mj"
}
