package model

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

type memoryUserCacheBackend struct {
	mu          sync.Mutex
	generations map[int]int64
	users       map[int]*UserBase
}

func newMemoryUserCacheBackend() *memoryUserCacheBackend {
	return &memoryUserCacheBackend{
		generations: make(map[int]int64),
		users:       make(map[int]*UserBase),
	}
}

func (backend *memoryUserCacheBackend) generation(userId int) (int64, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.generations[userId], nil
}

func (backend *memoryUserCacheBackend) setUserIfGeneration(
	user User,
	generation int64,
) (bool, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.generations[user.Id] != generation {
		return false, nil
	}
	updated := user.ToBaseUser()
	if cached, ok := backend.users[user.Id]; ok && cached.Id != 0 {
		updated.Quota = cached.Quota
	}
	backend.users[user.Id] = updated
	return true, nil
}

func (backend *memoryUserCacheBackend) invalidateUser(userId int) error {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.generations[userId]++
	delete(backend.users, userId)
	return nil
}

func (backend *memoryUserCacheBackend) invalidateUserPreservingQuota(userId int) error {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.generations[userId]++
	if cached, ok := backend.users[userId]; ok {
		backend.users[userId] = &UserBase{Id: cached.Id, Quota: cached.Quota}
	}
	return nil
}

func TestUserCacheStaleGenerationIsRejected(t *testing.T) {
	backend := newMemoryUserCacheBackend()
	staleUser := User{Id: 88, Status: 1, Username: "before-ban"}
	generation, err := backend.generation(staleUser.Id)
	require.NoError(t, err)

	require.NoError(t, backend.invalidateUser(staleUser.Id))
	written, err := updateUserCacheIfGenerationWithBackend(backend, staleUser, generation)
	require.NoError(t, err)
	require.False(t, written)
	require.NotContains(t, backend.users, staleUser.Id)
}

func TestUserCacheCurrentGenerationCanPopulate(t *testing.T) {
	backend := newMemoryUserCacheBackend()
	currentUser := User{Id: 99, Status: 2, Username: "after-ban"}
	generation, err := backend.generation(currentUser.Id)
	require.NoError(t, err)

	written, err := updateUserCacheIfGenerationWithBackend(backend, currentUser, generation)
	require.NoError(t, err)
	require.True(t, written)
	require.Equal(t, currentUser.Status, backend.users[currentUser.Id].Status)
}

func TestUserCacheSettingInvalidationPreservesLiveQuotaAndRejectsStaleSnapshot(t *testing.T) {
	backend := newMemoryUserCacheBackend()
	userId := 100
	backend.users[userId] = &UserBase{
		Id:      userId,
		Role:    1,
		Quota:   70,
		Setting: `{"language":"en"}`,
	}
	staleGeneration, err := backend.generation(userId)
	require.NoError(t, err)

	require.NoError(t, backend.invalidateUserPreservingQuota(userId))
	require.Equal(t, 70, backend.users[userId].Quota)
	require.Equal(t, userId, backend.users[userId].Id)
	require.Empty(t, backend.users[userId].Setting)

	written, err := updateUserCacheIfGenerationWithBackend(
		backend,
		User{Id: userId, Quota: 100, Setting: `{"language":"en"}`},
		staleGeneration,
	)
	require.NoError(t, err)
	require.False(t, written)

	currentGeneration, err := backend.generation(userId)
	require.NoError(t, err)
	written, err = updateUserCacheIfGenerationWithBackend(
		backend,
		User{Id: userId, Role: 1, Quota: 100, Setting: `{"language":"zh"}`},
		currentGeneration,
	)
	require.NoError(t, err)
	require.True(t, written)
	require.Equal(t, 70, backend.users[userId].Quota)
	require.Equal(t, `{"language":"zh"}`, backend.users[userId].Setting)
}

func TestUserCacheQuotaOnlyHashIsNotTreatedAsAbsoluteQuota(t *testing.T) {
	backend := newMemoryUserCacheBackend()
	userId := 101
	// 没有 Id 标记的历史或损坏 Hash 不是完整用户快照，Quota 不能当成绝对余额。
	backend.users[userId] = &UserBase{Quota: -30}
	generation, err := backend.generation(userId)
	require.NoError(t, err)

	written, err := updateUserCacheIfGenerationWithBackend(
		backend,
		User{Id: userId, Role: 1, Quota: 70},
		generation,
	)
	require.NoError(t, err)
	require.True(t, written)
	require.Equal(t, userId, backend.users[userId].Id)
	require.Equal(t, 70, backend.users[userId].Quota)
}

func TestUserCacheCorruptProfileFieldKeepsTrustedLiveQuota(t *testing.T) {
	userId := 102
	userCache := &UserBase{Id: userId, Quota: 70}
	profileErr := &common.RedisHashFieldDecodeError{
		Field: "Status",
		Err:   errors.New("invalid integer"),
	}

	require.True(t, shouldPreserveQuotaFromCorruptUserCache(userId, userCache, profileErr))
	require.Equal(t, 70, userCache.Quota)
	groupErr := &common.RedisHashFieldDecodeError{
		Field: "GroupId",
		Err:   errors.New("invalid integer"),
	}
	require.True(t, shouldPreserveQuotaFromCorruptUserCache(userId, userCache, groupErr))

	quotaErr := &common.RedisHashFieldDecodeError{
		Field: "Quota",
		Err:   errors.New("invalid integer"),
	}
	require.False(t, shouldPreserveQuotaFromCorruptUserCache(userId, userCache, quotaErr))
	require.False(t, shouldPreserveQuotaFromCorruptUserCache(userId+1, userCache, profileErr))
}

func TestUserCacheGenerationIsIsolatedPerUser(t *testing.T) {
	backend := newMemoryUserCacheBackend()
	userA := User{Id: 201, Role: 1, Quota: 100}
	userB := User{Id: 202, Role: 1, Quota: 200}
	generationA, err := backend.generation(userA.Id)
	require.NoError(t, err)

	require.NoError(t, backend.invalidateUser(userB.Id))
	written, err := updateUserCacheIfGenerationWithBackend(backend, userA, generationA)
	require.NoError(t, err)
	require.True(t, written)
	require.Equal(t, 100, backend.users[userA.Id].Quota)
}

func TestUserCacheMultipleSameUserGenerationConflicts(t *testing.T) {
	backend := newMemoryUserCacheBackend()
	user := User{Id: 203, Role: 1, Quota: 300}
	staleGeneration, err := backend.generation(user.Id)
	require.NoError(t, err)

	for range 3 {
		require.NoError(t, backend.invalidateUserPreservingQuota(user.Id))
	}
	written, err := updateUserCacheIfGenerationWithBackend(backend, user, staleGeneration)
	require.NoError(t, err)
	require.False(t, written)

	currentGeneration, err := backend.generation(user.Id)
	require.NoError(t, err)
	written, err = updateUserCacheIfGenerationWithBackend(backend, user, currentGeneration)
	require.NoError(t, err)
	require.True(t, written)
}

func TestUserCacheGenerationConflictRequiresFreshQuotaSnapshot(t *testing.T) {
	backend := newMemoryUserCacheBackend()
	userId := 204
	staleLiveQuota := 100
	databaseUser := User{Id: userId, Role: 1, Quota: 100}
	staleGeneration, err := backend.generation(userId)
	require.NoError(t, err)

	// 模拟先读到部分 Hash=100，随后并发事务提交 DB=90、提升 generation 并删除 Hash。
	backend.users[userId] = &UserBase{Id: userId, Quota: staleLiveQuota}
	delete(backend.users, userId)
	require.NoError(t, backend.invalidateUser(userId))
	databaseUser.Quota = 90

	staleSnapshot := userCacheSnapshotWithPreservedQuota(databaseUser, &staleLiveQuota)
	written, err := updateUserCacheIfGenerationWithBackend(backend, staleSnapshot, staleGeneration)
	require.NoError(t, err)
	require.False(t, written)

	// generation 冲突后的重试必须丢弃旧 preserved 值，使用新数据库额度。
	currentGeneration, err := backend.generation(userId)
	require.NoError(t, err)
	written, err = updateUserCacheIfGenerationWithBackend(backend, databaseUser, currentGeneration)
	require.NoError(t, err)
	require.True(t, written)
	require.Equal(t, 90, backend.users[userId].Quota)
}

func TestUserCacheWaitsForQuotaFallbackBeforeRetry(t *testing.T) {
	userId := 205
	checks := 0
	err := waitForUserQuotaFallbackWith(
		userId,
		time.Second,
		0,
		func(key string) (bool, error) {
			require.Equal(t, getUserQuotaFallbackKey(userId), key)
			checks++
			return checks < 3, nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, 3, checks)
}

func TestUserCacheQuotaFallbackTimeoutFailsClosed(t *testing.T) {
	err := waitForUserQuotaFallbackWith(
		206,
		0,
		0,
		func(string) (bool, error) { return true, nil },
	)
	require.ErrorContains(t, err, "同步超时")
	require.ErrorIs(t, err, ErrUserQuotaCacheSync)
}

func TestUserCacheQuotaFallbackReadErrorFailsClosed(t *testing.T) {
	expectedErr := errors.New("redis unavailable")
	err := waitForUserQuotaFallbackWith(
		207,
		time.Second,
		0,
		func(string) (bool, error) { return false, expectedErr },
	)
	require.ErrorIs(t, err, expectedErr)
	require.ErrorIs(t, err, ErrUserQuotaCacheSync)
}
