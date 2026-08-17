package service

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	SystemTaskTypeImageTaskMaintenance = "image_task_maintenance"
	imageTaskMaintenanceBatchSize      = 50
	imageTaskMaintenanceInterval       = time.Minute
)

type ImageTaskMaintenanceSummary struct {
	Reconciled int64 `json:"reconciled"`
	Cleared    int64 `json:"cleared"`
}

type imageTaskMaintenanceHandler struct{}

func (imageTaskMaintenanceHandler) Type() string { return SystemTaskTypeImageTaskMaintenance }

func (imageTaskMaintenanceHandler) Enabled() bool {
	if model.DB == nil {
		return false
	}
	now := time.Now().Unix()
	timeout := common.GetImageTaskTimeout()
	retention := time.Duration(common.GetImageTaskDataRetentionHours()) * time.Hour
	return model.HasImageTaskMaintenanceWork(now, int64(timeout/time.Second), int64(retention/time.Second))
}

func (imageTaskMaintenanceHandler) Interval() time.Duration { return imageTaskMaintenanceInterval }
func (imageTaskMaintenanceHandler) NewPayload() any         { return nil }

func (imageTaskMaintenanceHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary, err := RunImageTaskMaintenance(ctx, time.Now())
	if err != nil {
		common.SysError("image task maintenance failed: " + err.Error())
		_ = model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusFailed, nil, "image task maintenance failed")
		return
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, summary, ""); err != nil {
		common.SysError("image task maintenance result could not be saved: " + err.Error())
	}
}

// RegisterImageTaskMaintenanceSystemTask wires cleanup and crash reconciliation
// into the existing leased system-task runner. Call it before StartSystemTaskRunner.
func RegisterImageTaskMaintenanceSystemTask() {
	RegisterSystemTaskHandler(imageTaskMaintenanceHandler{})
}

func refundRecoveredImageTask(ctx context.Context, task *model.Task, reason string) {
	if task == nil || !RefundTaskQuota(ctx, task, reason) {
		return
	}
	if task.PrivateData.BillingSource == BillingSourceSubscription {
		if err := model.MarkImageTaskSubscriptionRefunded(ctx, task.TaskID); err != nil {
			common.SysError("image task subscription refund marker failed: " + err.Error())
		}
	}
}

// RunImageTaskMaintenance is synchronous and resumable. It can be invoked by a
// leased system task or directly by recovery tooling; each row update is CAS or
// idempotent and every loop checks cancellation.
func RunImageTaskMaintenance(ctx context.Context, now time.Time) (ImageTaskMaintenanceSummary, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if now.IsZero() {
		now = time.Now()
	}
	summary := ImageTaskMaintenanceSummary{}

	if timeout := common.GetImageTaskTimeout(); timeout > 0 {
		cutoff := now.Add(-timeout).Unix()
		const reason = "image generation was interrupted before completion"

		// Retry only rows already won by an earlier maintenance CAS whose
		// funding refund failed. quota=0 removes a completed refund from this set.
		retries, err := model.ClaimFailedImageTaskRefundRetries(ctx, now.Add(-imageTaskMaintenanceInterval).Unix(), imageTaskMaintenanceBatchSize, reason)
		if err != nil {
			return summary, err
		}
		for i := range retries {
			refundRecoveredImageTask(ctx, &retries[i], reason)
		}

		for {
			if err := ctx.Err(); err != nil {
				return summary, err
			}
			claimed, err := model.ClaimStaleImageTasksForBilling(ctx, cutoff, imageTaskMaintenanceBatchSize, reason)
			if err != nil {
				return summary, err
			}
			for i := range claimed {
				refundRecoveredImageTask(ctx, &claimed[i], reason)
			}
			summary.Reconciled += int64(len(claimed))
			if len(claimed) < imageTaskMaintenanceBatchSize {
				break
			}
		}
	}

	retentionHours := common.GetImageTaskDataRetentionHours()
	if retentionHours <= 0 {
		return summary, nil
	}
	cutoff := now.Add(-time.Duration(retentionHours) * time.Hour).Unix()
	for {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		count, err := model.ClearExpiredImageTaskData(ctx, cutoff, imageTaskMaintenanceBatchSize)
		if err != nil {
			return summary, err
		}
		summary.Cleared += count
		if count < imageTaskMaintenanceBatchSize {
			break
		}
	}
	return summary, nil
}

var _ ScheduledSystemTaskHandler = imageTaskMaintenanceHandler{}
