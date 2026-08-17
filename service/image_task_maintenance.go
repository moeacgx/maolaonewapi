package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

const (
	SystemTaskTypeImageTaskMaintenance = "image_task_maintenance"
	imageTaskMaintenanceBatchSize      = 50
	imageTaskMaintenanceInterval       = time.Minute
)

type ImageTaskMaintenanceSummary struct {
	Reconciled                   int64 `json:"reconciled"`
	Cleared                      int64 `json:"cleared"`
	PendingRefundReconciliations int64 `json:"pending_refund_reconciliations"`
	ManualRefundReconciliations  int64 `json:"manual_refund_reconciliations"`
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
	if task == nil {
		return
	}
	_ = RefundTaskQuota(ctx, task, reason)
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
	manual, err := model.FindUnreportedImageTaskRefundManualReconciliations(ctx, now.Add(-imageTaskMaintenanceInterval).Unix(), imageTaskMaintenanceBatchSize)
	if err != nil {
		return summary, err
	}
	for i := range manual {
		marker := manual[i].PrivateData.RefundReconciliation
		if marker == nil {
			continue
		}
		logger.LogWarn(ctx, fmt.Sprintf(
			"MANUAL image task refund audit reconciliation required task=%s idempotency_key=%s attempted_at=%d: %s",
			manual[i].TaskID, marker.LogIdempotencyKey, marker.LogWriteAttemptedAt, marker.ManualReconciliationReason,
		))
		if err := model.MarkImageTaskRefundManualReconciliationReported(ctx, manual[i].ID); err != nil {
			return summary, err
		}
		summary.ManualRefundReconciliations++
	}

	pending, err := model.ClaimPendingImageTaskRefundReconciliations(ctx, now.Add(-imageTaskMaintenanceInterval).Unix(), imageTaskMaintenanceBatchSize)
	if err != nil {
		return summary, err
	}
	for i := range pending {
		if err := reconcileImageTaskRefund(ctx, pending[i].ID); err != nil {
			if errors.Is(err, model.ErrImageTaskRefundManualReconciliationRequired) {
				logger.LogWarn(ctx, "MANUAL image task refund audit reconciliation required task "+pending[i].TaskID+": "+err.Error())
				if markErr := model.MarkImageTaskRefundManualReconciliationReported(ctx, pending[i].ID); markErr != nil {
					return summary, markErr
				}
				summary.ManualRefundReconciliations++
			} else {
				summary.PendingRefundReconciliations++
				common.SysError("PENDING image task refund reconciliation task " + pending[i].TaskID + ": " + err.Error())
			}
		}
	}

	// Funding-refund retries are independent of timeout recovery: these rows
	// already reached a terminal failure and retain their task-specific reason.
	retries, err := model.ClaimFailedImageTaskRefundRetries(ctx, now.Add(-imageTaskMaintenanceInterval).Unix(), imageTaskMaintenanceBatchSize)
	if err != nil {
		return summary, err
	}
	for i := range retries {
		refundRecoveredImageTask(ctx, &retries[i], retries[i].FailReason)
	}

	if timeout := common.GetImageTaskTimeout(); timeout > 0 {
		cutoff := now.Add(-timeout).Unix()
		const reason = "image generation was interrupted before completion"
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
