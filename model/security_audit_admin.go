package model

import (
	"errors"

	"gorm.io/gorm"
)

// DeletePromptAuditEventsByIDs 保留统一的导出命名，实际删除逻辑集中在
// DeletePromptAuditEventsByIds，避免两套实现产生不同的不存在事件语义。
func DeletePromptAuditEventsByIDs(ids []int64) (int64, int64, error) {
	return DeletePromptAuditEventsByIds(ids)
}

// CleanupFinishedPromptAuditJobs 按保留期清理已结束任务。
// 只匹配 done/failed 和 finished_at，绝不影响仍在队列中的任务。
func CleanupFinishedPromptAuditJobs(before int64, batch int) (int64, error) {
	if batch < 1 || batch > promptAuditDeleteBatchSize {
		batch = promptAuditDeleteBatchSize
	}
	var jobs []PromptAuditJob
	if err := DB.Where("status IN ? AND finished_at > 0 AND finished_at <= ?",
		[]string{PromptAuditJobDone, PromptAuditJobFailed}, before).
		Order("id ASC").Limit(batch).Select("id").Find(&jobs).Error; err != nil {
		return 0, err
	}
	if len(jobs) == 0 {
		return 0, nil
	}
	ids := make([]int64, 0, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.Id)
	}
	result := DB.Where("id IN ?", ids).Delete(&PromptAuditJob{})
	return result.RowsAffected, result.Error
}

// CleanupExpiredPromptAuditJobs 清理超过保留期且尚未开始处理的密文任务。
// 删除与队列计数修正位于同一事务，并再次约束状态，避免与 Worker 领取竞态。
func CleanupExpiredPromptAuditJobs(before int64, batch int) (int64, error) {
	if before <= 0 {
		return 0, errors.New("提示词审计任务清理截止时间无效")
	}
	if batch < 1 || batch > promptAuditDeleteBatchSize {
		batch = promptAuditDeleteBatchSize
	}
	var deleted int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		statuses := []string{PromptAuditJobQueued, PromptAuditJobRetry}
		var jobs []PromptAuditJob
		if err := tx.Where("status IN ? AND created_at > 0 AND created_at <= ?", statuses, before).
			Order("id ASC").Limit(batch).Select("id").Find(&jobs).Error; err != nil {
			return err
		}
		if len(jobs) == 0 {
			return nil
		}
		ids := make([]int64, 0, len(jobs))
		for _, job := range jobs {
			ids = append(ids, job.Id)
		}
		result := tx.Where("id IN ? AND status IN ? AND created_at > 0 AND created_at <= ?", ids, statuses, before).
			Delete(&PromptAuditJob{})
		if result.Error != nil {
			return result.Error
		}
		deleted = result.RowsAffected
		if deleted == 0 {
			return nil
		}
		return tx.Model(&PromptAuditQueueState{}).Where("id = ?", PromptAuditConfigID).
			Updates(map[string]interface{}{
				"active_count": gorm.Expr("CASE WHEN active_count >= ? THEN active_count - ? ELSE 0 END", deleted, deleted),
				"version":      gorm.Expr("version + ?", 1),
			}).Error
	})
	return deleted, err
}
