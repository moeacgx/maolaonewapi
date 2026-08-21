package middleware

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/require"
)

func TestPerformanceClientErrorReplacementIgnoresInternalError(t *testing.T) {
	require.NoError(t, common.UpdateErrorMessageReplacementRules(`[{"status_code":503,"match":"system cpu overloaded","mode":"contains","replace":"service busy","replace_status_code":429}]`))
	t.Cleanup(func() { require.NoError(t, common.UpdateErrorMessageReplacementRules(`[]`)) })
	apiErr := types.NewErrorWithStatusCode(errors.New("system cpu overloaded"), "system_cpu_overloaded", http.StatusServiceUnavailable)
	clientErr, clientStatus := performanceClientOpenAIError(apiErr)
	require.Equal(t, "system cpu overloaded", clientErr.Message)
	require.Equal(t, http.StatusServiceUnavailable, clientStatus)
	require.Equal(t, "system cpu overloaded", apiErr.Error())
	require.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
}
