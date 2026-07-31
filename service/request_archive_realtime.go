package service

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// QueueRealtimeRequestArchiveFrame 把每个客户端 Realtime 帧作为同一
// request_id 下的独立顺序任务保存。调用点必须位于屏蔽词改写与 Guard 判断
// 之前，因此文本、二进制 JSON 和原始音频都保持客户端提交的字节内容。
func QueueRealtimeRequestArchiveFrame(c *gin.Context, messageType int, payload []byte) {
	if c == nil || c.Request == nil {
		return
	}
	contentType := "application/octet-stream"
	method := "WS_BINARY"
	if messageType == websocket.TextMessage {
		contentType = "application/json"
		method = "WS_TEXT"
	}
	enabled, maxBodyBytes, archiveScope, configErr := RequestArchiveCaptureConfig(c.Request.Context())
	if configErr != nil {
		RecordRequestArchiveDropped(configErr)
		return
	}
	if !enabled {
		return
	}
	if int64(len(payload)) > maxBodyBytes {
		RecordRequestArchiveDropped(model.ErrRequestArchiveBodyTooLarge)
		return
	}
	request := BuildRequestArchiveRequest(c, payload, contentType, method)
	if archiveScope == model.RequestArchiveScopeAuditEvents {
		SetPendingRequestArchive(c, request)
		return
	}
	_, err := QueueRequestArchive(c.Request.Context(), request)
	if err != nil {
		RecordRequestArchiveDropped(err)
	}
}
