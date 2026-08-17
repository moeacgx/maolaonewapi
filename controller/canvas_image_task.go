package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

const (
	canvasImageTaskActionGenerations = "images/generations"
	canvasImageTaskActionEdits       = "images/edits"
	canvasImageTaskRelayPrefix       = "/canvas/v1"
	apiImageTaskRelayPrefix          = "/v1"
	imageTaskMaxHeaderBytes          = 32 << 10
	imageTaskResponseTooLargeReason  = "image generation response exceeded size limit"
	imageTaskAsyncContextKey         = "image_task_async"
	imageTaskAdmissionWindow         = time.Minute
	imageTaskAdmissionUserRate       = 12
	imageTaskAdmissionTokenRate      = 8
	imageTaskAdmissionUserActive     = 8
	imageTaskAdmissionTokenActive    = 4
)

const (
	imageTaskAdmissionInsertContextKey = "image_task_admission_insert"
)

type imageTaskRelayRequest struct {
	TaskID      string
	UserID      int
	Platform    constant.TaskPlatform
	Action      string
	RelayPrefix string
	Body        common.BodyStorage
	Header      http.Header
	RawQuery    string
	Keys        map[string]any
	Context     context.Context
}

type imageTaskRelayResult struct {
	Recorder         *httptest.ResponseRecorder
	ChannelID        int
	TrafficSource    string
	ResponseOverflow bool
}

type boundedImageTaskResponseRecorder struct {
	*httptest.ResponseRecorder
	remaining int64
	overflow  bool
}

func newBoundedImageTaskResponseRecorder(limit int64) *boundedImageTaskResponseRecorder {
	if limit < 0 {
		limit = 0
	}
	return &boundedImageTaskResponseRecorder{ResponseRecorder: httptest.NewRecorder(), remaining: limit}
}

func (recorder *boundedImageTaskResponseRecorder) Write(payload []byte) (int, error) {
	originalLength := len(payload)
	if int64(originalLength) > recorder.remaining {
		recorder.overflow = true
		payload = payload[:recorder.remaining]
	}
	if len(payload) > 0 {
		if _, err := recorder.ResponseRecorder.Write(payload); err != nil {
			return 0, err
		}
		recorder.remaining -= int64(len(payload))
	}
	return originalLength, nil
}

var imageTaskRelayExecutor = executeImageTaskRelay

var imageTaskAdmissionState = struct {
	sync.Mutex
	windows map[string][]time.Time
}{windows: make(map[string][]time.Time)}

// ImageTaskAdmissionGuard is installed before body storage. It takes the
// configured rate decision first, then locks the authenticated user's shared
// database row until the admitted task insert commits. Lock/count/insert is
// therefore atomic across backend nodes and fails closed on database errors.
func ImageTaskAdmissionGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetInt("id")
		tokenID := c.GetInt("token_id")
		if userID <= 0 {
			abortCanvasRequest(c, http.StatusForbidden, "image task identity is incomplete")
			return
		}

		now := time.Now()
		imageTaskAdmissionState.Lock()
		rateAllowed := takeImageTaskAdmissionRate("user:"+strconv.Itoa(userID), imageTaskAdmissionUserRate, now) &&
			(tokenID <= 0 || takeImageTaskAdmissionRate("token:"+strconv.Itoa(tokenID), imageTaskAdmissionTokenRate, now))
		imageTaskAdmissionState.Unlock()
		if !rateAllowed {
			abortCanvasRequest(c, http.StatusTooManyRequests, "image task submissions are temporarily rate limited")
			return
		}

		tx, admitted, err := model.BeginImageTaskAdmission(c.Request.Context(), userID, tokenID, imageTaskAdmissionUserActive, imageTaskAdmissionTokenActive)
		if err != nil {
			abortCanvasRequest(c, http.StatusInternalServerError, "failed to admit image task")
			return
		}
		if !admitted {
			abortCanvasRequest(c, http.StatusTooManyRequests, "too many active image tasks")
			return
		}
		committed := false
		defer func() {
			if !committed {
				_ = tx.Rollback().Error
			}
		}()
		c.Set(imageTaskAdmissionInsertContextKey, func(task *model.Task) error {
			if task == nil {
				return errors.New("image task is nil")
			}
			if err := tx.Create(task).Error; err != nil {
				return err
			}
			if err := tx.Commit().Error; err != nil {
				return err
			}
			committed = true
			return nil
		})
		c.Next()
	}
}

func takeImageTaskAdmissionRate(key string, limit int, now time.Time) bool {
	entries := imageTaskAdmissionState.windows[key]
	cutoff := now.Add(-imageTaskAdmissionWindow)
	first := 0
	for first < len(entries) && !entries[first].After(cutoff) {
		first++
	}
	entries = entries[first:]
	if len(entries) >= limit {
		imageTaskAdmissionState.windows[key] = entries
		return false
	}
	imageTaskAdmissionState.windows[key] = append(entries, now)
	return true
}

func CanvasImageTaskSubmit(c *gin.Context) {
	action, ok := parseImageTaskAction(c.Query("action"))
	if !ok {
		abortCanvasRequest(c, http.StatusBadRequest, "unsupported image task action")
		return
	}
	submitImageTask(c, action, constant.TaskPlatformCanvasImage, canvasImageTaskRelayPrefix)
}

func ImageTaskSubmit(c *gin.Context) {
	action, ok := parseImageTaskAction(c.Query("action"))
	if !ok {
		abortCanvasRequest(c, http.StatusBadRequest, "unsupported image task action")
		return
	}
	submitImageTask(c, action, constant.TaskPlatformImage, apiImageTaskRelayPrefix)
}

func submitImageTask(c *gin.Context, action string, platform constant.TaskPlatform, relayPrefix string) {
	body, err := cloneImageTaskBody(c)
	if err != nil {
		status := http.StatusBadRequest
		if common.IsRequestBodyTooLargeError(err) {
			status = http.StatusRequestEntityTooLarge
		}
		abortCanvasRequest(c, status, "invalid image task request body")
		return
	}

	header, err := sanitizedImageTaskHeaders(c.Request.Header)
	if err != nil {
		_ = body.Close()
		abortCanvasRequest(c, http.StatusBadRequest, "invalid image task request headers")
		return
	}
	modelName := extractImageTaskModel(body, header.Get("Content-Type"))
	now := time.Now().Unix()
	task := &model.Task{
		TaskID:      model.GenerateTaskID(),
		Platform:    platform,
		UserId:      c.GetInt("id"),
		Group:       common.GetContextKeyString(c, constant.ContextKeyUsingGroup),
		Action:      action,
		Status:      model.TaskStatusQueued,
		Progress:    "0%",
		SubmitTime:  now,
		UpdatedAt:   now,
		Properties:  model.Properties{OriginModelName: modelName},
		PrivateData: model.TaskPrivateData{TokenId: c.GetInt("token_id")},
	}
	if task.UserId <= 0 || task.Group == "" {
		_ = body.Close()
		abortCanvasRequest(c, http.StatusForbidden, "image task identity is incomplete")
		return
	}
	insertTask := task.Insert
	if admittedInsert, ok := c.Get(imageTaskAdmissionInsertContextKey); ok {
		if insert, valid := admittedInsert.(func(*model.Task) error); valid {
			insertTask = func() error { return insert(task) }
		}
	}
	if err := insertTask(); err != nil {
		_ = body.Close()
		abortCanvasRequest(c, http.StatusInternalServerError, "failed to create image task")
		return
	}

	keys := cloneImageTaskKeys(c.Keys)
	keys[imageTaskAsyncContextKey] = true
	keys[common.RequestIdKey] = task.TaskID
	header.Set(common.RequestIdKey, task.TaskID)
	relayRequest := imageTaskRelayRequest{
		TaskID:      task.TaskID,
		UserID:      task.UserId,
		Platform:    platform,
		Action:      action,
		RelayPrefix: relayPrefix,
		Body:        body,
		Header:      header,
		RawQuery:    imageTaskRelayRawQuery(c),
		Keys:        keys,
	}
	gopool.Go(func() {
		defer body.Close()
		runImageTaskRelay(relayRequest)
	})

	c.JSON(http.StatusAccepted, gin.H{
		"task_id": task.TaskID,
		"status":  mapImageTaskStatus(task.Status),
	})
}

func CanvasImageTaskFetch(c *gin.Context) {
	fetchImageTask(c, constant.TaskPlatformCanvasImage, canvasImageTaskRelayPrefix)
}

func ImageTaskFetch(c *gin.Context) {
	fetchImageTask(c, constant.TaskPlatformImage, apiImageTaskRelayPrefix)
}

func fetchImageTask(c *gin.Context, platform constant.TaskPlatform, responsePrefix string) {
	task, exists, err := model.GetImageTaskByOwnerPlatform(c.GetInt("id"), strings.TrimSpace(c.Param("task_id")), platform)
	if err != nil {
		abortCanvasRequest(c, http.StatusInternalServerError, "failed to load image task")
		return
	}
	if !exists {
		abortCanvasRequest(c, http.StatusNotFound, "task not found")
		return
	}
	c.JSON(http.StatusOK, buildImageTaskResponse(task, responsePrefix))
}

func CanvasImageTaskContent(c *gin.Context) {
	serveImageTaskContent(c, constant.TaskPlatformCanvasImage)
}

func ImageTaskContent(c *gin.Context) {
	serveImageTaskContent(c, constant.TaskPlatformImage)
}

func serveImageTaskContent(c *gin.Context, platform constant.TaskPlatform) {
	index, err := strconv.Atoi(strings.TrimSpace(c.Param("index")))
	if err != nil || index < 0 {
		abortCanvasRequest(c, http.StatusBadRequest, "invalid image content request")
		return
	}
	task, exists, err := model.GetImageTaskByOwnerPlatform(c.GetInt("id"), strings.TrimSpace(c.Param("task_id")), platform)
	if err != nil {
		abortCanvasRequest(c, http.StatusInternalServerError, "failed to load image task")
		return
	}
	if !exists {
		abortCanvasRequest(c, http.StatusNotFound, "task not found")
		return
	}
	writeImageTaskContent(c, task, index)
}

func writeImageTaskContent(c *gin.Context, task *model.Task, index int) {
	if task.Status != model.TaskStatusSuccess {
		abortCanvasRequest(c, http.StatusConflict, "image task is not completed")
		return
	}
	now := time.Now().Unix()
	if imageTaskDataExpired(task, now) || len(bytes.TrimSpace(task.Data)) == 0 {
		abortCanvasRequest(c, http.StatusGone, "image task data has expired")
		return
	}
	image, contentType, err := readImageTaskContent(task, index)
	if err != nil {
		abortCanvasRequest(c, http.StatusNotFound, "image content not found")
		return
	}
	c.Header("Cache-Control", fmt.Sprintf("private, max-age=%d", imageTaskContentMaxAge(task, now)))
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Disposition", "inline")
	c.Data(http.StatusOK, contentType, image)
}

func cloneImageTaskBody(c *gin.Context) (common.BodyStorage, error) {
	source, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, err
	}
	reader, err := source.NewReader()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return common.CreateBodyStorageFromReader(reader, source.Size(), canvasRequestBodyLimit())
}

func sanitizedImageTaskHeaders(source http.Header) (http.Header, error) {
	total := 0
	for key, values := range source {
		if strings.ContainsAny(key, "\r\n\x00") {
			return nil, errors.New("invalid header name")
		}
		for _, value := range values {
			total += len(key) + len(value)
			if strings.ContainsAny(value, "\r\n\x00") {
				return nil, errors.New("invalid header value")
			}
		}
	}
	if total > imageTaskMaxHeaderBytes {
		return nil, errors.New("request headers are too large")
	}
	header := source.Clone()
	for _, key := range []string{
		"Authorization", "Cookie", "Proxy-Authorization", "X-Goog-Api-Key", "Api-Key", "X-Api-Key",
		"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Connection", "Te", "Trailer", "Transfer-Encoding", "Upgrade",
	} {
		header.Del(key)
	}
	return header, nil
}

func extractImageTaskModel(storage common.BodyStorage, contentType string) string {
	reader, err := storage.NewReader()
	if err != nil {
		return ""
	}
	defer reader.Close()
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return ""
	}
	switch mediaType {
	case "", gin.MIMEJSON:
		var payload struct {
			Model string `json:"model"`
		}
		if common.DecodeJson(reader, &payload) == nil {
			return strings.TrimSpace(payload.Model)
		}
	case gin.MIMEPOSTForm:
		body, readErr := io.ReadAll(io.LimitReader(reader, 64<<10))
		if readErr == nil {
			if values, parseErr := url.ParseQuery(string(body)); parseErr == nil {
				return strings.TrimSpace(values.Get("model"))
			}
		}
	case gin.MIMEMultipartPOSTForm:
		return readCanvasMultipartModel(reader, params["boundary"])
	}
	return ""
}

func cloneImageTaskKeys(keys map[string]any) map[string]any {
	next := make(map[string]any, len(keys))
	for key, value := range keys {
		switch key {
		case common.KeyBodyStorage, common.KeyRequestBody, imageTaskAdmissionInsertContextKey:
			continue
		default:
			next[key] = value
		}
	}
	return next
}

func imageTaskRelayRawQuery(c *gin.Context) string {
	query := c.Request.URL.Query()
	query.Del("action")
	query.Del("group")
	return query.Encode()
}

func runImageTaskRelay(relayRequest imageTaskRelayRequest) {
	task, exists, err := model.GetImageTaskByOwnerPlatform(relayRequest.UserID, relayRequest.TaskID, relayRequest.Platform)
	if err != nil || !exists {
		return
	}
	trustedQuotaKey := string(constant.ContextKeyTokenQuotaExempt)
	trustedCanvasKey := string(constant.ContextKeyCanvasTrusted)
	quotaExempt, quotaExemptWasBool := relayRequest.Keys[trustedQuotaKey].(bool)
	canvasTrusted, canvasTrustedWasBool := relayRequest.Keys[trustedCanvasKey].(bool)
	if task.Platform != constant.TaskPlatformCanvasImage || !quotaExemptWasBool || !quotaExempt {
		delete(relayRequest.Keys, trustedQuotaKey)
	}
	if task.Platform != constant.TaskPlatformCanvasImage || !canvasTrustedWasBool || !canvasTrusted {
		delete(relayRequest.Keys, trustedCanvasKey)
	}

	defer func() {
		if recover() != nil {
			common.SysError("async image task relay panicked")
			_, _ = model.PersistImageTaskBillingFromConsumeLog(context.Background(), task)
			won := failImageTask(task, http.StatusInternalServerError, "image generation failed")
			if won && task.Quota > 0 {
				_ = service.RefundTaskQuota(context.Background(), task, "image generation failed")
			}
		}
	}()

	fromStatus := task.Status
	now := time.Now().Unix()
	task.Status = model.TaskStatusInProgress
	task.StartTime = now
	task.UpdatedAt = now
	task.Progress = "10%"
	won, err := task.UpdateWithStatus(fromStatus)
	if err != nil || !won {
		return
	}

	timeout := common.GetImageTaskTimeout()
	if timeout > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		relayRequest.Context = ctx
	} else {
		relayRequest.Context = context.Background()
	}
	result := imageTaskRelayExecutor(relayRequest)
	_, _ = model.PersistImageTaskBillingFromConsumeLog(context.Background(), task)
	if errors.Is(relayRequest.Context.Err(), context.DeadlineExceeded) {
		won := failImageTask(task, http.StatusGatewayTimeout, "image generation timed out")
		if won && task.Quota > 0 {
			_ = service.RefundTaskQuota(context.Background(), task, "image generation timed out")
		}
		return
	}
	if result.ResponseOverflow {
		won := failImageTask(task, http.StatusBadGateway, imageTaskResponseTooLargeReason)
		if won && task.Quota > 0 {
			_ = service.RefundTaskQuota(context.Background(), task, imageTaskResponseTooLargeReason)
		}
		return
	}
	won = finishImageTask(task, result.ChannelID, result.Recorder)
	if won && task.Status == model.TaskStatusFailure && task.Quota > 0 {
		_ = service.RefundTaskQuota(context.Background(), task, task.FailReason)
	}
}

func executeImageTaskRelay(relayRequest imageTaskRelayRequest) imageTaskRelayResult {
	recorder := newBoundedImageTaskResponseRecorder(maxImageTaskContentBytes())
	engine := gin.New()
	channelID := 0
	trafficSource := ""
	targetPath := strings.TrimRight(relayRequest.RelayPrefix, "/") + "/" + relayRequest.Action

	engine.Use(func(c *gin.Context) {
		for key, value := range relayRequest.Keys {
			c.Set(key, value)
		}
		c.Set(common.KeyBodyStorage, relayRequest.Body)
		defer func() {
			channelID = common.GetContextKeyInt(c, constant.ContextKeyChannelId)
			trafficSource = common.GetContextKeyString(c, constant.ContextKeyChannelMetricTrafficSource)
		}()
		c.Next()
	})
	engine.Use(middleware.BodyStorageCleanup())
	engine.POST(targetPath, middleware.PromptAudit(), middleware.Distribute(), func(c *gin.Context) {
		Relay(c, types.RelayFormatOpenAIImage)
	})

	targetURL := targetPath
	if relayRequest.RawQuery != "" {
		targetURL += "?" + relayRequest.RawQuery
	}
	request := httptest.NewRequest(http.MethodPost, targetURL, common.NewReplayableBodyReader(relayRequest.Body))
	if relayRequest.Context != nil {
		request = request.WithContext(relayRequest.Context)
	}
	request.Header = relayRequest.Header.Clone()
	request.ContentLength = relayRequest.Body.Size()
	engine.ServeHTTP(recorder, request)
	return imageTaskRelayResult{Recorder: recorder.ResponseRecorder, ChannelID: channelID, TrafficSource: trafficSource, ResponseOverflow: recorder.overflow}
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

func finishImageTask(task *model.Task, channelID int, recorder *httptest.ResponseRecorder) bool {
	if task == nil || task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure {
		return false
	}
	fromStatus := task.Status
	now := time.Now().Unix()
	task.FinishTime = now
	task.UpdatedAt = now
	task.Progress = "100%"
	if channelID > 0 {
		task.ChannelId = channelID
	}
	if recorder == nil {
		return failImageTask(task, http.StatusInternalServerError, "image generation failed")
	}
	body := bytes.TrimSpace(recorder.Body.Bytes())
	if recorder.Code >= http.StatusOK && recorder.Code < http.StatusMultipleChoices && validImageTaskResult(body) {
		task.Status = model.TaskStatusSuccess
		task.Data = append(task.Data[:0], body...)
		task.FailReason = ""
		won, _ := task.UpdateWithStatus(fromStatus)
		return won
	}
	status := recorder.Code
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		status = http.StatusBadGateway
	}
	return failImageTask(task, status, maskImageTaskFailure(status))
}

func validImageTaskResult(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	var payload struct {
		Data []any `json:"data"`
	}
	return common.Unmarshal(body, &payload) == nil && payload.Data != nil
}

func failImageTask(task *model.Task, statusCode int, publicReason string) bool {
	if task == nil || task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure {
		return false
	}
	fromStatus := task.Status
	now := time.Now().Unix()
	task.Status = model.TaskStatusFailure
	task.Progress = "100%"
	task.FinishTime = now
	task.UpdatedAt = now
	task.FailReason = publicReason
	task.Data = nil
	won, _ := task.UpdateWithStatus(fromStatus)
	return won
}

func maskImageTaskFailure(statusCode int) string {
	switch {
	case statusCode == http.StatusTooManyRequests:
		return "image generation is temporarily rate limited"
	case statusCode >= 400 && statusCode < 500:
		return "image generation request was rejected"
	case statusCode >= 500:
		return "image generation service is temporarily unavailable"
	default:
		return "image generation failed"
	}
}

func buildImageTaskResponse(task *model.Task, responsePrefix string) gin.H {
	response := gin.H{
		"task_id":  task.TaskID,
		"status":   mapImageTaskStatus(task.Status),
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
	retention := common.GetImageTaskDataRetentionHours()
	if retention <= 0 || task.FinishTime <= 0 {
		return 0, false
	}
	return task.FinishTime + int64(retention)*int64(time.Hour/time.Second), true
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
	if expiresAt <= nowUnix {
		return 0
	}
	return expiresAt - nowUnix
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
	if common.Unmarshal(task.Data, &payload) != nil {
		return gin.H{"data": []gin.H{}}
	}
	items := make([]gin.H, 0, len(payload.Data))
	for index, item := range payload.Data {
		next := gin.H{}
		itemURL := strings.TrimSpace(item.URL)
		switch {
		case strings.TrimSpace(item.B64JSON) != "" || isImageDataURL(itemURL):
			next["url"] = imageTaskContentPath(responsePrefix, task.TaskID, index)
		case itemURL != "":
			next["url"] = itemURL
		default:
			continue
		}
		if prompt := strings.TrimSpace(item.RevisedPrompt); prompt != "" {
			next["revised_prompt"] = prompt
		}
		items = append(items, next)
	}
	result := gin.H{"data": items}
	if payload.Created != nil {
		result["created"] = payload.Created
	}
	return result
}

func imageTaskContentPath(prefix, taskID string, index int) string {
	return strings.TrimRight(prefix, "/") + "/images/tasks/" + url.PathEscape(taskID) + "/content/" + strconv.Itoa(index)
}

func readImageTaskContent(task *model.Task, index int) ([]byte, string, error) {
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
		return nil, "", errors.New("image index out of range")
	}
	item := payload.Data[index]
	value := strings.TrimSpace(item.B64JSON)
	itemURL := strings.TrimSpace(item.URL)
	if value == "" && isImageDataURL(itemURL) {
		value = itemURL
	}
	if value != "" {
		return decodeImageData(value)
	}
	if itemURL == "" {
		return nil, "", errors.New("empty image data")
	}
	contentType, encoded, err := service.GetImageFromUrl(itemURL)
	if err != nil {
		return nil, "", err
	}
	image, normalizedType, err := decodeImageData("data:" + contentType + ";base64," + encoded)
	return image, normalizedType, err
}

func isImageDataURL(value string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "data:image/")
}

func decodeImageData(value string) ([]byte, string, error) {
	value = strings.TrimSpace(value)
	declaredType := ""
	if strings.HasPrefix(strings.ToLower(value), "data:") {
		parts := strings.SplitN(value, ",", 2)
		if len(parts) != 2 {
			return nil, "", errors.New("invalid image data URL")
		}
		header := parts[0][len("data:"):]
		if !strings.HasSuffix(strings.ToLower(header), ";base64") {
			return nil, "", errors.New("unsupported image data URL")
		}
		declaredType = strings.TrimSpace(header[:len(header)-len(";base64")])
		value = parts[1]
	}
	if value == "" || int64(base64.StdEncoding.DecodedLen(len(value))) > maxImageTaskContentBytes() {
		return nil, "", errors.New("image content is empty or too large")
	}
	image, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		image, err = base64.RawStdEncoding.DecodeString(value)
	}
	if err != nil || int64(len(image)) > maxImageTaskContentBytes() {
		return nil, "", errors.New("invalid image encoding")
	}
	detected, detectErr := normalizeImageMIMEType(http.DetectContentType(image))
	if declaredType == "" {
		if detectErr != nil {
			return nil, "", detectErr
		}
		return image, detected, nil
	}
	declared, err := normalizeImageMIMEType(declaredType)
	if err != nil {
		return nil, "", err
	}
	if detectErr == nil && declared != detected {
		return nil, "", errors.New("image MIME type does not match content")
	}
	if detectErr != nil && http.DetectContentType(image) != "application/octet-stream" {
		return nil, "", detectErr
	}
	return image, declared, nil
}

func normalizeImageMIMEType(contentType string) (string, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || len(params) != 0 {
		return "", errors.New("invalid image MIME type")
	}
	switch strings.ToLower(mediaType) {
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
		return "", errors.New("unsupported image MIME type")
	}
}

func maxImageTaskContentBytes() int64 {
	maxMB := constant.MaxFileDownloadMB
	if maxMB <= 0 {
		maxMB = 20
	}
	return int64(maxMB) << 20
}

func mapImageTaskStatus(status model.TaskStatus) string {
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
