package model

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetUserQuotaRejectsSnapshotWhenPendingDeltaAppearsDuringDatabaseRead(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 323

	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
	})

	queryStarted := make(chan struct{})
	continueQuery := make(chan struct{})
	callbackName := "test:block_user_quota_read"
	require.NoError(t, DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "users" {
			return
		}
		close(queryStarted)
		<-continueQuery
	}))
	t.Cleanup(func() {
		_ = DB.Callback().Query().Remove(callbackName)
	})

	quotaResult := make(chan error, 1)
	go func() {
		_, err := GetUserQuota(userId, true)
		quotaResult <- err
	}()

	<-queryStarted
	enqueueUserQuotaDeltaLocked(userId, -7)
	close(continueQuery)

	require.ErrorIs(t, <-quotaResult, ErrUserQuotaCacheSync)
}

func TestGetUserQuotaRejectsDatabaseFallbackWithPendingDelta(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 324
	enqueueUserQuotaDeltaLocked(userId, -9)

	t.Run("Redis 已关闭", func(t *testing.T) {
		previousRedisEnabled := common.RedisEnabled
		common.RedisEnabled = false
		t.Cleanup(func() {
			common.RedisEnabled = previousRedisEnabled
		})

		_, err := GetUserQuota(userId, false)
		require.ErrorIs(t, err, ErrUserQuotaCacheSync)
	})

	t.Run("强制读取数据库", func(t *testing.T) {
		_, err := GetUserQuota(userId, true)
		require.ErrorIs(t, err, ErrUserQuotaCacheSync)
	})
}

func TestGetUserCacheRejectsDatabaseFallbackWithPendingDelta(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 325
	enqueueUserQuotaDeltaLocked(userId, -5)

	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
	})

	_, err := GetUserCache(userId)
	require.ErrorIs(t, err, ErrUserQuotaCacheSync)
}

func TestUserQuotaDatabaseFallbackGuardAllowsTrustedLiveQuotaOnlyWhileQueued(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 330
	enqueueUserQuotaDeltaLocked(userId, -5)

	require.True(t, userQuotaDatabaseFallbackBlocked(userId, false))
	require.False(t, userQuotaDatabaseFallbackBlocked(userId, true))
	require.False(t, userQuotaCacheReadBlocked(userId))

	beginUserQuotaPersistence(userId)
	require.True(t, userQuotaDatabaseFallbackBlocked(userId, true))
	require.True(t, userQuotaCacheReadBlocked(userId))
	finishUserQuotaPersistence(userId)
}

func TestUserQuotaDatabaseSnapshotRequiresGenerationFence(t *testing.T) {
	preservedQuota := 100
	previousRedisEnabled := common.RedisEnabled
	previousBatchUpdateEnabled := common.BatchUpdateEnabled
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		common.BatchUpdateEnabled = previousBatchUpdateEnabled
	})

	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	require.ErrorIs(t, validateUserQuotaDatabaseSnapshot(&preservedQuota, false), ErrUserQuotaCacheSync)
	require.NoError(t, validateUserQuotaDatabaseSnapshot(&preservedQuota, true))
	require.NoError(t, validateUserQuotaDatabaseSnapshot(nil, false))

	common.RedisEnabled = true
	common.BatchUpdateEnabled = true
	require.ErrorIs(t, validateUserQuotaDatabaseSnapshot(nil, false), ErrUserQuotaCacheSync)
	require.NoError(t, validateUserQuotaDatabaseSnapshot(nil, true))
}

func TestGetUserQuotaRejectsDatabaseReadWhileBatchPersistenceIsInProgress(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 326
	enqueueUserQuotaDeltaLocked(userId, -13)

	persistStarted := make(chan struct{})
	continuePersist := make(chan struct{})
	flushDone := make(chan struct{})
	var releaseOnce sync.Once
	var waitOnce sync.Once
	waitForFlush := func() {
		waitOnce.Do(func() { <-flushDone })
	}
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(continuePersist) })
		waitForFlush()
	})

	go func() {
		defer close(flushDone)
		flushUserQuotaBatchUpdatesWith(
			func(id int, delta int) error {
				if id != userId || delta != -13 {
					t.Errorf("刷盘参数错误: id=%d delta=%d", id, delta)
				}
				close(persistStarted)
				<-continuePersist
				return nil
			},
			func(int) error {
				t.Error("成功刷盘不应建立回退锁")
				return nil
			},
		)
	}()

	<-persistStarted
	require.True(t, hasPendingUserQuotaDelta(userId))
	_, err := GetUserQuota(userId, true)
	require.ErrorIs(t, err, ErrUserQuotaCacheSync)
	_, err = GetUserCache(userId)
	require.ErrorIs(t, err, ErrUserQuotaCacheSync)
	releaseOnce.Do(func() { close(continuePersist) })
	waitForFlush()
	require.False(t, hasPendingUserQuotaDelta(userId))
}

func TestUserQuotaFallbackMarkerSurvivesFinalizeAndRecoveryFailure(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 327
	finishErr := errors.New("redis finish failed")
	recoveryErr := errors.New("redis invalidation failed")
	var recoveryReady atomic.Bool
	var renewCalls atomic.Int32

	require.NoError(t, holdUserQuotaDeferredFallback(
		userId,
		"retry-owner",
		func(string) error { return finishErr },
		func(string) error {
			renewCalls.Add(1)
			return nil
		},
		func() error {
			if recoveryReady.Load() {
				return nil
			}
			return recoveryErr
		},
	))

	err := finishUserQuotaDeferredFallback(userId)
	require.ErrorIs(t, err, finishErr)
	require.ErrorIs(t, err, recoveryErr)
	require.True(t, hasDeferredUserQuotaFallbackForTest(userId))
	require.EqualValues(t, 2, renewCalls.Load())

	recoveryReady.Store(true)
	err = finishUserQuotaDeferredFallback(userId)
	require.ErrorIs(t, err, finishErr)
	require.EqualValues(t, 3, renewCalls.Load())
	require.False(t, hasDeferredUserQuotaFallbackForTest(userId))
}

func TestUserQuotaFallbackBackgroundRetryReestablishesDistributedProtection(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 331
	finishErr := errors.New("redis finish failed")
	recoveryErr := errors.New("redis invalidation failed")
	var recoveryReady atomic.Bool
	var renewCalls atomic.Int32

	require.NoError(t, holdUserQuotaDeferredFallback(
		userId,
		"background-retry-owner",
		func(string) error { return finishErr },
		func(string) error {
			renewCalls.Add(1)
			return nil
		},
		func() error {
			if recoveryReady.Load() {
				return nil
			}
			return recoveryErr
		},
	))

	userQuotaDeferredFallbacksLock.Lock()
	fallback := userQuotaDeferredFallbacks[userId]
	userQuotaDeferredFallbacksLock.Unlock()
	require.NotNil(t, fallback)
	safeToRelease, err := finalizeHeldUserQuotaFallback(fallback)
	require.False(t, safeToRelease)
	require.ErrorIs(t, err, finishErr)
	require.ErrorIs(t, err, recoveryErr)
	require.EqualValues(t, 2, renewCalls.Load())

	recoveryReady.Store(true)
	retryUserQuotaFallbackFinalizationWithDelay(userId, fallback, 5*time.Millisecond)
	require.Eventually(t, func() bool {
		return !hasDeferredUserQuotaFallbackForTest(userId)
	}, 500*time.Millisecond, 5*time.Millisecond)
	require.GreaterOrEqual(t, renewCalls.Load(), int32(3))
}

func TestUserQuotaFallbackBackgroundRetryWaitsForReusedPendingDelta(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 332
	finishErr := errors.New("redis finish failed")
	recoveryErr := errors.New("redis invalidation failed")
	databaseErr := errors.New("database unavailable")
	var recoveryReady atomic.Bool
	var finishCalls atomic.Int32
	var ensureCalls atomic.Int32

	require.NoError(t, holdUserQuotaDeferredFallback(
		userId,
		"reused-pending-owner",
		func(string) error {
			finishCalls.Add(1)
			return finishErr
		},
		func(string) error {
			ensureCalls.Add(1)
			return nil
		},
		func() error {
			if recoveryReady.Load() {
				return nil
			}
			return recoveryErr
		},
	))

	userQuotaDeferredFallbacksLock.Lock()
	fallback := userQuotaDeferredFallbacks[userId]
	userQuotaDeferredFallbacksLock.Unlock()
	require.NotNil(t, fallback)
	safeToRelease, err := finalizeHeldUserQuotaFallback(fallback)
	require.False(t, safeToRelease)
	require.ErrorIs(t, err, finishErr)
	require.ErrorIs(t, err, recoveryErr)
	require.EqualValues(t, 1, finishCalls.Load())

	err = applyCommittedUserQuotaDeltaWithBatch(
		userId,
		-7,
		func() (userQuotaCacheUpdate, error) {
			return userQuotaCacheUpdate{}, errors.New("redis unavailable")
		},
		func(delta int) error {
			require.Equal(t, -7, delta)
			return databaseErr
		},
		func(string) error { return nil },
		func(string) error { return nil },
		func() error { return nil },
	)
	require.NoError(t, err)
	require.Equal(t, -7, pendingUserQuotaDeltaForTest(userId))
	userQuotaDeferredFallbacksLock.Lock()
	require.Same(t, fallback, userQuotaDeferredFallbacks[userId])
	userQuotaDeferredFallbacksLock.Unlock()

	recoveryReady.Store(true)
	retryUserQuotaFallbackFinalizationWithDelay(userId, fallback, 5*time.Millisecond)
	require.Eventually(t, func() bool {
		return ensureCalls.Load() >= 3
	}, 500*time.Millisecond, 5*time.Millisecond)
	require.EqualValues(t, 1, finishCalls.Load())
	require.True(t, hasDeferredUserQuotaFallbackForTest(userId))

	require.NoError(t, ConsumePendingUserQuotaDelta(userId, func(delta int) error {
		require.Equal(t, -7, delta)
		return nil
	}))
	require.EqualValues(t, 2, finishCalls.Load())
	require.Zero(t, pendingUserQuotaDeltaForTest(userId))
	require.False(t, hasDeferredUserQuotaFallbackForTest(userId))
}

func TestUserQuotaFallbackBackgroundRetryWaitsWhilePersistenceIsBlocked(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 333
	finishErr := errors.New("redis finish failed")
	recoveryErr := errors.New("redis invalidation failed")
	databaseErr := errors.New("database unavailable")
	var recoveryReady atomic.Bool
	var finishCalls atomic.Int32
	var ensureCalls atomic.Int32

	require.NoError(t, holdUserQuotaDeferredFallback(
		userId,
		"blocked-persistence-owner",
		func(string) error {
			finishCalls.Add(1)
			return finishErr
		},
		func(string) error {
			ensureCalls.Add(1)
			return nil
		},
		func() error {
			if recoveryReady.Load() {
				return nil
			}
			return recoveryErr
		},
	))
	userQuotaDeferredFallbacksLock.Lock()
	fallback := userQuotaDeferredFallbacks[userId]
	userQuotaDeferredFallbacksLock.Unlock()
	require.NotNil(t, fallback)
	safeToRelease, err := finalizeHeldUserQuotaFallback(fallback)
	require.False(t, safeToRelease)
	require.ErrorIs(t, err, finishErr)
	require.ErrorIs(t, err, recoveryErr)
	recoveryReady.Store(true)

	persistStarted := make(chan struct{})
	releasePersist := make(chan struct{})
	applyDone := make(chan error, 1)
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(releasePersist) })
	})
	go func() {
		applyDone <- applyCommittedUserQuotaDeltaWithBatch(
			userId,
			-8,
			func() (userQuotaCacheUpdate, error) {
				return userQuotaCacheUpdate{}, errors.New("redis unavailable")
			},
			func(delta int) error {
				if delta != -8 {
					return errors.New("unexpected persistence delta")
				}
				close(persistStarted)
				<-releasePersist
				return databaseErr
			},
			func(string) error { return nil },
			func(string) error { return nil },
			func() error { return nil },
		)
	}()
	select {
	case <-persistStarted:
	case <-time.After(time.Second):
		t.Fatal("等待额度持久化开始超时")
	}

	retryUserQuotaFallbackFinalizationWithDelay(userId, fallback, 5*time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	require.EqualValues(t, 1, finishCalls.Load())
	require.True(t, hasDeferredUserQuotaFallbackForTest(userId))

	releaseOnce.Do(func() { close(releasePersist) })
	require.NoError(t, <-applyDone)
	require.Eventually(t, func() bool {
		return ensureCalls.Load() >= 3
	}, 500*time.Millisecond, 5*time.Millisecond)
	require.EqualValues(t, 1, finishCalls.Load())
	require.Equal(t, -8, pendingUserQuotaDeltaForTest(userId))

	require.NoError(t, ConsumePendingUserQuotaDelta(userId, func(delta int) error {
		require.Equal(t, -8, delta)
		return nil
	}))
	require.EqualValues(t, 2, finishCalls.Load())
	require.False(t, hasDeferredUserQuotaFallbackForTest(userId))
}

func TestUserQuotaFallbackOldBackgroundRetryCannotDeleteReplacement(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 334
	finishErr := errors.New("redis finish failed")
	recoveryErr := errors.New("redis invalidation failed")
	var oldFinishCalls atomic.Int32
	var newFinishCalls atomic.Int32

	require.NoError(t, holdUserQuotaDeferredFallback(
		userId,
		"old-background-owner",
		func(string) error {
			oldFinishCalls.Add(1)
			return finishErr
		},
		func(string) error { return nil },
		func() error { return recoveryErr },
	))
	userQuotaDeferredFallbacksLock.Lock()
	oldFallback := userQuotaDeferredFallbacks[userId]
	userQuotaDeferredFallbacksLock.Unlock()
	require.NotNil(t, oldFallback)
	safeToRelease, err := finalizeHeldUserQuotaFallback(oldFallback)
	require.False(t, safeToRelease)
	require.ErrorIs(t, err, finishErr)
	require.ErrorIs(t, err, recoveryErr)

	applyLock := userQuotaBatchApplyLockFor(userId)
	applyLock.Lock()
	applyLockHeld := true
	defer func() {
		if applyLockHeld {
			applyLock.Unlock()
		}
	}()
	retryUserQuotaFallbackFinalizationWithDelay(userId, oldFallback, 5*time.Millisecond)
	time.Sleep(15 * time.Millisecond)
	require.NoError(t, holdUserQuotaDeferredFallback(
		userId,
		"replacement-owner",
		func(string) error {
			newFinishCalls.Add(1)
			return nil
		},
		func(string) error { return nil },
		func() error { return nil },
	))
	userQuotaDeferredFallbacksLock.Lock()
	replacement := userQuotaDeferredFallbacks[userId]
	userQuotaDeferredFallbacksLock.Unlock()
	require.NotNil(t, replacement)
	require.NotSame(t, oldFallback, replacement)
	oldFinishAfterReplacement := oldFinishCalls.Load()
	applyLock.Unlock()
	applyLockHeld = false

	time.Sleep(30 * time.Millisecond)
	userQuotaDeferredFallbacksLock.Lock()
	require.Same(t, replacement, userQuotaDeferredFallbacks[userId])
	userQuotaDeferredFallbacksLock.Unlock()
	require.Equal(t, oldFinishAfterReplacement, oldFinishCalls.Load())
	require.Zero(t, newFinishCalls.Load())

	require.NoError(t, finishUserQuotaDeferredFallback(userId))
	require.EqualValues(t, 1, newFinishCalls.Load())
	require.False(t, hasDeferredUserQuotaFallbackForTest(userId))
}

func TestUserQuotaFallbackKeepsRenewingWhileFinalizeIsBlocked(t *testing.T) {
	finishStarted := make(chan struct{})
	releaseFinish := make(chan struct{})
	var ensureCalls atomic.Int32
	var renewCalls atomic.Int32
	fallback := &userQuotaDeferredFallback{
		lockToken: "blocked-finalize-owner",
		finish: func(string) error {
			close(finishStarted)
			<-releaseFinish
			return nil
		},
		ensure: func(string) error {
			ensureCalls.Add(1)
			return nil
		},
		renew: func(string) error {
			renewCalls.Add(1)
			return nil
		},
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}

	result := make(chan error, 1)
	go func() {
		safeToRelease, err := finalizeHeldUserQuotaFallback(fallback)
		if !safeToRelease && err == nil {
			err = errors.New("finalize 未安全完成")
		}
		result <- err
	}()
	<-finishStarted
	fallback.startLeaseMaintenance(5 * time.Millisecond)
	require.Eventually(t, func() bool {
		return renewCalls.Load() > 0
	}, 500*time.Millisecond, 5*time.Millisecond)
	close(releaseFinish)
	require.NoError(t, <-result)

	// 已安全完成的 fallback 再次 finalize 必须直接返回，不能重新 Ensure 建锁。
	require.EqualValues(t, 1, ensureCalls.Load())
	safeToRelease, err := finalizeHeldUserQuotaFallback(fallback)
	require.NoError(t, err)
	require.True(t, safeToRelease)
	require.EqualValues(t, 1, ensureCalls.Load())
}

func TestGetUserQuotaRejectsDatabaseReadDuringImmediatePersistence(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 328
	persistStarted := make(chan struct{})
	continuePersist := make(chan struct{})
	result := make(chan error, 1)
	var releaseOnce sync.Once
	var waitOnce sync.Once
	var operationErr error
	waitForResult := func() error {
		waitOnce.Do(func() { operationErr = <-result })
		return operationErr
	}
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(continuePersist) })
		_ = waitForResult()
	})

	go func() {
		result <- applyUserQuotaDeltaWithBatch(
			userId,
			-3,
			func() (userQuotaCacheUpdate, error) {
				return userQuotaCacheUpdate{}, errors.New("redis unavailable")
			},
			func(delta int) error {
				if delta != -3 {
					t.Errorf("即时落库差额错误: %d", delta)
				}
				close(persistStarted)
				<-continuePersist
				return nil
			},
			func(string) error { return nil },
			func(string) error { return nil },
			func() error { return nil },
		)
	}()

	<-persistStarted
	require.True(t, hasPendingUserQuotaDelta(userId))
	_, err := GetUserQuota(userId, true)
	require.ErrorIs(t, err, ErrUserQuotaCacheSync)
	releaseOnce.Do(func() { close(continuePersist) })
	require.NoError(t, waitForResult())
	require.False(t, hasPendingUserQuotaDelta(userId))
}

func TestGetUserQuotaRejectsDatabaseReadDuringPendingDeltaConsumption(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 329
	enqueueUserQuotaDeltaLocked(userId, -4)
	persistStarted := make(chan struct{})
	continuePersist := make(chan struct{})
	result := make(chan error, 1)
	var releaseOnce sync.Once
	var waitOnce sync.Once
	var operationErr error
	waitForResult := func() error {
		waitOnce.Do(func() { operationErr = <-result })
		return operationErr
	}
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(continuePersist) })
		_ = waitForResult()
	})

	go func() {
		result <- ConsumePendingUserQuotaDelta(userId, func(delta int) error {
			if delta != -4 {
				t.Errorf("事务合并差额错误: %d", delta)
			}
			close(persistStarted)
			<-continuePersist
			return nil
		})
	}()

	<-persistStarted
	require.True(t, hasPendingUserQuotaDelta(userId))
	_, err := GetUserQuota(userId, true)
	require.ErrorIs(t, err, ErrUserQuotaCacheSync)
	releaseOnce.Do(func() { close(continuePersist) })
	require.NoError(t, waitForResult())
	require.False(t, hasPendingUserQuotaDelta(userId))
}
