package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestConversationArchiveListAndDetailHideExpiredRows(t *testing.T) {
	setupConversationArchiveTestDB(t)
	now := time.Now().Unix()
	rows := []model.ConversationArchive{
		{RequestId: "expired", UserId: 1, GroupCode: "prod", Content: model.RequestArchiveLargeText(`{"messages":[]}`), ContentCipherKind: model.PromptAuditCipherKindPlaintext, CreatedAt: now - 100, ExpiresAt: now - 1},
		{RequestId: "active", UserId: 1, GroupCode: "prod", Content: model.RequestArchiveLargeText(`{"messages":[]}`), ContentCipherKind: model.PromptAuditCipherKindPlaintext, CreatedAt: now, ExpiresAt: now + 3600},
		{RequestId: "legacy", UserId: 1, GroupCode: "prod", Content: model.RequestArchiveLargeText(`{"messages":[]}`), ContentCipherKind: model.PromptAuditCipherKindPlaintext, CreatedAt: now, ExpiresAt: 0},
	}
	require.NoError(t, model.DB.Create(&rows).Error)

	listed, total, err := ListConversationArchives(context.Background(), ConversationArchiveFilter{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, listed, 2)

	_, err = GetConversationArchive(context.Background(), rows[0].Id)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	detail, err := GetConversationArchive(context.Background(), rows[1].Id)
	require.NoError(t, err)
	require.Equal(t, "active", detail.RequestId)
}

func TestDeleteExpiredConversationArchiveBatchIsBounded(t *testing.T) {
	setupConversationArchiveTestDB(t)
	now := time.Now().Unix()
	rows := []model.ConversationArchive{
		{RequestId: "expired-1", Content: model.RequestArchiveLargeText(`{"messages":[]}`), ContentCipherKind: model.PromptAuditCipherKindPlaintext, ExpiresAt: now - 10},
		{RequestId: "expired-2", Content: model.RequestArchiveLargeText(`{"messages":[]}`), ContentCipherKind: model.PromptAuditCipherKindPlaintext, ExpiresAt: now - 5},
		{RequestId: "active", Content: model.RequestArchiveLargeText(`{"messages":[]}`), ContentCipherKind: model.PromptAuditCipherKindPlaintext, ExpiresAt: now + 3600},
	}
	require.NoError(t, model.DB.Create(&rows).Error)

	deleted, err := model.DeleteExpiredConversationArchiveBatch(context.Background(), now, 1)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)
	var remaining int64
	require.NoError(t, model.DB.Model(&model.ConversationArchive{}).Count(&remaining).Error)
	require.Equal(t, int64(2), remaining)

	deleted, err = model.DeleteExpiredConversationArchiveBatch(context.Background(), now, 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)
	require.NoError(t, model.DB.Model(&model.ConversationArchive{}).Where("request_id = ?", "active").First(&model.ConversationArchive{}).Error)
}

func TestGetConversationArchiveReadsLegacyPlaintextCipherKind(t *testing.T) {
	setupConversationArchiveTestDB(t)
	row := model.ConversationArchive{
		RequestId: "legacy-plaintext-kind", Content: model.RequestArchiveLargeText(`{"messages":[{"role":"user","text":"hello"}]}`),
		ContentCipherKind: "plaintext", ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	require.NoError(t, model.DB.Create(&row).Error)
	detail, err := GetConversationArchive(context.Background(), row.Id)
	require.NoError(t, err)
	require.Contains(t, string(detail.Content), "hello")
}
