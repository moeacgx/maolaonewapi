package model

import (
	"errors"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

const (
	BatchUpdateTypeUserQuota = iota
	BatchUpdateTypeTokenQuota
	BatchUpdateTypeUsedQuota
	BatchUpdateTypeChannelUsedQuota
	BatchUpdateTypeRequestCount
	BatchUpdateTypeCount // if you add a new type, you need to add a new map and a new lock
)

var batchUpdateStores []map[int]int
var batchUpdateLocks []sync.Mutex
var userQuotaBatchInFlight = make(map[int]int)
var userQuotaPersistenceInProgress = make(map[int]int)
var userQuotaDeferredFallbacks = make(map[int]*userQuotaDeferredFallback)
var userQuotaDeferredFallbacksLock sync.Mutex

const (
	userQuotaBatchApplyLockShards = 256
	// 跨实例回退锁已表示缓存不可写，继续固定轮询只会阻塞用户请求。
	// 一次判定后直接进入受保护的数据库回退，避免额外等待约 11 秒。
	userQuotaFallbackRetryLimit = 1
	userQuotaFallbackRetryDelay = 20 * time.Millisecond
)

var userQuotaBatchApplyLocks [userQuotaBatchApplyLockShards]sync.Mutex

func init() {
	for i := 0; i < BatchUpdateTypeCount; i++ {
		batchUpdateStores = append(batchUpdateStores, make(map[int]int))
		batchUpdateLocks = append(batchUpdateLocks, sync.Mutex{})
	}
}

func InitBatchUpdater() {
	gopool.Go(func() {
		for {
			time.Sleep(time.Duration(common.BatchUpdateInterval) * time.Second)
			batchUpdate()
		}
	})
}

func addNewRecord(type_ int, id int, value int) {
	batchUpdateLocks[type_].Lock()
	defer batchUpdateLocks[type_].Unlock()
	if _, ok := batchUpdateStores[type_][id]; !ok {
		batchUpdateStores[type_][id] = value
	} else {
		batchUpdateStores[type_][id] += value
	}
}

func userQuotaBatchApplyLockFor(id int) *sync.Mutex {
	return &userQuotaBatchApplyLocks[uint(id)%userQuotaBatchApplyLockShards]
}

func takePendingUserQuotaDeltaLocked(id int) int {
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	defer batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
	delta := batchUpdateStores[BatchUpdateTypeUserQuota][id] + userQuotaBatchInFlight[id]
	delete(batchUpdateStores[BatchUpdateTypeUserQuota], id)
	delete(userQuotaBatchInFlight, id)
	return delta
}

func restorePendingUserQuotaDeltaLocked(id int, delta int) {
	if delta == 0 {
		return
	}
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	batchUpdateStores[BatchUpdateTypeUserQuota][id] += delta
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
}

func enqueueUserQuotaDeltaLocked(id int, delta int) {
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	batchUpdateStores[BatchUpdateTypeUserQuota][id] += delta
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
}

func userQuotaLocalPersistenceState(id int) (queued bool, inProgress bool) {
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	defer batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
	return batchUpdateStores[BatchUpdateTypeUserQuota][id]+userQuotaBatchInFlight[id] != 0,
		userQuotaPersistenceInProgress[id] > 0
}

func hasPendingUserQuotaDelta(id int) bool {
	queued, inProgress := userQuotaLocalPersistenceState(id)
	return queued || inProgress
}

func beginUserQuotaPersistence(id int) {
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	userQuotaPersistenceInProgress[id]++
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
}

func finishUserQuotaPersistence(id int) {
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	if userQuotaPersistenceInProgress[id] <= 1 {
		delete(userQuotaPersistenceInProgress, id)
	} else {
		userQuotaPersistenceInProgress[id]--
	}
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
}

type userQuotaCacheAdjustFunc func() (userQuotaCacheUpdate, error)
type userQuotaPersistFunc func(delta int) error
type userQuotaFallbackFinishFunc func(lockToken string) error
type userQuotaFallbackRenewFunc func(lockToken string) error
type userQuotaCacheRecoveryFunc func() error

type userQuotaDeltaDurability int

const (
	userQuotaDeltaAbortable userQuotaDeltaDurability = iota
	userQuotaDeltaCommitted
)

type userQuotaDeferredFallback struct {
	lockToken    string
	finish       userQuotaFallbackFinishFunc
	ensure       userQuotaFallbackRenewFunc
	renew        userQuotaFallbackRenewFunc
	recover      userQuotaCacheRecoveryFunc
	stop         chan struct{}
	done         chan struct{}
	stopOnce     sync.Once
	retryOnce    sync.Once
	finalizeLock sync.Mutex
	leaseLock    sync.Mutex
	finalizing   bool
	completed    bool
}

func (fallback *userQuotaDeferredFallback) stopRenewalAndWait() {
	fallback.stopOnce.Do(func() {
		close(fallback.stop)
	})
	<-fallback.done
}

func (fallback *userQuotaDeferredFallback) startLeaseMaintenance(interval time.Duration) {
	go func() {
		defer close(fallback.done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := fallback.maintainLease(); err != nil {
					common.SysError("failed to renew deferred user quota fallback lock: " + err.Error())
				}
			case <-fallback.stop:
				return
			}
		}
	}()
}

func (fallback *userQuotaDeferredFallback) maintainLease() error {
	fallback.leaseLock.Lock()
	defer fallback.leaseLock.Unlock()
	if fallback.finalizing {
		// finalize 期间只续期当前锁，不能在 finish 已成功后重新建锁。
		return fallback.renew(fallback.lockToken)
	}
	// 普通待落库期间允许同一令牌在锁过期后重新建立保护并删除旧缓存。
	return fallback.ensure(fallback.lockToken)
}

func (fallback *userQuotaDeferredFallback) prepareToFinalize() error {
	fallback.leaseLock.Lock()
	defer fallback.leaseLock.Unlock()
	fallback.finalizing = true
	if err := fallback.ensure(fallback.lockToken); err != nil {
		fallback.finalizing = false
		return err
	}
	return nil
}

func (fallback *userQuotaDeferredFallback) resumeLeaseRecovery() {
	fallback.leaseLock.Lock()
	fallback.finalizing = false
	fallback.leaseLock.Unlock()
}

func holdUserQuotaDeferredFallback(
	id int,
	lockToken string,
	finish userQuotaFallbackFinishFunc,
	ensure userQuotaFallbackRenewFunc,
	recoverCache userQuotaCacheRecoveryFunc,
) error {
	if finish == nil || ensure == nil {
		return errors.New("用户额度缓存回退锁函数为空")
	}
	fallback := &userQuotaDeferredFallback{
		lockToken: lockToken,
		finish:    finish,
		ensure:    ensure,
		renew: func(token string) error {
			return renewUserQuotaCacheFallback(id, token)
		},
		recover: recoverCache,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	userQuotaDeferredFallbacksLock.Lock()
	previous := userQuotaDeferredFallbacks[id]
	if previous == nil {
		// 首个本地标记必须先于网络调用可见，避免 Redis 请求阻塞期间读取旧额度。
		userQuotaDeferredFallbacks[id] = fallback
		userQuotaDeferredFallbacksLock.Unlock()
		initialRenewErr := ensure(lockToken)
		fallback.startLeaseMaintenance(userQuotaFallbackLockExpiration / 3)
		return initialRenewErr
	}
	userQuotaDeferredFallbacksLock.Unlock()

	// 已有本地保护时先确认新令牌，避免先释放旧锁形成无保护窗口。
	initialRenewErr := ensure(lockToken)
	if initialRenewErr != nil {
		return initialRenewErr
	}
	userQuotaDeferredFallbacksLock.Lock()
	previous = userQuotaDeferredFallbacks[id]
	userQuotaDeferredFallbacks[id] = fallback
	userQuotaDeferredFallbacksLock.Unlock()
	fallback.startLeaseMaintenance(userQuotaFallbackLockExpiration / 3)

	if previous != nil {
		safeToRelease, err := finalizeHeldUserQuotaFallback(previous)
		if !safeToRelease {
			// 新 fallback 已确认并接管保护；旧令牌不得继续尝试重建锁。
			previous.stopRenewalAndWait()
		}
		if err != nil {
			common.SysError("failed to finalize replaced user quota fallback lock: " + err.Error())
		}
	}
	return nil
}

func hasUserQuotaDeferredFallback(id int) bool {
	userQuotaDeferredFallbacksLock.Lock()
	defer userQuotaDeferredFallbacksLock.Unlock()
	return userQuotaDeferredFallbacks[id] != nil
}

func finishUserQuotaDeferredFallback(id int) error {
	userQuotaDeferredFallbacksLock.Lock()
	fallback := userQuotaDeferredFallbacks[id]
	userQuotaDeferredFallbacksLock.Unlock()
	if fallback == nil {
		return nil
	}
	safeToRelease, err := finalizeHeldUserQuotaFallback(fallback)
	userQuotaDeferredFallbacksLock.Lock()
	if safeToRelease && userQuotaDeferredFallbacks[id] == fallback {
		delete(userQuotaDeferredFallbacks, id)
	}
	userQuotaDeferredFallbacksLock.Unlock()
	if !safeToRelease {
		retryUserQuotaFallbackFinalization(id, fallback)
	}
	return err
}

func finalizeHeldUserQuotaFallback(fallback *userQuotaDeferredFallback) (bool, error) {
	fallback.finalizeLock.Lock()
	defer fallback.finalizeLock.Unlock()
	if fallback.completed {
		return true, nil
	}
	// 先确认完整 TTL 的分布式保护。此后续租协程只允许 Renew 当前锁，
	// finish/recover 即使阻塞也不会停租，更不会在 finish 成功后重新建锁。
	if err := fallback.prepareToFinalize(); err != nil {
		return false, err
	}
	if err := fallback.finish(fallback.lockToken); err != nil {
		if fallback.recover != nil {
			if recoveryErr := fallback.recover(); recoveryErr != nil {
				fallback.resumeLeaseRecovery()
				return false, errors.Join(err, recoveryErr)
			}
			// 第一次 finish 可能已执行成功但响应丢失，也可能只是暂时超时。
			// 缓存恢复完成后、停止续租前再用同一令牌清理一次，避免本实例的
			// 分布式锁无谓残留到 TTL；Lua 的令牌校验不会删除其他实例的锁。
			retryErr := fallback.finish(fallback.lockToken)
			fallback.completed = true
			fallback.stopRenewalAndWait()
			return true, errors.Join(err, retryErr)
		}
		fallback.resumeLeaseRecovery()
		return false, err
	}
	fallback.completed = true
	fallback.stopRenewalAndWait()
	return true, nil
}

func retryUserQuotaFallbackFinalization(id int, fallback *userQuotaDeferredFallback) {
	retryUserQuotaFallbackFinalizationWithDelay(id, fallback, time.Second)
}

func retryUserQuotaFallbackFinalizationWithDelay(
	id int,
	fallback *userQuotaDeferredFallback,
	retryDelay time.Duration,
) {
	fallback.retryOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(retryDelay)
			defer ticker.Stop()
			for range ticker.C {
				applyLock := userQuotaBatchApplyLockFor(id)
				applyLock.Lock()
				userQuotaDeferredFallbacksLock.Lock()
				isCurrent := userQuotaDeferredFallbacks[id] == fallback
				userQuotaDeferredFallbacksLock.Unlock()
				if !isCurrent {
					applyLock.Unlock()
					return
				}
				queued, inProgress := userQuotaLocalPersistenceState(id)
				if queued || inProgress {
					err := fallback.maintainLease()
					applyLock.Unlock()
					if err != nil {
						common.SysError("failed to maintain pending user quota fallback lock: " + err.Error())
					}
					continue
				}

				safeToRelease, err := finalizeHeldUserQuotaFallback(fallback)
				if !safeToRelease {
					applyLock.Unlock()
					common.SysError("failed to retry user quota fallback finalization: " + err.Error())
					continue
				}
				userQuotaDeferredFallbacksLock.Lock()
				if userQuotaDeferredFallbacks[id] == fallback {
					delete(userQuotaDeferredFallbacks, id)
				}
				userQuotaDeferredFallbacksLock.Unlock()
				applyLock.Unlock()
				if err != nil {
					common.SysError("user quota fallback recovered after finalization error: " + err.Error())
				}
				return
			}
		}()
	})
}

func holdUserQuotaFallbackForPending(id int) error {
	return holdUserQuotaFallbackForPendingWith(
		id,
		common.GetUUID(),
		func(token string) error {
			return finishUserQuotaCacheFallback(id, token)
		},
		func(token string) error {
			return ensureUserQuotaCacheFallback(id, token)
		},
		func() error {
			return invalidateUserQuotaFallbackCache(id)
		},
	)
}

// ProtectCommittedUserQuotaCache 为已提交数据库、但缓存同步结果不确定的额度变更
// 建立失败关闭窗口。调用方不能因此重试已提交的非幂等业务操作；保护窗口会在
// ConsumePendingUserQuotaDelta 返回前尝试完成，并在 Redis 故障时后台继续清理缓存。
// 调用方必须位于同一用户的 ConsumePendingUserQuotaDelta 回调内，保持 applyLock
// 覆盖“数据库提交 -> 缓存调整 -> 建立保护”的完整窗口。
func ProtectCommittedUserQuotaCache(id int) error {
	return holdUserQuotaFallbackForPending(id)
}

func holdUserQuotaFallbackForPendingWith(
	id int,
	lockToken string,
	finish userQuotaFallbackFinishFunc,
	renew userQuotaFallbackRenewFunc,
	recoverCache userQuotaCacheRecoveryFunc,
) error {
	if hasUserQuotaDeferredFallback(id) {
		return nil
	}
	return holdUserQuotaDeferredFallback(id, lockToken, finish, renew, recoverCache)
}

// applyUserQuotaDeltaWithBatch 将缓存判定与本实例批量队列串行化。
// 缓存命中才允许入队；缺失时合并既有待落库增量并立即持久化。
func applyUserQuotaDeltaWithBatch(
	id int,
	delta int,
	adjustCache userQuotaCacheAdjustFunc,
	persist userQuotaPersistFunc,
	finishFallback userQuotaFallbackFinishFunc,
	renewFallback userQuotaFallbackRenewFunc,
	recoverCache userQuotaCacheRecoveryFunc,
) error {
	return applyUserQuotaDeltaWithBatchDurability(
		id,
		delta,
		userQuotaDeltaAbortable,
		adjustCache,
		persist,
		finishFallback,
		renewFallback,
		recoverCache,
	)
}

func applyCommittedUserQuotaDeltaWithBatch(
	id int,
	delta int,
	adjustCache userQuotaCacheAdjustFunc,
	persist userQuotaPersistFunc,
	finishFallback userQuotaFallbackFinishFunc,
	renewFallback userQuotaFallbackRenewFunc,
	recoverCache userQuotaCacheRecoveryFunc,
) error {
	return applyUserQuotaDeltaWithBatchDurability(
		id,
		delta,
		userQuotaDeltaCommitted,
		adjustCache,
		persist,
		finishFallback,
		renewFallback,
		recoverCache,
	)
}

func applyUserQuotaDeltaWithBatchDurability(
	id int,
	delta int,
	durability userQuotaDeltaDurability,
	adjustCache userQuotaCacheAdjustFunc,
	persist userQuotaPersistFunc,
	finishFallback userQuotaFallbackFinishFunc,
	renewFallback userQuotaFallbackRenewFunc,
	recoverCache userQuotaCacheRecoveryFunc,
) error {
	return applyUserQuotaDeltaWithBatchDurabilityAndRetry(
		id,
		delta,
		durability,
		adjustCache,
		persist,
		finishFallback,
		renewFallback,
		recoverCache,
		userQuotaFallbackRetryLimit,
		userQuotaFallbackRetryDelay,
	)
}

func applyUserQuotaDeltaWithBatchDurabilityAndRetry(
	id int,
	delta int,
	durability userQuotaDeltaDurability,
	adjustCache userQuotaCacheAdjustFunc,
	persist userQuotaPersistFunc,
	finishFallback userQuotaFallbackFinishFunc,
	renewFallback userQuotaFallbackRenewFunc,
	recoverCache userQuotaCacheRecoveryFunc,
	retryLimit int,
	retryDelay time.Duration,
) error {
	applyLock := userQuotaBatchApplyLockFor(id)
	persistWithoutTrustedCache := func(cacheUpdate userQuotaCacheUpdate, cacheCause error) error {
		beginUserQuotaPersistence(id)
		defer finishUserQuotaPersistence(id)

		if cacheUpdate.lockToken != "" {
			if holdErr := holdUserQuotaDeferredFallback(
				id,
				cacheUpdate.lockToken,
				finishFallback,
				renewFallback,
				recoverCache,
			); holdErr != nil {
				// Lua 可能已经取得锁但响应丢失。本地标记和续租任务仍需保留，
				// 数据库提交后再尽力结束保护窗口。
				common.SysError("failed to ensure uncertain user quota fallback lock: " + holdErr.Error())
			}
		}

		pendingDelta := takePendingUserQuotaDeltaLocked(id)
		persistErr := persist(pendingDelta + delta)
		restoredDelta := pendingDelta
		if persistErr != nil && durability == userQuotaDeltaCommitted {
			restoredDelta += delta
		}
		if persistErr != nil {
			restorePendingUserQuotaDeltaLocked(id, restoredDelta)
			if restoredDelta != 0 && !hasUserQuotaDeferredFallback(id) {
				// Redis 错误或其他实例持锁时也要留下本实例保护标记。
				// 首次续租即使失败，后台仍会继续尝试取得分布式回退锁。
				if holdErr := holdUserQuotaFallbackForPendingWith(
					id,
					common.GetUUID(),
					finishFallback,
					renewFallback,
					recoverCache,
				); holdErr != nil {
					common.SysError("failed to protect pending user quota after database error: " + holdErr.Error())
				}
			}
		}

		if persistErr == nil || restoredDelta == 0 {
			if finishErr := finishUserQuotaDeferredFallback(id); finishErr != nil {
				common.SysError("failed to release user quota fallback lock: " + finishErr.Error())
			}
		}
		if cacheUpdate.lockToken == "" {
			if recoveryErr := recoverCache(); recoveryErr != nil {
				common.SysError("failed to invalidate user quota cache after cache fallback: " + recoveryErr.Error())
				if persistErr == nil && !hasUserQuotaDeferredFallback(id) {
					// 数据库已提交但缓存失效结果不确定时，不能让其他实例继续读取
					// 旧 Hash。先建立本地失败关闭标记，再由后台争取分布式锁并清缓存。
					if holdErr := holdUserQuotaFallbackForPendingWith(
						id,
						common.GetUUID(),
						finishFallback,
						renewFallback,
						recoverCache,
					); holdErr != nil {
						common.SysError("failed to protect committed user quota after cache recovery error: " + holdErr.Error())
					}
					if finishErr := finishUserQuotaDeferredFallback(id); finishErr != nil {
						common.SysError("failed to finalize committed user quota cache protection: " + finishErr.Error())
					}
				}
			}
		}

		if persistErr != nil && durability == userQuotaDeltaCommitted {
			// 已完成的消费或退款已经进入受保护队列。向调用方返回成功，
			// 避免其重试同一笔非幂等额度变化或跳过后续账单记录。
			common.SysError("committed user quota delta deferred after database error: " + persistErr.Error())
			return nil
		}
		if persistErr != nil {
			return persistErr
		}
		if cacheCause != nil {
			common.SysLog("user quota cache update fell back to database: " + cacheCause.Error())
		}
		return nil
	}

	for attempt := 0; attempt < retryLimit; attempt++ {
		applyLock.Lock()
		cacheUpdate, cacheErr := adjustCache()
		if cacheErr != nil {
			persistErr := persistWithoutTrustedCache(cacheUpdate, cacheErr)
			applyLock.Unlock()
			return persistErr
		}

		switch cacheUpdate.state {
		case common.RedisHashIncrementUpdated:
			enqueueUserQuotaDeltaLocked(id, delta)
			applyLock.Unlock()
			return nil
		case common.RedisHashIncrementFallbackAcquired:
			persistErr := persistWithoutTrustedCache(cacheUpdate, nil)
			applyLock.Unlock()
			return persistErr
		case common.RedisHashIncrementFallbackBusy:
			if attempt == retryLimit-1 {
				persistErr := persistWithoutTrustedCache(
					userQuotaCacheUpdate{},
					errors.New("user quota fallback lock remained busy"),
				)
				applyLock.Unlock()
				return persistErr
			}
			applyLock.Unlock()
			time.Sleep(retryDelay)
		default:
			persistErr := persistWithoutTrustedCache(
				userQuotaCacheUpdate{},
				errors.New("invalid user quota cache update state"),
			)
			applyLock.Unlock()
			return persistErr
		}
	}
	return errors.New("unreachable user quota fallback state")
}

type userQuotaPendingFallbackFunc func(id int) error

func ConsumePendingUserQuotaDelta(id int, apply func(delta int) error) error {
	return consumePendingUserQuotaDeltaWithFallback(id, apply, holdUserQuotaFallbackForPending)
}

func consumePendingUserQuotaDeltaWithFallback(
	id int,
	apply func(delta int) error,
	holdFallback userQuotaPendingFallbackFunc,
) error {
	applyLock := userQuotaBatchApplyLockFor(id)
	applyLock.Lock()
	defer applyLock.Unlock()
	beginUserQuotaPersistence(id)
	defer finishUserQuotaPersistence(id)

	delta := takePendingUserQuotaDeltaLocked(id)

	if err := apply(delta); err != nil {
		restorePendingUserQuotaDeltaLocked(id, delta)
		if delta != 0 {
			if holdErr := holdFallback(id); holdErr != nil {
				return errors.Join(err, holdErr)
			}
		}
		return err
	}
	if err := finishUserQuotaDeferredFallback(id); err != nil {
		common.SysError("failed to finalize deferred user quota fallback after database commit: " + err.Error())
	}
	return nil
}

func flushUserQuotaBatchUpdates() {
	flushUserQuotaBatchUpdatesWith(
		func(id int, delta int) error {
			return increaseUserQuota(id, delta)
		},
		holdUserQuotaFallbackForPending,
	)
}

func flushUserQuotaBatchUpdatesWith(
	persist func(id int, delta int) error,
	holdFallback userQuotaPendingFallbackFunc,
) {
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	store := batchUpdateStores[BatchUpdateTypeUserQuota]
	batchUpdateStores[BatchUpdateTypeUserQuota] = make(map[int]int)
	for id, delta := range store {
		userQuotaBatchInFlight[id] += delta
	}
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()

	for id := range store {
		applyLock := userQuotaBatchApplyLockFor(id)
		applyLock.Lock()
		beginUserQuotaPersistence(id)
		batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
		delta := userQuotaBatchInFlight[id]
		delete(userQuotaBatchInFlight, id)
		batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
		if delta == 0 {
			if finishErr := finishUserQuotaDeferredFallback(id); finishErr != nil {
				common.SysError("failed to finalize zero-delta user quota fallback: " + finishErr.Error())
			}
			finishUserQuotaPersistence(id)
			applyLock.Unlock()
			continue
		}
		err := persist(id, delta)
		if err != nil {
			common.SysLog("failed to batch update user quota: " + err.Error())
			restorePendingUserQuotaDeltaLocked(id, delta)
			if holdErr := holdFallback(id); holdErr != nil {
				common.SysError("failed to protect pending user quota after batch update error: " + holdErr.Error())
			}
		} else if finishErr := finishUserQuotaDeferredFallback(id); finishErr != nil {
			common.SysError("failed to finalize deferred user quota fallback after batch commit: " + finishErr.Error())
		}
		finishUserQuotaPersistence(id)
		applyLock.Unlock()
	}
}

func batchUpdate() {
	// check if there's any data to update
	hasData := false
	for i := 0; i < BatchUpdateTypeCount; i++ {
		batchUpdateLocks[i].Lock()
		if len(batchUpdateStores[i]) > 0 {
			hasData = true
			batchUpdateLocks[i].Unlock()
			break
		}
		batchUpdateLocks[i].Unlock()
	}

	if !hasData {
		return
	}

	common.SysLog("batch update started")
	for i := 0; i < BatchUpdateTypeCount; i++ {
		if i == BatchUpdateTypeUserQuota {
			flushUserQuotaBatchUpdates()
			continue
		}
		batchUpdateLocks[i].Lock()
		store := batchUpdateStores[i]
		batchUpdateStores[i] = make(map[int]int)
		batchUpdateLocks[i].Unlock()
		// TODO: maybe we can combine updates with same key?
		for key, value := range store {
			switch i {
			case BatchUpdateTypeUserQuota:
				err := increaseUserQuota(key, value)
				if err != nil {
					common.SysLog("failed to batch update user quota: " + err.Error())
				}
			case BatchUpdateTypeTokenQuota:
				err := increaseTokenQuota(key, value)
				if err != nil {
					common.SysLog("failed to batch update token quota: " + err.Error())
				}
			case BatchUpdateTypeUsedQuota:
				updateUserUsedQuota(key, value)
			case BatchUpdateTypeRequestCount:
				updateUserRequestCount(key, value)
			case BatchUpdateTypeChannelUsedQuota:
				updateChannelUsedQuota(key, value)
			}
		}
	}
	common.SysLog("batch update finished")
}

func RecordExist(err error) (bool, error) {
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return false, err
}

func shouldUpdateRedis(fromDB bool, err error) bool {
	return common.RedisEnabled && fromDB && err == nil
}
