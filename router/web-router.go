package router

import (
	"embed"
	"io"
	"net/http"
	"path"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

// ThemeAssets holds the embedded dashboard frontend assets.
type ThemeAssets struct {
	DefaultBuildFS   embed.FS
	DefaultIndexPage []byte
	ClassicBuildFS   embed.FS
	ClassicIndexPage []byte
}

type WebAssets = ThemeAssets

type currentWebAssetPaths struct {
	defaultIndexJS  string
	defaultIndexCSS string
	classicIndexJS  string
	classicIndexCSS string
}

var (
	indexJSAssetPattern  = regexp.MustCompile(`/assets/index-[^"']+\.js`)
	indexCSSAssetPattern = regexp.MustCompile(`/assets/index-[^"']+\.css`)
)

func SetWebRouter(router *gin.Engine, assets ThemeAssets) {
	setThemeWebRouter(
		router,
		common.EmbedFolder(assets.DefaultBuildFS, "web/dist"),
		assets.DefaultIndexPage,
		common.EmbedFolder(assets.ClassicBuildFS, "web/classic/dist"),
		assets.ClassicIndexPage,
	)
}

func setWebRouter(router *gin.Engine, frontendFS static.ServeFileSystem, indexPage []byte) {
	setThemeWebRouter(router, frontendFS, indexPage, frontendFS, indexPage)
}

func setThemeWebRouter(router *gin.Engine, defaultFS static.ServeFileSystem, defaultIndexPage []byte, classicFS static.ServeFileSystem, classicIndexPage []byte) {
	frontendFS := common.NewThemeAwareFS(defaultFS, classicFS)
	currentAssets := currentWebAssetPaths{
		defaultIndexJS:  findIndexAssetPath(defaultIndexPage, indexJSAssetPattern),
		defaultIndexCSS: findIndexAssetPath(defaultIndexPage, indexCSSAssetPattern),
		classicIndexJS:  findIndexAssetPath(classicIndexPage, indexJSAssetPattern),
		classicIndexCSS: findIndexAssetPath(classicIndexPage, indexCSSAssetPattern),
	}

	router.Use(middleware.StatsMiddleware())
	router.Use(gzip.Gzip(gzip.DefaultCompression))
	router.Use(middleware.GlobalWebRateLimitWithAssetChecker(func(request *http.Request) bool {
		return isRealStaticWebAssetRequest(request, frontendFS)
	}))
	router.Use(middleware.Cache())
	router.Use(static.Serve("/", frontendFS))
	router.NoRoute(pathAwareCORS(), func(c *gin.Context) {
		c.Set(middleware.RouteTagKey, "web")
		if serveCurrentIndexAssetFallback(c, frontendFS, currentAssets) {
			return
		}
		if strings.HasPrefix(c.Request.RequestURI, "/v1") || strings.HasPrefix(c.Request.RequestURI, "/api") || strings.HasPrefix(c.Request.RequestURI, "/assets") {
			controller.RelayNotFound(c)
			return
		}
		c.Header("Cache-Control", "no-cache")
		if common.GetTheme() == "classic" {
			c.Data(http.StatusOK, "text/html; charset=utf-8", classicIndexPage)
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", defaultIndexPage)
	})
}

func findIndexAssetPath(indexPage []byte, pattern *regexp.Regexp) string {
	return pattern.FindString(string(indexPage))
}

func currentIndexAssetPath(requestPath string, assets currentWebAssetPaths) string {
	if common.GetTheme() == "classic" {
		if requestPath != "" && requestPath == assets.defaultIndexJS {
			return assets.classicIndexJS
		}
		if requestPath != "" && requestPath == assets.defaultIndexCSS {
			return assets.classicIndexCSS
		}
		return ""
	}

	if requestPath != "" && requestPath == assets.classicIndexJS {
		return assets.defaultIndexJS
	}
	if requestPath != "" && requestPath == assets.classicIndexCSS {
		return assets.defaultIndexCSS
	}
	return ""
}

func serveCurrentIndexAssetFallback(c *gin.Context, frontendFS static.ServeFileSystem, assets currentWebAssetPaths) bool {
	requestPath := c.Request.URL.Path
	currentPath := currentIndexAssetPath(requestPath, assets)
	if currentPath == "" || currentPath == requestPath {
		return false
	}

	file, err := frontendFS.Open(currentPath)
	if err != nil {
		return false
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return false
	}

	contentType := "application/octet-stream"
	if strings.HasSuffix(currentPath, ".js") {
		contentType = "text/javascript; charset=utf-8"
	} else if strings.HasSuffix(currentPath, ".css") {
		contentType = "text/css; charset=utf-8"
	}

	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, contentType, data)
	return true
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
