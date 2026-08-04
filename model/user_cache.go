package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"

	"github.com/gin-gonic/gin"
)

const (
	userCacheGenerationRedisKeyPrefix = "user-cache-generation:"
	userQuotaFallbackRedisKeyPrefix   = "user-quota-fallback:"
	userQuotaFallbackLockExpiration   = 10 * time.Second
)

var ErrUserQuotaCacheSync = errors.New("用户额度缓存正在同步")

type userCacheBackend interface {
	generation(userId int) (int64, error)
	setUserIfGeneration(user User, generation int64) (bool, error)
	invalidateUser(userId int) error
	invalidateUserPreservingQuota(userId int) error
}

type redisUserCacheBackend struct{}

// UserBase is the compact user snapshot stored in cache and request context.
type UserBase struct {
	Id       int    `json:"id"`
	Group    string `json:"group"`
	GroupId  int    `json:"group_id"`
	Email    string `json:"email"`
	Quota    int    `json:"quota"`
	Status   int    `json:"status"`
	Role     int    `json:"role"`
	Username string `json:"username"`
	Setting  string `json:"setting"`
}

func (user *UserBase) WriteContext(c *gin.Context) {
	common.SetContextKey(c, constant.ContextKeyUserGroup, user.Group)
	common.SetContextKey(c, constant.ContextKeyUserGroupId, user.GroupId)
	common.SetContextKey(c, constant.ContextKeyUserQuota, user.Quota)
	common.SetContextKey(c, constant.ContextKeyUserStatus, user.Status)
	common.SetContextKey(c, constant.ContextKeyUserEmail, user.Email)
	common.SetContextKey(c, constant.ContextKeyUserName, user.Username)
	common.SetContextKey(c, constant.ContextKeyUserSetting, user.GetSetting())
}

func (user *UserBase) GetSetting() dto.UserSetting {
	setting := dto.UserSetting{}
	if user.Setting != "" {
		err := common.Unmarshal([]byte(user.Setting), &setting)
		if err != nil {
			common.SysLog("failed to unmarshal setting: " + err.Error())
		}
	}
	return setting
}

// getUserCacheKey returns the key for user cache
func getUserCacheKey(userId int) string {
	return fmt.Sprintf("user:%d", userId)
}

func getUserCacheGenerationKey(userId int) string {
	return fmt.Sprintf("%s%d", userCacheGenerationRedisKeyPrefix, userId)
}

func getUserQuotaFallbackKey(userId int) string {
	return fmt.Sprintf("%s%d", userQuotaFallbackRedisKeyPrefix, userId)
}

func (redisUserCacheBackend) generation(userId int) (int64, error) {
	return common.RedisGetGeneration(getUserCacheGenerationKey(userId))
}

func (redisUserCacheBackend) setUserIfGeneration(user User, generation int64) (bool, error) {
	return common.RedisHSetObjIfGenerationWithPreserveGuardAndBlockKey(
		getUserCacheGenerationKey(user.Id),
		getUserCacheKey(user.Id),
		generation,
		user.ToBaseUser(),
		time.Duration(common.RedisKeyCacheSeconds())*time.Second,
		"Id",
		getUserQuotaFallbackKey(user.Id),
		"Quota",
	)
}

func (redisUserCacheBackend) invalidateUser(userId int) error {
	return common.RedisBumpGenerationAndDeleteKeys(
		getUserCacheGenerationKey(userId),
		[]string{getUserCacheKey(userId)},
	)
}

func (redisUserCacheBackend) invalidateUserPreservingQuota(userId int) error {
	return common.RedisBumpGenerationAndKeepHashFields(
		getUserCacheGenerationKey(userId),
		getUserCacheKey(userId),
		"Id",
		"Quota",
	)
}

// invalidateUserCache clears user cache
func invalidateUserCache(userId int) error {
	if !common.RedisEnabled {
		return nil
	}
	return redisUserCacheBackend{}.invalidateUser(userId)
}

// InvalidateUserCache 使活动用户资料快照失效，同时保留实时额度。
// 真正删除用户时由 Delete/HardDelete 直接调用完整删除助手。
func InvalidateUserCache(userId int) error {
	return invalidateUserCachePreservingQuota(userId)
}

// invalidateUserCachePreservingQuota 使资料快照失效，但保留批量计费尚未落库的实时额度。
func invalidateUserCachePreservingQuota(userId int) error {
	if !common.RedisEnabled {
		return nil
	}
	return redisUserCacheBackend{}.invalidateUserPreservingQuota(userId)
}

func updateUserCacheIfGenerationWithBackend(backend userCacheBackend, user User, generation int64) (bool, error) {
	return backend.setUserIfGeneration(user, generation)
}

func userCacheSnapshotWithPreservedQuota(user User, preservedLiveQuota *int) User {
	if preservedLiveQuota != nil {
		user.Quota = *preservedLiveQuota
	}
	return user
}

func isCompleteUserCacheForId(userId int, userCache *UserBase) bool {
	return userCache != nil &&
		userCache.Id == userId &&
		userCache.Role != common.RoleGuestUser &&
		common.IsValidateRole(userCache.Role)
}

func shouldPreserveQuotaFromCorruptUserCache(userId int, userCache *UserBase, err error) bool {
	if userCache == nil || userCache.Id != userId {
		return false
	}
	var fieldErr *common.RedisHashFieldDecodeError
	if !errors.As(err, &fieldErr) {
		return false
	}
	return fieldErr.Field != "Id" && fieldErr.Field != "Quota"
}

func waitForUserQuotaFallbackWith(
	userId int,
	maxWait time.Duration,
	retryDelay time.Duration,
	keyExists func(string) (bool, error),
) error {
	deadline := time.Now().Add(maxWait)
	for {
		locked, err := keyExists(getUserQuotaFallbackKey(userId))
		if err != nil {
			return fmt.Errorf("%w: 读取回退锁失败: %w", ErrUserQuotaCacheSync, err)
		}
		if !locked {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: 额度同步超时", ErrUserQuotaCacheSync)
		}
		time.Sleep(retryDelay)
	}
}

func waitForUserQuotaFallback(userId int) error {
	return waitForUserQuotaFallbackWith(
		userId,
		userQuotaFallbackLockExpiration+time.Second,
		userQuotaFallbackRetryDelay,
		common.RedisKeyExists,
	)
}

// GetUserCache gets complete user cache from hash
func GetUserCache(userId int) (userCache *UserBase, err error) {
	return getUserCacheWithRetry(userId, 0, nil)
}

func userQuotaDatabaseFallbackBlocked(userId int, hasTrustedLiveQuota bool) bool {
	queued, inProgress := userQuotaLocalPersistenceState(userId)
	return inProgress || hasUserQuotaDeferredFallback(userId) || (queued && !hasTrustedLiveQuota)
}

func userQuotaCacheReadBlocked(userId int) bool {
	_, inProgress := userQuotaLocalPersistenceState(userId)
	return inProgress || hasUserQuotaDeferredFallback(userId)
}

func userQuotaDatabaseSnapshotRequiresFence() bool {
	return common.RedisEnabled && common.BatchUpdateEnabled
}

func validateUserQuotaDatabaseSnapshot(preservedLiveQuota *int, cacheGenerationReady bool) error {
	if !cacheGenerationReady && (preservedLiveQuota != nil || userQuotaDatabaseSnapshotRequiresFence()) {
		return fmt.Errorf("%w: 无法确认数据库回退额度", ErrUserQuotaCacheSync)
	}
	return nil
}

func getUserCacheWithRetry(
	userId int,
	attempt int,
	preservedLiveQuota *int,
) (userCache *UserBase, err error) {
	if hasUserQuotaDeferredFallback(userId) {
		return nil, fmt.Errorf("%w: 本实例仍有待落库额度", ErrUserQuotaCacheSync)
	}
	var user *User
	var cacheGeneration int64
	var cacheGenerationReady bool
	var skipCachePopulation bool

	if _, expireErr := ExpireDueSubscriptionsForUser(userId); expireErr != nil {
		common.SysLog(fmt.Sprintf("failed to expire due subscriptions for user %d: %v", userId, expireErr))
	}

	// Try getting from Redis first
	userCache, err = cacheGetUserBase(userId)
	if err == nil {
		if userCache.Id != userId {
			if invalidateErr := invalidateUserCache(userId); invalidateErr != nil {
				common.SysLog(fmt.Sprintf(
					"failed to invalidate mismatched user cache: user_id=%d, cached_user_id=%d, error=%v",
					userId,
					userCache.Id,
					invalidateErr,
				))
				skipCachePopulation = true
			}
		} else if isCompleteUserCacheForId(userId, userCache) {
			if userQuotaCacheReadBlocked(userId) {
				return nil, fmt.Errorf("%w: 缓存读取期间正在持久化额度", ErrUserQuotaCacheSync)
			}
			return userCache, nil
		}
		// 仅保留 Id 与 Quota 的部分 hash 是资料更新后的主动失效标记。不能删除它，
		// 否则批量计费尚未落库的实时额度会丢失；后续数据库回填会保留该字段。
		if userCache.Role == common.RoleGuestUser && userCache.Id == userId {
			quota := userCache.Quota
			preservedLiveQuota = &quota
		} else if userCache.Role != common.RoleGuestUser {
			_ = invalidateUserCache(userId)
		}
	} else if errors.Is(err, common.ErrRedisHashCorrupt) {
		// cacheGetUserBase 按 Id、Quota、资料字段的顺序解码。只要前两项已通过且
		// Id 匹配，后续资料字段缺失或损坏都应保留实时额度，再从数据库重建资料。
		preserveQuota := shouldPreserveQuotaFromCorruptUserCache(userId, userCache, err)
		var invalidateErr error
		if preserveQuota {
			preservedLiveQuota = &userCache.Quota
			invalidateErr = invalidateUserCachePreservingQuota(userId)
		} else {
			invalidateErr = invalidateUserCache(userId)
		}
		if invalidateErr != nil {
			common.SysLog(fmt.Sprintf(
				"failed to invalidate corrupt user cache: user_id=%d, error=%v",
				userId,
				invalidateErr,
			))
			skipCachePopulation = true
		}
	}
	if userQuotaDatabaseFallbackBlocked(userId, preservedLiveQuota != nil) {
		return nil, fmt.Errorf("%w: 缓存未命中且本实例仍有未落库额度", ErrUserQuotaCacheSync)
	}
	if common.RedisEnabled && !skipCachePopulation {
		cacheGeneration, err = redisUserCacheBackend{}.generation(userId)
		if err != nil {
			common.SysLog("failed to read user cache generation: " + err.Error())
			err = nil
		} else {
			cacheGenerationReady = true
		}
	}
	if err := validateUserQuotaDatabaseSnapshot(preservedLiveQuota, cacheGenerationReady); err != nil {
		// 部分 Hash 中的 Quota 只代表 HGETALL 时刻的值。若无法取得 generation，
		// 批量计费也可能存在其他实例尚未落库的增量。两者都必须通过带栅栏回填
		// 并重读 Redis，才能确认数据库快照没有遗漏后续 HINCRBY。
		return nil, err
	}

	// If Redis fails, get from DB
	user, err = GetUserById(userId, false)
	if err != nil {
		return nil, err // Return nil and error if DB lookup fails
	}
	if userQuotaDatabaseFallbackBlocked(userId, preservedLiveQuota != nil) {
		return nil, fmt.Errorf("%w: 数据库读取期间出现待落库额度", ErrUserQuotaCacheSync)
	}

	// Create cache object from user data
	userCache = &UserBase{
		Id:       user.Id,
		Group:    user.Group,
		GroupId:  user.GroupId,
		Quota:    user.Quota,
		Status:   user.Status,
		Role:     user.Role,
		Username: user.Username,
		Setting:  user.Setting,
		Email:    user.Email,
	}
	if preservedLiveQuota != nil {
		userCache.Quota = *preservedLiveQuota
	}
	// 缓存 miss 时同步完成带 generation 栅栏的回填，避免计费增量先于快照落库。
	if cacheGenerationReady {
		if userQuotaDatabaseFallbackBlocked(userId, preservedLiveQuota != nil) {
			return nil, fmt.Errorf("%w: 缓存回填前出现待落库额度", ErrUserQuotaCacheSync)
		}
		// Id/Quota 部分 hash 可能在 HGETALL 后、Lua 回填前到期，快照也必须携带实时额度。
		userSnapshot := userCacheSnapshotWithPreservedQuota(*user, preservedLiveQuota)
		written, cacheErr := updateUserCacheIfGenerationWithBackend(
			redisUserCacheBackend{}, userSnapshot, cacheGeneration,
		)
		if errors.Is(cacheErr, common.ErrRedisHashWriteBlocked) {
			if waitErr := waitForUserQuotaFallback(userId); waitErr != nil {
				// 回退期间的数据库额度可能尚未提交，必须失败关闭，不能返回旧额度。
				return nil, waitErr
			}
			if attempt >= 2 {
				return nil, fmt.Errorf("%w: generation 连续冲突", ErrUserQuotaCacheSync)
			}
			// fallback 完成会提升 generation 并删除旧 Hash；此前捕获的绝对额度
			// 已失效，必须从当前 Hash 或已提交数据库重新读取。
			return getUserCacheWithRetry(userId, attempt+1, nil)
		} else if cacheErr != nil {
			if preservedLiveQuota != nil || userQuotaDatabaseSnapshotRequiresFence() {
				return nil, fmt.Errorf("%w: 缓存回填失败: %v", ErrUserQuotaCacheSync, cacheErr)
			}
			common.SysLog("failed to update user status cache: " + cacheErr.Error())
		} else if !written {
			if attempt >= 2 {
				if userQuotaDatabaseFallbackBlocked(userId, preservedLiveQuota != nil) {
					return nil, fmt.Errorf("%w: generation 冲突期间出现待落库额度", ErrUserQuotaCacheSync)
				}
				if preservedLiveQuota != nil || userQuotaDatabaseSnapshotRequiresFence() {
					return nil, fmt.Errorf("%w: generation 连续冲突", ErrUserQuotaCacheSync)
				}
				// generation 已按用户隔离；这里只可能是同一用户资料连续变更。
				// 重试耗尽时放弃缓存写入，数据库快照仍可用于当前请求。
				common.SysLog(fmt.Sprintf(
					"skip user cache population after repeated generation conflicts: user_id=%d",
					userId,
				))
				return userCache, nil
			}
			// generation 变化后不能把旧额度快照带入新一代缓存。
			return getUserCacheWithRetry(userId, attempt+1, nil)
		} else {
			// Lua 可能在回填时保留了 HGETALL 之后发生变化的最新 Quota，返回前必须
			// 重读完整 Hash，不能继续使用回填前的额度快照。
			refreshedCache, refreshErr := cacheGetUserBase(userId)
			if refreshErr == nil && isCompleteUserCacheForId(userId, refreshedCache) {
				if userQuotaCacheReadBlocked(userId) {
					return nil, fmt.Errorf("%w: 缓存重读期间正在持久化额度", ErrUserQuotaCacheSync)
				}
				return refreshedCache, nil
			}
			if errors.Is(refreshErr, common.ErrRedisHashCorrupt) {
				_ = invalidateUserCache(userId)
			}
			if attempt >= 2 {
				if refreshErr != nil {
					return nil, fmt.Errorf("%w: 回填后重读失败: %v", ErrUserQuotaCacheSync, refreshErr)
				}
				return nil, fmt.Errorf("%w: 回填后缓存再次失效", ErrUserQuotaCacheSync)
			}
			return getUserCacheWithRetry(userId, attempt+1, nil)
		}
	}
	if userQuotaDatabaseFallbackBlocked(userId, preservedLiveQuota != nil) {
		return nil, fmt.Errorf("%w: 返回数据库快照前出现待落库额度", ErrUserQuotaCacheSync)
	}
	if preservedLiveQuota != nil || userQuotaDatabaseSnapshotRequiresFence() {
		return nil, fmt.Errorf("%w: 数据库回退未完成缓存栅栏", ErrUserQuotaCacheSync)
	}

	return userCache, nil
}

func cacheGetUserBase(userId int) (*UserBase, error) {
	if !common.RedisEnabled {
		return nil, fmt.Errorf("redis is not enabled")
	}
	var userCache UserBase
	// Try getting from Redis first
	err := common.RedisHGetObjWithRequiredFields(
		getUserCacheKey(userId),
		&userCache,
		"Id",
		"Quota",
		"Group",
		"GroupId",
		"Email",
		"Status",
		"Role",
		"Username",
		"Setting",
	)
	if err != nil {
		return &userCache, err
	}
	return &userCache, nil
}

type userQuotaCacheUpdate struct {
	state     common.RedisHashIncrementState
	lockToken string
}

func tryAdjustUserQuotaCache(userId int, delta int64) (userQuotaCacheUpdate, error) {
	if !common.RedisEnabled {
		return userQuotaCacheUpdate{}, errors.New("redis is not enabled")
	}
	lockToken := common.GetUUID()
	state, err := common.RedisHIncrByOrAcquireFallback(
		getUserCacheKey(userId),
		getUserCacheGenerationKey(userId),
		getUserQuotaFallbackKey(userId),
		"Id",
		fmt.Sprintf("%d", userId),
		"Quota",
		delta,
		time.Duration(common.RedisKeyCacheSeconds())*time.Second,
		lockToken,
		userQuotaFallbackLockExpiration,
	)
	if err != nil {
		// Redis 可能已执行 Lua 但响应在网络层丢失；保留唯一令牌供调用方尝试释放锁。
		return userQuotaCacheUpdate{lockToken: lockToken}, err
	}
	if state != common.RedisHashIncrementFallbackAcquired {
		lockToken = ""
	}
	return userQuotaCacheUpdate{state: state, lockToken: lockToken}, nil
}

func finishUserQuotaCacheFallback(userId int, lockToken string) error {
	finished, err := common.RedisFinishHashFallback(
		getUserCacheKey(userId),
		getUserCacheGenerationKey(userId),
		getUserQuotaFallbackKey(userId),
		lockToken,
	)
	if err != nil {
		return err
	}
	if !finished {
		return errors.New("用户额度缓存回退锁已失效")
	}
	return nil
}

func ensureUserQuotaCacheFallback(userId int, lockToken string) error {
	protected, err := common.RedisEnsureHashFallback(
		getUserCacheKey(userId),
		getUserCacheGenerationKey(userId),
		getUserQuotaFallbackKey(userId),
		lockToken,
		userQuotaFallbackLockExpiration,
	)
	if err != nil {
		return err
	}
	if !protected {
		return errors.New("用户额度缓存回退锁由其他实例持有")
	}
	return nil
}

func renewUserQuotaCacheFallback(userId int, lockToken string) error {
	renewed, err := common.RedisRenewHashFallback(
		getUserQuotaFallbackKey(userId),
		lockToken,
		userQuotaFallbackLockExpiration,
	)
	if err != nil {
		return err
	}
	if !renewed {
		return errors.New("用户额度缓存回退锁已失效")
	}
	return nil
}

// Add atomic quota operations using hash fields
func cacheIncrUserQuota(userId int, delta int64) error {
	if !common.RedisEnabled {
		return nil
	}
	updated, err := common.RedisHIncrByIfExists(
		getUserCacheKey(userId),
		"Quota",
		delta,
		time.Duration(common.RedisKeyCacheSeconds())*time.Second,
	)
	if err != nil {
		return err
	}
	// 这些调用点已先完成数据库更新；缓存不存在时无需创建不完整 hash。
	if !updated {
		return nil
	}
	return nil
}

func cacheDecrUserQuota(userId int, delta int64) error {
	return cacheIncrUserQuota(userId, -delta)
}

func AdjustUserQuotaCache(userId int, delta int64) error {
	return cacheIncrUserQuota(userId, delta)
}

// Helper functions to get individual fields if needed
func getUserGroupCache(userId int) (string, error) {
	cache, err := GetUserCache(userId)
	if err != nil {
		return "", err
	}
	return cache.Group, nil
}

func getUserQuotaCache(userId int) (int, error) {
	cache, err := GetUserCache(userId)
	if err != nil {
		return 0, err
	}
	return cache.Quota, nil
}

func getUserStatusCache(userId int) (int, error) {
	cache, err := GetUserCache(userId)
	if err != nil {
		return 0, err
	}
	return cache.Status, nil
}

func getUserNameCache(userId int) (string, error) {
	cache, err := GetUserCache(userId)
	if err != nil {
		return "", err
	}
	return cache.Username, nil
}

// New functions for individual field updates
func updateUserStatusCache(userId int, status bool) error {
	if !common.RedisEnabled {
		return nil
	}
	statusInt := common.UserStatusEnabled
	if !status {
		statusInt = common.UserStatusDisabled
	}
	return common.RedisHSetField(getUserCacheKey(userId), "Status", fmt.Sprintf("%d", statusInt))
}

func updateUserQuotaCache(userId int, quota int) error {
	if !common.RedisEnabled {
		return nil
	}
	return common.RedisHSetField(getUserCacheKey(userId), "Quota", fmt.Sprintf("%d", quota))
}

func updateUserGroupCache(userId int, _ string) error {
	if !common.RedisEnabled {
		return nil
	}
	// 分组名称与稳定 group_id 必须作为一个整体刷新。保留 Id 与实时 Quota，
	// 其余资料由下一次请求从数据库重建，避免批量计费期间删除额度缓存。
	return invalidateUserCachePreservingQuota(userId)
}

func UpdateUserGroupCache(userId int, group string) error {
	return updateUserGroupCache(userId, group)
}

func updateUserNameCache(userId int, username string) error {
	if !common.RedisEnabled {
		return nil
	}
	return common.RedisHSetField(getUserCacheKey(userId), "Username", username)
}

// GetUserLanguage returns the user's language preference from cache
// Uses the existing GetUserCache mechanism for efficiency
func GetUserLanguage(userId int) string {
	userCache, err := GetUserCache(userId)
	if err != nil {
		return ""
	}
	return userCache.GetSetting().Language
}
