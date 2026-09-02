package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrConversationArchiveConfigConflict = errors.New("conversation archive config conflict")

type conversationArchiveConfigCacheState struct {
	mu       sync.RWMutex
	config   *ConversationArchiveConfigView
	loadedAt time.Time
}

var conversationArchiveConfigCache conversationArchiveConfigCacheState

const conversationArchiveConfigCacheTTL = 2 * time.Second

var conversationArchiveDropped struct {
	mu    sync.Mutex
	count int64
	last  string
}

// RecordConversationArchiveDropped 记录旁路归档失败，不把失败传播到主请求。
// 只保留稳定错误文本，避免把请求正文或存储细节写入日志/指标。
func RecordConversationArchiveDropped(err error) {
	conversationArchiveDropped.mu.Lock()
	conversationArchiveDropped.count++
	conversationArchiveDropped.last = conversationArchiveErrorCode(err)
	conversationArchiveDropped.mu.Unlock()
}

func conversationArchiveErrorCode(err error) string {
	if err == nil {
		return "conversation_archive_store_failed"
	}
	if errors.Is(err, ErrPromptAuditNoText) {
		return "conversation_archive_no_text"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "conversation_archive_canceled"
	}
	return "conversation_archive_store_failed"
}

func invalidateConversationArchiveConfig() {
	conversationArchiveConfigCache.mu.Lock()
	conversationArchiveConfigCache.config = nil
	conversationArchiveConfigCache.loadedAt = time.Time{}
	conversationArchiveConfigCache.mu.Unlock()
}

// InvalidateConversationArchiveConfig 清除配置缓存，供测试夹具和扩展宿主
// 在切换数据库连接后显式刷新配置。
func InvalidateConversationArchiveConfig() {
	invalidateConversationArchiveConfig()
}

type ConversationArchiveConfigView struct {
	ConfigVersion int64    `json:"config_version"`
	Enabled       bool     `json:"enabled"`
	GroupCodes    []string `json:"group_codes"`
	UserIds       []int    `json:"user_ids"`
	MaxBodyBytes  int64    `json:"max_body_bytes"`
	RetentionDays int      `json:"retention_days"`
}

type ConversationArchiveConfigUpdate struct {
	ExpectedConfigVersion int64    `json:"expected_version"`
	Enabled               bool     `json:"enabled"`
	GroupCodes            []string `json:"group_codes"`
	UserIds               []int    `json:"user_ids"`
	MaxBodyBytes          int64    `json:"max_body_bytes"`
	RetentionDays         int      `json:"retention_days"`
}

func GetConversationArchiveConfig(ctx context.Context) (*ConversationArchiveConfigView, error) {
	if model.DB == nil {
		return nil, errors.New("数据库尚未初始化")
	}
	conversationArchiveConfigCache.mu.RLock()
	if cached := conversationArchiveConfigCache.config; cached != nil &&
		time.Since(conversationArchiveConfigCache.loadedAt) < conversationArchiveConfigCacheTTL {
		result := cloneConversationArchiveConfig(cached)
		conversationArchiveConfigCache.mu.RUnlock()
		return result, nil
	}
	conversationArchiveConfigCache.mu.RUnlock()
	var row model.ConversationArchiveConfig
	query := model.DB.WithContext(ctx).Where("id = ?", model.ConversationArchiveConfigID)
	if err := query.First(&row).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		defaults := model.ConversationArchiveConfig{Id: model.ConversationArchiveConfigID, ConfigVersion: 1, GroupCodes: "[]", UserIds: "[]", MaxBodyBytes: ConversationArchiveMaxContentBytes, RetentionDays: 30, UpdatedAt: time.Now().Unix()}
		// 使用数据库原生的冲突忽略，避免多实例首次读取时先查后建的
		// 唯一键竞态；随后统一重读，确保返回的是已提交的持久化版本。
		if createErr := model.DB.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).Create(&defaults).Error; createErr != nil {
			return nil, createErr
		}
		if retryErr := query.First(&row).Error; retryErr != nil {
			return nil, retryErr
		}
	}
	if row.ConfigVersion < 1 {
		row.ConfigVersion = 1
	}
	if row.MaxBodyBytes < 64*1024 || row.MaxBodyBytes > ConversationArchiveMaxContentBytes {
		row.MaxBodyBytes = ConversationArchiveMaxContentBytes
	}
	if row.RetentionDays < 1 || row.RetentionDays > 3650 {
		row.RetentionDays = 30
	}
	var groups []string
	var users []int
	if err := common.UnmarshalJsonStr(row.GroupCodes, &groups); err != nil {
		return nil, errors.New("对话归档分组配置损坏")
	}
	if err := common.UnmarshalJsonStr(row.UserIds, &users); err != nil {
		return nil, errors.New("对话归档用户配置损坏")
	}
	result := &ConversationArchiveConfigView{ConfigVersion: row.ConfigVersion, Enabled: row.Enabled, GroupCodes: groups, UserIds: users, MaxBodyBytes: row.MaxBodyBytes, RetentionDays: row.RetentionDays}
	conversationArchiveConfigCache.mu.Lock()
	conversationArchiveConfigCache.config = cloneConversationArchiveConfig(result)
	conversationArchiveConfigCache.loadedAt = time.Now()
	conversationArchiveConfigCache.mu.Unlock()
	return result, nil
}

func cloneConversationArchiveConfig(source *ConversationArchiveConfigView) *ConversationArchiveConfigView {
	if source == nil {
		return nil
	}
	return &ConversationArchiveConfigView{
		ConfigVersion: source.ConfigVersion,
		Enabled:       source.Enabled,
		GroupCodes:    append([]string(nil), source.GroupCodes...),
		UserIds:       append([]int(nil), source.UserIds...),
		MaxBodyBytes:  source.MaxBodyBytes,
		RetentionDays: source.RetentionDays,
	}
}

func SaveConversationArchiveConfig(ctx context.Context, req ConversationArchiveConfigUpdate, actorID int) (*ConversationArchiveConfigView, error) {
	if err := model.EnsureConversationArchiveConfig(); err != nil {
		return nil, err
	}
	if req.ExpectedConfigVersion < 1 {
		return nil, errors.New("配置版本无效")
	}
	if req.MaxBodyBytes < 64*1024 || req.MaxBodyBytes > ConversationArchiveMaxContentBytes {
		return nil, errors.New("正文大小必须在 64 KiB 到 2 MiB 之间")
	}
	if req.RetentionDays < 1 || req.RetentionDays > 3650 {
		return nil, errors.New("保留天数必须在 1 到 3650 之间")
	}
	if len(req.GroupCodes) > 128 || len(req.UserIds) > 1024 {
		return nil, errors.New("筛选项数量超限")
	}
	groups := make([]string, 0, len(req.GroupCodes))
	seen := map[string]bool{}
	for _, group := range req.GroupCodes {
		group = model.NormalizeConversationArchiveGroupCode(group)
		if group != "" && !seen[group] {
			seen[group] = true
			groups = append(groups, group)
		}
	}
	users := make([]int, 0, len(req.UserIds))
	seenUsers := map[int]bool{}
	for _, id := range req.UserIds {
		if id <= 0 || id > 1_000_000_000 {
			return nil, errors.New("用户 ID 无效")
		}
		if !seenUsers[id] {
			seenUsers[id] = true
			users = append(users, id)
		}
	}
	sort.Strings(groups)
	sort.Ints(users)
	groupJSON, _ := common.Marshal(groups)
	userJSON, _ := common.Marshal(users)
	var row model.ConversationArchiveConfig
	if err := model.DB.WithContext(ctx).First(&row, model.ConversationArchiveConfigID).Error; err != nil {
		return nil, err
	}
	if row.ConfigVersion != req.ExpectedConfigVersion {
		return nil, ErrConversationArchiveConfigConflict
	}
	updates := map[string]interface{}{"config_version": row.ConfigVersion + 1, "enabled": req.Enabled, "group_codes": string(groupJSON), "user_ids": string(userJSON), "max_body_bytes": req.MaxBodyBytes, "retention_days": req.RetentionDays, "updated_at": time.Now().Unix(), "updated_by": actorID}
	result := model.DB.WithContext(ctx).Model(&model.ConversationArchiveConfig{}).
		Where("id = ? AND config_version = ?", model.ConversationArchiveConfigID, req.ExpectedConfigVersion).
		Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, ErrConversationArchiveConfigConflict
	}
	invalidateConversationArchiveConfig()
	return GetConversationArchiveConfig(ctx)
}

func ConversationArchiveMatchesFilter(userID int, groupCode string, cfg *ConversationArchiveConfigView) bool {
	if cfg == nil || !cfg.Enabled {
		return false
	}
	if len(cfg.UserIds) > 0 {
		found := false
		for _, id := range cfg.UserIds {
			if id == userID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(cfg.GroupCodes) > 0 {
		normalized := model.NormalizeConversationArchiveGroupCode(groupCode)
		for _, group := range cfg.GroupCodes {
			if normalized == group {
				return true
			}
		}
		return false
	}
	return true
}

func ParseConversationArchiveUserIDs(values []string) []int {
	result := make([]int, 0, len(values))
	for _, value := range values {
		if id, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && id > 0 {
			result = append(result, id)
		}
	}
	return result
}

const (
	ConversationArchiveMaxMessages     = 256
	ConversationArchiveMaxMessageText  = 64 * 1024
	ConversationArchiveMaxContentBytes = 2 * 1024 * 1024
	ConversationArchiveMaxGroups       = 128
	ConversationArchiveMaxUsers        = 1024
)

type ConversationMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type NormalizedConversation struct {
	Messages []ConversationMessage `json:"messages"`
}

// StoreConversationArchiveFromSnapshot 复用提示词审计已经完成的文本提取，
// 避免再次解码原始 JSON，也不会把媒体、工具定义或鉴权信息带入归档。
func StoreConversationArchiveFromSnapshot(ctx context.Context, snapshot PromptAuditSnapshot, retentionDays int, maxBodyBytes int64) (*model.ConversationArchive, error) {
	if model.DB == nil {
		return nil, errors.New("数据库尚未初始化")
	}
	if maxBodyBytes <= 0 || maxBodyBytes > ConversationArchiveMaxContentBytes {
		maxBodyBytes = ConversationArchiveMaxContentBytes
	}
	messages := make([]ConversationMessage, 0, minInt(len(snapshot.ContextSegments), ConversationArchiveMaxMessages))
	textBytes := 0
	for _, segment := range snapshot.ContextSegments {
		if segment.archiveIgnore {
			continue
		}
		text := cleanConversationText(segment.Text)
		if text == "" {
			continue
		}
		if len(messages) >= ConversationArchiveMaxMessages {
			break
		}
		remaining := int(maxBodyBytes) - textBytes
		if remaining <= 0 {
			break
		}
		if remaining < len(text) {
			text = truncateConversationTextBytes(text, remaining)
		}
		if text == "" {
			break
		}
		messages = append(messages, ConversationMessage{Role: normalizeConversationRole(segment.Role), Text: text})
		textBytes += len(text)
	}
	if len(messages) == 0 {
		return nil, ErrPromptAuditNoText
	}
	normalized := NormalizedConversation{Messages: messages}
	content, err := marshalNormalizedConversation(&normalized, maxBodyBytes)
	if err != nil {
		return nil, err
	}
	groupCode := model.NormalizeConversationArchiveGroupCode(snapshot.GroupCode)
	storedContent, cipherKind, err := StorePromptAuditSecret(string(content))
	if err != nil {
		return nil, err
	}
	record := &model.ConversationArchive{RequestId: snapshot.RequestId, UserId: snapshot.UserId, Username: snapshot.Username,
		GroupId: snapshot.GroupId, GroupCode: groupCode, GroupName: snapshot.GroupName, Model: snapshot.Model,
		Protocol: snapshot.Protocol, MessageCount: len(normalized.Messages), ByteSize: len(content), Content: model.RequestArchiveLargeText(storedContent), ContentCipherKind: cipherKind,
		CreatedAt: time.Now().Unix(), ExpiresAt: model.NewConversationArchiveExpiry(retentionDays)}
	if err := model.DB.WithContext(ctx).Create(record).Error; err != nil {
		return nil, err
	}
	return record, nil
}

func normalizeConversationRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	switch role {
	case "system", "developer", "assistant", "tool", "user":
		return role
	default:
		return "user"
	}
}

func cleanConversationText(value string) string {
	return truncateConversationText(strings.TrimSpace(strings.ReplaceAll(value, "\x00", "�")))
}

func truncateConversationTextBytes(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	cut := 0
	for index := range value {
		if index > maxBytes {
			break
		}
		cut = index
	}
	return value[:cut]
}

func marshalNormalizedConversation(normalized *NormalizedConversation, maxBodyBytes int64) ([]byte, error) {
	if normalized == nil {
		return nil, errors.New("conversation archive normalized body is nil")
	}
	if maxBodyBytes <= 0 || maxBodyBytes > ConversationArchiveMaxContentBytes {
		maxBodyBytes = ConversationArchiveMaxContentBytes
	}
	content, err := common.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	// 从尾部丢弃消息，优先保留最早上下文；每次重新编码确保字节上限可靠。
	for len(normalized.Messages) > 1 && int64(len(content)) > maxBodyBytes {
		normalized.Messages = normalized.Messages[:len(normalized.Messages)-1]
		content, err = common.Marshal(normalized)
		if err != nil {
			return nil, err
		}
	}
	for len(normalized.Messages) > 0 && int64(len(content)) > maxBodyBytes {
		last := &normalized.Messages[len(normalized.Messages)-1]
		if last.Text == "" {
			normalized.Messages = normalized.Messages[:len(normalized.Messages)-1]
			content, err = common.Marshal(normalized)
			if err != nil {
				return nil, err
			}
			continue
		}
		excess := int64(len(content)) - maxBodyBytes
		targetBytes := len(last.Text) - int(excess)
		if targetBytes >= len(last.Text) {
			targetBytes = len(last.Text) - 1
		}
		last.Text = truncateConversationTextBytes(last.Text, targetBytes)
		content, err = common.Marshal(normalized)
		if err != nil {
			return nil, err
		}
	}
	if int64(len(content)) > maxBodyBytes {
		return nil, errors.New("conversation archive normalized body too large")
	}
	return content, nil
}

// NormalizeConversation 仅提取角色和纯文本，主动丢弃媒体、工具 schema、请求头及其他冗余字段。
func NormalizeConversation(raw []byte, protocol string) (NormalizedConversation, error) {
	var payload map[string]interface{}
	if err := common.Unmarshal(raw, &payload); err != nil {
		return NormalizedConversation{}, err
	}
	result := NormalizedConversation{Messages: make([]ConversationMessage, 0)}
	if values, ok := payload["messages"].([]interface{}); ok {
		for _, value := range values {
			if len(result.Messages) >= ConversationArchiveMaxMessages {
				break
			}
			if item, ok := value.(map[string]interface{}); ok {
				role, _ := item["role"].(string)
				text := cleanConversationText(extractConversationText(item["content"]))
				if text == "" {
					text = cleanConversationText(extractConversationText(item["text"]))
				}
				role = normalizeConversationRole(role)
				if text != "" {
					result.Messages = append(result.Messages, ConversationMessage{Role: role, Text: text})
				}
			}
		}
	}
	if values, ok := payload["contents"].([]interface{}); ok {
		for _, value := range values {
			if len(result.Messages) >= ConversationArchiveMaxMessages {
				break
			}
			if item, ok := value.(map[string]interface{}); ok {
				role, _ := item["role"].(string)
				text := cleanConversationText(extractConversationText(item["parts"]))
				if text != "" {
					result.Messages = append(result.Messages, ConversationMessage{Role: normalizeConversationRole(role), Text: text})
				}
			}
		}
	}
	if values, ok := payload["input"].([]interface{}); ok {
		for _, value := range values {
			if len(result.Messages) >= ConversationArchiveMaxMessages {
				break
			}
			if item, ok := value.(map[string]interface{}); ok {
				role, _ := item["role"].(string)
				text := cleanConversationText(extractConversationText(item["content"]))
				if text != "" {
					result.Messages = append(result.Messages, ConversationMessage{Role: normalizeConversationRole(role), Text: text})
				}
			}
		}
	}
	if text, ok := payload["input"].(string); ok && len(result.Messages) < ConversationArchiveMaxMessages {
		text = cleanConversationText(text)
		if text != "" {
			result.Messages = append(result.Messages, ConversationMessage{Role: "user", Text: text})
		}
	}
	if len(result.Messages) == 0 {
		return result, fmt.Errorf("unsupported conversation protocol: %s", protocol)
	}
	return result, nil
}

func extractConversationText(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []interface{}:
		var parts []string
		for _, part := range typed {
			if item, ok := part.(map[string]interface{}); ok {
				if text, ok := item["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	case map[string]interface{}:
		if text, ok := typed["text"].(string); ok {
			return text
		}
	}
	return ""
}

func truncateConversationText(value string) string {
	if len(value) <= ConversationArchiveMaxMessageText {
		return value
	}
	runes := []rune(value)
	if len(runes) <= ConversationArchiveMaxMessageText {
		return value
	}
	return string(runes[:ConversationArchiveMaxMessageText])
}

type ConversationArchiveInput struct {
	RequestId     string
	UserId        int
	Username      string
	GroupId       int
	GroupCode     string
	GroupName     string
	Model         string
	Protocol      string
	RawBody       []byte
	RetentionDays int
}

func StoreConversationArchive(ctx context.Context, input ConversationArchiveInput) (*model.ConversationArchive, error) {
	if model.DB == nil {
		return nil, errors.New("数据库尚未初始化")
	}
	if len(input.RawBody) > ConversationArchiveMaxContentBytes {
		return nil, errors.New("conversation archive body too large")
	}
	normalized, err := NormalizeConversation(input.RawBody, input.Protocol)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Model) == "" {
		var envelope map[string]interface{}
		if common.Unmarshal(input.RawBody, &envelope) == nil {
			if value, ok := envelope["model"].(string); ok {
				input.Model = truncateConversationText(value)
			}
		}
	}
	content, err := marshalNormalizedConversation(&normalized, ConversationArchiveMaxContentBytes)
	if err != nil {
		return nil, err
	}
	storedContent, cipherKind, err := StorePromptAuditSecret(string(content))
	if err != nil {
		return nil, err
	}
	record := &model.ConversationArchive{RequestId: input.RequestId, UserId: input.UserId, Username: input.Username,
		GroupId: input.GroupId, GroupCode: model.NormalizeConversationArchiveGroupCode(input.GroupCode), GroupName: input.GroupName, Model: input.Model,
		Protocol: input.Protocol, MessageCount: len(normalized.Messages), ByteSize: len(content), Content: model.RequestArchiveLargeText(storedContent), ContentCipherKind: cipherKind,
		CreatedAt: time.Now().Unix(), ExpiresAt: model.NewConversationArchiveExpiry(input.RetentionDays)}
	if err := model.DB.WithContext(ctx).Create(record).Error; err != nil {
		return nil, err
	}
	return record, nil
}

type ConversationArchiveFilter struct {
	UserId     *int
	UserIds    []int
	GroupCode  string
	GroupCodes []string
	StartAt    int64
	EndAt      int64
	Page       int
	PageSize   int
}

func ListConversationArchives(ctx context.Context, filter ConversationArchiveFilter) ([]model.ConversationArchive, int64, error) {
	if model.DB == nil {
		return nil, 0, errors.New("数据库尚未初始化")
	}
	if len(filter.GroupCodes) > ConversationArchiveMaxGroups || len(filter.UserIds) > ConversationArchiveMaxUsers {
		return nil, 0, errors.New("归档筛选项数量超限")
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 || filter.PageSize > 100 {
		filter.PageSize = 50
	}
	// 过期正文不可再通过列表接口暴露。ExpiresAt 为零的历史记录视为
	// 未设置保留期，保留可见性以兼容早期迁移数据。
	now := time.Now().Unix()
	query := model.DB.WithContext(ctx).Model(&model.ConversationArchive{}).
		Where("(expires_at = 0 OR expires_at > ?)", now)
	if filter.UserId != nil && *filter.UserId > 0 {
		query = query.Where("user_id = ?", *filter.UserId)
	} else if len(filter.UserIds) > 0 {
		query = query.Where("user_id IN ?", filter.UserIds)
	}
	if strings.TrimSpace(filter.GroupCode) != "" {
		query = query.Where("group_code = ?", model.NormalizeConversationArchiveGroupCode(filter.GroupCode))
	} else if len(filter.GroupCodes) > 0 {
		groups := make([]string, 0, len(filter.GroupCodes))
		seen := make(map[string]struct{}, len(filter.GroupCodes))
		for _, group := range filter.GroupCodes {
			group = model.NormalizeConversationArchiveGroupCode(group)
			if group == "" {
				continue
			}
			if _, exists := seen[group]; exists {
				continue
			}
			seen[group] = struct{}{}
			groups = append(groups, group)
		}
		if len(groups) > 0 {
			query = query.Where("group_code IN ?", groups)
		}
	}
	if filter.StartAt > 0 {
		query = query.Where("created_at >= ?", filter.StartAt)
	}
	if filter.EndAt > 0 {
		query = query.Where("created_at <= ?", filter.EndAt)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.ConversationArchive
	if err := query.Order("created_at DESC").Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	for i := range rows {
		rows[i].Content = ""
	}
	return rows, total, nil
}

func GetConversationArchive(ctx context.Context, id int64) (*model.ConversationArchive, error) {
	if model.DB == nil {
		return nil, errors.New("数据库尚未初始化")
	}
	if id <= 0 {
		return nil, errors.New("invalid conversation archive id")
	}
	var row model.ConversationArchive
	if err := model.DB.WithContext(ctx).
		Where("(expires_at = 0 OR expires_at > ?)", time.Now().Unix()).
		First(&row, id).Error; err != nil {
		return nil, err
	}
	if row.Content != "" {
		cipherKind := row.ContentCipherKind
		if strings.TrimSpace(cipherKind) == "" {
			// 迁移前记录没有密文类型列，按历史明文兼容读取。
			cipherKind = model.PromptAuditCipherKindPlaintext
		}
		plain, decryptErr := LoadPromptAuditSecret(string(row.Content), cipherKind)
		if decryptErr != nil {
			return nil, decryptErr
		}
		row.Content = model.RequestArchiveLargeText(plain)
	}
	return &row, nil
}
