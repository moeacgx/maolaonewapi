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

func isolateUserQuotaBatchStore(t *testing.T) {
	t.Helper()
	for index := range userQuotaBatchApplyLocks {
		userQuotaBatchApplyLocks[index].Lock()
	}
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	previous := batchUpdateStores[BatchUpdateTypeUserQuota]
	previousInFlight := userQuotaBatchInFlight
	previousPersistenceInProgress := userQuotaPersistenceInProgress
	batchUpdateStores[BatchUpdateTypeUserQuota] = make(map[int]int)
	userQuotaBatchInFlight = make(map[int]int)
	userQuotaPersistenceInProgress = make(map[int]int)
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
	userQuotaDeferredFallbacksLock.Lock()
	previousDeferredFallbacks := userQuotaDeferredFallbacks
	userQuotaDeferredFallbacks = make(map[int]*userQuotaDeferredFallback)
	userQuotaDeferredFallbacksLock.Unlock()
	for index := len(userQuotaBatchApplyLocks) - 1; index >= 0; index-- {
		userQuotaBatchApplyLocks[index].Unlock()
	}
	t.Cleanup(func() {
		for index := range userQuotaBatchApplyLocks {
			userQuotaBatchApplyLocks[index].Lock()
		}
		batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
		batchUpdateStores[BatchUpdateTypeUserQuota] = previous
		userQuotaBatchInFlight = previousInFlight
		userQuotaPersistenceInProgress = previousPersistenceInProgress
		batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
		userQuotaDeferredFallbacksLock.Lock()
		for _, fallback := range userQuotaDeferredFallbacks {
			fallback.stopRenewalAndWait()
		}
		userQuotaDeferredFallbacks = previousDeferredFallbacks
		userQuotaDeferredFallbacksLock.Unlock()
		for index := len(userQuotaBatchApplyLocks) - 1; index >= 0; index-- {
			userQuotaBatchApplyLocks[index].Unlock()
		}
	})
}

func pendingUserQuotaDeltaForTest(userId int) int {
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	defer batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
	return batchUpdateStores[BatchUpdateTypeUserQuota][userId] + userQuotaBatchInFlight[userId]
}

func hasDeferredUserQuotaFallbackForTest(userId int) bool {
	userQuotaDeferredFallbacksLock.Lock()
	defer userQuotaDeferredFallbacksLock.Unlock()
	return userQuotaDeferredFallbacks[userId] != nil
}

func TestUserQuotaBatchCacheHitOnlyQueuesDelta(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 301
	persistCalls := 0

	err := applyUserQuotaDeltaWithBatch(
		userId,
		-10,
		func() (userQuotaCacheUpdate, error) {
			return userQuotaCacheUpdate{state: common.RedisHashIncrementUpdated}, nil
		},
		func(int) error {
			persistCalls++
			return nil
		},
		func(string) error { return nil },
		func(string) error { return nil },
		func() error { return nil },
	)
	require.NoError(t, err)
	require.Zero(t, persistCalls)
	require.Equal(t, -10, pendingUserQuotaDeltaForTest(userId))
}

func TestUserQuotaBatchCacheMissPersistsPendingAndCurrentExactlyOnce(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 302
	enqueueUserQuotaDeltaLocked(userId, -20)
	var persisted []int
	var finishedTokens []string

	err := applyUserQuotaDeltaWithBatch(
		userId,
		-5,
		func() (userQuotaCacheUpdate, error) {
			return userQuotaCacheUpdate{
				state:     common.RedisHashIncrementFallbackAcquired,
				lockToken: "fallback-owner",
			}, nil
		},
		func(delta int) error {
			persisted = append(persisted, delta)
			return nil
		},
		func(lockToken string) error {
			finishedTokens = append(finishedTokens, lockToken)
			return nil
		},
		func(string) error { return nil },
		func() error { return nil },
	)
	require.NoError(t, err)
	require.Equal(t, []int{-25}, persisted)
	require.Equal(t, []string{"fallback-owner"}, finishedTokens)
	require.Zero(t, pendingUserQuotaDeltaForTest(userId))
}

func TestUserQuotaBatchDatabaseFailureRestoresOnlyPreviousPendingDelta(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 303
	enqueueUserQuotaDeltaLocked(userId, -30)
	expectedErr := errors.New("database unavailable")
	finishCalls := 0
	renewCalls := 0

	err := applyUserQuotaDeltaWithBatch(
		userId,
		-7,
		func() (userQuotaCacheUpdate, error) {
			return userQuotaCacheUpdate{
				state:     common.RedisHashIncrementFallbackAcquired,
				lockToken: "fallback-owner",
			}, nil
		},
		func(delta int) error {
			require.Equal(t, -37, delta)
			return expectedErr
		},
		func(string) error {
			finishCalls++
			return nil
		},
		func(string) error {
			renewCalls++
			return nil
		},
		func() error { return nil },
	)
	require.ErrorIs(t, err, expectedErr)
	require.Zero(t, finishCalls)
	require.Equal(t, 1, renewCalls)
	require.True(t, hasDeferredUserQuotaFallbackForTest(userId))
	require.Equal(t, -30, pendingUserQuotaDeltaForTest(userId))
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = true
	_, quotaErr := GetUserQuota(userId, false)
	common.RedisEnabled = previousRedisEnabled
	require.ErrorIs(t, quotaErr, ErrUserQuotaCacheSync)

	require.NoError(t, ConsumePendingUserQuotaDelta(userId, func(delta int) error {
		require.Equal(t, -30, delta)
		return nil
	}))
	require.Equal(t, 1, finishCalls)
	require.False(t, hasDeferredUserQuotaFallbackForTest(userId))
}

func TestUserQuotaBatchDatabaseFailureWithoutPendingReleasesFallback(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 308
	expectedErr := errors.New("database unavailable")
	finishCalls := 0
	renewCalls := 0

	err := applyUserQuotaDeltaWithBatch(
		userId,
		-7,
		func() (userQuotaCacheUpdate, error) {
			return userQuotaCacheUpdate{
				state:     common.RedisHashIncrementFallbackAcquired,
				lockToken: "fallback-owner",
			}, nil
		},
		func(delta int) error {
			require.Equal(t, -7, delta)
			return expectedErr
		},
		func(string) error {
			finishCalls++
			return nil
		},
		func(string) error {
			renewCalls++
			return nil
		},
		func() error { return nil },
	)
	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, 1, finishCalls)
	require.Equal(t, 2, renewCalls)
	require.False(t, hasDeferredUserQuotaFallbackForTest(userId))
	require.Zero(t, pendingUserQuotaDeltaForTest(userId))
}

func TestCommittedUserQuotaBatchDatabaseFailureKeepsCurrentDeltaAndReturnsAccepted(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 315
	expectedErr := errors.New("database unavailable")
	finishCalls := 0
	renewCalls := 0

	err := applyCommittedUserQuotaDeltaWithBatch(
		userId,
		-7,
		func() (userQuotaCacheUpdate, error) {
			return userQuotaCacheUpdate{
				state:     common.RedisHashIncrementFallbackAcquired,
				lockToken: "committed-owner",
			}, nil
		},
		func(delta int) error {
			require.Equal(t, -7, delta)
			return expectedErr
		},
		func(string) error {
			finishCalls++
			return nil
		},
		func(string) error {
			renewCalls++
			return nil
		},
		func() error { return nil },
	)

	require.NoError(t, err)
	require.Zero(t, finishCalls)
	require.Equal(t, 1, renewCalls)
	require.Equal(t, -7, pendingUserQuotaDeltaForTest(userId))
	require.True(t, hasDeferredUserQuotaFallbackForTest(userId))

	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	_, quotaErr := GetUserQuota(userId, false)
	common.RedisEnabled = previousRedisEnabled
	require.ErrorIs(t, quotaErr, ErrUserQuotaCacheSync)

	require.NoError(t, ConsumePendingUserQuotaDelta(userId, func(delta int) error {
		require.Equal(t, -7, delta)
		return nil
	}))
	require.Equal(t, 1, finishCalls)
	require.Zero(t, pendingUserQuotaDeltaForTest(userId))
	require.False(t, hasDeferredUserQuotaFallbackForTest(userId))
}

func TestCommittedUserQuotaBatchDatabaseFailureRestoresPreviousAndCurrentDelta(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 316
	enqueueUserQuotaDeltaLocked(userId, -30)
	expectedErr := errors.New("database unavailable")

	err := applyCommittedUserQuotaDeltaWithBatch(
		userId,
		-7,
		func() (userQuotaCacheUpdate, error) {
			return userQuotaCacheUpdate{
				state:     common.RedisHashIncrementFallbackAcquired,
				lockToken: "committed-owner",
			}, nil
		},
		func(delta int) error {
			require.Equal(t, -37, delta)
			return expectedErr
		},
		func(string) error { return nil },
		func(string) error { return nil },
		func() error { return nil },
	)

	require.NoError(t, err)
	require.Equal(t, -37, pendingUserQuotaDeltaForTest(userId))
	require.True(t, hasDeferredUserQuotaFallbackForTest(userId))
	require.NoError(t, ConsumePendingUserQuotaDelta(userId, func(delta int) error {
		require.Equal(t, -37, delta)
		return nil
	}))
}

func TestCommittedUserQuotaBatchRedisErrorProtectsDeferredDelta(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 317
	redisErr := errors.New("redis unavailable")
	databaseErr := errors.New("database unavailable")
	renewCalls := 0
	finishCalls := 0
	recoveryCalls := 0

	err := applyCommittedUserQuotaDeltaWithBatch(
		userId,
		9,
		func() (userQuotaCacheUpdate, error) {
			return userQuotaCacheUpdate{}, redisErr
		},
		func(delta int) error {
			require.Equal(t, 9, delta)
			return databaseErr
		},
		func(string) error {
			finishCalls++
			return nil
		},
		func(string) error {
			renewCalls++
			return nil
		},
		func() error {
			recoveryCalls++
			return nil
		},
	)

	require.NoError(t, err)
	require.Equal(t, 1, renewCalls)
	require.Zero(t, finishCalls)
	require.Equal(t, 1, recoveryCalls)
	require.Equal(t, 9, pendingUserQuotaDeltaForTest(userId))
	require.True(t, hasDeferredUserQuotaFallbackForTest(userId))
	require.NoError(t, ConsumePendingUserQuotaDelta(userId, func(delta int) error {
		require.Equal(t, 9, delta)
		return nil
	}))
	require.Equal(t, 1, finishCalls)
}

func TestAbortableUserQuotaBatchRedisErrorProtectsOnlyPreviousPendingDelta(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 318
	enqueueUserQuotaDeltaLocked(userId, -6)
	expectedErr := errors.New("database unavailable")

	err := applyUserQuotaDeltaWithBatch(
		userId,
		-4,
		func() (userQuotaCacheUpdate, error) {
			return userQuotaCacheUpdate{}, errors.New("redis unavailable")
		},
		func(delta int) error {
			require.Equal(t, -10, delta)
			return expectedErr
		},
		func(string) error { return nil },
		func(string) error { return nil },
		func() error { return nil },
	)

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, -6, pendingUserQuotaDeltaForTest(userId))
	require.True(t, hasDeferredUserQuotaFallbackForTest(userId))
	require.NoError(t, ConsumePendingUserQuotaDelta(userId, func(delta int) error {
		require.Equal(t, -6, delta)
		return nil
	}))
}

func TestCommittedUserQuotaBatchBusyFallbackExhaustionDefersOnDatabaseFailure(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 319
	attempts := 0
	persistCalls := 0

	err := applyUserQuotaDeltaWithBatchDurabilityAndRetry(
		userId,
		-5,
		userQuotaDeltaCommitted,
		func() (userQuotaCacheUpdate, error) {
			attempts++
			return userQuotaCacheUpdate{state: common.RedisHashIncrementFallbackBusy}, nil
		},
		func(delta int) error {
			persistCalls++
			require.Equal(t, -5, delta)
			return errors.New("database unavailable")
		},
		func(string) error { return nil },
		func(string) error { return nil },
		func() error { return nil },
		2,
		0,
	)

	require.NoError(t, err)
	require.Equal(t, 2, attempts)
	require.Equal(t, 1, persistCalls)
	require.Equal(t, -5, pendingUserQuotaDeltaForTest(userId))
	require.True(t, hasDeferredUserQuotaFallbackForTest(userId))
	require.NoError(t, ConsumePendingUserQuotaDelta(userId, func(delta int) error {
		require.Equal(t, -5, delta)
		return nil
	}))
}

func TestAbortableUserQuotaBatchBusyFallbackUsesImmediateDatabaseFallback(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 321
	attempts := 0
	persistCalls := 0
	recoveryCalls := 0

	err := applyUserQuotaDeltaWithBatch(
		userId,
		-5,
		func() (userQuotaCacheUpdate, error) {
			attempts++
			return userQuotaCacheUpdate{state: common.RedisHashIncrementFallbackBusy}, nil
		},
		func(delta int) error {
			persistCalls++
			require.Equal(t, -5, delta)
			return nil
		},
		func(string) error { return nil },
		func(string) error { return nil },
		func() error {
			recoveryCalls++
			return nil
		},
	)

	require.NoError(t, err)
	require.Equal(t, 1, attempts)
	require.Equal(t, 1, persistCalls)
	require.Equal(t, 1, recoveryCalls)
	require.Zero(t, pendingUserQuotaDeltaForTest(userId))
	require.False(t, hasDeferredUserQuotaFallbackForTest(userId))
}

func TestCommittedUserQuotaBatchInvalidCacheStateFallsBackToDatabase(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 320
	persistCalls := 0
	recoveryCalls := 0

	err := applyUserQuotaDeltaWithBatchDurabilityAndRetry(
		userId,
		-3,
		userQuotaDeltaCommitted,
		func() (userQuotaCacheUpdate, error) {
			return userQuotaCacheUpdate{state: common.RedisHashIncrementState(99)}, nil
		},
		func(delta int) error {
			persistCalls++
			require.Equal(t, -3, delta)
			return nil
		},
		func(string) error { return nil },
		func(string) error { return nil },
		func() error {
			recoveryCalls++
			return nil
		},
		1,
		0,
	)

	require.NoError(t, err)
	require.Equal(t, 1, persistCalls)
	require.Equal(t, 1, recoveryCalls)
	require.Zero(t, pendingUserQuotaDeltaForTest(userId))
}

func TestUserQuotaBatchDatabaseCommitWithCacheRecoveryFailureStaysFailClosed(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 325
	recoveryErr := errors.New("redis invalidation failed")
	ensureErr := errors.New("redis fallback unavailable")
	protectionReady := false
	ensureCalls := 0
	finishCalls := 0

	err := applyUserQuotaDeltaWithBatchDurabilityAndRetry(
		userId,
		-3,
		userQuotaDeltaCommitted,
		func() (userQuotaCacheUpdate, error) {
			return userQuotaCacheUpdate{state: common.RedisHashIncrementState(99)}, nil
		},
		func(delta int) error {
			require.Equal(t, -3, delta)
			return nil
		},
		func(string) error {
			finishCalls++
			return nil
		},
		func(string) error {
			ensureCalls++
			if protectionReady {
				return nil
			}
			return ensureErr
		},
		func() error { return recoveryErr },
		1,
		0,
	)

	require.NoError(t, err)
	require.True(t, hasDeferredUserQuotaFallbackForTest(userId))
	require.GreaterOrEqual(t, ensureCalls, 2)
	require.Zero(t, finishCalls)
	require.Zero(t, pendingUserQuotaDeltaForTest(userId))

	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	_, quotaErr := GetUserQuota(userId, true)
	common.RedisEnabled = previousRedisEnabled
	require.ErrorIs(t, quotaErr, ErrUserQuotaCacheSync)

	protectionReady = true
	require.NoError(t, finishUserQuotaDeferredFallback(userId))
	require.Equal(t, 1, finishCalls)
	require.False(t, hasDeferredUserQuotaFallbackForTest(userId))
}

func TestCommittedQuotaPublicAPISelectsDeferredFailureSemantics(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	previousBatchUpdateEnabled := common.BatchUpdateEnabled
	previousRedisEnabled := common.RedisEnabled
	common.BatchUpdateEnabled = true
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.BatchUpdateEnabled = previousBatchUpdateEnabled
		common.RedisEnabled = previousRedisEnabled
	})

	expectedErr := errors.New("database unavailable")
	callbackName := "test:reject_user_quota_update"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			tx.AddError(expectedErr)
		}
	}))
	t.Cleanup(func() {
		_ = DB.Callback().Update().Remove(callbackName)
	})

	abortableUserId := 321
	err := DecreaseUserQuota(abortableUserId, 11, false)
	require.ErrorIs(t, err, expectedErr)
	require.Zero(t, pendingUserQuotaDeltaForTest(abortableUserId))
	require.False(t, hasDeferredUserQuotaFallbackForTest(abortableUserId))

	committedUserId := 322
	err = DecreaseUserQuotaCommitted(committedUserId, 11)
	require.NoError(t, err)
	require.Equal(t, -11, pendingUserQuotaDeltaForTest(committedUserId))
	require.True(t, hasDeferredUserQuotaFallbackForTest(committedUserId))
	_, quotaErr := GetUserQuota(committedUserId, true)
	require.ErrorIs(t, quotaErr, ErrUserQuotaCacheSync)
	require.NoError(t, ConsumePendingUserQuotaDelta(committedUserId, func(delta int) error {
		require.Equal(t, -11, delta)
		return nil
	}))
}

func TestUserQuotaBatchFallbackSetupFailureStillPersistsExactlyOnce(t *testing.T) {
	t.Run("已取得回退锁", func(t *testing.T) {
		isolateUserQuotaBatchStore(t)
		userId := 311
		enqueueUserQuotaDeltaLocked(userId, -20)
		setupErr := errors.New("redis ensure failed")
		persistCalls := 0
		finishCalls := 0
		ensureCalls := 0

		err := applyUserQuotaDeltaWithBatch(
			userId,
			-3,
			func() (userQuotaCacheUpdate, error) {
				return userQuotaCacheUpdate{
					state:     common.RedisHashIncrementFallbackAcquired,
					lockToken: "fallback-owner",
				}, nil
			},
			func(delta int) error {
				persistCalls++
				require.Equal(t, -23, delta)
				return nil
			},
			func(string) error {
				finishCalls++
				return nil
			},
			func(string) error {
				ensureCalls++
				if ensureCalls == 1 {
					return setupErr
				}
				return nil
			},
			func() error { return nil },
		)

		require.NoError(t, err)
		require.Equal(t, 1, persistCalls)
		require.Equal(t, 1, finishCalls)
		require.Equal(t, 2, ensureCalls)
		require.Zero(t, pendingUserQuotaDeltaForTest(userId))
		require.False(t, hasDeferredUserQuotaFallbackForTest(userId))
	})

	t.Run("Redis 响应不确定", func(t *testing.T) {
		isolateUserQuotaBatchStore(t)
		userId := 312
		setupErr := errors.New("redis ensure failed")
		cacheErr := errors.New("redis response lost")
		persistCalls := 0
		finishCalls := 0
		ensureCalls := 0

		err := applyUserQuotaDeltaWithBatch(
			userId,
			-3,
			func() (userQuotaCacheUpdate, error) {
				return userQuotaCacheUpdate{lockToken: "fallback-owner"}, cacheErr
			},
			func(delta int) error {
				persistCalls++
				require.Equal(t, -3, delta)
				return nil
			},
			func(string) error {
				finishCalls++
				return nil
			},
			func(string) error {
				ensureCalls++
				if ensureCalls == 1 {
					return setupErr
				}
				return nil
			},
			func() error { return nil },
		)

		require.NoError(t, err)
		require.Equal(t, 1, persistCalls)
		require.Equal(t, 1, finishCalls)
		require.Equal(t, 2, ensureCalls)
		require.Zero(t, pendingUserQuotaDeltaForTest(userId))
		require.False(t, hasDeferredUserQuotaFallbackForTest(userId))
	})
}

func TestConsumePendingUserQuotaDeltaFailureProtectsRestoredDelta(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 313
	enqueueUserQuotaDeltaLocked(userId, -12)
	expectedErr := errors.New("database unavailable")
	holdCalls := 0

	err := consumePendingUserQuotaDeltaWithFallback(
		userId,
		func(delta int) error {
			require.Equal(t, -12, delta)
			return expectedErr
		},
		func(id int) error {
			require.Equal(t, userId, id)
			require.Equal(t, -12, pendingUserQuotaDeltaForTest(id))
			holdCalls++
			return nil
		},
	)

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, 1, holdCalls)
	require.Equal(t, -12, pendingUserQuotaDeltaForTest(userId))
}

func TestUserQuotaBatchFlushFailureProtectsRestoredDelta(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 314
	enqueueUserQuotaDeltaLocked(userId, -14)
	expectedErr := errors.New("database unavailable")
	renewCalls := 0

	flushUserQuotaBatchUpdatesWith(
		func(id int, delta int) error {
			require.Equal(t, userId, id)
			require.Equal(t, -14, delta)
			return expectedErr
		},
		func(id int) error {
			require.Equal(t, -14, pendingUserQuotaDeltaForTest(id))
			return holdUserQuotaFallbackForPendingWith(
				id,
				"pending-owner",
				func(string) error { return nil },
				func(string) error {
					renewCalls++
					return nil
				},
				func() error { return nil },
			)
		},
	)

	require.Equal(t, -14, pendingUserQuotaDeltaForTest(userId))
	require.True(t, hasDeferredUserQuotaFallbackForTest(userId))
	require.Equal(t, 1, renewCalls)
}

func TestUserQuotaBatchBusyFallbackPersistsOnceWithoutRetry(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 304
	attempts := 0
	persistCalls := 0

	err := applyUserQuotaDeltaWithBatch(
		userId,
		12,
		func() (userQuotaCacheUpdate, error) {
			attempts++
			return userQuotaCacheUpdate{state: common.RedisHashIncrementFallbackBusy}, nil
		},
		func(delta int) error {
			persistCalls++
			require.Equal(t, 12, delta)
			return nil
		},
		func(string) error { return nil },
		func(string) error { return nil },
		func() error { return nil },
	)
	require.NoError(t, err)
	require.Equal(t, 1, attempts)
	require.Equal(t, 1, persistCalls)
	require.Zero(t, pendingUserQuotaDeltaForTest(userId))
}

func TestUserQuotaBatchRedisErrorFallsBackToImmediateDatabaseWrite(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 305
	enqueueUserQuotaDeltaLocked(userId, 9)
	persistCalls := 0
	recoveryCalls := 0

	err := applyUserQuotaDeltaWithBatch(
		userId,
		4,
		func() (userQuotaCacheUpdate, error) {
			return userQuotaCacheUpdate{}, errors.New("redis unavailable")
		},
		func(delta int) error {
			persistCalls++
			require.Equal(t, 13, delta)
			return nil
		},
		func(string) error {
			t.Fatal("没有取得回退锁时不应调用完成回调")
			return nil
		},
		func(string) error { return nil },
		func() error {
			recoveryCalls++
			return nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, 1, persistCalls)
	require.Equal(t, 1, recoveryCalls)
	require.Zero(t, pendingUserQuotaDeltaForTest(userId))
}

func TestUserQuotaBatchFinalizeErrorInvalidatesCacheAfterDatabaseCommit(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 307
	recoveryCalls := 0

	err := applyUserQuotaDeltaWithBatch(
		userId,
		-6,
		func() (userQuotaCacheUpdate, error) {
			return userQuotaCacheUpdate{
				state:     common.RedisHashIncrementFallbackAcquired,
				lockToken: "fallback-owner",
			}, nil
		},
		func(delta int) error {
			require.Equal(t, -6, delta)
			return nil
		},
		func(string) error { return errors.New("lock ownership lost") },
		func(string) error { return nil },
		func() error {
			recoveryCalls++
			return nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, 1, recoveryCalls)
}

func TestUserQuotaBatchFallbackConsumesSwappedInFlightDelta(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 306
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	userQuotaBatchInFlight[userId] = -11
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()

	err := applyUserQuotaDeltaWithBatch(
		userId,
		-4,
		func() (userQuotaCacheUpdate, error) {
			return userQuotaCacheUpdate{
				state:     common.RedisHashIncrementFallbackAcquired,
				lockToken: "fallback-owner",
			}, nil
		},
		func(delta int) error {
			require.Equal(t, -15, delta)
			return nil
		},
		func(string) error { return nil },
		func(string) error { return nil },
		func() error { return nil },
	)
	require.NoError(t, err)
	require.Zero(t, pendingUserQuotaDeltaForTest(userId))
}

func TestUserQuotaDeferredFallbackMarksLocalStateBeforeInitialRenewal(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 323
	renewStarted := make(chan struct{})
	continueRenewal := make(chan struct{})
	holdDone := make(chan error, 1)
	var initialEnsureOnce sync.Once

	go func() {
		holdDone <- holdUserQuotaDeferredFallback(
			userId,
			"initial-owner",
			func(string) error { return nil },
			func(string) error {
				initialEnsureOnce.Do(func() {
					close(renewStarted)
					<-continueRenewal
				})
				return nil
			},
			func() error { return nil },
		)
	}()

	select {
	case <-renewStarted:
	case <-time.After(time.Second):
		t.Fatal("等待首次回退锁续租超时")
	}
	require.True(t, hasDeferredUserQuotaFallbackForTest(userId))
	close(continueRenewal)
	require.NoError(t, <-holdDone)
	require.NoError(t, finishUserQuotaDeferredFallback(userId))
}

func TestUserQuotaDeferredFallbackKeepsOldLeaseWhenReplacementIsUnconfirmed(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 324
	oldFinishCalls := 0
	require.NoError(t, holdUserQuotaDeferredFallback(
		userId,
		"old-owner",
		func(lockToken string) error {
			require.Equal(t, "old-owner", lockToken)
			oldFinishCalls++
			return nil
		},
		func(string) error { return nil },
		func() error { return nil },
	))

	replacementErr := errors.New("new lease not confirmed")
	err := holdUserQuotaDeferredFallback(
		userId,
		"new-owner",
		func(string) error {
			t.Fatal("未确认的新租约不应进入完成流程")
			return nil
		},
		func(string) error { return replacementErr },
		func() error { return nil },
	)
	require.ErrorIs(t, err, replacementErr)
	require.Zero(t, oldFinishCalls)
	require.True(t, hasDeferredUserQuotaFallbackForTest(userId))
	require.NoError(t, finishUserQuotaDeferredFallback(userId))
	require.Equal(t, 1, oldFinishCalls)
}

func TestUserQuotaBatchSuccessfulFallbackFinalizesPreviousDeferredLock(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 309
	oldFinishCalls := 0
	newFinishCalls := 0

	holdUserQuotaDeferredFallback(
		userId,
		"old-owner",
		func(lockToken string) error {
			require.Equal(t, "old-owner", lockToken)
			oldFinishCalls++
			return nil
		},
		func(string) error { return nil },
		func() error { return nil },
	)

	err := applyUserQuotaDeltaWithBatch(
		userId,
		-4,
		func() (userQuotaCacheUpdate, error) {
			return userQuotaCacheUpdate{
				state:     common.RedisHashIncrementFallbackAcquired,
				lockToken: "new-owner",
			}, nil
		},
		func(delta int) error {
			require.Equal(t, -4, delta)
			return nil
		},
		func(lockToken string) error {
			require.Equal(t, "new-owner", lockToken)
			newFinishCalls++
			return nil
		},
		func(string) error { return nil },
		func() error { return nil },
	)
	require.NoError(t, err)
	require.Equal(t, 1, oldFinishCalls)
	require.Equal(t, 1, newFinishCalls)
	require.False(t, hasDeferredUserQuotaFallbackForTest(userId))
}

func TestUserQuotaBatchZeroDeltaFinalizesDeferredLock(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 310
	finishCalls := 0

	holdUserQuotaDeferredFallback(
		userId,
		"fallback-owner",
		func(lockToken string) error {
			require.Equal(t, "fallback-owner", lockToken)
			finishCalls++
			return nil
		},
		func(string) error { return nil },
		func() error { return nil },
	)
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	batchUpdateStores[BatchUpdateTypeUserQuota][userId] = 0
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()

	flushUserQuotaBatchUpdates()

	require.Equal(t, 1, finishCalls)
	require.False(t, hasDeferredUserQuotaFallbackForTest(userId))
}

func TestUserQuotaBatchDifferentUsersDoNotShareApplyLock(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	firstUserId := 401
	secondUserId := 402
	require.NotSame(t, userQuotaBatchApplyLockFor(firstUserId), userQuotaBatchApplyLockFor(secondUserId))

	firstPersistStarted := make(chan struct{})
	releaseFirstPersist := make(chan struct{})
	secondFinished := make(chan error, 1)
	var waitGroup sync.WaitGroup
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(releaseFirstPersist) })
	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		_ = applyUserQuotaDeltaWithBatch(
			firstUserId,
			-1,
			func() (userQuotaCacheUpdate, error) {
				return userQuotaCacheUpdate{
					state:     common.RedisHashIncrementFallbackAcquired,
					lockToken: "first-owner",
				}, nil
			},
			func(int) error {
				close(firstPersistStarted)
				<-releaseFirstPersist
				return nil
			},
			func(string) error { return nil },
			func(string) error { return nil },
			func() error { return nil },
		)
	}()

	<-firstPersistStarted
	go func() {
		secondFinished <- applyUserQuotaDeltaWithBatch(
			secondUserId,
			-2,
			func() (userQuotaCacheUpdate, error) {
				return userQuotaCacheUpdate{
					state:     common.RedisHashIncrementFallbackAcquired,
					lockToken: "second-owner",
				}, nil
			},
			func(int) error { return nil },
			func(string) error { return nil },
			func(string) error { return nil },
			func() error { return nil },
		)
	}()

	select {
	case err := <-secondFinished:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("不同用户的额度结算被全局互斥锁阻塞")
	}
	releaseOnce.Do(func() { close(releaseFirstPersist) })
	waitGroup.Wait()
}
