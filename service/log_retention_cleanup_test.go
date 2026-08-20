package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunLogRetentionCleanupOnceHonorsRetentionSetting(t *testing.T) {
	truncate(t)
	previous := common.GetLogRetentionDays()
	t.Cleanup(func() {
		common.SetLogRetentionDays(previous)
		logRetentionCleanupRunning.Store(false)
	})

	now := time.Now()
	oldLog := &model.Log{UserId: 1, Username: "old", Type: model.LogTypeConsume, CreatedAt: now.Add(-48 * time.Hour).Unix()}
	recentLog := &model.Log{UserId: 2, Username: "recent", Type: model.LogTypeConsume, CreatedAt: now.Add(-12 * time.Hour).Unix()}
	require.NoError(t, model.LOG_DB.Create(oldLog).Error)
	require.NoError(t, model.LOG_DB.Create(recentLog).Error)

	common.SetLogRetentionDays(0)
	runLogRetentionCleanupOnceAt(now)
	assertLogExists(t, oldLog.Id)
	assertLogExists(t, recentLog.Id)

	common.SetLogRetentionDays(1)
	runLogRetentionCleanupOnceAt(now)
	assertLogMissing(t, oldLog.Id)
	assertLogExists(t, recentLog.Id)
}

func assertLogExists(t *testing.T, id int) {
	t.Helper()
	var count int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("id = ?", id).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func assertLogMissing(t *testing.T, id int) {
	t.Helper()
	var count int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("id = ?", id).Count(&count).Error)
	assert.Zero(t, count)
}
