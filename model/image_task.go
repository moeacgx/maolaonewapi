package model

import (
	"context"
	"errors"
	"fmt"
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

// RefundImageTaskMoney atomically credits the image task's original funding
// source, clears its positive quota claim, and persists the post-commit
// reconciliation marker. A zero quota is an already-completed money refund,
// never a new claim.
func RefundImageTaskMoney(ctx context.Context, taskID int64, expectedQuota int, reason string) (*Task, bool, error) {
	if taskID <= 0 || expectedQuota <= 0 {
		return nil, false, fmt.Errorf("invalid image task refund claim")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var locked Task
	claimed := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).
			Where("id = ? AND platform IN ?", taskID, constant.ImageTaskPlatforms()).
			First(&locked).Error; err != nil {
			return err
		}
		if locked.Quota == 0 {
			return nil
		}
		if locked.Quota < 0 || locked.Quota != expectedQuota {
			return fmt.Errorf("image task %s refund quota changed: expected=%d actual=%d", locked.TaskID, expectedQuota, locked.Quota)
		}
		if locked.PrivateData.RefundReconciliation != nil {
			return fmt.Errorf("image task %s has refund reconciliation with positive quota", locked.TaskID)
		}
		walletQuotaVersion := int64(0)
		walletQuota := int64(0)
		if locked.PrivateData.BillingSource == "subscription" {
			if locked.PrivateData.SubscriptionId <= 0 {
				return fmt.Errorf("image task %s has invalid subscription funding", locked.TaskID)
			}
			if err := PostConsumeUserSubscriptionDeltaWithTx(tx, locked.PrivateData.SubscriptionId, -int64(locked.Quota)); err != nil {
				return err
			}
		} else {
			var err error
			walletQuotaVersion, walletQuota, err = IncreaseUserQuotaWithTx(tx, locked.UserId, locked.Quota)
			if err != nil {
				return err
			}
		}
		modelName := locked.Properties.OriginModelName
		if locked.PrivateData.BillingContext != nil && locked.PrivateData.BillingContext.OriginModelName != "" {
			modelName = locked.PrivateData.BillingContext.OriginModelName
		}
		locked.PrivateData.RefundReconciliation = &TaskRefundReconciliation{
			Amount: locked.Quota, Reason: reason, UserId: locked.UserId,
			ChannelId: locked.ChannelId, TokenId: locked.PrivateData.TokenId,
			BillingSource:  locked.PrivateData.BillingSource,
			SubscriptionId: locked.PrivateData.SubscriptionId, Group: locked.Group,
			ModelName: modelName, NodeName: locked.PrivateData.NodeName,
			BillingContext:     locked.PrivateData.BillingContext,
			OriginModelName:    locked.Properties.OriginModelName,
			UpstreamModelName:  locked.Properties.UpstreamModelName,
			WalletQuotaVersion: walletQuotaVersion,
			WalletQuota:        walletQuota,
			CacheRepairDone:    locked.PrivateData.BillingSource == "subscription",
		}
		locked.Quota = 0
		locked.UpdatedAt = time.Now().Unix()
		locked.RefundReconciliationState = TaskRefundReconciliationStatePending
		result := tx.Model(&Task{}).Where("id = ? AND quota = ?", locked.ID, expectedQuota).
			Updates(map[string]any{
				"quota": 0, "updated_at": locked.UpdatedAt,
				"private_data": locked.PrivateData, "refund_reconciliation_state": locked.RefundReconciliationState,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("image task %s refund claim was lost", locked.TaskID)
		}
		claimed = true
		return nil
	})
	return &locked, claimed, err
}

// ReconcileImageTaskRefundAccounting performs token compensation, usage
// counter reversal, and subscription-record reconciliation in a second atomic
// transaction. Its durable progress bit is written only with those effects.
func ReconcileImageTaskRefundAccounting(ctx context.Context, taskID int64) (*Task, bool, error) {
	if taskID <= 0 {
		return nil, false, fmt.Errorf("invalid image task reconciliation")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var locked Task
	completed := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("id = ? AND platform IN ?", taskID, constant.ImageTaskPlatforms()).First(&locked).Error; err != nil {
			return err
		}
		marker := locked.PrivateData.RefundReconciliation
		if marker == nil || marker.AccountingDone {
			return nil
		}
		if locked.Quota != 0 || marker.Amount <= 0 {
			return fmt.Errorf("image task %s has invalid refund reconciliation state", locked.TaskID)
		}
		if marker.TokenId > 0 {
			if err := IncreaseTokenQuotaWithTx(tx, marker.TokenId, marker.Amount); err != nil && !errors.Is(err, ErrTokenNotFound) {
				return err
			}
		}
		if err := UpdateUserUsedQuotaWithTx(tx, marker.UserId, -marker.Amount); err != nil {
			return err
		}
		if marker.ChannelId > 0 {
			if err := UpdateChannelUsedQuotaWithTx(tx, marker.ChannelId, -marker.Amount); err != nil {
				return err
			}
		}
		if marker.BillingSource == "subscription" {
			if err := markImageTaskSubscriptionRefundedWithTx(tx, locked.TaskID); err != nil {
				return err
			}
		}
		marker.AccountingDone = true
		locked.PrivateData.RefundReconciliation = marker
		locked.RefundReconciliationState = TaskRefundReconciliationStatePending
		if err := tx.Model(&Task{}).Where("id = ? AND quota = 0", locked.ID).
			Updates(map[string]any{
				"private_data":                locked.PrivateData,
				"refund_reconciliation_state": locked.RefundReconciliationState,
			}).Error; err != nil {
			return err
		}
		completed = true
		return nil
	})
	return &locked, completed, err
}

// MarkImageTaskRefundCacheRepaired durably records that the idempotent wallet
// cache generation was published. A crash before this bit commits safely
// repeats the same authoritative repair without re-crediting money.
func MarkImageTaskRefundCacheRepaired(ctx context.Context, taskID int64) (*Task, error) {
	if taskID <= 0 {
		return nil, fmt.Errorf("invalid image task cache repair")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var locked Task
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("id = ? AND platform IN ?", taskID, constant.ImageTaskPlatforms()).First(&locked).Error; err != nil {
			return err
		}
		marker := locked.PrivateData.RefundReconciliation
		if marker == nil || marker.CacheRepairDone {
			return nil
		}
		if locked.Quota != 0 || !marker.AccountingDone || marker.BillingSource == "subscription" || marker.WalletQuotaVersion <= 0 {
			return fmt.Errorf("image task %s has invalid cache repair state", locked.TaskID)
		}
		marker.CacheRepairDone = true
		locked.PrivateData.RefundReconciliation = marker
		locked.RefundReconciliationState = TaskRefundReconciliationStatePending
		return tx.Model(&Task{}).Where("id = ? AND quota = 0", locked.ID).
			Updates(map[string]any{
				"private_data":                locked.PrivateData,
				"refund_reconciliation_state": locked.RefundReconciliationState,
			}).Error
	})
	return &locked, err
}

func markImageTaskSubscriptionRefundedWithTx(tx *gorm.DB, requestID string) error {
	if tx == nil || requestID == "" {
		return nil
	}
	result := tx.Model(&SubscriptionPreConsumeRecord{}).
		Where("request_id = ? AND status = ?", requestID, "consumed").
		Update("status", "refunded")
	if result.Error != nil || result.RowsAffected > 0 {
		return result.Error
	}
	var status string
	result = tx.Model(&SubscriptionPreConsumeRecord{}).Where("request_id = ?", requestID).
		Select("status").Limit(1).Scan(&status)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 || status == "refunded" {
		return nil
	}
	return fmt.Errorf("subscription refund record for task %s is not consumed", requestID)
}

const (
	imageTaskRefundLogClaimLease = 2 * time.Minute
	clickHouseRefundManualReason = "ClickHouse refund audit log write outcome requires manual reconciliation"
)

var ErrImageTaskRefundManualReconciliationRequired = errors.New("image task refund audit log requires manual reconciliation")

func validateImageTaskRefundFinalization(task *Task, requestID string) (*TaskRefundReconciliation, error) {
	if task == nil || task.TaskID != requestID {
		return nil, fmt.Errorf("image task refund request does not match")
	}
	marker := task.PrivateData.RefundReconciliation
	if marker == nil {
		return nil, nil
	}
	if task.Quota != 0 || !marker.AccountingDone || !marker.CacheRepairDone {
		return nil, fmt.Errorf("image task %s refund reconciliation is incomplete", task.TaskID)
	}
	return marker, nil
}

// claimImageTaskRefundLog durably elects one sink writer. Relational sinks use
// an expiring claim because their unique key makes retry safe. ClickHouse uses
// a non-expiring attempted-write fence: once persisted, no automatic worker may
// issue another insert whose earlier outcome could be ambiguous.
func claimImageTaskRefundLog(ctx context.Context, taskID int64, requestID string) (*Task, string, bool, error) {
	var task Task
	claimToken := ""
	claimed := false
	clickHouse := LOG_DB != nil && LOG_DB.Dialector != nil && LOG_DB.Dialector.Name() == "clickhouse"
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("id = ? AND platform IN ?", taskID, constant.ImageTaskPlatforms()).First(&task).Error; err != nil {
			return err
		}
		marker, err := validateImageTaskRefundFinalization(&task, requestID)
		if err != nil || marker == nil {
			return err
		}
		if marker.LogWriteAttempted || marker.ManualReconciliationRequired {
			return nil
		}
		now := time.Now().Unix()
		if !clickHouse && marker.LogClaimToken != "" && marker.LogClaimUntil > now {
			return nil
		}
		claimToken = common.GetUUID()
		marker.LogClaimToken = claimToken
		if clickHouse {
			marker.LogClaimUntil = 0
			marker.LogWriteAttempted = true
			marker.LogWriteAttemptedAt = now
			marker.LogIdempotencyKey = "task-refund:" + requestID
			marker.ManualReconciliationRequired = true
			marker.ManualReconciliationReason = clickHouseRefundManualReason
			marker.ManualReconciliationReported = false
			task.RefundReconciliationState = TaskRefundReconciliationStateManualUnreported
		} else {
			marker.LogClaimUntil = now + int64(imageTaskRefundLogClaimLease/time.Second)
			task.RefundReconciliationState = TaskRefundReconciliationStatePending
		}
		task.PrivateData.RefundReconciliation = marker
		result := tx.Model(&Task{}).Where("id = ? AND quota = 0", task.ID).
			Updates(map[string]any{
				"private_data":                task.PrivateData,
				"refund_reconciliation_state": task.RefundReconciliationState,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("image task %s refund log claim was not persisted", task.TaskID)
		}
		claimed = true
		return nil
	})
	return &task, claimToken, claimed, err
}

func releaseImageTaskRefundLogClaim(ctx context.Context, taskID int64, claimToken string) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task Task
		if err := lockForUpdate(tx).Where("id = ? AND platform IN ?", taskID, constant.ImageTaskPlatforms()).First(&task).Error; err != nil {
			return err
		}
		marker := task.PrivateData.RefundReconciliation
		if marker == nil || marker.LogClaimToken != claimToken {
			return nil
		}
		if marker.LogWriteAttempted || marker.ManualReconciliationRequired {
			return nil
		}
		marker.LogClaimToken = ""
		marker.LogClaimUntil = 0
		task.PrivateData.RefundReconciliation = marker
		task.RefundReconciliationState = TaskRefundReconciliationStatePending
		return tx.Model(&Task{}).Where("id = ? AND quota = 0", task.ID).
			Updates(map[string]any{
				"private_data":                task.PrivateData,
				"refund_reconciliation_state": task.RefundReconciliationState,
			}).Error
	})
}

func completeImageTaskRefundLogClaim(ctx context.Context, taskID int64, requestID string, claimToken string) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task Task
		if err := lockForUpdate(tx).Where("id = ? AND platform IN ?", taskID, constant.ImageTaskPlatforms()).First(&task).Error; err != nil {
			return err
		}
		marker, err := validateImageTaskRefundFinalization(&task, requestID)
		if err != nil || marker == nil {
			return err
		}
		if marker.LogClaimToken != claimToken {
			return fmt.Errorf("image task %s refund log claim changed", task.TaskID)
		}
		task.PrivateData.RefundReconciliation = nil
		task.RefundReconciliationState = TaskRefundReconciliationStateNone
		result := tx.Model(&Task{}).Where("id = ? AND quota = 0", task.ID).
			Updates(map[string]any{
				"private_data":                task.PrivateData,
				"refund_reconciliation_state": task.RefundReconciliationState,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("image task %s refund reconciliation was not cleared", task.TaskID)
		}
		return nil
	})
}

// FinalizeImageTaskRefundReconciliation writes the audit row after obtaining a
// durable main-database fence. Relational sinks may safely retry behind their
// unique key. ClickHouse gets exactly one plain insert attempt; any crash or
// returned error leaves the pre-insert manual-reconciliation marker intact.
func FinalizeImageTaskRefundReconciliation(ctx context.Context, taskID int64, params RecordTaskBillingLogParams) error {
	if taskID <= 0 || params.RequestId == "" {
		return fmt.Errorf("invalid image task refund finalization")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	clickHouse := LOG_DB != nil && LOG_DB.Dialector != nil && LOG_DB.Dialector.Name() == "clickhouse"
	_, claimToken, claimed, err := claimImageTaskRefundLog(ctx, taskID, params.RequestId)
	if err != nil || !claimed {
		return err
	}
	log := buildTaskBillingLog(params)
	if err := recordTaskBillingLogOnceWithDB(LOG_DB, log); err != nil {
		if clickHouse {
			return fmt.Errorf("%w: %w", ErrImageTaskRefundManualReconciliationRequired, err)
		}
		_ = releaseImageTaskRefundLogClaim(ctx, taskID, claimToken)
		return err
	}
	err = completeImageTaskRefundLogClaim(ctx, taskID, params.RequestId, claimToken)
	if clickHouse && err != nil {
		return fmt.Errorf("%w: %w", ErrImageTaskRefundManualReconciliationRequired, err)
	}
	return err
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
func ClaimFailedImageTaskRefundRetries(ctx context.Context, cutoffUnix int64, limit int) ([]Task, error) {
	if cutoffUnix <= 0 || limit <= 0 {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var tasks []Task
	if err := DB.WithContext(ctx).
		Where("platform IN ? AND status = ? AND quota > 0 AND updated_at <= ?", constant.ImageTaskPlatforms(), TaskStatusFailure, cutoffUnix).
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

func imageTaskRefundReconciliationsQuery(db *gorm.DB, state TaskRefundReconciliationState, cutoffUnix int64, limit int) *gorm.DB {
	return db.
		Where("refund_reconciliation_state = ? AND status = ? AND platform IN ? AND quota = 0 AND updated_at <= ?", state, TaskStatusFailure, constant.ImageTaskPlatforms(), cutoffUnix).
		Order("updated_at, id").
		Limit(limit)
}

// ClaimPendingImageTaskRefundReconciliations returns money-refunded rows whose
// durable post-commit state remains eligible. The persisted state keeps the
// selection bounded and portable without database-specific JSON predicates.
func ClaimPendingImageTaskRefundReconciliations(ctx context.Context, cutoffUnix int64, limit int) ([]Task, error) {
	if cutoffUnix <= 0 || limit <= 0 {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var pending []Task
	err := imageTaskRefundReconciliationsQuery(
		DB.WithContext(ctx), TaskRefundReconciliationStatePending, cutoffUnix, limit,
	).Find(&pending).Error
	return pending, err
}

// FindUnreportedImageTaskRefundManualReconciliations returns durable
// ClickHouse ambiguity signals for operator warning only. These rows are never
// returned by ClaimPendingImageTaskRefundReconciliations and must not be
// automatically inserted again.
func FindUnreportedImageTaskRefundManualReconciliations(ctx context.Context, cutoffUnix int64, limit int) ([]Task, error) {
	if cutoffUnix <= 0 || limit <= 0 {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var manual []Task
	err := imageTaskRefundReconciliationsQuery(
		DB.WithContext(ctx), TaskRefundReconciliationStateManualUnreported, cutoffUnix, limit,
	).Find(&manual).Error
	return manual, err
}

// MarkImageTaskRefundManualReconciliationReported acknowledges only the
// operator warning. It deliberately retains the attempted-write fence and all
// task/idempotency context until a human reconciles the audit row.
func MarkImageTaskRefundManualReconciliationReported(ctx context.Context, taskID int64) error {
	if taskID <= 0 {
		return fmt.Errorf("invalid image task refund manual reconciliation")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task Task
		if err := lockForUpdate(tx).Where("id = ? AND platform IN ?", taskID, constant.ImageTaskPlatforms()).First(&task).Error; err != nil {
			return err
		}
		marker := task.PrivateData.RefundReconciliation
		if marker == nil || (!marker.LogWriteAttempted && !marker.ManualReconciliationRequired) || marker.ManualReconciliationReported {
			return nil
		}
		marker.ManualReconciliationRequired = true
		marker.ManualReconciliationReported = true
		task.PrivateData.RefundReconciliation = marker
		task.RefundReconciliationState = TaskRefundReconciliationStateManualReported
		result := tx.Model(&Task{}).Where("id = ? AND quota = 0", task.ID).
			Updates(map[string]any{
				"private_data":                task.PrivateData,
				"refund_reconciliation_state": task.RefundReconciliationState,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("image task %s manual reconciliation warning was not persisted", task.TaskID)
		}
		return nil
	})
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

func imageTaskMaintenanceWorkQuery(db *gorm.DB, nowUnix int64, timeoutSeconds int64, retentionSeconds int64) *gorm.DB {
	eligibility := db.Session(&gorm.Session{NewDB: true}).
		Where(
			"status = ? AND updated_at <= ? AND (quota > 0 OR (quota = 0 AND refund_reconciliation_state IN ?))",
			TaskStatusFailure,
			nowUnix,
			[]TaskRefundReconciliationState{
				TaskRefundReconciliationStatePending,
				TaskRefundReconciliationStateManualUnreported,
			},
		)
	if timeoutSeconds > 0 {
		eligibility = eligibility.Or(
			"status IN ? AND submit_time > 0 AND submit_time <= ?",
			imageTaskActiveStatuses,
			nowUnix-timeoutSeconds,
		)
	}
	if retentionSeconds > 0 {
		eligibility = eligibility.Or(
			"status IN ? AND finish_time > 0 AND finish_time <= ? AND data IS NOT NULL",
			[]TaskStatus{TaskStatusSuccess, TaskStatusFailure},
			nowUnix-retentionSeconds,
		)
	}
	return db.Session(&gorm.Session{NewDB: true}).
		Model(&Task{}).
		Where("platform IN ?", constant.ImageTaskPlatforms()).
		Where(eligibility).
		Limit(1)
}

func HasImageTaskMaintenanceWork(nowUnix int64, timeoutSeconds int64, retentionSeconds int64) bool {
	if nowUnix <= 0 || DB == nil {
		return false
	}
	var id int64
	err := imageTaskMaintenanceWorkQuery(DB, nowUnix, timeoutSeconds, retentionSeconds).
		Pluck("id", &id).Error
	return err == nil && id != 0
}
