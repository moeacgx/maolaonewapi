package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestUserCacheReadStatusUsesServiceUnavailableForQuotaSync(t *testing.T) {
	err := fmt.Errorf("%w: 等待同步完成超时", model.ErrUserQuotaCacheSync)
	require.Equal(t, http.StatusServiceUnavailable, userCacheReadStatus(err))
	require.Equal(t, http.StatusInternalServerError, userCacheReadStatus(errors.New("database unavailable")))
}
