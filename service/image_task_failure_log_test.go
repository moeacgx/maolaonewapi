package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestSweepTimedOutImageTaskRecordsFailureUsageLog(t *testing.T) {
	truncate(t)
	now := time.Now().Unix()
	task := &model.Task{
		TaskID:     "task_sweep_failure_log",
		UserId:     601,
		Platform:   constant.TaskPlatformCanvasImage,
		Group:      "canvas-group",
		Action:     "images/generations",
		Status:     model.TaskStatusInProgress,
		Progress:   "10%",
		SubmitTime: now - 120,
		StartTime:  now - 110,
		Properties: model.Properties{OriginModelName: "gpt-image-sweep"},
		PrivateData: model.TaskPrivateData{
			TokenId:   71,
			TokenName: "sweep-token",
			Username:  "sweep-user",
			RequestId: "req-sweep-image",
		},
	}
	require.NoError(t, task.Insert())

	sweepTimedOutTaskBatch(context.Background(), []*model.Task{task}, "图片生成任务超时（1分钟）")

	var log model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ? AND type = ?", task.UserId, model.LogTypeError).First(&log).Error)
	require.Equal(t, "gpt-image-sweep", log.ModelName)
	require.Equal(t, "sweep-user", log.Username)
	require.Equal(t, "canvas-group", log.Group)
	require.Equal(t, "sweep-token", log.TokenName)
	require.Equal(t, 71, log.TokenId)
	require.Equal(t, "req-sweep-image", log.RequestId)
	require.Contains(t, log.Content, "status_code=504")
	require.Contains(t, log.Content, "图片生成任务超时")

	other, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	require.Equal(t, task.TaskID, other["task_id"])
	require.Equal(t, string(constant.TaskPlatformCanvasImage), other["task_platform"])
	require.EqualValues(t, http.StatusGatewayTimeout, other["status_code"])
}
