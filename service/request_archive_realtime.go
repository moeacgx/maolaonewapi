package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
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
	requestPath := ""
	if c.Request.URL != nil {
		requestPath = c.Request.URL.Path
	}
	_, err := QueueRequestArchive(c.Request.Context(), RequestArchiveRequest{
		Body:        payload,
		ContentType: contentType,
		Method:      method,
		Path:        requestPath,
		RequestId:   c.GetString(common.RequestIdKey),
		UserId:      common.GetContextKeyInt(c, constant.ContextKeyUserId),
		Username:    common.GetContextKeyString(c, constant.ContextKeyUserName),
		UserEmail:   common.GetContextKeyString(c, constant.ContextKeyUserEmail),
		TokenId:     common.GetContextKeyInt(c, constant.ContextKeyTokenId),
		TokenName:   c.GetString("token_name"),
		GroupId:     common.GetContextKeyInt(c, constant.ContextKeyUserGroupId),
		GroupName:   common.GetContextKeyString(c, constant.ContextKeyUsingGroup),
	})
	if err != nil {
		RecordRequestArchiveDropped(err)
	}
}
