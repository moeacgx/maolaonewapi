package common

import "sync/atomic"

const (
	// DefaultLogRetentionDays disables automatic database business log cleanup by default.
	DefaultLogRetentionDays = 0
	// MaxLogRetentionDays caps the retention setting at 10 years.
	MaxLogRetentionDays = 3650
)

var logRetentionDays atomic.Int64

func init() {
	logRetentionDays.Store(DefaultLogRetentionDays)
}

// GetLogRetentionDays returns how many days database business logs are retained.
// 0 disables automatic cleanup.
func GetLogRetentionDays() int {
	return int(logRetentionDays.Load())
}

// SetLogRetentionDays updates the database business log retention period.
// The configuration layer validates the accepted range.
func SetLogRetentionDays(days int) {
	logRetentionDays.Store(int64(days))
}
