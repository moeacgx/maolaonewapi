package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystemInstanceInfoSerializesTrafficMetrics(t *testing.T) {
	data, err := common.Marshal(&SystemInstanceInfo{
		SchemaVersion: 2,
		Metrics:       SystemInstanceTrafficMetrics{RPM: 14, ActiveRequests: 3},
	})
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(data, &payload))
	assert.Equal(t, float64(2), payload["schema_version"])
	assert.Equal(t, map[string]any{"rpm": float64(14), "active_requests": float64(3)}, payload["metrics"])
}

func TestSystemInstanceRuntimeMetricsTrackRecentRequestsAndActiveRequests(t *testing.T) {
	resetSystemInstanceRuntimeMetricsForTest()
	now := time.Unix(1_800_000_000, 0)
	recordSystemInstanceRequestAt(now)
	recordSystemInstanceRequestAt(now)
	RecordSystemInstanceRequestEnd()

	metrics := snapshotSystemInstanceRuntimeMetrics(now)
	assert.Equal(t, int64(2), metrics.RPM)
	assert.Equal(t, int64(1), metrics.ActiveRequests)
}

func TestSystemInstanceRuntimeMetricsIgnoreRequestsOutsideRollingMinute(t *testing.T) {
	resetSystemInstanceRuntimeMetricsForTest()
	now := time.Unix(1_800_000_000, 0)
	recordSystemInstanceRequestAt(now.Add(-61 * time.Second))

	metrics := snapshotSystemInstanceRuntimeMetrics(now)
	require.Equal(t, int64(0), metrics.RPM)
	assert.Equal(t, int64(1), metrics.ActiveRequests)
}
