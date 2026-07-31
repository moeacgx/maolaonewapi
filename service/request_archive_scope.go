package service

import (
	"context"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const requestArchiveCandidateContextKey = "request_archive_candidate"

// RequestArchiveCaptureConfig 返回热路径决定“立即归档”还是“等待审计事件”
// 所需的最小配置。旧配置中的空范围按全部请求处理。
func RequestArchiveCaptureConfig(ctx context.Context) (bool, int64, string, error) {
	if err := ctx.Err(); err != nil {
		return false, 0, model.RequestArchiveScopeAllRequests, err
	}
	config, err := loadRequestArchivePrivateConfig(ctx)
	if err != nil || config == nil || config.Config == nil {
		return false, 0, model.RequestArchiveScopeAllRequests, err
	}
	return config.Config.Enabled, config.Config.MaxBodyBytes,
		normalizeRequestArchiveScope(config.Config.ArchiveScope), nil
}

// BuildRequestArchiveRequest 只复制允许进入归档元数据的字段。请求头、Cookie
// 和 URL 查询参数不会进入候选载荷。
func BuildRequestArchiveRequest(c *gin.Context, body []byte, contentType, method string) RequestArchiveRequest {
	request := RequestArchiveRequest{Body: body, ContentType: contentType, Method: method}
	if c == nil {
		return request
	}
	if c.Request != nil && c.Request.URL != nil {
		request.Path = c.Request.URL.Path
	}
	request.RequestId = c.GetString(common.RequestIdKey)
	request.UserId = common.GetContextKeyInt(c, constant.ContextKeyUserId)
	request.Username = common.GetContextKeyString(c, constant.ContextKeyUserName)
	request.UserEmail = common.GetContextKeyString(c, constant.ContextKeyUserEmail)
	request.TokenId = common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	request.TokenName = c.GetString("token_name")
	request.GroupId = common.GetContextKeyInt(c, constant.ContextKeyUserGroupId)
	request.GroupName = common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	return request
}

// SetPendingRequestArchive 保存当前 HTTP 正文或 Realtime 客户端帧的原始候选。
// 候选仅存在于请求上下文；异步 Guard 会把它放入已有的受保护任务载荷。
func SetPendingRequestArchive(c *gin.Context, request RequestArchiveRequest) {
	if c == nil {
		return
	}
	id := uuid.NewString()
	request.ArchiveId = id
	request.DedupeKey = id
	request.Body = append([]byte(nil), request.Body...)
	c.Set(requestArchiveCandidateContextKey, &request)
}

func pendingRequestArchive(c *gin.Context) *RequestArchiveRequest {
	if c == nil {
		return nil
	}
	value, ok := c.Get(requestArchiveCandidateContextKey)
	if !ok {
		return nil
	}
	request, _ := value.(*RequestArchiveRequest)
	return cloneRequestArchiveRequest(request)
}

// AttachPendingRequestArchiveToPromptAuditRequest 让异步 Guard 在请求结束后仍能
// 只为最终保留的审计事件归档原始正文。
func AttachPendingRequestArchiveToPromptAuditRequest(c *gin.Context, request *PromptAuditRequest) {
	if request == nil {
		return
	}
	request.RequestArchive = pendingRequestArchive(c)
}

// QueuePendingRequestArchiveForAuditEvent 只能在审计事件成功落库后调用。
// 同一候选由数据库去重键保证最多产生一个归档任务。
func QueuePendingRequestArchiveForAuditEvent(c *gin.Context, eventID int64) {
	if c == nil || c.Request == nil {
		return
	}
	request := pendingRequestArchive(c)
	if request == nil || eventID <= 0 {
		return
	}
	if queueRequestArchiveForAuditEvent(c.Request.Context(), request, eventID) {
		c.Set(requestArchiveCandidateContextKey, nil)
	}
}

func queueRequestArchiveForAuditEvent(ctx context.Context, request *RequestArchiveRequest, eventID int64) bool {
	if request == nil || eventID <= 0 || strings.TrimSpace(request.DedupeKey) == "" {
		return false
	}
	copy := cloneRequestArchiveRequest(request)
	copy.AuditEventId = eventID
	_, err := QueueRequestArchive(ctx, *copy)
	if err != nil {
		RecordRequestArchiveDropped(err)
		return false
	}
	return true
}

func cloneRequestArchiveRequest(request *RequestArchiveRequest) *RequestArchiveRequest {
	if request == nil {
		return nil
	}
	copy := *request
	copy.Body = append([]byte(nil), request.Body...)
	return &copy
}
