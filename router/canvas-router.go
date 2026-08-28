package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func registerCanvasRelayRoutes(router *gin.Engine) {
	canvasRoute := router.Group("/canvas/v1")
	canvasRoute.Use(middleware.CanvasOriginGuard())
	canvasRoute.Use(middleware.DecompressRequestMiddleware())
	canvasRoute.Use(middleware.BodyStorageCleanup())
	canvasRoute.Use(middleware.StatsMiddleware())
	canvasRoute.Use(middleware.RouteTag("relay"))
	canvasRoute.Use(middleware.SystemPerformanceCheck())
	canvasRoute.Use(middleware.UserSessionAuth())
	registerAsyncImageTaskSubmitRoute(canvasRoute, "/images/tasks", controller.CanvasImageTaskSubmit, controller.CanvasPrepareRequest, middleware.PromptAudit())

	canvasPreparedRoute := canvasRoute.Group("")
	canvasPreparedRoute.Use(controller.CanvasPrepareRequest)
	{
		canvasPreparedRoute.GET("/models", controller.CanvasListModels)
		canvasPreparedRoute.GET("/images/tasks/:task_id", controller.CanvasImageTaskFetch)
		canvasPreparedRoute.GET("/images/tasks/:task_id/content/:index", controller.CanvasImageTaskContent)

		canvasSyncRoute := canvasPreparedRoute.Group("")
		canvasSyncRoute.Use(middleware.ModelRequestRateLimit())
		canvasSyncRoute.Use(middleware.PromptAudit())
		canvasSyncRoute.Use(middleware.Distribute())
		{
			canvasSyncRoute.POST("/chat/completions", controller.CanvasChatCompletions)
			canvasSyncRoute.POST("/images/generations", controller.CanvasImageGenerations)
			canvasSyncRoute.POST("/images/edits", controller.CanvasImageEdits)
			canvasSyncRoute.POST("/audio/speech", controller.CanvasAudioSpeech)
			canvasSyncRoute.POST("/videos", controller.CanvasVideoSubmit)
			canvasSyncRoute.GET("/videos/:task_id", controller.CanvasVideoFetch)
			canvasSyncRoute.GET("/videos/:task_id/content", controller.CanvasVideoContent)
		}
	}

	imageTaskRoute := router.Group("/v1/images/tasks")
	imageTaskRoute.Use(middleware.RelayCORS())
	imageTaskRoute.Use(middleware.DecompressRequestMiddleware())
	imageTaskRoute.Use(middleware.BodyStorageCleanup())
	imageTaskRoute.Use(middleware.StatsMiddleware())
	imageTaskRoute.Use(middleware.RouteTag("relay"))
	imageTaskRoute.Use(middleware.SystemPerformanceCheck())
	imageTaskRoute.Use(middleware.TokenAuth())
	{
		registerAsyncImageTaskSubmitRoute(imageTaskRoute, "", controller.ImageTaskSubmit, middleware.PromptAudit())
		imageTaskRoute.GET("/:task_id", controller.ImageTaskFetch)
		imageTaskRoute.GET("/:task_id/content/:index", controller.ImageTaskContent)
	}
}

// registerAsyncImageTaskSubmitRoute 让异步任务准入与同步模型请求限流隔离。
// 异步准入守卫负责专用的用户/令牌任务速率和活动任务数限制。
func registerAsyncImageTaskSubmitRoute(routes gin.IRoutes, path string, handler gin.HandlerFunc, middlewares ...gin.HandlerFunc) {
	chain := make(gin.HandlersChain, 0, len(middlewares)+2)
	chain = append(chain, controller.ImageTaskAdmissionGuard())
	chain = append(chain, middlewares...)
	chain = append(chain, handler)
	routes.POST(path, chain...)
}
