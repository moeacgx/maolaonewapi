package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

// ImageTaskFailureLogMetadata 保存异步图片请求运行期才能获得的日志字段。
// 任务本身已经持久化的字段会作为兜底，保证进程重启后的超时扫描仍可落日志。
type ImageTaskFailureLogMetadata struct {
	StatusCode        int
	ChannelId         int
	ChannelName       string
	ChannelType       int
	ModelName         string
	Group             string
	Username          string
	TokenName         string
	TokenId           int
	RequestId         string
	UpstreamRequestId string
	RequestIP         string
	RequestPath       string
	ErrorType         string
	ErrorCode         string
	UsedChannels      []string
}

// RecordImageTaskFailureLog 为 image/canvas_image 失败终态写入普通使用日志。
// 调用方必须先通过任务状态 CAS 赢得 FAILURE 转换，避免超时扫描与迟到响应重复记录。
func RecordImageTaskFailureLog(ctx context.Context, task *model.Task, reason string, metadata ImageTaskFailureLogMetadata) error {
	if task == nil || !constant.IsImageTaskPlatform(task.Platform) || task.Status != model.TaskStatusFailure {
		return nil
	}

	statusCode := metadata.StatusCode
	// 2xx 但正文为空/无效仍属于异步包装器失败，不能在错误日志里伪装成成功状态。
	// 3xx 及 4xx/5xx 保留原始上游状态，便于定位重定向和风控问题。
	if statusCode < http.StatusMultipleChoices || statusCode > 599 {
		statusCode = http.StatusInternalServerError
	}
	maskedReason := strings.TrimSpace(common.MaskSensitiveInfo(reason))
	if maskedReason == "" {
		maskedReason = "image generation failed"
	}

	modelName := strings.TrimSpace(metadata.ModelName)
	if modelName == "" {
		modelName = strings.TrimSpace(task.Properties.OriginModelName)
	}
	group := strings.TrimSpace(metadata.Group)
	if group == "" {
		group = strings.TrimSpace(task.Group)
	}
	channelId := metadata.ChannelId
	if channelId <= 0 {
		channelId = task.ChannelId
	}
	tokenId := metadata.TokenId
	if tokenId <= 0 {
		tokenId = task.PrivateData.TokenId
	}
	tokenName := strings.TrimSpace(metadata.TokenName)
	if tokenName == "" {
		tokenName = strings.TrimSpace(task.PrivateData.TokenName)
	}
	username := strings.TrimSpace(metadata.Username)
	if username == "" {
		username = strings.TrimSpace(task.PrivateData.Username)
	}
	requestId := strings.TrimSpace(metadata.RequestId)
	if requestId == "" {
		requestId = strings.TrimSpace(task.PrivateData.RequestId)
	}
	upstreamRequestId := strings.TrimSpace(metadata.UpstreamRequestId)
	if upstreamRequestId == "" {
		upstreamRequestId = strings.TrimSpace(task.PrivateData.UpstreamRequestId)
	}
	requestPath := strings.TrimSpace(metadata.RequestPath)
	if requestPath == "" {
		if task.Platform == constant.TaskPlatformCanvasImage {
			requestPath = "/canvas/v1/images/tasks"
		} else {
			requestPath = "/v1/images/tasks"
		}
	}
	errorType := strings.TrimSpace(metadata.ErrorType)
	if errorType == "" {
		errorType = "async_image_task_error"
	}
	errorCode := strings.TrimSpace(metadata.ErrorCode)
	if errorCode == "" {
		errorCode = "async_image_task_failed"
	}

	other := map[string]interface{}{
		"is_task":       true,
		"task_id":       task.TaskID,
		"task_platform": string(task.Platform),
		"task_action":   task.Action,
		"task_status":   string(task.Status),
		"status_code":   statusCode,
		"error_type":    errorType,
		"error_code":    errorCode,
		"request_path":  requestPath,
		"channel_id":    channelId,
	}
	if task.SubmitTime > 0 {
		other["task_submit_time"] = task.SubmitTime
	}
	if task.StartTime > 0 {
		other["task_start_time"] = task.StartTime
	}
	if task.FinishTime > 0 {
		other["task_finish_time"] = task.FinishTime
	}
	if metadata.ChannelName != "" {
		other["channel_name"] = metadata.ChannelName
	}
	if metadata.ChannelType != 0 {
		other["channel_type"] = metadata.ChannelType
	}
	if len(metadata.UsedChannels) > 0 {
		other["admin_info"] = map[string]interface{}{
			"use_channel":   append([]string(nil), metadata.UsedChannels...),
			"attempt_count": len(metadata.UsedChannels),
		}
	}

	useTimeSeconds := 0
	startTime := task.StartTime
	if startTime <= 0 {
		startTime = task.SubmitTime
	}
	if task.FinishTime > startTime && startTime > 0 {
		useTimeSeconds = int(task.FinishTime - startTime)
	}

	createdAt := task.FinishTime
	if createdAt <= 0 {
		createdAt = task.SubmitTime
	}

	return model.RecordErrorLogWithParams(ctx, task.UserId, model.RecordErrorLogParams{
		ChannelId:         channelId,
		ModelName:         modelName,
		TokenName:         tokenName,
		Content:           fmt.Sprintf("status_code=%d, %s", statusCode, maskedReason),
		TokenId:           tokenId,
		UseTimeSeconds:    useTimeSeconds,
		IsStream:          false,
		Group:             group,
		Other:             other,
		Username:          username,
		RequestId:         requestId,
		UpstreamRequestId: upstreamRequestId,
		RequestIP:         metadata.RequestIP,
		CreatedAt:         createdAt,
	})
}
