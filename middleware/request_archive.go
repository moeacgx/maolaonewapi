package middleware

import (
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const requestArchiveAttemptedContextKey = "request_archive_attempted"

// RequestArchive 在已经完成认证、但尚未进行协议转换的特殊路由上保存原始
// 请求正文。常规 Relay 仍由 PromptAudit 调用同一幂等入口。
func RequestArchive() gin.HandlerFunc {
	return func(c *gin.Context) {
		QueueRequestArchive(c)
		c.Next()
	}
}

// QueueRequestArchive 在任何屏蔽词 mask、Guard 判断和协议
// 转换之前持久化客户端原始 HTTP 正文。实际写入本地或对象存储由后台 Worker
// 完成；这里的持久队列写入失败只能记运行指标，绝不能影响 Relay 主链路。
func QueueRequestArchive(c *gin.Context) {
	if c == nil || c.Request == nil {
		return
	}
	if attempted, ok := c.Get(requestArchiveAttemptedContextKey); ok && attempted == true {
		return
	}
	c.Set(requestArchiveAttemptedContextKey, true)
	if !requestArchiveHTTPMethodHasBody(c.Request.Method) {
		return
	}
	enabled, maxBodyBytes, err := service.RequestArchiveBodyLimit(c.Request.Context())
	if err != nil {
		service.RecordRequestArchiveDropped(err)
		return
	}
	if !enabled {
		return
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		service.RecordRequestArchiveDropped(err)
		return
	}
	if maxBodyBytes < 1 || storage.Size() > maxBodyBytes {
		service.RecordRequestArchiveDropped(model.ErrRequestArchiveBodyTooLarge)
		return
	}
	requestPath := ""
	if c.Request.URL != nil {
		requestPath = c.Request.URL.Path
	}
	archiveRequest := service.RequestArchiveRequest{
		ContentType: c.GetHeader("Content-Type"),
		Method:      c.Request.Method,
		Path:        requestPath,
		RequestId:   c.GetString(common.RequestIdKey),
		UserId:      common.GetContextKeyInt(c, constant.ContextKeyUserId),
		Username:    common.GetContextKeyString(c, constant.ContextKeyUserName),
		UserEmail:   common.GetContextKeyString(c, constant.ContextKeyUserEmail),
		TokenId:     common.GetContextKeyInt(c, constant.ContextKeyTokenId),
		TokenName:   c.GetString("token_name"),
		GroupId:     common.GetContextKeyInt(c, constant.ContextKeyUserGroupId),
		GroupName:   common.GetContextKeyString(c, constant.ContextKeyUsingGroup),
	}
	defer func() {
		if _, seekErr := storage.Seek(0, io.SeekStart); seekErr != nil {
			service.RecordRequestArchiveDropped(seekErr)
		}
		c.Request.Body = io.NopCloser(storage)
	}()
	if storage.IsDisk() {
		_, err = service.QueueRequestArchiveFromReader(c.Request.Context(), archiveRequest, storage, storage.Size())
	} else {
		body, bodyErr := storage.Bytes()
		if bodyErr != nil {
			service.RecordRequestArchiveDropped(bodyErr)
			return
		}
		archiveRequest.Body = body
		_, err = service.QueueRequestArchive(c.Request.Context(), archiveRequest)
	}
	if err != nil {
		service.RecordRequestArchiveDropped(err)
	}
}

func queueRequestArchiveBeforePromptAudit(c *gin.Context) {
	QueueRequestArchive(c)
}

func requestArchiveHTTPMethodHasBody(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return true
	default:
		return false
	}
}
