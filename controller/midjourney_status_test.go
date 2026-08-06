package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestMidjourneyResponseStatusCode(t *testing.T) {
	require.Equal(t, http.StatusServiceUnavailable, midjourneyResponseStatusCode(&dto.MidjourneyResponse{
		Code:       4,
		StatusCode: http.StatusServiceUnavailable,
	}))
	require.Equal(t, http.StatusTooManyRequests, midjourneyResponseStatusCode(&dto.MidjourneyResponse{
		Code:       30,
		StatusCode: http.StatusServiceUnavailable,
	}))
	require.Equal(t, http.StatusBadRequest, midjourneyResponseStatusCode(&dto.MidjourneyResponse{
		Code: 4,
	}))
	require.Equal(t, http.StatusBadRequest, midjourneyResponseStatusCode(nil))
}
