package model

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func isolateTokenQuotaBatchTest(t *testing.T) {
	t.Helper()
	previousBatchUpdateEnabled := common.BatchUpdateEnabled
	previousRedisEnabled := common.RedisEnabled
	batchUpdateLocks[BatchUpdateTypeTokenQuota].Lock()
	previousStore := batchUpdateStores[BatchUpdateTypeTokenQuota]
	previousInFlight := tokenQuotaBatchInFlight
	batchUpdateStores[BatchUpdateTypeTokenQuota] = make(map[int]int)
	tokenQuotaBatchInFlight = make(map[int]int)
	batchUpdateLocks[BatchUpdateTypeTokenQuota].Unlock()
	common.BatchUpdateEnabled = true
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.BatchUpdateEnabled = previousBatchUpdateEnabled
		common.RedisEnabled = previousRedisEnabled
		batchUpdateLocks[BatchUpdateTypeTokenQuota].Lock()
		batchUpdateStores[BatchUpdateTypeTokenQuota] = previousStore
		tokenQuotaBatchInFlight = previousInFlight
		batchUpdateLocks[BatchUpdateTypeTokenQuota].Unlock()
	})
}

func prepareTokenQuotaCompensationTest(t *testing.T, tokenID int, tokenKey string) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&TokenQuotaCompensation{}))
	require.NoError(t, DB.Where("token_id = ?", tokenID).Delete(&TokenQuotaCompensation{}).Error)
	require.NoError(t, DB.Unscoped().Where("id = ?", tokenID).Delete(&Token{}).Error)
	require.NoError(t, DB.Create(&Token{
		Id: tokenID, UserId: tokenID, Key: tokenKey, Name: "compensation-test",
		Status: common.TokenStatusEnabled, RemainQuota: 90, UsedQuota: 10,
	}).Error)
	t.Cleanup(func() {
		_ = DB.Where("token_id = ?", tokenID).Delete(&TokenQuotaCompensation{}).Error
		_ = DB.Unscoped().Where("id = ?", tokenID).Delete(&Token{}).Error
	})
}

func TestApplyTokenQuotaCompensationIsIdempotent(t *testing.T) {
	const (
		tokenID  = 19501
		tokenKey = "idempotent-token-compensation"
	)
	prepareTokenQuotaCompensationTest(t, tokenID, tokenKey)

	require.NoError(t, ApplyTokenQuotaCompensation("request:reserve:100", tokenID, tokenKey, 10))
	require.NoError(t, ApplyTokenQuotaCompensation("request:reserve:100", tokenID, tokenKey, 10))

	var token Token
	require.NoError(t, DB.First(&token, tokenID).Error)
	require.Equal(t, 100, token.RemainQuota)
	require.Zero(t, token.UsedQuota)

	var count int64
	require.NoError(t, DB.Model(&TokenQuotaCompensation{}).Where("token_id = ?", tokenID).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestApplyTokenQuotaCompensationRejectsPayloadConflict(t *testing.T) {
	const (
		tokenID  = 19502
		tokenKey = "conflicting-token-compensation"
	)
	prepareTokenQuotaCompensationTest(t, tokenID, tokenKey)

	require.NoError(t, ApplyTokenQuotaCompensation("request:reserve:200", tokenID, tokenKey, 10))
	err := ApplyTokenQuotaCompensation("request:reserve:200", tokenID, tokenKey, 11)
	require.ErrorIs(t, err, ErrTokenQuotaCompensationConflict)

	var token Token
	require.NoError(t, DB.First(&token, tokenID).Error)
	require.Equal(t, 100, token.RemainQuota)
	require.Zero(t, token.UsedQuota)
}

func TestApplyTokenQuotaCompensationConcurrentReplayAppliesOnce(t *testing.T) {
	const (
		tokenID  = 19504
		tokenKey = "concurrent-token-compensation"
	)
	prepareTokenQuotaCompensationTest(t, tokenID, tokenKey)

	const workers = 8
	errs := make(chan error, workers)
	var waitGroup sync.WaitGroup
	for i := 0; i < workers; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			errs <- ApplyTokenQuotaCompensation("request:reserve:concurrent", tokenID, tokenKey, 10)
		}()
	}
	waitGroup.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	var token Token
	require.NoError(t, DB.First(&token, tokenID).Error)
	require.Equal(t, 100, token.RemainQuota)
	require.Zero(t, token.UsedQuota)
}

func TestPendingTokenQuotaCompensationCanBeRecovered(t *testing.T) {
	const (
		tokenID  = 19503
		tokenKey = "recoverable-token-compensation"
	)
	prepareTokenQuotaCompensationTest(t, tokenID, tokenKey)

	injectedErr := errors.New("forced token compensation update failure")
	callbackName := "test:pending_token_compensation"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "tokens" {
			tx.AddError(injectedErr)
		}
	}))
	callbackRegistered := true
	t.Cleanup(func() {
		if callbackRegistered {
			_ = DB.Callback().Update().Remove(callbackName)
		}
	})

	err := ApplyTokenQuotaCompensation("request:reserve:300", tokenID, tokenKey, 10)
	require.ErrorIs(t, err, injectedErr)

	operationKey := normalizeTokenQuotaCompensationKey("request:reserve:300")
	var pending TokenQuotaCompensation
	require.NoError(t, DB.Where("operation_key = ?", operationKey).First(&pending).Error)
	require.Equal(t, "pending", pending.Status)

	require.NoError(t, DB.Callback().Update().Remove(callbackName))
	callbackRegistered = false
	processed, err := ProcessPendingTokenQuotaCompensations(10)
	require.NoError(t, err)
	require.Equal(t, 1, processed)

	var token Token
	require.NoError(t, DB.First(&token, tokenID).Error)
	require.Equal(t, 100, token.RemainQuota)
	require.Zero(t, token.UsedQuota)
	require.NoError(t, DB.Where("operation_key = ?", operationKey).First(&pending).Error)
	require.Equal(t, "applied", pending.Status)
	require.True(t, pending.CacheInvalidated)
}

func TestBatchTokenQuotaReservationCompensatesBeforeConcurrentFlush(t *testing.T) {
	const (
		tokenID  = 19505
		tokenKey = "batch-token-compensation"
	)
	prepareTokenQuotaCompensationTest(t, tokenID, tokenKey)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", tokenID).Updates(map[string]interface{}{
		"remain_quota": 100,
		"used_quota":   0,
	}).Error)
	isolateTokenQuotaBatchTest(t)

	reservation, err := BeginTokenQuotaReservation(tokenID, tokenKey, 10, false)
	require.NoError(t, err)

	flushDone := make(chan struct{})
	go func() {
		flushTokenQuotaBatchUpdates()
		close(flushDone)
	}()
	require.Eventually(t, func() bool {
		batchUpdateLocks[BatchUpdateTypeTokenQuota].Lock()
		defer batchUpdateLocks[BatchUpdateTypeTokenQuota].Unlock()
		return tokenQuotaBatchInFlight[tokenID] == -10
	}, time.Second, time.Millisecond)

	require.NoError(t, reservation.Compensate("batch-token-compensation"))
	select {
	case <-flushDone:
	case <-time.After(time.Second):
		t.Fatal("token quota batch flush did not finish")
	}

	var token Token
	require.NoError(t, DB.First(&token, tokenID).Error)
	require.Equal(t, 100, token.RemainQuota)
	require.Zero(t, token.UsedQuota)
	require.Zero(t, pendingTokenQuotaDeltaLocked(tokenID))
}

func TestBatchTokenQuotaReservationCommitFlushesOnce(t *testing.T) {
	const (
		tokenID  = 19506
		tokenKey = "batch-token-commit"
	)
	prepareTokenQuotaCompensationTest(t, tokenID, tokenKey)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", tokenID).Updates(map[string]interface{}{
		"remain_quota": 100,
		"used_quota":   0,
	}).Error)
	isolateTokenQuotaBatchTest(t)

	reservation, err := BeginTokenQuotaReservation(tokenID, tokenKey, 10, false)
	require.NoError(t, err)
	reservation.Commit()
	flushTokenQuotaBatchUpdates()
	flushTokenQuotaBatchUpdates()

	var token Token
	require.NoError(t, DB.First(&token, tokenID).Error)
	require.Equal(t, 90, token.RemainQuota)
	require.Equal(t, 10, token.UsedQuota)
	require.Zero(t, pendingTokenQuotaDeltaLocked(tokenID))
}

func TestTokenQuotaBatchFlushFailureRestoresDelta(t *testing.T) {
	const (
		tokenID  = 19507
		tokenKey = "batch-token-flush-retry"
	)
	prepareTokenQuotaCompensationTest(t, tokenID, tokenKey)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", tokenID).Updates(map[string]interface{}{
		"remain_quota": 100,
		"used_quota":   0,
	}).Error)
	isolateTokenQuotaBatchTest(t)

	reservation, err := BeginTokenQuotaReservation(tokenID, tokenKey, 10, false)
	require.NoError(t, err)
	reservation.Commit()

	injectedErr := errors.New("forced token batch flush failure")
	callbackName := "test:token_batch_flush_retry"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "tokens" {
			tx.AddError(injectedErr)
		}
	}))
	callbackRegistered := true
	t.Cleanup(func() {
		if callbackRegistered {
			_ = DB.Callback().Update().Remove(callbackName)
		}
	})

	flushTokenQuotaBatchUpdates()
	require.Equal(t, -10, pendingTokenQuotaDeltaLocked(tokenID))

	var token Token
	require.NoError(t, DB.First(&token, tokenID).Error)
	require.Equal(t, 100, token.RemainQuota)
	require.Zero(t, token.UsedQuota)

	require.NoError(t, DB.Callback().Update().Remove(callbackName))
	callbackRegistered = false
	flushTokenQuotaBatchUpdates()
	require.Zero(t, pendingTokenQuotaDeltaLocked(tokenID))
	require.NoError(t, DB.First(&token, tokenID).Error)
	require.Equal(t, 90, token.RemainQuota)
	require.Equal(t, 10, token.UsedQuota)
}

func TestCleanupTokenQuotaCompensationsDeletesOnlyCompletedExpiredRecords(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&TokenQuotaCompensation{}))
	keys := []string{
		normalizeTokenQuotaCompensationKey("cleanup-expired"),
		normalizeTokenQuotaCompensationKey("cleanup-cache-pending"),
		normalizeTokenQuotaCompensationKey("cleanup-operation-pending"),
		normalizeTokenQuotaCompensationKey("cleanup-recent"),
	}
	require.NoError(t, DB.Where("operation_key IN ?", keys).Delete(&TokenQuotaCompensation{}).Error)
	t.Cleanup(func() {
		_ = DB.Where("operation_key IN ?", keys).Delete(&TokenQuotaCompensation{}).Error
	})

	now := common.GetTimestamp()
	records := []TokenQuotaCompensation{
		{OperationKey: keys[0], TokenId: 1, Quota: 1, Status: "applied", CacheInvalidated: true},
		{OperationKey: keys[1], TokenId: 1, Quota: 1, Status: "applied", CacheInvalidated: false},
		{OperationKey: keys[2], TokenId: 1, Quota: 1, Status: "pending", CacheInvalidated: true},
		{OperationKey: keys[3], TokenId: 1, Quota: 1, Status: "applied", CacheInvalidated: true},
	}
	require.NoError(t, DB.Create(&records).Error)
	require.NoError(t, DB.Model(&TokenQuotaCompensation{}).
		Where("operation_key IN ?", keys[:3]).
		UpdateColumn("updated_at", now-1000).Error)

	deleted, err := CleanupTokenQuotaCompensations(100)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)

	var remaining []TokenQuotaCompensation
	require.NoError(t, DB.Where("operation_key IN ?", keys).Order("operation_key").Find(&remaining).Error)
	require.Len(t, remaining, 3)
	remainingKeys := make(map[string]bool, len(remaining))
	for _, record := range remaining {
		remainingKeys[record.OperationKey] = true
	}
	require.False(t, remainingKeys[keys[0]])
	require.True(t, remainingKeys[keys[1]])
	require.True(t, remainingKeys[keys[2]])
	require.True(t, remainingKeys[keys[3]])
}
