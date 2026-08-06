package service

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestUserQuotaCacheSyncErrorUsesServiceUnavailable(t *testing.T) {
	err := newUserQuotaQueryError(model.ErrUserQuotaCacheSync)
	require.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	require.Equal(t, types.ErrorCodeQueryDataError, err.GetErrorCode())
	require.True(t, types.IsSkipRetryError(err))
}

func TestUserQuotaDatabaseErrorKeepsInternalServerError(t *testing.T) {
	err := newUserQuotaQueryError(errors.New("database unavailable"))
	require.Equal(t, http.StatusInternalServerError, err.StatusCode)
}

func TestUserQuotaUpdateErrorUsesServiceUnavailable(t *testing.T) {
	cause := errors.New("database unavailable")
	err := NewUserQuotaUpdateError(cause)

	require.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	require.Equal(t, types.ErrorCodeUpdateDataError, err.GetErrorCode())
	require.True(t, types.IsSkipRetryError(err))
	require.ErrorIs(t, err, cause)
}
