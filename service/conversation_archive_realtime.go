package service

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const conversationArchiveRealtimeStateKey = "conversation_archive_realtime_state"
const conversationArchiveRealtimeFinalizedKey = "conversation_archive_realtime_finalized"

// ConversationArchiveRealtimeState 保存单个 Realtime 连接的文本片段。
// 只保留清洗后的短文本，并设置总字节上限，避免长连接把正文累积在内存中。
type ConversationArchiveRealtimeState struct {
	mu       sync.Mutex
	snapshot PromptAuditSnapshot
	segments []PromptAuditContextSegment
	bytes    int
}

const conversationArchiveRealtimeMaxBytes = ConversationArchiveMaxContentBytes

// CaptureConversationArchiveRealtimeFrame 提取 Realtime 帧中的文本并加入当前
// 会话。direction 为 client 或 assistant；媒体、URL、base64 与工具 schema 会被
// 丢弃。该函数是旁路操作，任何失败都不应影响 WebSocket 转发。
func CaptureConversationArchiveRealtimeFrame(c *gin.Context, payload []byte, direction string) {
	if c == nil || len(payload) == 0 {
		return
	}
	archiveCtx := context.Background()
	if c.Request != nil {
		archiveCtx = context.WithoutCancel(c.Request.Context())
	}
	cfg, err := GetConversationArchiveConfig(archiveCtx)
	if err != nil || cfg == nil || !cfg.Enabled {
		return
	}
	userID := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	if len(cfg.UserIds) > 0 && !conversationArchiveUserSelected(userID, cfg.UserIds) {
		return
	}
	segments := extractConversationArchiveRealtimeSegments(payload, direction)
	if len(segments) == 0 {
		return
	}
	state := conversationArchiveRealtimeState(c, true)
	if state == nil {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.snapshot.RequestId == "" {
		state.snapshot = PromptAuditSnapshot{
			RequestId: c.GetString(common.RequestIdKey),
			UserId:    userID,
			Username:  common.GetContextKeyString(c, constant.ContextKeyUserName),
			UserEmail: common.GetContextKeyString(c, constant.ContextKeyUserEmail),
			TokenId:   common.GetContextKeyInt(c, constant.ContextKeyTokenId),
			GroupId:   common.GetContextKeyInt(c, constant.ContextKeyPromptAuditGroupId),
			GroupCode: realtimeArchiveInitialGroupCode(c),
			GroupName: common.GetContextKeyString(c, constant.ContextKeyPromptAuditGroupName),
			Endpoint:  requestPath(c),
			Protocol:  "openai_realtime",
			Model:     strings.TrimSpace(c.Query("model")),
			Stage:     "realtime",
		}
	}
	for _, segment := range segments {
		if state.bytes >= conversationArchiveRealtimeMaxBytes || len(state.segments) >= ConversationArchiveMaxMessages*2 {
			break
		}
		segment.Text = truncateConversationText(strings.TrimSpace(strings.ReplaceAll(segment.Text, "\x00", "�")))
		if segment.Text == "" {
			continue
		}
		remaining := conversationArchiveRealtimeMaxBytes - state.bytes
		if len(segment.Text) > remaining {
			runes := []rune(segment.Text)
			for len(runes) > 0 && len(string(runes)) > remaining {
				runes = runes[:len(runes)-1]
			}
			segment.Text = string(runes)
		}
		if segment.Text == "" {
			break
		}
		segment.Role = normalizeConversationRole(segment.Role)
		state.segments = append(state.segments, segment)
		state.bytes += len(segment.Text)
	}
}

// FinalizeConversationArchiveRealtime 在连接结束时按最终 Distributor 分组
// 保存一条清洗后的会话记录。失败只计数，不改变连接结果。
func FinalizeConversationArchiveRealtime(c *gin.Context) {
	if c == nil || c.Request == nil {
		return
	}
	state := conversationArchiveRealtimeState(c, false)
	if state == nil {
		return
	}
	if finalized, ok := c.Get(conversationArchiveRealtimeFinalizedKey); ok && finalized == true {
		return
	}
	c.Set(conversationArchiveRealtimeFinalizedKey, true)
	// The WebSocket request context is cancelled when the connection closes.
	// Use a detached context so the archive write succeeds after disconnect.
	ctx := context.WithoutCancel(c.Request.Context())
	cfg, err := GetConversationArchiveConfig(ctx)
	if err != nil || cfg == nil || !cfg.Enabled {
		return
	}
	state.mu.Lock()
	snapshot := state.snapshot
	snapshot.ContextSegments = append([]PromptAuditContextSegment(nil), state.segments...)
	state.mu.Unlock()
	if len(snapshot.ContextSegments) == 0 {
		return
	}
	groupCode := realtimeArchiveFinalGroupCode(c, snapshot)
	if groupCode == "" || !ConversationArchiveMatchesFilter(snapshot.UserId, groupCode, cfg) {
		return
	}
	snapshot.GroupCode = groupCode
	if snapshot.GroupId <= 0 {
		snapshot.GroupId = common.GetContextKeyInt(c, constant.ContextKeyPromptAuditGroupId)
	}
	if snapshot.GroupName == "" {
		snapshot.GroupName = common.GetContextKeyString(c, constant.ContextKeyPromptAuditGroupName)
		if snapshot.GroupName == "" {
			snapshot.GroupName = common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
		}
	}
	if _, err := StoreConversationArchiveFromSnapshot(ctx, snapshot, cfg.RetentionDays, cfg.MaxBodyBytes); err != nil {
		RecordConversationArchiveDropped(err)
	}
}

func conversationArchiveRealtimeState(c *gin.Context, create bool) *ConversationArchiveRealtimeState {
	if c == nil {
		return nil
	}
	if value, exists := c.Get(conversationArchiveRealtimeStateKey); exists {
		state, _ := value.(*ConversationArchiveRealtimeState)
		return state
	}
	if !create {
		return nil
	}
	state := &ConversationArchiveRealtimeState{}
	c.Set(conversationArchiveRealtimeStateKey, state)
	return state
}

func conversationArchiveUserSelected(userID int, selected []int) bool {
	for _, id := range selected {
		if id == userID {
			return true
		}
	}
	return false
}

func requestPath(c *gin.Context) string {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return ""
	}
	return c.Request.URL.Path
}

func realtimeArchiveInitialGroupCode(c *gin.Context) string {
	for _, key := range []constant.ContextKey{
		constant.ContextKeyPromptAuditGroupCode,
		constant.ContextKeySelectedChannelGroup,
		constant.ContextKeyUsingGroup,
		constant.ContextKeyUserGroup,
	} {
		if value := strings.TrimSpace(common.GetContextKeyString(c, key)); value != "" {
			return value
		}
	}
	return ""
}

func realtimeArchiveFinalGroupCode(c *gin.Context, snapshot PromptAuditSnapshot) string {
	candidates := []string{
		common.GetContextKeyString(c, constant.ContextKeySelectedChannelGroup),
		common.GetContextKeyString(c, constant.ContextKeyAutoGroup),
		common.GetContextKeyString(c, constant.ContextKeyUsingGroup),
		snapshot.GroupCode,
	}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || strings.EqualFold(candidate, "auto") {
			continue
		}
		if model.DB != nil {
			if group, lookupErr := model.GetGroupByCodeOrAlias(candidate); lookupErr == nil && group != nil {
				candidate = group.Code
			}
		}
		return model.NormalizeConversationArchiveGroupCode(candidate)
	}
	if snapshot.GroupId > 0 && model.DB != nil {
		if group, lookupErr := model.GetGroupById(snapshot.GroupId); lookupErr == nil && group != nil {
			return model.NormalizeConversationArchiveGroupCode(group.Code)
		}
	}
	return ""
}

func extractConversationArchiveRealtimeSegments(payload []byte, direction string) []PromptAuditContextSegment {
	if strings.EqualFold(strings.TrimSpace(direction), "client") {
		request := PromptAuditRequest{Body: payload, Protocol: "openai_realtime"}
		snapshot, err := ExtractPromptAuditSnapshot(request)
		if err != nil {
			return nil
		}
		return append([]PromptAuditContextSegment(nil), snapshot.ContextSegments...)
	}
	var root map[string]interface{}
	if err := common.Unmarshal(payload, &root); err != nil {
		return nil
	}
	eventType, _ := root["type"].(string)
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(eventType)), ".done") ||
		strings.EqualFold(strings.TrimSpace(eventType), "response.done") {
		// delta 事件已经包含完整的增量正文，done 事件通常是同一正文的汇总，
		// 忽略它可避免在线预览重复显示整段助手回复。
		return nil
	}
	texts := collectConversationArchiveRealtimeText(root)
	result := make([]PromptAuditContextSegment, 0, len(texts))
	for _, text := range texts {
		if strings.TrimSpace(text) == "" || looksLikePromptAuditMediaPayload(text) {
			continue
		}
		result = append(result, PromptAuditContextSegment{Role: "assistant", Kind: "assistant", Text: text})
	}
	return result
}

func collectConversationArchiveRealtimeText(root map[string]interface{}) []string {
	if root == nil {
		return nil
	}
	ordered := []string{"delta", "text", "transcript", "output_text", "content", "item", "response", "part", "output"}
	result := make([]string, 0, 2)
	var walk func(interface{}, string, int)
	walk = func(value interface{}, key string, depth int) {
		if depth > 8 || value == nil {
			return
		}
		switch typed := value.(type) {
		case string:
			if key == "delta" || key == "text" || key == "transcript" || key == "output_text" || key == "refusal" {
				text := strings.TrimSpace(typed)
				if text != "" && !looksLikePromptAuditMediaPayload(text) {
					result = append(result, text)
				}
			}
		case []interface{}:
			for _, item := range typed {
				walk(item, key, depth+1)
			}
		case map[string]interface{}:
			seen := map[string]bool{}
			for _, childKey := range ordered {
				if child, ok := typed[childKey]; ok {
					walk(child, childKey, depth+1)
					seen[childKey] = true
				}
			}
			keys := make([]string, 0, len(typed))
			for childKey := range typed {
				if !seen[childKey] && childKey != "type" && childKey != "id" && childKey != "usage" {
					keys = append(keys, childKey)
				}
			}
			sort.Strings(keys)
			for _, childKey := range keys {
				walk(typed[childKey], childKey, depth+1)
			}
		}
	}
	walk(root, "", 0)
	return result
}
