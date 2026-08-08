package atlascloud

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	baseatlas "github.com/QuantumNous/new-api/relay/channel/atlascloud"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

type TaskAdaptor struct {
	taskcommon.BaseBilling
	apiKey  string
	baseURL string
}

var _ channel.OpenAIVideoConverter = (*TaskAdaptor)(nil)

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	if info == nil {
		return
	}
	baseatlas.EnsureChannelMeta(info)
	a.apiKey = info.ApiKey
	a.baseURL = info.ChannelBaseUrl
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	var req relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if strings.TrimSpace(req.Model) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("model field is required"), "missing_model", http.StatusBadRequest)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("prompt is required"), "invalid_request", http.StatusBadRequest)
	}
	if req.InputReference != "" && len(req.Images) == 0 {
		req.Images = []string{req.InputReference}
	}
	if req.Image != "" && len(req.Images) == 0 {
		req.Images = []string{req.Image}
	}
	info.Action = constant.TaskActionTextGenerate
	if len(req.Images) > 0 {
		info.Action = constant.TaskActionGenerate
	}
	c.Set("task_request", req)
	return nil
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, _ *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	seconds := req.Duration
	if seconds == 0 && req.Seconds != "" {
		seconds, _ = strconv.Atoi(req.Seconds)
	}
	if seconds <= 0 {
		return nil
	}
	return map[string]float64{"seconds": float64(seconds)}
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return baseatlas.BuildAPIURL(a.baseURL, "/api/v1/model/generateVideo"), nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	baseatlas.EnsureChannelMeta(info)
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	modelName := strings.TrimSpace(info.UpstreamModelName)
	if modelName == "" {
		modelName = strings.TrimSpace(req.Model)
	}
	payload := map[string]any{
		"model":  modelName,
		"prompt": req.Prompt,
	}
	if len(req.Images) > 0 {
		imageURL, err := baseatlas.NormalizeImageURL(c, info, req.Images[0])
		if err != nil {
			return nil, err
		}
		if imageURL != "" {
			payload["image_url"] = imageURL
		}
		if len(req.Images) > 1 {
			var urls []string
			for _, raw := range req.Images {
				imageURL, err := baseatlas.NormalizeImageURL(c, info, raw)
				if err != nil {
					return nil, err
				}
				if imageURL != "" {
					urls = append(urls, imageURL)
				}
			}
			if len(urls) > 0 {
				payload["image_urls"] = urls
			}
		}
	}
	if req.Size != "" {
		payload["size"] = req.Size
	}
	if req.Resolution != "" {
		payload["resolution"] = req.Resolution
	}
	if req.Duration > 0 {
		payload["duration"] = req.Duration
	} else if req.Seconds != "" {
		if seconds, err := strconv.Atoi(req.Seconds); err == nil && seconds > 0 {
			payload["duration"] = seconds
		}
	}
	for key, value := range req.Metadata {
		switch key {
		case "model", "prompt", "image", "images":
			continue
		default:
			payload[key] = value
		}
	}
	data, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	var submitResp baseatlas.APIResponse
	if err := common.Unmarshal(responseBody, &submitResp); err != nil {
		return "", nil, service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
	}
	upstreamID := strings.TrimSpace(submitResp.Data.ID)
	if upstreamID == "" {
		upstreamID = strings.TrimSpace(submitResp.Data.TaskID)
	}
	if upstreamID == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("prediction id is empty"), "invalid_response", http.StatusInternalServerError)
	}

	video := dto.NewOpenAIVideo()
	video.ID = info.PublicTaskID
	video.TaskID = info.PublicTaskID
	video.Model = info.OriginModelName
	video.CreatedAt = time.Now().Unix()
	if req, reqErr := relaycommon.GetTaskRequest(c); reqErr == nil {
		if req.Duration > 0 {
			video.Seconds = strconv.Itoa(req.Duration)
		} else {
			video.Seconds = req.Seconds
		}
		video.Size = firstNonEmpty(req.Size, req.Resolution)
	}
	c.JSON(http.StatusOK, video)
	return upstreamID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	req, err := http.NewRequest(http.MethodGet, baseatlas.BuildAPIURL(baseURL, "/api/v1/model/prediction/"+url.PathEscape(taskID)), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var result baseatlas.APIResponse
	if err := common.Unmarshal(respBody, &result); err != nil {
		return nil, errors.Wrap(err, "unmarshal atlascloud prediction failed")
	}
	taskInfo := &relaycommon.TaskInfo{}
	switch strings.ToLower(strings.TrimSpace(result.Data.Status)) {
	case "":
		return taskInfo, nil
	case "processing":
		taskInfo.Status = model.TaskStatusInProgress
		taskInfo.Progress = taskcommon.ProgressInProgress
	case "completed":
		taskInfo.Status = model.TaskStatusSuccess
		taskInfo.Progress = taskcommon.ProgressComplete
		taskInfo.Url = baseatlas.FirstOutput(result.Data.Outputs)
		if taskInfo.Url == "" {
			taskInfo.Status = model.TaskStatusFailure
			taskInfo.Reason = "atlascloud prediction completed without output"
		}
	case "failed", "cancelled", "canceled":
		taskInfo.Status = model.TaskStatusFailure
		taskInfo.Progress = taskcommon.ProgressComplete
		taskInfo.Reason = baseatlas.ErrorText(result.Data.Error)
		if taskInfo.Reason == "" {
			taskInfo.Reason = "atlascloud prediction failed"
		}
	default:
		return nil, fmt.Errorf("unknown atlascloud prediction status: %s", result.Data.Status)
	}
	return taskInfo, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	video := dto.NewOpenAIVideo()
	video.ID = task.TaskID
	video.TaskID = task.TaskID
	video.Model = firstNonEmpty(task.Properties.OriginModelName, task.Properties.UpstreamModelName)
	video.Status = task.Status.ToVideoStatus()
	video.SetProgressStr(task.Progress)
	video.CreatedAt = task.CreatedAt
	if task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure {
		video.CompletedAt = firstPositive(task.FinishTime, task.UpdatedAt)
	}
	if task.Status == model.TaskStatusFailure {
		message := strings.TrimSpace(task.FailReason)
		if message == "" {
			message = "atlascloud video generation failed"
		}
		video.Error = &dto.OpenAIVideoError{Code: "video_generation_failed", Message: message}
	}
	if url := strings.TrimSpace(task.GetResultURL()); url != "" {
		video.SetMetadata("url", url)
	}
	return common.Marshal(video)
}

func (a *TaskAdaptor) GetModelList() []string { return baseatlas.ModelList }
func (a *TaskAdaptor) GetChannelName() string { return baseatlas.ChannelName }

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstPositive(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
