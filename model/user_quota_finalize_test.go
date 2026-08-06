package model

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserQuotaFallbackRetriesTokenSafeFinishAfterCacheRecovery(t *testing.T) {
	finishErr := errors.New("redis finish response lost")
	finishCalls := 0
	fallback := &userQuotaDeferredFallback{
		lockToken: "retry-finish-owner",
		finish: func(lockToken string) error {
			require.Equal(t, "retry-finish-owner", lockToken)
			finishCalls++
			if finishCalls == 1 {
				return finishErr
			}
			return nil
		},
		ensure: func(string) error { return nil },
		renew:  func(string) error { return nil },
		recover: func() error {
			return nil
		},
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	fallback.startLeaseMaintenance(userQuotaFallbackLockExpiration / 3)

	safeToRelease, err := finalizeHeldUserQuotaFallback(fallback)
	require.True(t, safeToRelease)
	require.ErrorIs(t, err, finishErr)
	require.Equal(t, 2, finishCalls)

	safeToRelease, err = finalizeHeldUserQuotaFallback(fallback)
	require.True(t, safeToRelease)
	require.NoError(t, err)
	require.Equal(t, 2, finishCalls)
}
