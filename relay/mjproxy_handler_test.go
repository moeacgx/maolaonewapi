package relay

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestMidjourneyQuotaReadErrorUsesServiceUnavailableForQuotaSync(t *testing.T) {
	err := fmt.Errorf("%w: wait timeout", model.ErrUserQuotaCacheSync)
	response := midjourneyQuotaReadError(err)

	require.Equal(t, 4, response.Code)
	require.Equal(t, http.StatusServiceUnavailable, response.StatusCode)
	require.True(t, errors.Is(err, model.ErrUserQuotaCacheSync))
}

func TestMidjourneyQuotaReadErrorKeepsOtherFailuresAsBadRequest(t *testing.T) {
	response := midjourneyQuotaReadError(errors.New("database unavailable"))

	require.Equal(t, http.StatusBadRequest, response.StatusCode)
}
