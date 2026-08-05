package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPromptAuditDeleteConfirmationUsesFilterSnapshot(t *testing.T) {
	db := setupPromptAuditAdminServiceTest(t)
	first := promptAuditDeleteTestEvent("flag", "first")
	require.NoError(t, db.Create(&first).Error)

	filter := model.PromptAuditEventFilter{Decision: "flag"}
	preview, err := PreviewPromptAuditDelete(filter)
	require.NoError(t, err)
	require.EqualValues(t, 1, preview.MatchedCount)
	require.Equal(t, first.Id, preview.SnapshotMaxId)
	require.NotEmpty(t, preview.FilterHash)
	require.NotEmpty(t, preview.ConfirmationToken)

	// 预览后新增的匹配事件不能进入本次删除快照。
	second := promptAuditDeleteTestEvent("flag", "second")
	require.NoError(t, db.Create(&second).Error)
	result, err := DeletePromptAuditByConfirmedFilter(filter, preview.ConfirmationToken)
	require.NoError(t, err)
	require.EqualValues(t, 1, result.DeletedEvents)

	var remaining []model.PromptAuditEvent
	require.NoError(t, db.Order("id ASC").Find(&remaining).Error)
	require.Len(t, remaining, 1)
	require.Equal(t, second.Id, remaining[0].Id)

	_, err = DeletePromptAuditByConfirmedFilter(
		model.PromptAuditEventFilter{Decision: "critical"}, preview.ConfirmationToken,
	)
	require.ErrorContains(t, err, "筛选条件与预览不一致")
}

func TestPromptAuditDeleteConfirmationWithZeroMatchesIsNoOp(t *testing.T) {
	db := setupPromptAuditAdminServiceTest(t)
	filter := model.PromptAuditEventFilter{Decision: "critical"}
	preview, err := PreviewPromptAuditDelete(filter)
	require.NoError(t, err)
	require.Zero(t, preview.MatchedCount)
	require.Zero(t, preview.SnapshotMaxId)

	result, err := DeletePromptAuditByConfirmedFilter(filter, preview.ConfirmationToken)
	require.NoError(t, err)
	require.Zero(t, result.DeletedEvents)
	require.Zero(t, result.DeletedJobs)

	var count int64
	require.NoError(t, db.Model(&model.PromptAuditEvent{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestPromptAuditDeleteConfirmationSupportsChannelFilter(t *testing.T) {
	db := setupPromptAuditAdminServiceTest(t)
	first := promptAuditDeleteTestEvent("flag", "channel-41")
	first.ChannelId = 41
	second := promptAuditDeleteTestEvent("flag", "channel-42")
	second.ChannelId = 42
	require.NoError(t, db.Create(&first).Error)
	require.NoError(t, db.Create(&second).Error)

	filter := model.PromptAuditEventFilter{ChannelId: 42}
	preview, err := PreviewPromptAuditDelete(filter)
	require.NoError(t, err)
	require.EqualValues(t, 1, preview.MatchedCount)

	result, err := DeletePromptAuditByConfirmedFilter(filter, preview.ConfirmationToken)
	require.NoError(t, err)
	require.EqualValues(t, 1, result.DeletedEvents)

	var remaining model.PromptAuditEvent
	require.NoError(t, db.First(&remaining).Error)
	require.Equal(t, 41, remaining.ChannelId)
}

func TestPromptAuditDeleteConfirmationBindsAdmin(t *testing.T) {
	db := setupPromptAuditAdminServiceTest(t)
	event := promptAuditDeleteTestEvent("flag", "admin-bound")
	require.NoError(t, db.Create(&event).Error)
	filter := model.PromptAuditEventFilter{Decision: "flag"}
	preview, err := PreviewPromptAuditDeleteForActor(filter, 11)
	require.NoError(t, err)
	_, err = DeletePromptAuditByConfirmedFilterForActor(filter, preview.ConfirmationToken, 12)
	require.ErrorContains(t, err, "不属于当前管理员")
	result, err := DeletePromptAuditByConfirmedFilterForActor(filter, preview.ConfirmationToken, 11)
	require.NoError(t, err)
	require.EqualValues(t, 1, result.DeletedEvents)
}

func TestPromptAuditDeleteConfirmationNormalizesUsernameSnapshotFilter(t *testing.T) {
	db := setupPromptAuditAdminServiceTest(t)
	alice := promptAuditDeleteTestEvent("flag", "username-delete-alice")
	alice.Username = "Alice.Admin"
	bob := promptAuditDeleteTestEvent("flag", "username-delete-bob")
	bob.Username = "Bob.Admin"
	require.NoError(t, db.Create(&alice).Error)
	require.NoError(t, db.Create(&bob).Error)

	preview, err := PreviewPromptAuditDelete(model.PromptAuditEventFilter{Username: "  ALICE.ad  "})
	require.NoError(t, err)
	require.EqualValues(t, 1, preview.MatchedCount)

	result, err := DeletePromptAuditByConfirmedFilter(
		model.PromptAuditEventFilter{Username: "alice.AD"},
		preview.ConfirmationToken,
	)
	require.NoError(t, err)
	require.EqualValues(t, 1, result.DeletedEvents)

	var remaining model.PromptAuditEvent
	require.NoError(t, db.First(&remaining).Error)
	require.Equal(t, bob.RequestId, remaining.RequestId)
}

func TestPromptAuditDeleteConfirmationRejectsChangedUsernameFilter(t *testing.T) {
	db := setupPromptAuditAdminServiceTest(t)
	alice := promptAuditDeleteTestEvent("flag", "username-token-alice")
	alice.Username = "Alice"
	bob := promptAuditDeleteTestEvent("flag", "username-token-bob")
	bob.Username = "Bob"
	require.NoError(t, db.Create(&alice).Error)
	require.NoError(t, db.Create(&bob).Error)

	preview, err := PreviewPromptAuditDelete(model.PromptAuditEventFilter{Username: "alice"})
	require.NoError(t, err)
	_, err = DeletePromptAuditByConfirmedFilter(
		model.PromptAuditEventFilter{Username: "bob"},
		preview.ConfirmationToken,
	)
	require.ErrorContains(t, err, "删除筛选条件与预览不一致")
}

func TestPromptAuditDeleteConfirmationBindsActionFilter(t *testing.T) {
	db := setupPromptAuditAdminServiceTest(t)
	blocked := promptAuditDeleteTestEvent("critical", "action-token-block")
	blocked.Action = "Block"
	masked := promptAuditDeleteTestEvent("flag", "action-token-mask")
	masked.Action = "Mask"
	require.NoError(t, db.Create(&masked).Error)
	require.NoError(t, db.Create(&blocked).Error)

	preview, err := PreviewPromptAuditDelete(model.PromptAuditEventFilter{Action: " BLOCK "})
	require.NoError(t, err)
	require.EqualValues(t, 1, preview.MatchedCount)

	_, err = DeletePromptAuditByConfirmedFilter(
		model.PromptAuditEventFilter{Action: "mask"},
		preview.ConfirmationToken,
	)
	require.ErrorContains(t, err, "删除筛选条件与预览不一致")

	result, err := DeletePromptAuditByConfirmedFilter(
		model.PromptAuditEventFilter{Action: "block"},
		preview.ConfirmationToken,
	)
	require.NoError(t, err)
	require.EqualValues(t, 1, result.DeletedEvents)

	var remaining model.PromptAuditEvent
	require.NoError(t, db.Where("request_id = ?", masked.RequestId).First(&remaining).Error)
	require.Equal(t, "Mask", remaining.Action)
}

func TestPromptAuditDeleteConfirmationRejectsExpiredOrMissingKey(t *testing.T) {
	setupPromptAuditAdminServiceTest(t)
	filterHash, err := promptAuditFilterHash(model.PromptAuditEventFilter{Decision: "flag"})
	require.NoError(t, err)
	expired, err := signPromptAuditDeleteClaims(promptAuditDeleteClaims{
		MatchedCount: 1, SnapshotMaxId: 1, FilterHash: filterHash,
		ExpiresAt: time.Now().Add(-time.Second).Unix(), Nonce: "expired",
	})
	require.NoError(t, err)
	_, err = verifyPromptAuditDeleteClaims(expired)
	require.ErrorContains(t, err, "已过期")

	common.CryptoSecret = ""
	t.Setenv("CRYPTO_SECRET", "")
	_, err = signPromptAuditDeleteClaims(promptAuditDeleteClaims{
		FilterHash: filterHash, ExpiresAt: time.Now().Add(time.Minute).Unix(), Nonce: "no-key",
	})
	require.ErrorContains(t, err, "CRYPTO_SECRET")
}

func TestPromptAuditDeletePreviewRejectsNegativeNoOpFilters(t *testing.T) {
	db := setupPromptAuditAdminServiceTest(t)
	event := promptAuditDeleteTestEvent("flag", "must-survive-negative-filter")
	require.NoError(t, db.Create(&event).Error)

	tests := []model.PromptAuditEventFilter{
		{UserId: -1},
		{TokenId: -1},
		{GroupId: -1},
		{ChannelId: -1},
		{StartAt: -1},
		{EndAt: -1},
	}
	for _, filter := range tests {
		_, err := PreviewPromptAuditDelete(filter)
		require.ErrorContains(t, err, "不能为负数")
	}

	var count int64
	require.NoError(t, db.Model(&model.PromptAuditEvent{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func setupPromptAuditAdminServiceTest(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := model.DB
	oldSecret := common.CryptoSecret
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "prompt-audit-admin.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PromptAuditJob{}, &model.PromptAuditEvent{}))
	model.DB = db
	t.Setenv("CRYPTO_SECRET", "stable-delete-confirmation-secret")
	common.CryptoSecret = "stable-delete-confirmation-secret"
	t.Cleanup(func() {
		common.CryptoSecret = oldSecret
		model.DB = oldDB
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func promptAuditDeleteTestEvent(decision, requestId string) model.PromptAuditEvent {
	now := time.Now().Unix()
	return model.PromptAuditEvent{
		RequestId: requestId, Decision: decision, RiskLevel: "medium", Action: "Warn",
		Safety: "Controversial", Categories: "[]", MatchedScanners: "[]",
		CreatedAt: now, ExpiresAt: now + 3600,
	}
}
