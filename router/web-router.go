package router

import (
	"embed"
	"net/http"
	"path"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

// WebAssets holds the embedded dashboard frontend assets.
type WebAssets struct {
	BuildFS   embed.FS
	IndexPage []byte
}

func SetWebRouter(router *gin.Engine, assets WebAssets) {
	setWebRouter(router, common.EmbedFolder(assets.BuildFS, "web/dist"), assets.IndexPage)
}

func setWebRouter(router *gin.Engine, frontendFS static.ServeFileSystem, indexPage []byte) {
	router.Use(middleware.StatsMiddleware())
	router.Use(gzip.Gzip(gzip.DefaultCompression))
	router.Use(middleware.GlobalWebRateLimitWithAssetChecker(func(request *http.Request) bool {
		return isRealStaticWebAssetRequest(request, frontendFS)
	}))
	router.Use(middleware.Cache())
	router.Use(static.Serve("/", frontendFS))
	router.NoRoute(func(c *gin.Context) {

		c.Set(middleware.RouteTagKey, "web")
		if strings.HasPrefix(c.Request.RequestURI, "/v1") || strings.HasPrefix(c.Request.RequestURI, "/api") || strings.HasPrefix(c.Request.RequestURI, "/assets") {
			controller.RelayNotFound(c)
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexPage)
	})
}
func isRealStaticWebAssetRequest(request *http.Request, frontendFS static.ServeFileSystem) bool {
	if request == nil || request.URL == nil || frontendFS == nil {
		return false
	}
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		return false
	}
	extension := strings.ToLower(path.Ext(request.URL.Path))
	if extension == "" || extension == ".html" || extension == ".htm" {
		return false
	}
	return frontendFS.Exists("/", request.URL.Path)
}
