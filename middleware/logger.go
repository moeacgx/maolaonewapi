package middleware

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

const RouteTagKey = "route_tag"

func RouteTag(tag string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(RouteTagKey, tag)
		c.Next()
	}
}

func SetUpLogger(server *gin.Engine) {
	config := gin.LoggerConfig{
		Formatter: func(param gin.LogFormatterParams) string {
			var requestID string
			if param.Keys != nil {
				requestID, _ = param.Keys[common.RequestIdKey].(string)
			}
			tag, _ := param.Keys[RouteTagKey].(string)
			if tag == "" {
				tag = "web"
			}
			return fmt.Sprintf("[GIN] %s | %s | %s | %3d | %13v | %15s | %7s %s\n",
				param.TimeStamp.Format("2006/01/02 - 15:04:05"),
				tag,
				requestID,
				param.StatusCode,
				param.Latency,
				param.ClientIP,
				param.Method,
				param.Path,
			)
		},
		SkipPaths: logSkipPaths(),
	}
	server.Use(gin.LoggerWithConfig(config))
}

func logSkipPaths() []string {
	paths := common.GetEnvOrDefaultString("GIN_LOG_SKIP_PATHS", "/api/status")
	if strings.EqualFold(strings.TrimSpace(paths), "none") {
		return nil
	}
	result := make([]string, 0)
	for _, path := range strings.Split(paths, ",") {
		path = strings.TrimSpace(path)
		if path != "" {
			result = append(result, path)
		}
	}
	return result
}
