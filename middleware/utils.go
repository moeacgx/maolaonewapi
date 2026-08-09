package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func abortWithOpenAiMessage(c *gin.Context, statusCode int, message string, code ...types.ErrorCode) {
	codeStr := ""
	if len(code) > 0 {
		codeStr = string(code[0])
	}
	userId := c.GetInt("id")
	message = common.MessageWithRequestId(message, c.GetString(common.RequestIdKey))
	// PromptAuditRealtime 会在渠道分配前完成 WebSocket 升级。此后如果
	// Distribute 因模型、分组或渠道失败，不能再向已劫持的 HTTP Writer
	// 写 JSON；应改为标准 Realtime error 事件并主动关闭连接。
	if ws, ok := common.GetContextKeyType[*websocket.Conn](c, constant.ContextKeyPromptAuditRealtimeClientWs); ok && ws != nil {
		helper.WssError(c, ws, types.OpenAIError{
			Message: message, Type: string(types.ErrorTypeNewAPIError), Param: "", Code: codeStr,
		})
		closeCode := websocket.ClosePolicyViolation
		if statusCode == http.StatusTooManyRequests || statusCode >= http.StatusInternalServerError {
			closeCode = websocket.CloseTryAgainLater
		}
		closeReason := codeStr
		if closeReason == "" {
			closeReason = "realtime_request_rejected"
		}
		_ = ws.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(closeCode, closeReason), time.Now().Add(time.Second))
		c.Abort()
		logger.LogError(c.Request.Context(), fmt.Sprintf("user %d | %s", userId, message))
		return
	}
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"message": message,
			"type":    "new_api_error",
			"code":    codeStr,
		},
	})
	c.Abort()
	logger.LogError(c.Request.Context(), fmt.Sprintf("user %d | %s", userId, message))
}

func abortWithMidjourneyMessage(c *gin.Context, statusCode int, code int, description string) {
	c.JSON(statusCode, gin.H{
		"description": description,
		"type":        "new_api_error",
		"code":        code,
	})
	c.Abort()
	logger.LogError(c.Request.Context(), description)
}
