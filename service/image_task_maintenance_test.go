package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageTaskMaintenanceRetriesStaleTaskWithoutConsumeLogAndClearsExpiredData(t *testing.T) {
	truncate(t)
	t.Setenv("IMAGE_TASK_TIMEOUT_MINUTES", "30")
	previousRetention := common.GetImageTaskDataRetentionHours()
	common.SetImageTaskDataRetentionHours(1)
	t.Cleanup(func() { common.SetImageTaskDataRetentionHours(previousRetention) })
	now := time.Now()
	for _, task := range []*model.Task{
		{
			TaskID: "restart-pending", UserId: 1, Platform: constant.TaskPlatformCanvasImage,
			Status: model.TaskStatusInProgress, Progress: "10%", SubmitTime: now.Add(-time.Hour).Unix(),
		},
		{
			TaskID: "expired-payload", UserId: 1, Platform: constant.TaskPlatformImage,
			Status: model.TaskStatusSuccess, Progress: "100%", FinishTime: now.Add(-2 * time.Hour).Unix(), Data: []byte(`{"data":[1]}`),
		},
	} {
		require.NoError(t, task.Insert())
	}

	summary, err := RunImageTaskMaintenance(context.Background(), now)
	require.NoError(t, err)
	assert.EqualValues(t, 0, summary.Reconciled)
	assert.EqualValues(t, 1, summary.Cleared)

	var restarted model.Task
	require.NoError(t, model.DB.Where("task_id = ?", "restart-pending").First(&restarted).Error)
	assert.EqualValues(t, model.TaskStatusInProgress, restarted.Status)
	assert.Zero(t, restarted.Quota)
	var expired model.Task
	require.NoError(t, model.DB.Where("task_id = ?", "expired-payload").First(&expired).Error)
	assert.Empty(t, expired.Data)
}

func seedImageTaskConsumeLog(t *testing.T, taskID string, userID, tokenID, channelID, quota int) {
	t.Helper()
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		UserId: userID, CreatedAt: time.Now().Unix(), Type: model.LogTypeConsume, RequestId: taskID,
		TokenId: tokenID, ChannelId: channelID, Quota: quota,
		ModelName: "gpt-image-1", Group: "default",
		Other: `{"model_price":0.02,"model_ratio":1,"group_ratio":1}`,
	}).Error)
}

func seedImageTaskRecoveryUser(t *testing.T, id int, quota int) {
	t.Helper()
	user := &model.User{
		Id: id, Username: fmt.Sprintf("image-recovery-%d", id), AffCode: fmt.Sprintf("image-aff-%d", id),
		Quota: int64(quota), Status: common.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(user).Error)
}

func setupImageTaskBillingEvidenceTable(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(&model.SubscriptionPreConsumeRecord{}))
	require.NoError(t, model.DB.Exec("DELETE FROM subscription_pre_consume_records").Error)
}

func TestImageTaskMaintenanceRecoversWalletSubscriptionAndTokenChargesExactlyOnce(t *testing.T) {
	truncate(t)
	setupImageTaskBillingEvidenceTable(t)
	t.Setenv("IMAGE_TASK_TIMEOUT_MINUTES", "30")
	now := time.Now()
	const charged = 100

	seedImageTaskRecoveryUser(t, 101, 900)
	seedToken(t, 201, 101, "sk-image-recovery", 400)
	seedChannel(t, 301)
	seedChargedAccounting(t, 101, 301, 201, charged, 1)
	walletTask := &model.Task{
		TaskID: "image-wallet-token-recovery", UserId: 101, Platform: constant.TaskPlatformImage,
		Group: "default", Status: model.TaskStatusInProgress, Progress: "10%",
		SubmitTime: now.Add(-time.Hour).Unix(), PrivateData: model.TaskPrivateData{TokenId: 201},
	}
	require.NoError(t, walletTask.Insert())
	seedImageTaskConsumeLog(t, walletTask.TaskID, 101, 201, 301, charged)

	seedImageTaskRecoveryUser(t, 102, 1000)
	seedSubscription(t, 202, 102, 1000, charged)
	seedChannel(t, 302)
	seedChargedAccounting(t, 102, 302, 0, charged, 1)
	subscriptionTask := &model.Task{
		TaskID: "image-subscription-recovery", UserId: 102, Platform: constant.TaskPlatformCanvasImage,
		Group: "default", Status: model.TaskStatusInProgress, Progress: "10%",
		SubmitTime: now.Add(-time.Hour).Unix(),
	}
	require.NoError(t, subscriptionTask.Insert())
	seedImageTaskConsumeLog(t, subscriptionTask.TaskID, 102, 0, 302, charged)
	require.NoError(t, model.DB.Create(&model.SubscriptionPreConsumeRecord{
		RequestId: subscriptionTask.TaskID, UserId: 102, UserSubscriptionId: 202,
		PreConsumed: charged, Status: "consumed",
	}).Error)

	summary, err := RunImageTaskMaintenance(context.Background(), now)
	require.NoError(t, err)
	assert.EqualValues(t, 2, summary.Reconciled)
	assert.Equal(t, 1000, getUserQuota(t, 101))
	assert.Equal(t, 500, getTokenRemainQuota(t, 201))
	assert.EqualValues(t, 0, getSubscriptionUsed(t, 202))
	assert.Equal(t, 1000, getUserQuota(t, 102), "subscription recovery must not credit the wallet")

	for _, taskID := range []string{walletTask.TaskID, subscriptionTask.TaskID} {
		var recovered model.Task
		require.NoError(t, model.DB.Where("task_id = ?", taskID).First(&recovered).Error)
		assert.EqualValues(t, model.TaskStatusFailure, recovered.Status)
		assert.Zero(t, recovered.Quota)
		assert.NotEmpty(t, recovered.PrivateData.BillingSource)
		assert.NotNil(t, recovered.PrivateData.BillingContext)
	}
	var persistedSubscription model.Task
	require.NoError(t, model.DB.Where("task_id = ?", subscriptionTask.TaskID).First(&persistedSubscription).Error)
	assert.Equal(t, BillingSourceSubscription, persistedSubscription.PrivateData.BillingSource)
	assert.Equal(t, 202, persistedSubscription.PrivateData.SubscriptionId)
	var preConsume model.SubscriptionPreConsumeRecord
	require.NoError(t, model.DB.Where("request_id = ?", subscriptionTask.TaskID).First(&preConsume).Error)
	assert.Equal(t, "refunded", preConsume.Status)

	second, err := RunImageTaskMaintenance(context.Background(), now.Add(time.Minute))
	require.NoError(t, err)
	assert.Zero(t, second.Reconciled)
	assert.Equal(t, 1000, getUserQuota(t, 101))
	assert.Equal(t, 500, getTokenRemainQuota(t, 201))
	assert.EqualValues(t, 0, getSubscriptionUsed(t, 202))
	var refunds int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("type = ?", model.LogTypeRefund).Count(&refunds).Error)
	assert.EqualValues(t, 2, refunds)
}

func TestImageTaskMaintenanceNeverRefundsSuccessfulTask(t *testing.T) {
	truncate(t)
	setupImageTaskBillingEvidenceTable(t)
	t.Setenv("IMAGE_TASK_TIMEOUT_MINUTES", "30")
	now := time.Now()
	seedImageTaskRecoveryUser(t, 111, 900)
	seedChannel(t, 311)
	task := &model.Task{
		TaskID: "image-success-no-refund", UserId: 111, Platform: constant.TaskPlatformImage,
		Status: model.TaskStatusSuccess, Progress: "100%", SubmitTime: now.Add(-time.Hour).Unix(),
		FinishTime: now.Unix(), Quota: 100, ChannelId: 311,
	}
	require.NoError(t, task.Insert())
	seedImageTaskConsumeLog(t, task.TaskID, 111, 0, 311, 100)

	summary, err := RunImageTaskMaintenance(context.Background(), now)
	require.NoError(t, err)
	assert.Zero(t, summary.Reconciled)
	assert.Equal(t, 900, getUserQuota(t, 111))
	assert.Equal(t, 100, getTaskQuota(t, task.ID))
}

func TestImageTaskMaintenanceConcurrentCASRefundsOnce(t *testing.T) {
	truncate(t)
	setupImageTaskBillingEvidenceTable(t)
	t.Setenv("IMAGE_TASK_TIMEOUT_MINUTES", "30")
	now := time.Now()
	seedImageTaskRecoveryUser(t, 121, 900)
	seedChannel(t, 321)
	seedChargedAccounting(t, 121, 321, 0, 100, 1)
	task := &model.Task{
		TaskID: "image-concurrent-recovery", UserId: 121, Platform: constant.TaskPlatformCanvasImage,
		Group: "default", Status: model.TaskStatusInProgress, Progress: "10%",
		SubmitTime: now.Add(-time.Hour).Unix(),
	}
	require.NoError(t, task.Insert())
	seedImageTaskConsumeLog(t, task.TaskID, 121, 0, 321, 100)

	var wait sync.WaitGroup
	errorsByWorker := make([]error, 2)
	for i := range errorsByWorker {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, errorsByWorker[index] = RunImageTaskMaintenance(context.Background(), now)
		}(i)
	}
	wait.Wait()
	for _, err := range errorsByWorker {
		require.NoError(t, err)
	}
	// SQLite deliberately lets a true overlapping writer fail with BUSY. The
	// failed positive-quota row must remain a durable candidate for the next run.
	_, err := RunImageTaskMaintenance(context.Background(), now.Add(2*time.Minute))
	require.NoError(t, err)
	assert.Equal(t, 1000, getUserQuota(t, 121))
	var refunds int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("type = ?", model.LogTypeRefund).Count(&refunds).Error)
	assert.EqualValues(t, 1, refunds)
}
func TestImageTaskRefundRetrySelectionIgnoresLiveFailReason(t *testing.T) {
	truncate(t)
	cutoff := time.Now().Add(-time.Minute).Unix()
	reasons := []string{"", "provider rejected the request", "image generation was interrupted before completion"}
	for i, reason := range reasons {
		task := &model.Task{
			TaskID: fmt.Sprintf("failed-refund-%d", i), UserId: 1,
			Platform: constant.TaskPlatformImage, Status: model.TaskStatusFailure,
			Quota: 100, FailReason: reason, UpdatedAt: cutoff - 1,
		}
		require.NoError(t, task.Insert())
	}
	claimed, err := model.ClaimFailedImageTaskRefundRetries(context.Background(), cutoff, 10)
	require.NoError(t, err)
	require.Len(t, claimed, len(reasons))
	selected := make(map[string]bool, len(claimed))
	for _, task := range claimed {
		selected[task.FailReason] = true
	}
	for _, reason := range reasons {
		assert.True(t, selected[reason], "fail reason %q must remain retryable", reason)
	}
}

func TestImageTaskMaintenanceRetriesFailedRefundWithTimeoutDisabled(t *testing.T) {
	truncate(t)
	t.Setenv("IMAGE_TASK_TIMEOUT_MINUTES", "0")
	now := time.Now()
	const reason = "provider rejected this exact image request"
	seedImageTaskRecoveryUser(t, 131, 900)
	task := &model.Task{
		TaskID: "failed-refund-timeout-disabled", UserId: 131,
		Platform: constant.TaskPlatformImage, Status: model.TaskStatusFailure,
		Quota: 100, FailReason: reason, UpdatedAt: now.Add(-2 * time.Minute).Unix(),
		PrivateData: model.TaskPrivateData{BillingSource: BillingSourceWallet},
	}
	require.NoError(t, task.Insert())

	_, err := RunImageTaskMaintenance(context.Background(), now)
	require.NoError(t, err)
	assert.Equal(t, 1000, getUserQuota(t, 131))
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.Zero(t, reloaded.Quota)
	assert.Equal(t, reason, reloaded.FailReason)
	assert.Nil(t, reloaded.PrivateData.RefundReconciliation)
	var refund model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", task.TaskID, model.LogTypeRefund).First(&refund).Error)
	assert.Contains(t, refund.Other, reason)
}

func TestImageTaskMaintenanceEnabledByRefundWorkAlone(t *testing.T) {
	t.Run("positive quota failed retry", func(t *testing.T) {
		truncate(t)
		now := time.Now().Unix()
		require.NoError(t, (&model.Task{
			TaskID: "maintenance-positive-refund", Platform: constant.TaskPlatformCanvasImage,
			Status: model.TaskStatusFailure, Quota: 25, UpdatedAt: now - 1,
		}).Insert())
		assert.True(t, model.HasImageTaskMaintenanceWork(now, 0, 0))
	})

	t.Run("postcommit marker", func(t *testing.T) {
		truncate(t)
		now := time.Now().Unix()
		require.NoError(t, (&model.Task{
			TaskID: "maintenance-postcommit-marker", Platform: constant.TaskPlatformImage,
			Status: model.TaskStatusFailure, UpdatedAt: now - 1,
			RefundReconciliationState: model.TaskRefundReconciliationStatePending,
			PrivateData: model.TaskPrivateData{RefundReconciliation: &model.TaskRefundReconciliation{
				Amount: 25, UserId: 1, BillingSource: BillingSourceSubscription,
				AccountingDone: true, CacheRepairDone: true,
			}},
		}).Insert())
		assert.True(t, model.HasImageTaskMaintenanceWork(now, 0, 0))
	})
}

func TestImageTaskMaintenanceReportsClickHouseManualReconciliationOnce(t *testing.T) {
	truncate(t)
	t.Setenv("IMAGE_TASK_TIMEOUT_MINUTES", "0")
	previousRetention := common.GetImageTaskDataRetentionHours()
	common.SetImageTaskDataRetentionHours(0)
	t.Cleanup(func() { common.SetImageTaskDataRetentionHours(previousRetention) })
	now := time.Now()
	task := &model.Task{
		TaskID: "clickhouse-manual-reconciliation", Platform: constant.TaskPlatformImage,
		Status: model.TaskStatusFailure, UpdatedAt: now.Add(-2 * time.Minute).Unix(),
		RefundReconciliationState: model.TaskRefundReconciliationStateManualUnreported,
		PrivateData: model.TaskPrivateData{RefundReconciliation: &model.TaskRefundReconciliation{
			Amount: 100, UserId: 1, AccountingDone: true, CacheRepairDone: true,
			LogClaimToken: "attempt-owner", LogWriteAttempted: true,
			LogWriteAttemptedAt:          now.Add(-2 * time.Minute).Unix(),
			LogIdempotencyKey:            "task-refund:clickhouse-manual-reconciliation",
			ManualReconciliationRequired: true,
			ManualReconciliationReason:   "ClickHouse refund audit log write outcome requires manual reconciliation",
		}},
	}
	require.NoError(t, task.Insert())
	assert.True(t, model.HasImageTaskMaintenanceWork(now.Unix(), 0, 0))

	first, err := RunImageTaskMaintenance(context.Background(), now)
	require.NoError(t, err)
	assert.EqualValues(t, 1, first.ManualRefundReconciliations)
	assert.Zero(t, first.PendingRefundReconciliations)
	var retained model.Task
	require.NoError(t, model.DB.First(&retained, task.ID).Error)
	require.NotNil(t, retained.PrivateData.RefundReconciliation)
	assert.True(t, retained.PrivateData.RefundReconciliation.ManualReconciliationReported)
	assert.Zero(t, retained.Quota)
	var count int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("request_id = ? AND type = ?", task.TaskID, model.LogTypeRefund).Count(&count).Error)
	assert.Zero(t, count)

	second, err := RunImageTaskMaintenance(context.Background(), now.Add(2*time.Minute))
	require.NoError(t, err)
	assert.Zero(t, second.ManualRefundReconciliations)
	assert.Zero(t, second.PendingRefundReconciliations)
	assert.False(t, model.HasImageTaskMaintenanceWork(now.Add(2*time.Minute).Unix(), 0, 0))
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("request_id = ? AND type = ?", task.TaskID, model.LogTypeRefund).Count(&count).Error)
	assert.Zero(t, count, "maintenance must never reinsert an ambiguous ClickHouse audit row")
}

func TestRegisterImageTaskMaintenanceSystemTaskExposesScheduledHook(t *testing.T) {
	RegisterImageTaskMaintenanceSystemTask()
	found := false
	for _, handler := range registeredSystemTaskHandlers() {
		if handler.Type() != SystemTaskTypeImageTaskMaintenance {
			continue
		}
		scheduled, ok := handler.(ScheduledSystemTaskHandler)
		require.True(t, ok)
		assert.Equal(t, time.Minute, scheduled.Interval())
		found = true
	}
	assert.True(t, found)
}
