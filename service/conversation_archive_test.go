package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupConversationArchiveTestDB(t *testing.T) {
	t.Helper()
	oldDB := model.DB
	dsn := fmt.Sprintf("file:conversation-archive-%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.ConversationArchiveConfig{}, &model.ConversationArchive{}))
	require.NoError(t, db.Create(&model.ConversationArchiveConfig{
		Id: model.ConversationArchiveConfigID, ConfigVersion: 1, GroupCodes: "[]", UserIds: "[]",
		MaxBodyBytes: ConversationArchiveMaxContentBytes, RetentionDays: 30,
	}).Error)
	invalidateConversationArchiveConfig()
	t.Cleanup(func() {
		invalidateConversationArchiveConfig()
		model.DB = oldDB
		_ = sqlDB.Close()
	})
}

func TestNormalizeConversationDropsMediaAndToolFields(t *testing.T) {
	raw := []byte(`{"model":"x","messages":[{"role":"user","content":[{"type":"text","text":"hello"},{"type":"image_url","image_url":{"url":"data:image/png;base64,SECRET"}}]},{"role":"assistant","content":"world","tool_calls":[{"function":{"name":"secret","parameters":{"token":"x"}}}]}]}`)
	result, err := NormalizeConversation(raw, "openai")
	require.NoError(t, err)
	require.Len(t, result.Messages, 2)
	require.Equal(t, "hello", result.Messages[0].Text)
	require.Equal(t, "world", result.Messages[1].Text)
}

func TestNormalizeConversationCleansRoleAndNulBytes(t *testing.T) {
	raw := []byte("{\"messages\":[{\"role\":\"  UNKNOWN  \",\"content\":\" hi\\u0000there \"}]}")
	result, err := NormalizeConversation(raw, "openai")
	require.NoError(t, err)
	require.Len(t, result.Messages, 1)
	require.Equal(t, "user", result.Messages[0].Role)
	require.Equal(t, "hi�there", result.Messages[0].Text)
}

func TestConversationArchiveMatchesFilterUsesANDSemantics(t *testing.T) {
	cfg := &ConversationArchiveConfigView{Enabled: true, UserIds: []int{7}, GroupCodes: []string{"prod"}}
	require.True(t, ConversationArchiveMatchesFilter(7, "prod", cfg))
	require.False(t, ConversationArchiveMatchesFilter(8, "prod", cfg))
	require.False(t, ConversationArchiveMatchesFilter(7, "dev", cfg))
}

func TestConversationArchiveConfigUsesCASAndNormalizesFilters(t *testing.T) {
	setupConversationArchiveTestDB(t)

	updated, err := SaveConversationArchiveConfig(context.Background(), ConversationArchiveConfigUpdate{
		ExpectedConfigVersion: 1,
		Enabled:               true,
		GroupCodes:            []string{" PROD ", "prod", "staging"},
		UserIds:               []int{7, 7, 8},
		MaxBodyBytes:          128 * 1024,
		RetentionDays:         14,
	}, 99)
	require.NoError(t, err)
	require.Equal(t, int64(2), updated.ConfigVersion)
	// 分组稳定代码保留大小写，PROD 与 prod 可以是两个独立分组。
	require.Equal(t, []string{"PROD", "prod", "staging"}, updated.GroupCodes)
	require.Equal(t, []int{7, 8}, updated.UserIds)

	_, err = SaveConversationArchiveConfig(context.Background(), ConversationArchiveConfigUpdate{
		ExpectedConfigVersion: 1,
		Enabled:               false,
		GroupCodes:            []string{}, UserIds: []int{},
		MaxBodyBytes: 128 * 1024, RetentionDays: 14,
	}, 99)
	require.ErrorIs(t, err, ErrConversationArchiveConfigConflict)
}

func TestSaveConversationArchiveConfigImmediatelyTrimsExistingArchives(t *testing.T) {
	setupConversationArchiveTestDB(t)
	expiresAt := time.Now().Add(time.Hour).Unix()
	records := []model.ConversationArchive{
		{RequestId: "archive-oldest", Content: model.RequestArchiveLargeText(`{"messages":[]}`), CreatedAt: 10, ExpiresAt: expiresAt},
		{RequestId: "archive-middle", Content: model.RequestArchiveLargeText(`{"messages":[]}`), CreatedAt: 20, ExpiresAt: expiresAt},
		{RequestId: "archive-newest", Content: model.RequestArchiveLargeText(`{"messages":[]}`), CreatedAt: 30, ExpiresAt: expiresAt},
	}
	require.NoError(t, model.DB.Create(&records).Error)

	updated, err := SaveConversationArchiveConfig(context.Background(), ConversationArchiveConfigUpdate{
		ExpectedConfigVersion: 1,
		MaxBodyBytes:          64 * 1024,
		RetentionDays:         30,
		MaxArchiveCount:       2,
	}, 99)
	require.NoError(t, err)
	require.Equal(t, 2, updated.MaxArchiveCount)

	var remaining []model.ConversationArchive
	require.NoError(t, model.DB.Order("created_at ASC, id ASC").Find(&remaining).Error)
	require.Len(t, remaining, 2)
	assert.Equal(t, []string{"archive-middle", "archive-newest"}, []string{remaining[0].RequestId, remaining[1].RequestId})
}

func TestSaveConversationArchiveConfigInitializesMissingRow(t *testing.T) {
	setupConversationArchiveTestDB(t)
	require.NoError(t, model.DB.Delete(&model.ConversationArchiveConfig{}, model.ConversationArchiveConfigID).Error)
	invalidateConversationArchiveConfig()

	updated, err := SaveConversationArchiveConfig(context.Background(), ConversationArchiveConfigUpdate{
		ExpectedConfigVersion: 1,
		Enabled:               true,
		MaxBodyBytes:          128 * 1024,
		RetentionDays:         7,
	}, 99)
	require.NoError(t, err)
	require.Equal(t, int64(2), updated.ConfigVersion)
	require.True(t, updated.Enabled)
}

func TestConversationArchiveConfigRejectsCorruptFilterJSON(t *testing.T) {
	setupConversationArchiveTestDB(t)
	require.NoError(t, model.DB.Model(&model.ConversationArchiveConfig{}).
		Where("id = ?", model.ConversationArchiveConfigID).
		Updates(map[string]interface{}{"enabled": true, "group_codes": "not-json"}).Error)

	_, err := GetConversationArchiveConfig(context.Background())
	require.EqualError(t, err, "对话归档分组配置损坏")
}

func TestConversationArchiveRealtimeExtractsTextAndDropsMedia(t *testing.T) {
	segments := extractConversationArchiveRealtimeSegments([]byte(`{"type":"response.output_text.delta","delta":"assistant text","response":{"output":[{"text":"duplicate path"}]},"audio":"data:audio/wav;base64,AAAA"}`), "assistant")
	require.Len(t, segments, 2)
	assert.Equal(t, "assistant text", segments[0].Text)
	assert.Equal(t, "duplicate path", segments[1].Text)
	for _, segment := range segments {
		assert.NotContains(t, segment.Text, "data:audio")
	}
}

func TestConversationArchiveRealtimeIgnoresDoneSummary(t *testing.T) {
	segments := extractConversationArchiveRealtimeSegments([]byte(`{"type":"response.output_text.done","text":"full response"}`), "assistant")
	require.Empty(t, segments)
}

func TestFinalizeConversationArchiveRealtimeStoresCleanConversation(t *testing.T) {
	setupConversationArchiveTestDB(t)
	config, err := GetConversationArchiveConfig(context.Background())
	require.NoError(t, err)
	_, err = SaveConversationArchiveConfig(context.Background(), ConversationArchiveConfigUpdate{
		ExpectedConfigVersion: config.ConfigVersion,
		Enabled:               true,
		GroupCodes:            []string{"prod"},
		MaxBodyBytes:          ConversationArchiveMaxContentBytes,
		RetentionDays:         30,
	}, 1)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime?model=gpt-realtime", nil)
	c.Set(common.RequestIdKey, "realtime-archive-request")
	common.SetContextKey(c, constant.ContextKeyUserId, 7)
	common.SetContextKey(c, constant.ContextKeyUserName, "archive-user")
	common.SetContextKey(c, constant.ContextKeySelectedChannelGroup, "prod")

	CaptureConversationArchiveRealtimeFrame(c, []byte(`{"type":"conversation.item.create","item":{"role":"user","content":[{"type":"input_text","text":"hello"}]}}`), "client")
	CaptureConversationArchiveRealtimeFrame(c, []byte(`{"type":"response.output_text.delta","delta":"world","audio":"data:audio/wav;base64,SECRET"}`), "assistant")
	FinalizeConversationArchiveRealtime(c)

	var row model.ConversationArchive
	require.NoError(t, model.DB.First(&row, "request_id = ?", "realtime-archive-request").Error)
	detail, err := GetConversationArchive(context.Background(), row.Id)
	require.NoError(t, err)
	require.Contains(t, string(detail.Content), `"text":"hello"`)
	require.Contains(t, string(detail.Content), `"text":"world"`)
	require.NotContains(t, string(detail.Content), "data:audio")
	require.Equal(t, "prod", detail.GroupCode)
}

func TestStoreConversationArchiveFromSnapshotExcludesToolDefinitions(t *testing.T) {
	setupConversationArchiveTestDB(t)
	snapshot, err := ExtractPromptAuditSnapshot(PromptAuditRequest{
		RequestId: "req-1", UserId: 7, GroupCode: " PROD ", Protocol: "openai_chat_completions",
		Body: []byte(`{"messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"secret_tool","description":"must not persist","parameters":{"token":"secret"}}}]}`),
	})
	require.NoError(t, err)
	record, err := StoreConversationArchiveFromSnapshot(context.Background(), snapshot, 30, ConversationArchiveMaxContentBytes)
	require.NoError(t, err)
	require.Equal(t, "PROD", record.GroupCode)
	require.Equal(t, 1, record.MessageCount)
	require.NotContains(t, string(record.Content), "secret_tool")
	require.NotContains(t, string(record.Content), "token")
	require.Contains(t, string(record.Content), "hello")
}

func TestStoreConversationArchiveFromSnapshotRespectsNormalizedBodyLimit(t *testing.T) {
	setupConversationArchiveTestDB(t)
	segments := make([]PromptAuditContextSegment, 0, 4)
	for i := 0; i < 4; i++ {
		segments = append(segments, PromptAuditContextSegment{Role: "user", Text: strings.Repeat("x", 64*1024)})
	}
	record, err := StoreConversationArchiveFromSnapshot(context.Background(), PromptAuditSnapshot{
		RequestId: "bounded-body", UserId: 7, GroupCode: "prod", ContextSegments: segments,
	}, 30, 128*1024)
	require.NoError(t, err)
	require.LessOrEqual(t, record.ByteSize, 128*1024)
	require.LessOrEqual(t, record.MessageCount, 2)
}

func TestStoreConversationArchiveFromSnapshotTruncatesSingleMessageToFitEnvelope(t *testing.T) {
	setupConversationArchiveTestDB(t)
	record, err := StoreConversationArchiveFromSnapshot(context.Background(), PromptAuditSnapshot{
		RequestId: "single-message-envelope", UserId: 7, GroupCode: "prod",
		ContextSegments: []PromptAuditContextSegment{{Role: "user", Text: strings.Repeat("x", ConversationArchiveMaxMessageText)}},
	}, 30, 64*1024)
	require.NoError(t, err)
	require.LessOrEqual(t, record.ByteSize, 64*1024)
	require.Equal(t, 1, record.MessageCount)
}

func TestFinalizeConversationArchiveRealtimeSucceedsAfterContextCancelled(t *testing.T) {
	setupConversationArchiveTestDB(t)
	config, err := GetConversationArchiveConfig(context.Background())
	require.NoError(t, err)
	_, err = SaveConversationArchiveConfig(context.Background(), ConversationArchiveConfigUpdate{
		ExpectedConfigVersion: config.ConfigVersion,
		Enabled:               true,
		GroupCodes:            []string{"prod"},
		MaxBodyBytes:          ConversationArchiveMaxContentBytes,
		RetentionDays:         30,
	}, 1)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	// Simulate a cancelled request context, as happens when the WebSocket disconnects.
	ctx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime?model=gpt-realtime", nil).WithContext(ctx)
	c.Set(common.RequestIdKey, "realtime-cancelled-ctx")
	common.SetContextKey(c, constant.ContextKeyUserId, 7)
	common.SetContextKey(c, constant.ContextKeyUserName, "archive-user")
	common.SetContextKey(c, constant.ContextKeySelectedChannelGroup, "prod")

	CaptureConversationArchiveRealtimeFrame(c, []byte(`{"type":"conversation.item.create","item":{"role":"user","content":[{"type":"input_text","text":"hello cancelled"}]}}`), "client")
	// Cancel before finalizing — this is the WebSocket disconnect scenario.
	cancel()

	FinalizeConversationArchiveRealtime(c)

	var row model.ConversationArchive
	require.NoError(t, model.DB.First(&row, "request_id = ?", "realtime-cancelled-ctx").Error,
		"archive must be written even when request context is cancelled")
	detail, err := GetConversationArchive(context.Background(), row.Id)
	require.NoError(t, err)
	require.Contains(t, string(detail.Content), "hello cancelled")
}

func TestListConversationArchivesSupportsMultipleUsersAndGroupsWithoutContent(t *testing.T) {
	setupConversationArchiveTestDB(t)
	expiresAt := time.Now().Add(time.Hour).Unix()
	records := []model.ConversationArchive{
		{RequestId: "one", UserId: 7, GroupCode: "prod", Content: model.RequestArchiveLargeText(`{"messages":[]}`), CreatedAt: 10, ExpiresAt: expiresAt},
		{RequestId: "two", UserId: 8, GroupCode: "staging", Content: model.RequestArchiveLargeText(`{"messages":[]}`), CreatedAt: 20, ExpiresAt: expiresAt},
		{RequestId: "three", UserId: 9, GroupCode: "prod", Content: model.RequestArchiveLargeText(`{"messages":[]}`), CreatedAt: 30, ExpiresAt: expiresAt},
	}
	require.NoError(t, model.DB.Create(&records).Error)
	rows, total, err := ListConversationArchives(context.Background(), ConversationArchiveFilter{
		UserIds: []int{7, 8}, GroupCodes: []string{"prod", "staging"}, Page: 1, PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, rows, 2)
	require.Empty(t, rows[0].Content)
	require.Empty(t, rows[1].Content)
}
