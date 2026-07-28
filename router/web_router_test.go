package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type webRouterTestFS struct {
	http.FileSystem
}

func (f *webRouterTestFS) Exists(prefix string, requestPath string) bool {
	name := strings.TrimPrefix(requestPath, prefix)
	file, err := f.Open(name)
	if err != nil {
		return false
	}
	_ = file.Close()
	return true
}

func newWebRouterTestFS() static.ServeFileSystem {
	return &webRouterTestFS{FileSystem: http.FS(fstest.MapFS{
		"assets/classic.js":    &fstest.MapFile{Data: []byte("classic")},
		"static/js/default.js": &fstest.MapFile{Data: []byte("default")},
	})}
}

func TestRegisterWebMiddlewareLimitsPagesButNotExistingStaticAssets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalRedisEnabled := common.RedisEnabled
	originalEnabled := common.GlobalWebRateLimitEnable
	originalLimit := common.GlobalWebRateLimitNum
	originalDuration := common.GlobalWebRateLimitDuration
	t.Cleanup(func() {
		common.RedisEnabled = originalRedisEnabled
		common.GlobalWebRateLimitEnable = originalEnabled
		common.GlobalWebRateLimitNum = originalLimit
		common.GlobalWebRateLimitDuration = originalDuration
	})

	common.RedisEnabled = false
	common.GlobalWebRateLimitEnable = true
	common.GlobalWebRateLimitNum = 1
	common.GlobalWebRateLimitDuration = 180

	router := gin.New()
	registerWebMiddleware(router, newWebRouterTestFS())
	router.NoRoute(func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	for _, requestPath := range []string{"/assets/classic.js", "/static/js/default.js"} {
		for i := 0; i < 2; i++ {
			response := performWebRouterRequest(router, requestPath, "192.0.2.205:12345")
			require.Equal(t, http.StatusOK, response.Code, requestPath)
		}
	}

	require.Equal(t, http.StatusOK, performWebRouterRequest(router, "/console/log", "192.0.2.205:12345").Code)
	require.Equal(t, http.StatusTooManyRequests, performWebRouterRequest(router, "/console/log", "192.0.2.205:12345").Code)
	require.Equal(t, http.StatusOK, performWebRouterRequest(router, "/static/js/missing.js", "192.0.2.206:12345").Code)
	require.Equal(t, http.StatusTooManyRequests, performWebRouterRequest(router, "/static/js/missing.js", "192.0.2.206:12345").Code)
}

func TestServeCurrentIndexAssetFallbackOnlyForOtherThemeEntry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalTheme := common.GetTheme()
	t.Cleanup(func() {
		common.SetTheme(originalTheme)
	})

	themeFS := &webRouterTestFS{FileSystem: http.FS(fstest.MapFS{
		"assets/index-default.js":  &fstest.MapFile{Data: []byte("default-js")},
		"assets/index-default.css": &fstest.MapFile{Data: []byte("default-css")},
		"assets/index-classic.js":  &fstest.MapFile{Data: []byte("classic-js")},
		"assets/index-classic.css": &fstest.MapFile{Data: []byte("classic-css")},
	})}
	currentAssets := currentWebAssetPaths{
		defaultIndexJS:  "/assets/index-default.js",
		defaultIndexCSS: "/assets/index-default.css",
		classicIndexJS:  "/assets/index-classic.js",
		classicIndexCSS: "/assets/index-classic.css",
	}

	router := gin.New()
	router.NoRoute(func(c *gin.Context) {
		if serveCurrentIndexAssetFallback(c, themeFS, currentAssets) {
			return
		}
		c.Status(http.StatusNotFound)
	})

	tests := []struct {
		name        string
		theme       string
		requestPath string
		wantStatus  int
		wantBody    string
		contentType string
	}{
		{
			name:        "Default 主入口切换到 Classic JavaScript",
			theme:       "classic",
			requestPath: "/assets/index-default.js",
			wantStatus:  http.StatusOK,
			wantBody:    "classic-js",
			contentType: "text/javascript",
		},
		{
			name:        "Default 主入口切换到 Classic CSS",
			theme:       "classic",
			requestPath: "/assets/index-default.css",
			wantStatus:  http.StatusOK,
			wantBody:    "classic-css",
			contentType: "text/css",
		},
		{
			name:        "Classic 主入口切换到 Default JavaScript",
			theme:       "default",
			requestPath: "/assets/index-classic.js",
			wantStatus:  http.StatusOK,
			wantBody:    "default-js",
			contentType: "text/javascript",
		},
		{
			name:        "Classic 主入口切换到 Default CSS",
			theme:       "default",
			requestPath: "/assets/index-classic.css",
			wantStatus:  http.StatusOK,
			wantBody:    "default-css",
			contentType: "text/css",
		},
		{
			name:        "未知动态分块不替换",
			theme:       "classic",
			requestPath: "/assets/index-security-audit-chunk.js",
			wantStatus:  http.StatusNotFound,
		},
		{
			name:        "当前主题主入口不回退",
			theme:       "classic",
			requestPath: "/assets/index-classic.js",
			wantStatus:  http.StatusNotFound,
		},
		{
			name:        "入口许可证文件不替换",
			theme:       "classic",
			requestPath: "/assets/index-default.js.LICENSE.txt",
			wantStatus:  http.StatusNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			common.SetTheme(test.theme)
			response := performWebRouterRequest(router, test.requestPath, "192.0.2.207:12345")
			require.Equal(t, test.wantStatus, response.Code)
			if test.wantBody != "" {
				require.Equal(t, test.wantBody, response.Body.String())
			}
			if test.contentType != "" {
				require.Contains(t, response.Header().Get("Content-Type"), test.contentType)
			}
		})
	}
}

func performWebRouterRequest(router http.Handler, requestPath string, remoteAddr string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, requestPath, nil)
	request.RemoteAddr = remoteAddr
	router.ServeHTTP(recorder, request)
	return recorder
}
