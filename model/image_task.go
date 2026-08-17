package model

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"gorm.io/gorm"
)

var imageTaskActiveStatuses = []TaskStatus{
	TaskStatusNotStart,
	TaskStatusSubmitted,
	TaskStatusQueued,
	TaskStatusInProgress,
}

type ImageTaskBillingEvidence struct {
	Quota          int
	ChannelID      int
	TokenID        int
	BillingSource  string
	SubscriptionID int
	ModelName      string
	ModelPrice     float64
	ModelRatio     float64
	GroupRatio     float64
}

type imageTaskConsumeLogMetadata struct {
	BillingSource  string  `json:"billing_source"`
	SubscriptionID int     `json:"subscription_id"`
	ModelPrice     float64 `json:"model_price"`
	ModelRatio     float64 `json:"model_ratio"`
	GroupRatio     float64 `json:"group_ratio"`
}

// CountActiveImageTasksForAdmission returns both the owner's total active work
// and the subset submitted with the same token. Reading PrivateData in Go keeps
// this path portable across SQLite, MySQL, and PostgreSQL JSON encodings.
func CountActiveImageTasksForAdmission(ctx context.Context, userID int, tokenID int) (int, int, error) {
	if userID <= 0 {
		return 0, 0, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return countActiveImageTasksForAdmission(DB.WithContext(ctx), userID, tokenID)
}

func countActiveImageTasksForAdmission(db *gorm.DB, userID int, tokenID int) (int, int, error) {
	var tasks []Task
	if err := db.
		Select("private_data").
		Where("user_id = ? AND platform IN ? AND status IN ?", userID, constant.ImageTaskPlatforms(), imageTaskActiveStatuses).
		Find(&tasks).Error; err != nil {
		return 0, 0, err
	}
	matchingToken := 0
	if tokenID > 0 {
		for i := range tasks {
			if tasks[i].PrivateData.TokenId == tokenID {
				matchingToken++
			}
		}
	}
	return len(tasks), matchingToken, nil
}

// BeginImageTaskAdmission serializes count+insert across nodes by locking the
// authenticated user's database row. The caller must insert and commit through
// the returned transaction, or roll it back on every rejected request.
func BeginImageTaskAdmission(ctx context.Context, userID int, tokenID int, userLimit int, tokenLimit int) (*gorm.DB, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	tx := DB.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, false, tx.Error
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback().Error
		}
	}()
	var user User
	if err := lockForUpdate(tx).Select("id").Where("id = ?", userID).First(&user).Error; err != nil {
		return nil, false, err
	}
	userActive, tokenActive, err := countActiveImageTasksForAdmission(tx, userID, tokenID)
	if err != nil {
		return nil, false, err
	}
	if userActive >= userLimit || (tokenID > 0 && tokenActive >= tokenLimit) {
		return nil, false, nil
	}
	rollback = false
	return tx, true, nil
}

// FindImageTaskBillingEvidence accepts only one exact consume log for the
// public task request ID. Missing or ambiguous logs are intentionally not
// actionable: recovery must never guess which charge to refund.
func FindImageTaskBillingEvidence(ctx context.Context, task *Task) (*ImageTaskBillingEvidence, bool, error) {
	if task == nil || task.ID <= 0 || task.UserId <= 0 || task.TaskID == "" || LOG_DB == nil {
		return nil, false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var logs []Log
	logQuery := LOG_DB.WithContext(ctx).
		Where("request_id = ? AND user_id = ? AND type = ? AND quota > 0", task.TaskID, task.UserId, LogTypeConsume)
	if task.SubmitTime > 0 {
		logQuery = logQuery.Where("created_at >= ?", task.SubmitTime)
	}
	if err := logQuery.Limit(2).Find(&logs).Error; err != nil {
		return nil, false, err
	}
	if len(logs) != 1 || (task.PrivateData.TokenId > 0 && logs[0].TokenId != task.PrivateData.TokenId) {
		return nil, false, nil
	}
	logRow := &logs[0]
	metadata := imageTaskConsumeLogMetadata{}
	if logRow.Other != "" {
		_ = common.Unmarshal([]byte(logRow.Other), &metadata)
	}
	evidence := &ImageTaskBillingEvidence{
		Quota: logRow.Quota, ChannelID: logRow.ChannelId, TokenID: logRow.TokenId,
		BillingSource: metadata.BillingSource, SubscriptionID: metadata.SubscriptionID,
		ModelName: logRow.ModelName, ModelPrice: metadata.ModelPrice,
		ModelRatio: metadata.ModelRatio, GroupRatio: metadata.GroupRatio,
	}
	var subscription SubscriptionPreConsumeRecord
	query := DB.WithContext(ctx).
		Where("request_id = ? AND user_id = ?", task.TaskID, task.UserId).
		Limit(1).
		Find(&subscription)
	if query.Error != nil && !errors.Is(query.Error, gorm.ErrRecordNotFound) {
		return nil, false, query.Error
	}
	if query.RowsAffected > 0 {
		if subscription.Status != "consumed" || subscription.UserSubscriptionId <= 0 {
			return nil, false, nil
		}
		evidence.BillingSource = "subscription"
		evidence.SubscriptionID = subscription.UserSubscriptionId
	} else {
		if metadata.BillingSource == "subscription" {
			return nil, false, nil
		}
		evidence.BillingSource = "wallet"
		evidence.SubscriptionID = 0
	}
	return evidence, true, nil
}
func applyImageTaskBillingEvidence(task *Task, evidence *ImageTaskBillingEvidence) {
	task.Quota = evidence.Quota
	task.ChannelId = evidence.ChannelID
	task.PrivateData.BillingSource = evidence.BillingSource
	task.PrivateData.SubscriptionId = evidence.SubscriptionID
	task.PrivateData.TokenId = evidence.TokenID
	task.PrivateData.NodeName = common.NodeName
	task.PrivateData.BillingContext = &TaskBillingContext{
		ModelPrice: evidence.ModelPrice, GroupRatio: evidence.GroupRatio,
		ModelRatio: evidence.ModelRatio, OriginModelName: evidence.ModelName,
	}
	if task.Properties.OriginModelName == "" {
		task.Properties.OriginModelName = evidence.ModelName
	}
}

func MarkImageTaskSubscriptionRefunded(ctx context.Context, requestID string) error {
	if requestID == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return DB.WithContext(ctx).Model(&SubscriptionPreConsumeRecord{}).
		Where("request_id = ? AND status = ?", requestID, "consumed").
		Update("status", "refunded").Error
}

// PersistImageTaskBillingFromConsumeLog closes the ordinary success-path crash
// window. Maintenance performs the same exact correlation if the worker dies
// before reaching this call.
func PersistImageTaskBillingFromConsumeLog(ctx context.Context, task *Task) (bool, error) {
	evidence, found, err := FindImageTaskBillingEvidence(ctx, task)
	if err != nil || !found {
		return false, err
	}
	applyImageTaskBillingEvidence(task, evidence)
	result := DB.WithContext(ctx).Model(&Task{}).
		Where("id = ? AND platform IN ? AND status IN ?", task.ID, constant.ImageTaskPlatforms(), imageTaskActiveStatuses).
		Updates(map[string]any{
			"quota": task.Quota, "channel_id": task.ChannelId,
			"private_data": task.PrivateData, "properties": task.Properties,
		})
	return result.RowsAffected > 0, result.Error
}

// ClaimStaleImageTasksForBilling fails only tasks backed by one exact consume
// log and returns the CAS winners to the service layer for RefundTaskQuota.
func ClaimStaleImageTasksForBilling(ctx context.Context, cutoffUnix int64, limit int, publicReason string) ([]Task, error) {
	if cutoffUnix <= 0 || limit <= 0 {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if publicReason == "" {
		publicReason = "image generation was interrupted before completion"
	}

	claimed := make([]Task, 0, limit)
	var cursorSubmit int64
	var cursorID int64
	hasCursor := false
	for len(claimed) < limit {
		query := DB.WithContext(ctx).
			Where("platform IN ? AND status IN ? AND submit_time > 0 AND submit_time <= ?", constant.ImageTaskPlatforms(), imageTaskActiveStatuses, cutoffUnix)
		if hasCursor {
			query = query.Where("submit_time > ? OR (submit_time = ? AND id > ?)", cursorSubmit, cursorSubmit, cursorID)
		}
		var tasks []Task
		if err := query.Order("submit_time, id").Limit(limit).Find(&tasks).Error; err != nil {
			return claimed, err
		}
		if len(tasks) == 0 {
			break
		}
		for i := range tasks {
			if err := ctx.Err(); err != nil {
				return claimed, err
			}
			task := &tasks[i]
			cursorSubmit, cursorID, hasCursor = task.SubmitTime, task.ID, true
			evidence, found, err := FindImageTaskBillingEvidence(ctx, task)
			if err != nil {
				return claimed, err
			}
			if !found {
				continue
			}
			fromStatus := task.Status
			applyImageTaskBillingEvidence(task, evidence)
			now := time.Now().Unix()
			task.Status, task.Progress = TaskStatusFailure, "100%"
			task.FinishTime, task.UpdatedAt = now, now
			task.FailReason, task.Data = publicReason, nil
			result := DB.WithContext(ctx).Model(&Task{}).
				Where("id = ? AND platform IN ? AND status = ?", task.ID, constant.ImageTaskPlatforms(), fromStatus).
				Select("status", "progress", "finish_time", "updated_at", "fail_reason", "data", "quota", "channel_id", "private_data", "properties").
				Updates(task)
			if result.Error != nil {
				return claimed, result.Error
			}
			if result.RowsAffected > 0 {
				claimed = append(claimed, *task)
				if len(claimed) == limit {
					break
				}
			}
		}
		if len(tasks) < limit {
			break
		}
	}
	return claimed, nil
}

// ClaimFailedImageTaskRefundRetries serializes retries using updated_at as a
// portable CAS version. Rows disappear from this set once RefundTaskQuota
// clears quota, making repeated maintenance runs idempotent.
func ClaimFailedImageTaskRefundRetries(ctx context.Context, cutoffUnix int64, limit int, publicReason string) ([]Task, error) {
	if cutoffUnix <= 0 || limit <= 0 {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var tasks []Task
	if err := DB.WithContext(ctx).
		Where("platform IN ? AND status = ? AND quota > 0 AND fail_reason = ? AND updated_at <= ?", constant.ImageTaskPlatforms(), TaskStatusFailure, publicReason, cutoffUnix).
		Order("updated_at, id").Limit(limit).Find(&tasks).Error; err != nil {
		return nil, err
	}
	claimed := make([]Task, 0, len(tasks))
	for i := range tasks {
		task := &tasks[i]
		nextVersion := time.Now().Unix()
		if nextVersion <= task.UpdatedAt {
			nextVersion = task.UpdatedAt + 1
		}
		result := DB.WithContext(ctx).Model(&Task{}).
			Where("id = ? AND status = ? AND quota > 0 AND updated_at = ?", task.ID, TaskStatusFailure, task.UpdatedAt).
			Update("updated_at", nextVersion)
		if result.Error != nil {
			return claimed, result.Error
		}
		if result.RowsAffected > 0 {
			task.UpdatedAt = nextVersion
			claimed = append(claimed, *task)
		}
	}
	return claimed, nil
}

// GetImageTaskByOwnerPlatform resolves a task only when all three security
// dimensions match. Callers intentionally map every mismatch to not found.
func GetImageTaskByOwnerPlatform(userID int, taskID string, platform constant.TaskPlatform) (*Task, bool, error) {
	if userID <= 0 || taskID == "" || !constant.IsImageTaskPlatform(platform) {
		return nil, false, nil
	}
	var task Task
	err := DB.Where("user_id = ? AND task_id = ? AND platform = ?", userID, taskID, platform).
		First(&task).Error
	exists, err := RecordExist(err)
	if err != nil || !exists {
		return nil, exists, err
	}
	return &task, true, nil
}

// ClearExpiredImageTaskData clears only payload bytes. Task ownership, status,
// billing, timestamps, and audit metadata remain intact. Selecting IDs before
// updating avoids database-specific UPDATE LIMIT behavior.
func ClearExpiredImageTaskData(ctx context.Context, cutoffUnix int64, limit int) (int64, error) {
	if cutoffUnix <= 0 || limit <= 0 {
		return 0, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var ids []int64
	err := DB.WithContext(ctx).Model(&Task{}).
		Where("platform IN ?", constant.ImageTaskPlatforms()).
		Where("status IN ?", []TaskStatus{TaskStatusSuccess, TaskStatusFailure}).
		Where("finish_time > 0 AND finish_time <= ?", cutoffUnix).
		Where("data IS NOT NULL").
		Order("finish_time DESC, id DESC").
		Limit(limit).
		Pluck("id", &ids).Error
	if err != nil || len(ids) == 0 {
		return 0, err
	}

	result := DB.WithContext(ctx).Model(&Task{}).
		Where("id IN ?", ids).
		Where("platform IN ?", constant.ImageTaskPlatforms()).
		Where("status IN ?", []TaskStatus{TaskStatusSuccess, TaskStatusFailure}).
		Where("finish_time > 0 AND finish_time <= ?", cutoffUnix).
		Where("data IS NOT NULL").
		UpdateColumn("data", nil)
	return result.RowsAffected, result.Error
}

// ReconcileStaleImageTasks deterministically closes local tasks whose owning
// process disappeared. Every row uses a status CAS so a concurrent successful
// relay result always wins or loses atomically.
func ReconcileStaleImageTasks(ctx context.Context, cutoffUnix int64, limit int, publicReason string) (int64, error) {
	if cutoffUnix <= 0 || limit <= 0 {
		return 0, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if publicReason == "" {
		publicReason = "image generation was interrupted before completion"
	}

	var tasks []Task
	if err := DB.WithContext(ctx).
		Where("platform IN ?", constant.ImageTaskPlatforms()).
		Where("status IN ?", imageTaskActiveStatuses).
		Where("submit_time > 0 AND submit_time <= ?", cutoffUnix).
		Order("submit_time, id").
		Limit(limit).
		Find(&tasks).Error; err != nil {
		return 0, err
	}

	now := time.Now().Unix()
	var reconciled int64
	for i := range tasks {
		if err := ctx.Err(); err != nil {
			return reconciled, err
		}
		task := &tasks[i]
		fromStatus := task.Status
		task.Status = TaskStatusFailure
		task.Progress = "100%"
		task.FinishTime = now
		task.UpdatedAt = now
		task.FailReason = publicReason
		task.Data = nil
		result := DB.WithContext(ctx).Model(&Task{}).
			Where("id = ? AND platform IN ? AND status = ?", task.ID, constant.ImageTaskPlatforms(), fromStatus).
			Select("status", "progress", "finish_time", "updated_at", "fail_reason", "data").
			Updates(task)
		if result.Error != nil {
			return reconciled, result.Error
		}
		reconciled += result.RowsAffected
	}
	return reconciled, nil
}

func HasImageTaskMaintenanceWork(nowUnix int64, timeoutSeconds int64, retentionSeconds int64) bool {
	if nowUnix <= 0 {
		return false
	}
	if timeoutSeconds > 0 {
		var id int64
		err := DB.Model(&Task{}).
			Where("platform IN ?", constant.ImageTaskPlatforms()).
			Where("status IN ?", imageTaskActiveStatuses).
			Where("submit_time > 0 AND submit_time <= ?", nowUnix-timeoutSeconds).
			Limit(1).
			Pluck("id", &id).Error
		if err == nil && id != 0 {
			return true
		}
	}
	if retentionSeconds > 0 {
		var id int64
		err := DB.Model(&Task{}).
			Where("platform IN ?", constant.ImageTaskPlatforms()).
			Where("status IN ?", []TaskStatus{TaskStatusSuccess, TaskStatusFailure}).
			Where("finish_time > 0 AND finish_time <= ?", nowUnix-retentionSeconds).
			Where("data IS NOT NULL").
			Limit(1).
			Pluck("id", &id).Error
		return err == nil && id != 0
	}
	return false
}
