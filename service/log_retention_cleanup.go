package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	logRetentionCleanupInterval   = time.Hour
	logRetentionCleanupBatchSize  = 5000
	logRetentionCleanupMaxBatches = 12
)

var (
	logRetentionCleanupOnce    sync.Once
	logRetentionCleanupRunning atomic.Bool
)

// StartLogRetentionCleanupTask starts scheduled database business log cleanup.
// Distributed deployments run it only on the master node to avoid duplicate deletes.
func StartLogRetentionCleanupTask() {
	logRetentionCleanupOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf(
				"log retention cleanup started: tick=%s", logRetentionCleanupInterval,
			))
			ticker := time.NewTicker(logRetentionCleanupInterval)
			defer ticker.Stop()

			runLogRetentionCleanupOnce()
			for range ticker.C {
				runLogRetentionCleanupOnce()
			}
		})
	})
}

func runLogRetentionCleanupOnce() {
	runLogRetentionCleanupOnceAt(time.Now())
}

func runLogRetentionCleanupOnceAt(now time.Time) {
	retentionDays := common.GetLogRetentionDays()
	if retentionDays <= 0 || !logRetentionCleanupRunning.CompareAndSwap(false, true) {
		return
	}
	defer logRetentionCleanupRunning.Store(false)

	cutoff := now.Add(-time.Duration(retentionDays) * 24 * time.Hour).Unix()
	ctx, cancel := context.WithTimeout(context.Background(), logRetentionCleanupInterval)
	defer cancel()

	var deleted int64
	for range logRetentionCleanupMaxBatches {
		count, err := model.DeleteOldLogBatch(ctx, cutoff, logRetentionCleanupBatchSize)
		if err != nil {
			logger.LogWarn(context.Background(), fmt.Sprintf("log retention cleanup failed: %v", err))
			return
		}
		deleted += count
		if count < logRetentionCleanupBatchSize {
			break
		}
	}
	if deleted > 0 {
		logger.LogInfo(context.Background(), fmt.Sprintf("log retention cleanup deleted %d records", deleted))
	}
}
