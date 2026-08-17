package model

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageTaskStorageOwnershipCleanupAndReconciliation(t *testing.T) {
	truncateTables(t)
	now := time.Now().Unix()
	tasks := []*Task{
		{TaskID: "expired-success", UserId: 1, Platform: constant.TaskPlatformCanvasImage, Status: TaskStatusSuccess, FinishTime: now - 7200, Data: []byte(`{"data":[1]}`)},
		{TaskID: "expired-failure", UserId: 1, Platform: constant.TaskPlatformImage, Status: TaskStatusFailure, FinishTime: now - 7200, Data: []byte(`{"error":true}`)},
		{TaskID: "recent-success", UserId: 1, Platform: constant.TaskPlatformCanvasImage, Status: TaskStatusSuccess, FinishTime: now - 60, Data: []byte(`{"data":[2]}`)},
		{TaskID: "stale-pending", UserId: 1, Platform: constant.TaskPlatformCanvasImage, Status: TaskStatusQueued, SubmitTime: now - 7200, Data: []byte(`{"transient":true}`)},
		{TaskID: "recent-pending", UserId: 1, Platform: constant.TaskPlatformImage, Status: TaskStatusInProgress, SubmitTime: now - 60},
		{TaskID: "other-platform", UserId: 1, Platform: constant.TaskPlatform("video"), Status: TaskStatusSuccess, FinishTime: now - 7200, Data: []byte(`{"video":true}`)},
	}
	for _, task := range tasks {
		require.NoError(t, task.Insert())
	}

	owned, exists, err := GetImageTaskByOwnerPlatform(1, "expired-success", constant.TaskPlatformCanvasImage)
	require.NoError(t, err)
	require.True(t, exists)
	assert.Equal(t, "expired-success", owned.TaskID)
	for _, lookup := range []struct {
		userID   int
		platform constant.TaskPlatform
	}{{2, constant.TaskPlatformCanvasImage}, {1, constant.TaskPlatformImage}} {
		_, exists, err = GetImageTaskByOwnerPlatform(lookup.userID, "expired-success", lookup.platform)
		require.NoError(t, err)
		assert.False(t, exists)
	}

	cleared, err := ClearExpiredImageTaskData(context.Background(), now-3600, 50)
	require.NoError(t, err)
	assert.EqualValues(t, 2, cleared)

	reconciled, err := ReconcileStaleImageTasks(context.Background(), now-3600, 50, "image generation was interrupted before completion")
	require.NoError(t, err)
	assert.EqualValues(t, 1, reconciled)

	var stale Task
	require.NoError(t, DB.Where("task_id = ?", "stale-pending").First(&stale).Error)
	assert.EqualValues(t, TaskStatusFailure, stale.Status)
	assert.Equal(t, "100%", stale.Progress)
	assert.Empty(t, stale.Data)
	assert.Equal(t, "image generation was interrupted before completion", stale.FailReason)

	var recent Task
	require.NoError(t, DB.Where("task_id = ?", "recent-pending").First(&recent).Error)
	assert.EqualValues(t, TaskStatusInProgress, recent.Status)
	var other Task
	require.NoError(t, DB.Where("task_id = ?", "other-platform").First(&other).Error)
	assert.NotEmpty(t, other.Data)
}

func TestImageTaskAdmissionTransactionCountsCommittedTask(t *testing.T) {
	truncateTables(t)
	user := &User{Id: 701, Username: "image-admission", AffCode: "image-admission-aff", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(user).Error)

	tx, admitted, err := BeginImageTaskAdmission(context.Background(), user.Id, 801, 1, 1)
	require.NoError(t, err)
	require.True(t, admitted)
	task := &Task{
		TaskID: "transactional-admission", UserId: user.Id, Platform: constant.TaskPlatformImage,
		Status: TaskStatusQueued, PrivateData: TaskPrivateData{TokenId: 801},
	}
	require.NoError(t, tx.Create(task).Error)
	require.NoError(t, tx.Commit().Error)

	blockedTx, admitted, err := BeginImageTaskAdmission(context.Background(), user.Id, 801, 1, 1)
	require.NoError(t, err)
	assert.False(t, admitted)
	assert.Nil(t, blockedTx)
}

func TestGenericTaskPollingExcludesLocalImageWrappers(t *testing.T) {
	truncateTables(t)
	now := time.Now().Unix()
	for _, task := range []*Task{
		{TaskID: "canvas-local", Platform: constant.TaskPlatformCanvasImage, Status: TaskStatusInProgress, Progress: "10%", SubmitTime: now - 7200},
		{TaskID: "image-local", Platform: constant.TaskPlatformImage, Status: TaskStatusQueued, Progress: "0%", SubmitTime: now - 7200},
		{TaskID: "video-upstream", Platform: constant.TaskPlatform("sora"), Status: TaskStatusInProgress, Progress: "10%", SubmitTime: now - 7200},
	} {
		require.NoError(t, task.Insert())
	}
	unfinished := GetAllUnFinishSyncTasks(20)
	require.Len(t, unfinished, 1)
	assert.Equal(t, "video-upstream", unfinished[0].TaskID)
	timedOut := GetTimedOutUnfinishedTasks(now-3600, 20)
	require.Len(t, timedOut, 1)
	assert.Equal(t, "video-upstream", timedOut[0].TaskID)
}
