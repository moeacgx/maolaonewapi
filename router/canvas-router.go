package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func registerCanvasRelayRoutes(router *gin.Engine) {
	canvasRoute := router.Group("/canvas/v1")
	canvasRoute.Use(middleware.RouteTag("relay"))
	canvasRoute.Use(middleware.SystemPerformanceCheck())
	canvasRoute.Use(middleware.UserSessionAuth())
	canvasRoute.POST("/images/tasks", controller.ImageTaskAdmissionGuard(), middleware.ModelRequestRateLimit(), controller.CanvasPrepareRequest, controller.CanvasImageTaskSubmit)

	canvasPreparedRoute := canvasRoute.Group("")
	canvasPreparedRoute.Use(controller.CanvasPrepareRequest)
	{
		canvasPreparedRoute.GET("/models", controller.CanvasListModels)
		canvasPreparedRoute.GET("/images/tasks/:task_id", controller.CanvasImageTaskFetch)
		canvasPreparedRoute.GET("/images/tasks/:task_id/content/:index", controller.CanvasImageTaskContent)

		canvasSyncRoute := canvasPreparedRoute.Group("")
		canvasSyncRoute.Use(middleware.Distribute())
		canvasSyncRoute.Use(middleware.ModelRequestRateLimit())
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
	imageTaskRoute.Use(middleware.RouteTag("relay"))
	imageTaskRoute.Use(middleware.SystemPerformanceCheck())
	imageTaskRoute.Use(middleware.TokenAuth())
	{
		imageTaskRoute.POST("", controller.ImageTaskAdmissionGuard(), middleware.ModelRequestRateLimit(), controller.ImageTaskSubmit)
		imageTaskRoute.GET("/:task_id", controller.ImageTaskFetch)
		imageTaskRoute.GET("/:task_id/content/:index", controller.ImageTaskContent)
	}
}
