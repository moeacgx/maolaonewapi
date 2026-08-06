package model

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

func TestGetUserQuotaWithContextRetriesAfterCacheSync(t *testing.T) {
	var calls atomic.Int32
	quota, err := getUserQuotaWithContextAndRetry(
		context.Background(),
		901,
		false,
		200*time.Millisecond,
		time.Millisecond,
		func(int, bool) (int, error) {
			if calls.Add(1) < 3 {
				return 0, fmt.Errorf("%w: 正在持久化", ErrUserQuotaCacheSync)
			}
			return 1234, nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, 1234, quota)
	require.EqualValues(t, 3, calls.Load())
}

func TestGetUserQuotaWithContextStopsWhenRequestCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := getUserQuotaWithContextAndRetry(
		ctx,
		902,
		false,
		200*time.Millisecond,
		time.Millisecond,
		func(int, bool) (int, error) {
			return 0, ErrUserQuotaCacheSync
		},
	)
	require.ErrorIs(t, err, context.Canceled)
}

func TestGetUserQuotaWithContextFailsClosedAfterTimeout(t *testing.T) {
	_, err := getUserQuotaWithContextAndRetry(
		context.Background(),
		903,
		false,
		5*time.Millisecond,
		time.Millisecond,
		func(int, bool) (int, error) {
			return 0, ErrUserQuotaCacheSync
		},
	)
	require.ErrorIs(t, err, ErrUserQuotaCacheSync)
	require.Contains(t, err.Error(), "等待同步完成超时")
}

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
	require.True(t, userQuotaDatabaseFallbackBlocked(userId, false))
	require.False(t, userQuotaDatabaseFallbackBlocked(userId, true))
	require.False(t, userQuotaCacheReadBlocked(userId))
	finishUserQuotaPersistence(userId)
}

func TestUserQuotaCacheReadBlockedOnlyForDeferredFallback(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 331
	beginUserQuotaPersistence(userId)
	require.False(t, userQuotaCacheReadBlocked(userId))

	done := make(chan struct{})
	close(done)
	userQuotaDeferredFallbacksLock.Lock()
	userQuotaDeferredFallbacks[userId] = &userQuotaDeferredFallback{
		stop: make(chan struct{}),
		done: done,
	}
	userQuotaDeferredFallbacksLock.Unlock()
	require.True(t, userQuotaCacheReadBlocked(userId))
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
	require.EqualValues(t, 3, finishCalls.Load())
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
	require.EqualValues(t, 3, finishCalls.Load())
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

func TestRetryUserQuotaReadWaitsOnLocalStateWithoutReadStorm(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 904
	enqueueUserQuotaDeltaLocked(userId, -1)

	var calls atomic.Int32
	go func() {
		time.Sleep(25 * time.Millisecond)
		takePendingUserQuotaDeltaLocked(userId)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	err := retryUserQuotaReadWithContext(ctx, userId, func() error {
		calls.Add(1)
		if hasPendingUserQuotaDelta(userId) {
			return ErrUserQuotaCacheSync
		}
		return nil
	})
	require.NoError(t, err)
	require.EqualValues(t, 2, calls.Load())
}

func TestWaitForUserQuotaReadRetryBacksOffRemoteProbes(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	var calls atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 130*time.Millisecond)
	defer cancel()

	err := waitForUserQuotaReadRetryWithProbe(
		ctx,
		905,
		true,
		func(context.Context, int) (bool, error) {
			calls.Add(1)
			return true, nil
		},
	)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.LessOrEqual(t, calls.Load(), int32(4))
}

func TestRemoteQuotaFallbackProbeCoalescesConcurrentCallers(t *testing.T) {
	var group singleflight.Group
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	keyExists := func(context.Context, string) (bool, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return true, nil
	}

	const waiterCount = 16
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var ready sync.WaitGroup
	ready.Add(waiterCount)
	start := make(chan struct{})
	results := make(chan error, waiterCount)
	for range waiterCount {
		go func() {
			ready.Done()
			<-start
			locked, err := probeUserQuotaRemoteFallbackWithGroup(
				ctx,
				906,
				&group,
				keyExists,
			)
			if err == nil && !locked {
				err = errors.New("remote fallback lock should be visible")
			}
			results <- err
		}()
	}
	ready.Wait()
	close(start)
	<-started
	time.Sleep(50 * time.Millisecond)
	close(release)

	for range waiterCount {
		require.NoError(t, <-results)
	}
	require.EqualValues(t, 1, calls.Load())
}

func TestNormalizeUserQuotaWaitErrorDistinguishesInternalAndCallerDeadlines(t *testing.T) {
	requestCtx := context.Background()
	waitCtx, cancelWait := context.WithTimeout(requestCtx, time.Millisecond)
	defer cancelWait()
	<-waitCtx.Done()

	syncWaitErr := fmt.Errorf("%w: %w", ErrUserQuotaCacheSync, waitCtx.Err())
	require.ErrorIs(
		t,
		normalizeUserQuotaWaitError(requestCtx, waitCtx, syncWaitErr),
		ErrUserQuotaCacheSync,
	)

	for _, dependency := range []string{"Redis", "database", "subscription transaction"} {
		t.Run(dependency, func(t *testing.T) {
			dependencyErr := fmt.Errorf("%s deadline: %w", dependency, waitCtx.Err())
			normalizedErr := normalizeUserQuotaWaitError(requestCtx, waitCtx, dependencyErr)
			require.Equal(t, dependencyErr, normalizedErr)
			require.ErrorIs(t, normalizedErr, context.DeadlineExceeded)
			require.NotErrorIs(t, normalizedErr, ErrUserQuotaCacheSync)
		})
	}

	callerCtx, cancelCaller := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancelCaller()
	<-callerCtx.Done()
	waitFromCaller, cancelDerived := context.WithCancel(callerCtx)
	defer cancelDerived()
	err := normalizeUserQuotaWaitError(
		callerCtx,
		waitFromCaller,
		fmt.Errorf("%w: %w", ErrUserQuotaCacheSync, waitFromCaller.Err()),
	)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotErrorIs(t, err, ErrUserQuotaCacheSync)

	canceledCtx, cancelCanceled := context.WithCancel(context.Background())
	cancelCanceled()
	canceledErr := normalizeUserQuotaWaitError(
		canceledCtx,
		canceledCtx,
		fmt.Errorf("%w: caller canceled", ErrUserQuotaCacheSync),
	)
	require.ErrorIs(t, canceledErr, context.Canceled)
	require.NotErrorIs(t, canceledErr, ErrUserQuotaCacheSync)
}

func TestRetryUserQuotaReadPreservesSyncMarkerOnInternalDeadline(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 907
	enqueueUserQuotaDeltaLocked(userId, -1)

	waitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	err := retryUserQuotaReadWithContext(waitCtx, userId, func() error {
		return fmt.Errorf("%w: pending quota", ErrUserQuotaCacheSync)
	})

	require.ErrorIs(t, err, ErrUserQuotaCacheSync)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorIs(
		t,
		normalizeUserQuotaWaitError(context.Background(), waitCtx, err),
		ErrUserQuotaCacheSync,
	)
}
