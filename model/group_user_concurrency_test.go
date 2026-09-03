package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupUserConcurrencyIsolatedByUserAndGroup(t *testing.T) {
	lease, acquired := TryAcquireGroupUserConcurrency(7001, 11001, 1)
	require.True(t, acquired)
	require.NotNil(t, lease)
	t.Cleanup(lease.Release)

	_, same := TryAcquireGroupUserConcurrency(7001, 11001, 1)
	otherGroup, otherGroupAcquired := TryAcquireGroupUserConcurrency(7001, 11002, 1)
	require.True(t, otherGroupAcquired)
	t.Cleanup(otherGroup.Release)
	otherUser, otherUserAcquired := TryAcquireGroupUserConcurrency(7002, 11001, 1)
	require.True(t, otherUserAcquired)
	t.Cleanup(otherUser.Release)

	assert.False(t, same)
}

func TestGroupUserConcurrencyReleaseIsIdempotentAndZeroLimitIsUnlimited(t *testing.T) {
	first, acquired := TryAcquireGroupUserConcurrency(7003, 11003, 1)
	require.True(t, acquired)
	first.Release()
	first.Release()
	second, acquired := TryAcquireGroupUserConcurrency(7003, 11003, 1)
	require.True(t, acquired)
	second.Release()

	unlimited, acquired := TryAcquireGroupUserConcurrency(7004, 11004, 0)
	assert.True(t, acquired)
	assert.Nil(t, unlimited)
}
