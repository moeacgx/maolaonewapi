package common

import (
	"sync/atomic"
	"time"
)

const (
	DefaultImageTaskDataRetentionHours = 1
	MaxImageTaskDataRetentionHours     = 24 * 365
	DefaultImageTaskTimeoutMinutes     = 30
	MaxImageTaskTimeoutMinutes         = 24 * 60
)

var imageTaskDataRetentionHours atomic.Int64

func init() {
	imageTaskDataRetentionHours.Store(DefaultImageTaskDataRetentionHours)
}

// GetImageTaskDataRetentionHours returns how long terminal image payloads remain
// available. Zero disables payload cleanup without deleting task audit records.
func GetImageTaskDataRetentionHours() int {
	return int(imageTaskDataRetentionHours.Load())
}

func SetImageTaskDataRetentionHours(hours int) {
	if hours < 0 || hours > MaxImageTaskDataRetentionHours {
		hours = DefaultImageTaskDataRetentionHours
	}
	imageTaskDataRetentionHours.Store(int64(hours))
}

// GetImageTaskTimeout returns the deterministic reconciliation boundary for a
// local image task whose in-process relay was interrupted. Zero disables it.
func GetImageTaskTimeout() time.Duration {
	minutes := GetEnvOrDefault("IMAGE_TASK_TIMEOUT_MINUTES", DefaultImageTaskTimeoutMinutes)
	if minutes <= 0 {
		return 0
	}
	if minutes > MaxImageTaskTimeoutMinutes {
		minutes = MaxImageTaskTimeoutMinutes
	}
	return time.Duration(minutes) * time.Minute
}
