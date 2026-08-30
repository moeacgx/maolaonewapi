package service

import (
	"sync"
	"sync/atomic"
	"time"
)

const systemInstanceMetricsBucketCount = 60

type systemInstanceRuntimeMetricsState struct {
	sync.Mutex
	bucketSeconds [systemInstanceMetricsBucketCount]int64
	requestCounts [systemInstanceMetricsBucketCount]int64
	active        atomic.Int64
}

var systemInstanceRuntimeMetrics systemInstanceRuntimeMetricsState

// SystemInstanceTrafficMetrics 是当前节点的请求流量快照。
type SystemInstanceTrafficMetrics struct {
	RPM            int64 `json:"rpm"`
	ActiveRequests int64 `json:"active_requests"`
}

// RecordSystemInstanceRequestStart 记录一个进入 Relay 的请求。
func RecordSystemInstanceRequestStart() {
	recordSystemInstanceRequestAt(time.Now())
}

// RecordSystemInstanceRequestEnd 记录一个 Relay 请求结束，并防止计数变成负数。
func RecordSystemInstanceRequestEnd() {
	for {
		current := systemInstanceRuntimeMetrics.active.Load()
		if current <= 0 || systemInstanceRuntimeMetrics.active.CompareAndSwap(current, current-1) {
			return
		}
	}
}

// GetSystemInstanceTrafficMetrics 返回最近 60 秒 RPM 和当前活动请求数。
func GetSystemInstanceTrafficMetrics() SystemInstanceTrafficMetrics {
	return snapshotSystemInstanceRuntimeMetrics(time.Now())
}

func recordSystemInstanceRequestAt(now time.Time) {
	systemInstanceRuntimeMetrics.active.Add(1)
	second := now.Unix()
	index := int(second % systemInstanceMetricsBucketCount)
	if index < 0 {
		index += systemInstanceMetricsBucketCount
	}

	systemInstanceRuntimeMetrics.Lock()
	if systemInstanceRuntimeMetrics.bucketSeconds[index] != second {
		systemInstanceRuntimeMetrics.bucketSeconds[index] = second
		systemInstanceRuntimeMetrics.requestCounts[index] = 0
	}
	systemInstanceRuntimeMetrics.requestCounts[index]++
	systemInstanceRuntimeMetrics.Unlock()
}

func snapshotSystemInstanceRuntimeMetrics(now time.Time) SystemInstanceTrafficMetrics {
	nowSecond := now.Unix()
	firstSecond := nowSecond - systemInstanceMetricsBucketCount + 1
	var rpm int64

	systemInstanceRuntimeMetrics.Lock()
	for index, second := range systemInstanceRuntimeMetrics.bucketSeconds {
		if second >= firstSecond && second <= nowSecond {
			rpm += systemInstanceRuntimeMetrics.requestCounts[index]
		}
	}
	systemInstanceRuntimeMetrics.Unlock()

	return SystemInstanceTrafficMetrics{
		RPM:            rpm,
		ActiveRequests: systemInstanceRuntimeMetrics.active.Load(),
	}
}

func resetSystemInstanceRuntimeMetricsForTest() {
	systemInstanceRuntimeMetrics.Lock()
	systemInstanceRuntimeMetrics.bucketSeconds = [systemInstanceMetricsBucketCount]int64{}
	systemInstanceRuntimeMetrics.requestCounts = [systemInstanceMetricsBucketCount]int64{}
	systemInstanceRuntimeMetrics.Unlock()
	systemInstanceRuntimeMetrics.active.Store(0)
}
