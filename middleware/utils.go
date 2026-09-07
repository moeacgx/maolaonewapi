package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func abortWithOpenAiMessage(c *gin.Context, statusCode int, message string, code ...types.ErrorCode) {
	codeStr := ""
	if len(code) > 0 {
		codeStr = string(code[0])
	}
	message = common.MessageWithRequestId(message, c.GetString(common.RequestIdKey))
	userId := c.GetInt("id")
	// PromptAuditRealtime upgrades before distribution. Distributor failures
	// must therefore use the Realtime protocol instead of writing HTTP JSON.
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

// recordAuthErrorLog 记录在渠道选择前终止的鉴权失败。
// 鉴权失败尚未产生上游渠道，因此 channel_id 固定为 0。
func recordAuthErrorLog(c *gin.Context, statusCode int, message string, code types.ErrorCode, group string) {
	if c == nil || !constant.ErrorLogEnabled || c.GetInt("id") <= 0 || model.LOG_DB == nil {
		return
	}
	other := map[string]interface{}{
		"error_stage": "authentication",
		"request_path": func() string {
			if c.Request == nil || c.Request.URL == nil {
				return ""
			}
			return c.Request.URL.Path
		}(),
		"status_code": statusCode,
		"error_code":  string(code),
		"error_type":  string(types.ErrorTypeNewAPIError),
	}
	useTimeSeconds := 0
	if startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime); !startTime.IsZero() {
		useTimeSeconds = max(0, int(time.Since(startTime).Seconds()))
	}
	model.RecordErrorLog(c, c.GetInt("id"), 0, common.GetContextKeyString(c, constant.ContextKeyOriginalModel), c.GetString("token_name"), message, c.GetInt("token_id"), useTimeSeconds, common.GetContextKeyBool(c, constant.ContextKeyIsStream), group, other)
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
