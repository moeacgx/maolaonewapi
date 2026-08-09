package model

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type memoryUserCacheBackend struct {
	mu              sync.Mutex
	generationValue int64
	users           map[int]*UserBase
}

func newMemoryUserCacheBackend() *memoryUserCacheBackend {
	return &memoryUserCacheBackend{users: make(map[int]*UserBase)}
}

func (backend *memoryUserCacheBackend) generation() (int64, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.generationValue, nil
}

func (backend *memoryUserCacheBackend) setUserIfGeneration(
	user User,
	generation int64,
) (bool, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.generationValue != generation {
		return false, nil
	}
	backend.users[user.Id] = user.ToBaseUser()
	return true, nil
}

func (backend *memoryUserCacheBackend) invalidateUser(userId int) error {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.generationValue++
	delete(backend.users, userId)
	return nil
}

func TestUserCacheStaleGenerationIsRejected(t *testing.T) {
	backend := newMemoryUserCacheBackend()
	generation, err := backend.generation()
	require.NoError(t, err)

	staleUser := User{Id: 88, Status: 1, Username: "before-ban"}
	require.NoError(t, backend.invalidateUser(staleUser.Id))
	written, err := updateUserCacheIfGenerationWithBackend(backend, staleUser, generation)
	require.NoError(t, err)
	require.False(t, written)
	require.NotContains(t, backend.users, staleUser.Id)
}

func TestUserCacheCurrentGenerationCanPopulate(t *testing.T) {
	backend := newMemoryUserCacheBackend()
	generation, err := backend.generation()
	require.NoError(t, err)

	currentUser := User{Id: 99, Status: 2, Username: "after-ban"}
	written, err := updateUserCacheIfGenerationWithBackend(backend, currentUser, generation)
	require.NoError(t, err)
	require.True(t, written)
	require.Equal(t, currentUser.Status, backend.users[currentUser.Id].Status)
}
