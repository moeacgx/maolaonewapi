package model

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/constant"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTaskLogQueriesKeepResultDataForDTOPreparation(t *testing.T) {
	const userID = 9901
	taskIDs := []string{"task_log_image_payload", "task_log_suno_payload"}
	t.Cleanup(func() {
		_ = DB.Where("task_id IN ?", taskIDs).Delete(&Task{}).Error
	})

	imageData := json.RawMessage(`{"data":[{"b64_json":"very-large-image-payload"}]}`)
	sunoData := json.RawMessage(`[{"audio_url":"https://example.com/audio.mp3"}]`)
	require.NoError(t, DB.Create(&[]*Task{
		{
			TaskID:   taskIDs[0],
			UserId:   userID,
			Platform: constant.TaskPlatformCanvasImage,
			Data:     imageData,
		},
		{
			TaskID:   taskIDs[1],
			UserId:   userID,
			Platform: constant.TaskPlatformSuno,
			Data:     sunoData,
		},
	}).Error)

	assertTaskLogData := func(t *testing.T, tasks []*Task) {
		t.Helper()
		byID := make(map[string]*Task, len(tasks))
		for _, task := range tasks {
			byID[task.TaskID] = task
		}
		require.Contains(t, byID, taskIDs[0])
		require.Contains(t, byID, taskIDs[1])
		require.JSONEq(t, string(imageData), string(byID[taskIDs[0]].Data))
		require.JSONEq(t, string(sunoData), string(byID[taskIDs[1]].Data))
	}

	t.Run("admin list", func(t *testing.T) {
		assertTaskLogData(t, TaskGetAllTasks(0, 10, SyncTaskQueryParams{UserID: "9901"}))
	})
	t.Run("user list", func(t *testing.T) {
		assertTaskLogData(t, TaskGetAllUserTask(userID, 0, 10, SyncTaskQueryParams{}))
	})
}

func TestTaskLogListQueriesSkipImageDataOnly(t *testing.T) {
	const userID = 9902
	taskIDs := []string{"task_log_light_image", "task_log_light_suno"}
	t.Cleanup(func() {
		_ = DB.Where("task_id IN ?", taskIDs).Delete(&Task{}).Error
	})

	imageData := json.RawMessage(`{"data":[{"b64_json":"very-large-image-payload"}]}`)
	sunoData := json.RawMessage(`[{"audio_url":"https://example.com/audio.mp3"}]`)
	require.NoError(t, DB.Create(&[]*Task{
		{
			TaskID:   taskIDs[0],
			UserId:   userID,
			Platform: constant.TaskPlatformImage,
			Data:     imageData,
		},
		{
			TaskID:   taskIDs[1],
			UserId:   userID,
			Platform: constant.TaskPlatformSuno,
			Data:     sunoData,
		},
	}).Error)

	assertLightweightData := func(t *testing.T, tasks []*Task) {
		t.Helper()
		byID := make(map[string]*Task, len(tasks))
		for _, task := range tasks {
			byID[task.TaskID] = task
		}
		require.Contains(t, byID, taskIDs[0])
		require.Contains(t, byID, taskIDs[1])
		require.Empty(t, byID[taskIDs[0]].Data)
		require.JSONEq(t, string(sunoData), string(byID[taskIDs[1]].Data))
	}

	t.Run("admin log list", func(t *testing.T) {
		assertLightweightData(t, TaskGetAllTasksForLog(0, 10, SyncTaskQueryParams{UserID: "9902"}))
	})
	t.Run("user log list", func(t *testing.T) {
		assertLightweightData(t, TaskGetAllUserTaskForLog(userID, 0, 10, SyncTaskQueryParams{}))
	})
}

func TestTaskLogSelectOmitsImageDataColumn(t *testing.T) {
	lightweightSQL := DB.ToSQL(func(tx *gorm.DB) *gorm.DB {
		var tasks []*Task
		return taskLogSelect(tx, true, false).Find(&tasks)
	})
	fullSQL := DB.ToSQL(func(tx *gorm.DB) *gorm.DB {
		var tasks []*Task
		return taskLogSelect(tx, false, false).Find(&tasks)
	})

	// 轻量查询生成明确的列清单，而不是 SELECT *，
	// 从数据库读取时便会跳过图片任务的 Base64 大字段。
	require.NotContains(t, lightweightSQL, "`data`")
	require.NotContains(t, lightweightSQL, `"data"`)
	require.NotContains(t, lightweightSQL, "SELECT *")
	require.Contains(t, fullSQL, "SELECT *")
}
