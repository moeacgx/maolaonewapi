package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const (
	canvasImageTaskActionGenerations = "images/generations"
	canvasImageTaskActionEdits       = "images/edits"
	canvasImageTaskRelayPrefix       = "/canvas/v1"
	apiImageTaskRelayPrefix          = "/v1"
	canvasImageTaskQuotaSyncRetries  = 2
)

type canvasImageTaskRelayRequest struct {
	TaskID      string
	Action      string
	RelayPrefix string
	Body        []byte
	Header      http.Header
	RawQuery    string
	RequestIP   string
	Keys        map[string]any
	Context     context.Context
}

func CanvasImageTaskSubmit(c *gin.Context) {
	submitImageTask(
		c,
		normalizeCanvasImageTaskAction(c.Query("action")),
		constant.TaskPlatformCanvasImage,
		canvasImageTaskRelayPrefix,
	)
}

// ImageTaskSubmit 为令牌 API 提供与 Canvas 相同的异步图片任务能力。
func ImageTaskSubmit(c *gin.Context) {
	action, ok := parseImageTaskAction(c.Query("action"))
	if !ok {
		abortCanvasRequest(c, http.StatusBadRequest, "unsupported image task action")
		return
	}
	submitImageTask(c, action, constant.TaskPlatformImage, apiImageTaskRelayPrefix)
}

func submitImageTask(c *gin.Context, action string, platform constant.TaskPlatform, relayPrefix string) {
	body, err := readCanvasImageTaskBody(c)
	if err != nil {
		abortCanvasRequest(c, http.StatusBadRequest, err.Error())
		return
	}

	group := imageTaskGroup(c, relayPrefix)
	modelName := extractImageTaskModel(body, c.GetHeader("Content-Type"))

	now := time.Now().Unix()
	task := &model.Task{
		TaskID:     model.GenerateTaskID(),
		Platform:   platform,
		UserId:     c.GetInt("id"),
		Group:      group,
		Action:     action,
		Status:     model.TaskStatusQueued,
		Progress:   "0%",
		SubmitTime: now,
		Properties: model.Properties{OriginModelName: modelName},
		PrivateData: model.TaskPrivateData{
			TokenId:   c.GetInt("token_id"),
			TokenName: c.GetString("token_name"),
			Username:  c.GetString("username"),
			RequestId: c.GetString(common.RequestIdKey),
		},
	}
	if err := task.Insert(); err != nil {
		abortCanvasRequest(c, http.StatusInternalServerError, "failed to create image task")
		return
	}

	relayReq := canvasImageTaskRelayRequest{
		TaskID:      task.TaskID,
		Action:      action,
		RelayPrefix: relayPrefix,
		Body:        append([]byte(nil), body...),
		Header:      c.Request.Header.Clone(),
		RawQuery:    imageTaskRelayRawQuery(c),
		RequestIP:   c.ClientIP(),
		Keys:        cloneCanvasImageTaskKeys(c.Keys),
	}
	// 提交接口会立即返回，真正的 Relay 在后台执行。内部请求携带该标记后，
	// Relay 跳过普通错误日志，由任务终态统一写入，避免重复记录。
	relayReq.Keys[string(constant.ContextKeyAsyncImageTask)] = true
	go runCanvasImageTaskRelay(relayReq)

	c.JSON(http.StatusAccepted, gin.H{
		"task_id": task.TaskID,
		"status":  mapCanvasImageTaskStatus(task.Status),
	})
}

func CanvasImageTaskFetch(c *gin.Context) {
	fetchImageTask(c, constant.TaskPlatformCanvasImage, canvasImageTaskRelayPrefix)
}

// ImageTaskFetch 查询令牌 API 提交的异步图片任务。
func ImageTaskFetch(c *gin.Context) {
	fetchImageTask(c, constant.TaskPlatformImage, apiImageTaskRelayPrefix)
}

func fetchImageTask(c *gin.Context, platform constant.TaskPlatform, responsePrefix string) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	task, exists, err := model.GetByTaskId(c.GetInt("id"), taskID)
	if err != nil {
		abortCanvasRequest(c, http.StatusInternalServerError, "failed to load image task")
		return
	}
	if !exists || task.Platform != platform {
		abortCanvasRequest(c, http.StatusNotFound, "task not found")
		return
	}

	c.JSON(http.StatusOK, buildImageTaskResponse(task, responsePrefix))
}

func CanvasImageTaskContent(c *gin.Context) {
	serveImageTaskContent(c, constant.TaskPlatformCanvasImage)
}

// ImageTaskContent 返回令牌 API 异步图片任务中暂存的图片内容。
func ImageTaskContent(c *gin.Context) {
	serveImageTaskContent(c, constant.TaskPlatformImage)
}

func serveImageTaskContent(c *gin.Context, platform constant.TaskPlatform) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	index, err := strconv.Atoi(strings.TrimSpace(c.Param("index")))
	if taskID == "" || err != nil || index < 0 {
		abortCanvasRequest(c, http.StatusBadRequest, "invalid image content request")
		return
	}

	task, exists, err := model.GetByTaskId(c.GetInt("id"), taskID)
	if err != nil {
		abortCanvasRequest(c, http.StatusInternalServerError, "failed to load image task")
		return
	}
	if !exists || task.Platform != platform {
		abortCanvasRequest(c, http.StatusNotFound, "task not found")
		return
	}
	writeImageTaskContent(c, task, index)
}

func writeImageTaskContent(c *gin.Context, task *model.Task, index int) {
	if task.Status != model.TaskStatusSuccess {
		abortCanvasRequest(c, http.StatusBadRequest, "image task is not completed")
		return
	}
	if imageTaskDataExpired(task, time.Now().Unix()) || len(bytes.TrimSpace(task.Data)) == 0 {
		abortCanvasRequest(c, http.StatusGone, "image task data has expired")
		return
	}

	image, mimeType, err := readCanvasImageTaskContent(task, index)
	if err != nil {
		abortCanvasRequest(c, http.StatusNotFound, "image content not found")
		return
	}

	c.Header("Cache-Control", fmt.Sprintf("private, max-age=%d", imageTaskContentMaxAge(task, time.Now().Unix())))
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, mimeType, image)
}

func imageTaskRelayRawQuery(c *gin.Context) string {
	query := c.Request.URL.Query()
	query.Del("action")
	return query.Encode()
}

func imageTaskGroup(c *gin.Context, relayPrefix string) string {
	if relayPrefix == canvasImageTaskRelayPrefix {
		return strings.TrimSpace(c.Query("group"))
	}
	return common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
}

func readCanvasImageTaskBody(c *gin.Context) ([]byte, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, err
	}
	return storage.Bytes()
}

func extractImageTaskModel(body []byte, contentType string) string {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])
	}
	switch mediaType {
	case "", gin.MIMEJSON:
		var payload struct {
			Model string `json:"model"`
		}
		if common.Unmarshal(body, &payload) == nil {
			return strings.TrimSpace(payload.Model)
		}
	case gin.MIMEPOSTForm:
		if values, parseErr := url.ParseQuery(string(body)); parseErr == nil {
			return strings.TrimSpace(values.Get("model"))
		}
	case gin.MIMEMultipartPOSTForm:
		boundary := strings.TrimSpace(params["boundary"])
		if boundary == "" {
			return ""
		}
		reader := multipart.NewReader(bytes.NewReader(body), boundary)
		for {
			part, nextErr := reader.NextPart()
			if errors.Is(nextErr, io.EOF) {
				break
			}
			if nextErr != nil {
				return ""
			}
			if part.FormName() != "model" || part.FileName() != "" {
				_ = part.Close()
				continue
			}
			value, readErr := io.ReadAll(io.LimitReader(part, 4097))
			_ = part.Close()
			if readErr != nil || len(value) > 4096 {
				return ""
			}
			return strings.TrimSpace(string(value))
		}
	}
	return ""
}

func cloneCanvasImageTaskKeys(keys map[string]any) map[string]any {
	next := make(map[string]any, len(keys))
	for key, value := range keys {
		if key == common.KeyBodyStorage || key == common.KeyRequestBody {
			continue
		}
		next[key] = value
	}
	return next
}

func runCanvasImageTaskRelay(relayReq canvasImageTaskRelayRequest) {
	timeout := time.Duration(constant.ImageTaskTimeoutMinutes) * time.Minute
	runCanvasImageTaskRelayWithExecutor(relayReq, timeout, executeCanvasImageRelay)
}

func runCanvasImageTaskRelayWithExecutor(
	relayReq canvasImageTaskRelayRequest,
	timeout time.Duration,
	execute func(canvasImageTaskRelayRequest) (*httptest.ResponseRecorder, int),
) {
	runCanvasImageTaskRelayWithRetryPolicy(
		relayReq,
		timeout,
		execute,
		canvasImageTaskQuotaSyncRetries,
		canvasImageTaskQuotaSyncRetryDelay,
	)
}

func runCanvasImageTaskRelayWithRetryPolicy(
	relayReq canvasImageTaskRelayRequest,
	timeout time.Duration,
	execute func(canvasImageTaskRelayRequest) (*httptest.ResponseRecorder, int),
	quotaSyncRetries int,
	retryDelay func(int) time.Duration,
) {
	task, exists, err := model.GetByTaskId(canvasImageTaskUserID(relayReq.Keys), relayReq.TaskID)
	if err != nil || !exists {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			common.SysError(fmt.Sprintf("canvas image task panic: %v", recovered))
			failCanvasImageTaskWithLog(
				task,
				fmt.Sprintf("image generation failed: %v", recovered),
				nil,
				http.StatusInternalServerError,
				relayReq,
			)
		}
	}()

	now := time.Now().Unix()
	expectedStatus := task.Status
	task.Status = model.TaskStatusInProgress
	task.StartTime = now
	task.UpdatedAt = now
	task.Progress = "10%"
	won, err := task.UpdateWithStatus(expectedStatus)
	if err != nil {
		common.SysError(fmt.Sprintf("failed to start image task %s: %v", task.TaskID, err))
		return
	}
	if !won {
		return
	}

	if timeout > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		relayReq.Context = ctx
	} else {
		relayReq.Context = context.Background()
	}

	baseKeys := cloneCanvasImageTaskKeys(relayReq.Keys)
	var recorder *httptest.ResponseRecorder
	var channelID int
	for attempt := 0; ; attempt++ {
		relayReq.Keys = cloneCanvasImageTaskKeys(baseKeys)
		if attempt > 0 {
			relayReq.Keys[string(constant.ContextKeyAsyncImageTaskQuotaSyncRetry)] = true
		}
		recorder, channelID = execute(relayReq)
		if errors.Is(relayReq.Context.Err(), context.DeadlineExceeded) {
			failCanvasImageTaskWithLog(task, imageTaskTimeoutReason(timeout), nil, http.StatusGatewayTimeout, relayReq)
			return
		}
		if !canvasImageTaskQuotaSyncBlocked(recorder, relayReq.Keys) || attempt >= quotaSyncRetries {
			break
		}

		delay := time.Duration(0)
		if retryDelay != nil {
			delay = retryDelay(attempt)
		}
		logger.LogInfo(relayReq.Context, fmt.Sprintf("image task %s waiting to retry after quota sync", task.TaskID))
		if !waitCanvasImageTaskRetry(relayReq.Context, delay) {
			failCanvasImageTaskWithLog(task, imageTaskTimeoutReason(timeout), nil, http.StatusGatewayTimeout, relayReq)
			return
		}
	}
	finishCanvasImageTaskWithLog(task, channelID, recorder, relayReq)
}

func canvasImageTaskQuotaSyncRetryDelay(retry int) time.Duration {
	return time.Duration(retry+1) * time.Second
}

func canvasImageTaskQuotaSyncBlocked(recorder *httptest.ResponseRecorder, keys map[string]any) bool {
	if recorder == nil {
		return false
	}
	value, ok := keys[string(constant.ContextKeyAsyncImageTaskQuotaSync)]
	return ok && value == true
}

func canvasImageTaskRelayStatusCode(recorder *httptest.ResponseRecorder, keys map[string]any) int {
	if recorder == nil {
		return 0
	}
	if canvasImageTaskQuotaSyncBlocked(recorder, keys) {
		// 客户端错误替换可能把内部 503 改成 2xx/3xx；任务重试与终态
		// 必须继续使用上游调用前记录的内部额度同步状态。
		return http.StatusServiceUnavailable
	}
	return recorder.Code
}

func waitCanvasImageTaskRetry(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return ctx == nil || ctx.Err() == nil
	}
	if ctx == nil {
		time.Sleep(delay)
		return true
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func imageTaskTimeoutReason(timeout time.Duration) string {
	if timeout >= time.Minute && timeout%time.Minute == 0 {
		return fmt.Sprintf("image generation timed out after %d minutes", int(timeout/time.Minute))
	}
	return "image generation timed out"
}

func executeCanvasImageRelay(relayReq canvasImageTaskRelayRequest) (*httptest.ResponseRecorder, int) {
	return executeCanvasImageRelayWithHandler(relayReq, nil)
}

func executeCanvasImageRelayWithHandler(relayReq canvasImageTaskRelayRequest, handler gin.HandlerFunc) (*httptest.ResponseRecorder, int) {
	recorder := httptest.NewRecorder()
	engine := gin.New()
	channelID := 0
	if relayReq.Keys == nil {
		relayReq.Keys = make(map[string]any)
	}
	action := normalizeCanvasImageTaskAction(relayReq.Action)
	relayPrefix := strings.TrimRight(strings.TrimSpace(relayReq.RelayPrefix), "/")
	if relayPrefix == "" {
		relayPrefix = canvasImageTaskRelayPrefix
	}
	targetPath := relayPrefix + "/" + action

	engine.Use(func(c *gin.Context) {
		for key, value := range relayReq.Keys {
			c.Set(key, value)
		}
		defer func() {
			channelID = common.GetContextKeyInt(c, constant.ContextKeyChannelId)
			// Relay 与 Distributor 会在内部上下文补充模型、渠道及分组等
			// 运行字段；同步回任务上下文供最终失败收尾使用。
			for key, value := range c.Keys {
				relayReq.Keys[key] = value
			}
		}()
		c.Next()
	})
	engine.Use(middleware.BodyStorageCleanup())
	if handler != nil {
		engine.POST(targetPath, handler)
	} else {
		engine.POST(targetPath,
			middleware.Distribute(),
			middleware.ModelRequestRateLimit(),
			func(c *gin.Context) {
				Relay(c, types.RelayFormatOpenAIImage)
			},
		)
	}
	targetURL := targetPath
	if relayReq.RawQuery != "" {
		targetURL += "?" + relayReq.RawQuery
	}
	requestContext := relayReq.Context
	if requestContext == nil {
		requestContext = context.Background()
	}
	request := httptest.NewRequest(http.MethodPost, targetURL, bytes.NewReader(relayReq.Body)).WithContext(requestContext)
	request.Header = relayReq.Header.Clone()
	request.ContentLength = int64(len(relayReq.Body))
	engine.ServeHTTP(recorder, request)

	return recorder, channelID
}

func normalizeCanvasImageTaskAction(action string) string {
	normalized, ok := parseImageTaskAction(action)
	if ok {
		return normalized
	}
	return canvasImageTaskActionGenerations
}

func parseImageTaskAction(action string) (string, bool) {
	switch strings.Trim(strings.TrimSpace(action), "/") {
	case "", "generations", canvasImageTaskActionGenerations:
		return canvasImageTaskActionGenerations, true
	case "edits", canvasImageTaskActionEdits:
		return canvasImageTaskActionEdits, true
	default:
		return "", false
	}
}

func finishCanvasImageTask(task *model.Task, channelID int, recorder *httptest.ResponseRecorder) {
	finishCanvasImageTaskWithLog(task, channelID, recorder, canvasImageTaskRelayRequest{})
}

func finishCanvasImageTaskWithLog(
	task *model.Task,
	channelID int,
	recorder *httptest.ResponseRecorder,
	relayReq canvasImageTaskRelayRequest,
) {
	if task == nil {
		return
	}
	if task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure {
		return
	}
	applyCanvasImageTaskRelayMetadata(task, channelID, relayReq.Keys)
	expectedStatus := task.Status
	now := time.Now().Unix()
	task.FinishTime = now
	task.UpdatedAt = now
	task.Progress = "100%"
	if channelID > 0 {
		task.ChannelId = channelID
	}
	if recorder == nil {
		failCanvasImageTaskWithLog(task, "image generation failed: empty relay response", nil, http.StatusInternalServerError, relayReq)
		return
	}

	body := bytes.TrimSpace(recorder.Body.Bytes())
	responseStatusCode := canvasImageTaskRelayStatusCode(recorder, relayReq.Keys)
	if responseStatusCode >= http.StatusOK && responseStatusCode < http.StatusMultipleChoices && len(body) > 0 {
		task.Status = model.TaskStatusSuccess
		task.Data = json.RawMessage(append([]byte(nil), body...))
		task.FailReason = ""
		if _, err := task.UpdateWithStatus(expectedStatus); err != nil {
			common.SysError(fmt.Sprintf("failed to finish image task %s: %v", task.TaskID, err))
		}
		return
	}

	failureStatusCode := responseStatusCode
	if failureStatusCode >= http.StatusOK && failureStatusCode < http.StatusMultipleChoices {
		// 2xx 却没有任何结果正文不是成功响应。使用网关错误记录，避免使用日志出现
		// “任务失败但 status_code=200”这种会误导排障和统计的状态。
		failureStatusCode = http.StatusBadGateway
	}
	failCanvasImageTaskWithLog(task, extractCanvasImageRelayError(body), body, failureStatusCode, relayReq)
}

func failCanvasImageTask(task *model.Task, reason string, body []byte) {
	failCanvasImageTaskWithLog(task, reason, body, 0, canvasImageTaskRelayRequest{})
}

func failCanvasImageTaskWithLog(
	task *model.Task,
	reason string,
	body []byte,
	statusCode int,
	relayReq canvasImageTaskRelayRequest,
) {
	if task == nil {
		return
	}
	if task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure {
		return
	}
	applyCanvasImageTaskRelayMetadata(task, task.ChannelId, relayReq.Keys)
	expectedStatus := task.Status
	task.Status = model.TaskStatusFailure
	task.Progress = "100%"
	task.FinishTime = time.Now().Unix()
	task.UpdatedAt = task.FinishTime
	task.FailReason = reason
	if len(body) > 0 {
		task.Data = json.RawMessage(append([]byte(nil), body...))
	}
	won, err := task.UpdateWithStatus(expectedStatus)
	if err != nil {
		common.SysError(fmt.Sprintf("failed to mark image task %s as failed: %v", task.TaskID, err))
		return
	}
	if won {
		recordAsyncImageTaskFailureLog(task, reason, statusCode, relayReq)
	}
}

func recordAsyncImageTaskFailureLog(
	task *model.Task,
	reason string,
	statusCode int,
	relayReq canvasImageTaskRelayRequest,
) {
	if task == nil || !canvasImageTaskContextBool(relayReq.Keys, constant.ContextKeyAsyncImageTask) {
		return
	}

	logContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	for key, value := range relayReq.Keys {
		logContext.Set(key, value)
	}
	requestPath := imageTaskRelayRequestPath(task, relayReq)
	modelName := common.GetContextKeyString(logContext, constant.ContextKeyOriginalModel)
	if modelName == "" {
		modelName = strings.TrimSpace(task.Properties.OriginModelName)
	}
	channelID := task.ChannelId
	if channelID <= 0 {
		channelID = common.GetContextKeyInt(logContext, constant.ContextKeyChannelId)
	}
	group := common.GetContextKeyString(logContext, constant.ContextKeyUsingGroup)
	if group == "" {
		group = strings.TrimSpace(task.Group)
	}

	err := service.RecordImageTaskFailureLog(context.Background(), task, reason, service.ImageTaskFailureLogMetadata{
		StatusCode:        statusCode,
		ChannelId:         channelID,
		ChannelName:       common.GetContextKeyString(logContext, constant.ContextKeyChannelName),
		ChannelType:       common.GetContextKeyInt(logContext, constant.ContextKeyChannelType),
		ModelName:         modelName,
		Group:             group,
		Username:          logContext.GetString("username"),
		TokenName:         logContext.GetString("token_name"),
		TokenId:           common.GetContextKeyInt(logContext, constant.ContextKeyTokenId),
		RequestId:         logContext.GetString(common.RequestIdKey),
		UpstreamRequestId: logContext.GetString(common.UpstreamRequestIdKey),
		RequestIP:         relayReq.RequestIP,
		RequestPath:       requestPath,
		ErrorType:         common.GetContextKeyString(logContext, constant.ContextKeyAsyncImageTaskErrorType),
		ErrorCode:         common.GetContextKeyString(logContext, constant.ContextKeyAsyncImageTaskErrorCode),
		UsedChannels:      append([]string(nil), logContext.GetStringSlice("use_channel")...),
	})
	if err != nil {
		common.SysError(fmt.Sprintf("failed to record image task %s error log: %v", task.TaskID, err))
	}
}

func applyCanvasImageTaskRelayMetadata(task *model.Task, channelID int, keys map[string]any) {
	if task == nil {
		return
	}
	if channelID > 0 {
		task.ChannelId = channelID
	}
	if len(keys) == 0 {
		return
	}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	for key, value := range keys {
		ctx.Set(key, value)
	}
	if modelName := strings.TrimSpace(common.GetContextKeyString(ctx, constant.ContextKeyOriginalModel)); modelName != "" {
		task.Properties.OriginModelName = modelName
	}
	if group := strings.TrimSpace(common.GetContextKeyString(ctx, constant.ContextKeyUsingGroup)); group != "" {
		task.Group = group
	}
	if task.PrivateData.TokenId <= 0 {
		task.PrivateData.TokenId = common.GetContextKeyInt(ctx, constant.ContextKeyTokenId)
	}
	if task.PrivateData.TokenName == "" {
		task.PrivateData.TokenName = ctx.GetString("token_name")
	}
	if task.PrivateData.Username == "" {
		task.PrivateData.Username = ctx.GetString("username")
	}
	if task.PrivateData.RequestId == "" {
		task.PrivateData.RequestId = ctx.GetString(common.RequestIdKey)
	}
	if upstreamRequestId := ctx.GetString(common.UpstreamRequestIdKey); upstreamRequestId != "" {
		task.PrivateData.UpstreamRequestId = upstreamRequestId
	}
}

func canvasImageTaskContextBool(keys map[string]any, key constant.ContextKey) bool {
	if len(keys) == 0 {
		return false
	}
	value, ok := keys[string(key)].(bool)
	return ok && value
}

func imageTaskRelayRequestPath(task *model.Task, relayReq canvasImageTaskRelayRequest) string {
	prefix := strings.TrimRight(strings.TrimSpace(relayReq.RelayPrefix), "/")
	if prefix == "" {
		if task != nil && task.Platform == constant.TaskPlatformImage {
			prefix = apiImageTaskRelayPrefix
		} else {
			prefix = canvasImageTaskRelayPrefix
		}
	}
	action := normalizeCanvasImageTaskAction(relayReq.Action)
	if strings.TrimSpace(relayReq.Action) == "" && task != nil {
		action = normalizeCanvasImageTaskAction(task.Action)
	}
	return prefix + "/" + action
}

func extractCanvasImageRelayError(body []byte) string {
	if len(bytes.TrimSpace(body)) == 0 {
		return "image generation failed"
	}
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := common.Unmarshal(body, &payload); err == nil && strings.TrimSpace(payload.Error.Message) != "" {
		return payload.Error.Message
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		return "image generation failed"
	}
	return common.LocalLogPreview(text)
}

func buildCanvasImageTaskResponse(task *model.Task) gin.H {
	return buildImageTaskResponse(task, canvasImageTaskRelayPrefix)
}

func buildAPIImageTaskResponse(task *model.Task) gin.H {
	return buildImageTaskResponse(task, apiImageTaskRelayPrefix)
}

func buildImageTaskResponse(task *model.Task, responsePrefix string) gin.H {
	response := gin.H{
		"task_id":  task.TaskID,
		"status":   mapCanvasImageTaskStatus(task.Status),
		"progress": task.Progress,
	}
	if expiresAt, ok := imageTaskDataExpiresAt(task); ok {
		response["expires_at"] = expiresAt
	}
	expired := imageTaskDataExpired(task, time.Now().Unix())
	if task.Status == model.TaskStatusSuccess && !expired && len(bytes.TrimSpace(task.Data)) > 0 {
		response["result"] = buildImageTaskResult(task, responsePrefix)
	}
	if task.Status == model.TaskStatusSuccess && (expired || len(bytes.TrimSpace(task.Data)) == 0) {
		response["result_expired"] = true
	}
	if task.Status == model.TaskStatusFailure {
		response["error"] = task.FailReason
	}
	return response
}

func imageTaskDataExpiresAt(task *model.Task) (int64, bool) {
	retentionHours := common.GetImageTaskDataRetentionHours()
	if retentionHours <= 0 || task.FinishTime <= 0 {
		return 0, false
	}
	return task.FinishTime + int64(retentionHours)*int64(time.Hour/time.Second), true
}

func imageTaskDataExpired(task *model.Task, nowUnix int64) bool {
	expiresAt, ok := imageTaskDataExpiresAt(task)
	return ok && nowUnix >= expiresAt
}

func imageTaskContentMaxAge(task *model.Task, nowUnix int64) int64 {
	expiresAt, ok := imageTaskDataExpiresAt(task)
	if !ok {
		return int64((24 * time.Hour) / time.Second)
	}
	remaining := expiresAt - nowUnix
	if remaining < 0 {
		return 0
	}
	return remaining
}

func buildCanvasImageTaskResult(task *model.Task) gin.H {
	return buildImageTaskResult(task, canvasImageTaskRelayPrefix)
}

func buildImageTaskResult(task *model.Task, responsePrefix string) gin.H {
	var payload struct {
		Created any `json:"created,omitempty"`
		Data    []struct {
			URL           string `json:"url,omitempty"`
			B64JSON       string `json:"b64_json,omitempty"`
			RevisedPrompt string `json:"revised_prompt,omitempty"`
		} `json:"data"`
	}
	if err := common.Unmarshal(task.Data, &payload); err != nil {
		return gin.H{"data": []gin.H{}}
	}

	items := make([]gin.H, 0, len(payload.Data))
	for index, item := range payload.Data {
		next := gin.H{}
		itemURL := strings.TrimSpace(item.URL)
		switch {
		case strings.TrimSpace(item.B64JSON) != "" || isCanvasImageDataURL(itemURL):
			next["url"] = imageTaskContentPath(responsePrefix, task.TaskID, index)
		case itemURL != "":
			next["url"] = itemURL
		default:
			continue
		}
		if strings.TrimSpace(item.RevisedPrompt) != "" {
			next["revised_prompt"] = item.RevisedPrompt
		}
		items = append(items, next)
	}

	result := gin.H{"data": items}
	if payload.Created != nil {
		result["created"] = payload.Created
	}
	return result
}

func canvasImageTaskContentPath(taskID string, index int) string {
	return imageTaskContentPath(canvasImageTaskRelayPrefix, taskID, index)
}

func imageTaskContentPath(responsePrefix string, taskID string, index int) string {
	responsePrefix = strings.TrimRight(strings.TrimSpace(responsePrefix), "/")
	if responsePrefix == "" {
		responsePrefix = canvasImageTaskRelayPrefix
	}
	return fmt.Sprintf("%s/images/tasks/%s/content/%d", responsePrefix, url.PathEscape(taskID), index)
}

func readCanvasImageTaskContent(task *model.Task, index int) ([]byte, string, error) {
	var payload struct {
		Data []struct {
			URL     string `json:"url,omitempty"`
			B64JSON string `json:"b64_json,omitempty"`
		} `json:"data"`
	}
	if err := common.Unmarshal(task.Data, &payload); err != nil {
		return nil, "", err
	}
	if index < 0 || index >= len(payload.Data) {
		return nil, "", fmt.Errorf("image index out of range")
	}
	item := payload.Data[index]
	value := strings.TrimSpace(item.B64JSON)
	itemURL := strings.TrimSpace(item.URL)
	if value == "" && isCanvasImageDataURL(itemURL) {
		value = itemURL
	}
	if value != "" {
		return decodeCanvasImageData(value)
	}
	if itemURL != "" {
		return downloadImageTaskContent(itemURL)
	}
	return nil, "", fmt.Errorf("empty image data")
}

func downloadImageTaskContent(imageURL string) ([]byte, string, error) {
	mimeType, data, err := service.GetImageFromUrl(imageURL)
	if err != nil {
		return nil, "", err
	}
	image, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		image, err = base64.RawStdEncoding.DecodeString(data)
	}
	if err != nil {
		return nil, "", err
	}
	return image, mimeType, nil
}

func isCanvasImageDataURL(value string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "data:image/")
}

func decodeCanvasImageData(value string) ([]byte, string, error) {
	value = strings.TrimSpace(value)
	declaredMIMEType := ""
	if strings.HasPrefix(value, "data:") {
		parts := strings.SplitN(value, ",", 2)
		if len(parts) != 2 {
			return nil, "", fmt.Errorf("invalid image data url")
		}
		header := strings.TrimPrefix(parts[0], "data:")
		if !strings.Contains(header, ";base64") {
			return nil, "", fmt.Errorf("unsupported image data url")
		}
		declaredMIMEType = strings.TrimSpace(strings.TrimSuffix(header, ";base64"))
		value = parts[1]
	}
	if value == "" {
		return nil, "", fmt.Errorf("empty image data")
	}
	image, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		image, err = base64.RawStdEncoding.DecodeString(value)
	}
	if err != nil {
		return nil, "", err
	}

	detectedContentType := http.DetectContentType(image)
	detectedMIMEType, detectErr := normalizeCanvasImageMIMEType(detectedContentType)
	if declaredMIMEType == "" {
		if detectErr != nil {
			return nil, "", detectErr
		}
		return image, detectedMIMEType, nil
	}

	declaredMIMEType, err = normalizeCanvasImageMIMEType(declaredMIMEType)
	if err != nil {
		return nil, "", err
	}
	if detectErr == nil {
		if declaredMIMEType != detectedMIMEType {
			return nil, "", fmt.Errorf("image MIME type does not match content")
		}
		return image, detectedMIMEType, nil
	}
	// net/http 暂时无法识别部分现代图片格式（例如 AVIF）。只有在内容
	// 未被识别为其他类型时，才接受白名单内的声明类型。
	if detectedContentType == "application/octet-stream" {
		return image, declaredMIMEType, nil
	}
	return nil, "", detectErr
}

func normalizeCanvasImageMIMEType(mimeType string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png":
		return "image/png", nil
	case "image/jpeg", "image/jpg":
		return "image/jpeg", nil
	case "image/webp":
		return "image/webp", nil
	case "image/gif":
		return "image/gif", nil
	case "image/avif":
		return "image/avif", nil
	default:
		return "", fmt.Errorf("unsupported image MIME type")
	}
}

func mapCanvasImageTaskStatus(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusSuccess:
		return "succeeded"
	case model.TaskStatusFailure:
		return "failed"
	case model.TaskStatusInProgress:
		return "processing"
	case model.TaskStatusNotStart, model.TaskStatusQueued, model.TaskStatusSubmitted:
		return "queued"
	default:
		return "processing"
	}
}

func canvasImageTaskUserID(keys map[string]any) int {
	value, ok := keys[string(constant.ContextKeyUserId)]
	if !ok {
		return 0
	}
	id, _ := value.(int)
	return id
}
