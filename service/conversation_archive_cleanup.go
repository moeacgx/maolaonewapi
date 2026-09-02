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
	conversationArchiveCleanupInterval   = time.Hour
	conversationArchiveCleanupBatchSize  = 5000
	conversationArchiveCleanupMaxBatches = 12
)

var (
	conversationArchiveCleanupOnce    sync.Once
	conversationArchiveCleanupRunning atomic.Bool
)

// StartConversationArchiveCleanupTask 启动归档保留期清理任务。分布式部署中
// 仅主节点执行，避免多个实例重复扫描和删除同一批记录。
func StartConversationArchiveCleanupTask() {
	conversationArchiveCleanupOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf(
				"conversation archive cleanup started: tick=%s", conversationArchiveCleanupInterval,
			))
			runConversationArchiveCleanupOnce()
			ticker := time.NewTicker(conversationArchiveCleanupInterval)
			defer ticker.Stop()
			for range ticker.C {
				runConversationArchiveCleanupOnce()
			}
		})
	})
}

func runConversationArchiveCleanupOnce() {
	if !conversationArchiveCleanupRunning.CompareAndSwap(false, true) {
		return
	}
	defer conversationArchiveCleanupRunning.Store(false)

	ctx, cancel := context.WithTimeout(context.Background(), conversationArchiveCleanupInterval)
	defer cancel()
	now := time.Now().Unix()
	var deleted int64
	for range conversationArchiveCleanupMaxBatches {
		count, err := model.DeleteExpiredConversationArchiveBatch(ctx, now, conversationArchiveCleanupBatchSize)
		if err != nil {
			logger.LogWarn(context.Background(), fmt.Sprintf("conversation archive cleanup failed: %v", err))
			return
		}
		deleted += count
		if count < conversationArchiveCleanupBatchSize {
			break
		}
	}
	if deleted > 0 {
		logger.LogInfo(context.Background(), fmt.Sprintf("conversation archive cleanup deleted %d records", deleted))
	}
}
