package controller

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelMonitorTestFailedIncludesLocalErrors(t *testing.T) {
	require.True(t, channelMonitorTestFailed(testResult{localErr: errors.New("local setup failed")}))
	require.False(t, channelMonitorTestFailed(testResult{}))
}
