package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func requestArchiveControllerTestLocalPath(t *testing.T, components ...string) string {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return filepath.Join(append([]string{base}, components...)...)
}

func TestInjectCanvasGroupIntoJSONBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/canvas/v1/chat/completions?group=vip", strings.NewReader(`{"model":"gpt-4o"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	err := injectCanvasGroup(ctx)
	require.NoError(t, err)

	body, err := io.ReadAll(ctx.Request.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"gpt-4o","group":"vip"}`, string(body))
	require.Equal(t, int64(len(body)), ctx.Request.ContentLength)
}

func TestInjectCanvasGroupIntoMultipartBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-1"))
	require.NoError(t, writer.WriteField("prompt", "test"))
	require.NoError(t, writer.Close())

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/canvas/v1/images/edits?group=vip", bytes.NewReader(body.Bytes()))
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())

	err := injectCanvasGroup(ctx)
	require.NoError(t, err)

	reader, err := ctx.Request.MultipartReader()
	require.NoError(t, err)
	form, err := reader.ReadForm(32 << 20)
	require.NoError(t, err)
	defer form.RemoveAll()

	require.Equal(t, []string{"vip"}, form.Value["group"])
	require.Equal(t, []string{"gpt-image-1"}, form.Value["model"])
	require.NotEmpty(t, ctx.Request.Header.Get("Content-Type"))
	require.NotEqual(t, writer.FormDataContentType(), ctx.Request.Header.Get("Content-Type"))
	require.Greater(t, ctx.Request.ContentLength, int64(0))
}

func TestCanvasPrepareRequestArchivesOriginalJSONBeforeGroupInjection(t *testing.T) {
	db := setupCanvasRequestArchiveTestDB(t)
	original := []byte(` {"model":"gpt-4o","messages":[{"role":"user","content":"keep exact JSON bytes"}]} `)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/canvas/v1/chat/completions",
		func(c *gin.Context) {
			c.Set("id", 1)
			c.Set(common.RequestIdKey, "canvas-json-request")
			c.Next()
		},
		CanvasPrepareRequest,
		func(c *gin.Context) {
			// 后续通用 PromptAudit 仍会调用同一入口，不能产生重复任务。
			middleware.QueueRequestArchive(c)
			storage, err := common.GetBodyStorage(c)
			require.NoError(t, err)
			converted, err := storage.Bytes()
			require.NoError(t, err)
			require.JSONEq(t,
				`{"model":"gpt-4o","messages":[{"role":"user","content":"keep exact JSON bytes"}],"group":"default"}`,
				string(converted))
			c.Status(http.StatusNoContent)
		},
	)
	request := httptest.NewRequest(http.MethodPost,
		"/canvas/v1/chat/completions?group=default&credential=must-not-be-stored", bytes.NewReader(original))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusNoContent, response.Code)

	requireCanvasArchivedBody(t, db, original, "/canvas/v1/chat/completions", "application/json")
}

func TestCanvasPrepareRequestArchivesOriginalMultipartBeforeGroupInjection(t *testing.T) {
	db := setupCanvasRequestArchiveTestDB(t)
	var originalBody bytes.Buffer
	writer := multipart.NewWriter(&originalBody)
	require.NoError(t, writer.SetBoundary("canvas-request-archive-boundary"))
	require.NoError(t, writer.WriteField("model", "gpt-image-1"))
	require.NoError(t, writer.WriteField("prompt", "keep exact multipart bytes"))
	part, err := writer.CreateFormFile("image", "source.png")
	require.NoError(t, err)
	_, err = part.Write([]byte{0x89, 0x50, 0x4e, 0x47, 0x00, 0xff})
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	original := append([]byte(nil), originalBody.Bytes()...)
	originalContentType := writer.FormDataContentType()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/canvas/v1/images/edits",
		func(c *gin.Context) {
			c.Set("id", 1)
			c.Set(common.RequestIdKey, "canvas-multipart-request")
			c.Next()
		},
		CanvasPrepareRequest,
		func(c *gin.Context) {
			middleware.QueueRequestArchive(c)
			form, formErr := c.MultipartForm()
			require.NoError(t, formErr)
			defer form.RemoveAll()
			require.Equal(t, []string{"default"}, form.Value["group"])
			require.Equal(t, []string{"gpt-image-1"}, form.Value["model"])
			require.Len(t, form.File["image"], 1)
			c.Status(http.StatusNoContent)
		},
	)
	request := httptest.NewRequest(http.MethodPost,
		"/canvas/v1/images/edits?group=default&credential=must-not-be-stored", bytes.NewReader(original))
	request.Header.Set("Content-Type", originalContentType)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusNoContent, response.Code)

	requireCanvasArchivedBody(t, db, original, "/canvas/v1/images/edits", originalContentType)
}

func TestBuildCanvasImageTaskResponseReturnsLightweightContentURLs(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_canvas",
		Status:   model.TaskStatusSuccess,
		Progress: "100%",
		Data:     json.RawMessage(`{"data":[{"b64_json":"abc"}]}`),
	}

	response := buildCanvasImageTaskResponse(task)

	require.Equal(t, "task_canvas", response["task_id"])
	require.Equal(t, "succeeded", response["status"])
	result, ok := response["result"].(gin.H)
	require.True(t, ok)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.JSONEq(t, `{"data":[{"url":"/canvas/v1/images/tasks/task_canvas/content/0"}]}`, string(encoded))
	require.NotContains(t, string(encoded), "b64_json")
	require.NotContains(t, string(encoded), "abc")
}

func TestBuildAPIImageTaskResponseReturnsRegularContentURLs(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_api",
		Status:   model.TaskStatusSuccess,
		Progress: "100%",
		Data:     json.RawMessage(`{"data":[{"b64_json":"abc"}]}`),
	}

	response := buildAPIImageTaskResponse(task)

	result, ok := response["result"].(gin.H)
	require.True(t, ok)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.JSONEq(t, `{"data":[{"url":"/v1/images/tasks/task_api/content/0"}]}`, string(encoded))
}

func TestBuildAPIImageTaskResponsePrefersStableContentURLWhenBase64Exists(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_api_with_cdn",
		Status:   model.TaskStatusSuccess,
		Progress: "100%",
		Data:     json.RawMessage(`{"data":[{"url":"https://cdn.example.com/image.png","b64_json":"abc"}]}`),
	}

	response := buildAPIImageTaskResponse(task)

	result, ok := response["result"].(gin.H)
	require.True(t, ok)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.JSONEq(t, `{"data":[{"url":"/v1/images/tasks/task_api_with_cdn/content/0"}]}`, string(encoded))
}

func TestBuildAPIImageTaskResponseMarksExpiredData(t *testing.T) {
	previous := common.GetImageTaskDataRetentionHours()
	common.SetImageTaskDataRetentionHours(1)
	t.Cleanup(func() { common.SetImageTaskDataRetentionHours(previous) })

	task := &model.Task{
		TaskID:     "task_expired",
		Status:     model.TaskStatusSuccess,
		Progress:   "100%",
		FinishTime: time.Now().Add(-2 * time.Hour).Unix(),
		Data:       json.RawMessage(`{"data":[{"b64_json":"abc"}]}`),
	}

	response := buildAPIImageTaskResponse(task)
	require.Equal(t, true, response["result_expired"])
	require.NotContains(t, response, "result")
	require.Equal(t, task.FinishTime+3600, response["expires_at"])
}

func TestCanvasImageTaskContentReturnsStoredBase64Image(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCanvasImageTaskTestDB(t)
	imageBytes := testPNGImageBytes

	require.NoError(t, (&model.Task{
		TaskID:   "task_image",
		Platform: constant.TaskPlatformCanvasImage,
		UserId:   1,
		Status:   model.TaskStatusSuccess,
		Data:     json.RawMessage(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(imageBytes) + `"}]}`),
	}).Insert())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Params = gin.Params{{Key: "task_id", Value: "task_image"}, {Key: "index", Value: "0"}}
	ctx.Request = httptest.NewRequest("GET", "/canvas/v1/images/tasks/task_image/content/0?group=vip", nil)

	CanvasImageTaskContent(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "image/png", recorder.Header().Get("Content-Type"))
	require.Equal(t, imageBytes, recorder.Body.Bytes())
}

func TestImageTaskContentReturnsGoneAfterRetentionExpires(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCanvasImageTaskTestDB(t)
	previous := common.GetImageTaskDataRetentionHours()
	common.SetImageTaskDataRetentionHours(1)
	t.Cleanup(func() { common.SetImageTaskDataRetentionHours(previous) })

	require.NoError(t, (&model.Task{
		TaskID:     "task_expired_content",
		Platform:   constant.TaskPlatformImage,
		UserId:     1,
		Status:     model.TaskStatusSuccess,
		FinishTime: time.Now().Add(-2 * time.Hour).Unix(),
		Data:       json.RawMessage(`{"data":[{"b64_json":"aW1hZ2U="}]}`),
	}).Insert())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Params = gin.Params{{Key: "task_id", Value: "task_expired_content"}, {Key: "index", Value: "0"}}
	ctx.Request = httptest.NewRequest("GET", "/v1/images/tasks/task_expired_content/content/0", nil)

	ImageTaskContent(ctx)

	require.Equal(t, http.StatusGone, recorder.Code)
	require.Contains(t, recorder.Body.String(), "image task data has expired")
}

func TestCanvasImageTaskFetchRejectsOtherUsersTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCanvasImageTaskTestDB(t)

	require.NoError(t, (&model.Task{
		TaskID: "task_other",
		UserId: 2,
		Status: model.TaskStatusSuccess,
		Data:   json.RawMessage(`{"data":[]}`),
	}).Insert())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Params = gin.Params{{Key: "task_id", Value: "task_other"}}
	ctx.Request = httptest.NewRequest("GET", "/canvas/v1/images/tasks/task_other?group=vip", nil)

	CanvasImageTaskFetch(ctx)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Contains(t, recorder.Body.String(), "task not found")
}

func TestImageTaskFetchRejectsNonImageTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCanvasImageTaskTestDB(t)

	require.NoError(t, (&model.Task{
		TaskID:   "task_video",
		Platform: constant.TaskPlatform("video"),
		UserId:   1,
		Status:   model.TaskStatusSuccess,
	}).Insert())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Params = gin.Params{{Key: "task_id", Value: "task_video"}}
	ctx.Request = httptest.NewRequest("GET", "/v1/images/tasks/task_video", nil)

	ImageTaskFetch(ctx)

	require.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestImageTaskFetchAcceptsGenericImageTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCanvasImageTaskTestDB(t)

	require.NoError(t, (&model.Task{
		TaskID:   "task_api_image",
		Platform: constant.TaskPlatformImage,
		UserId:   1,
		Status:   model.TaskStatusQueued,
		Progress: "0%",
	}).Insert())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Params = gin.Params{{Key: "task_id", Value: "task_api_image"}}
	ctx.Request = httptest.NewRequest("GET", "/v1/images/tasks/task_api_image", nil)

	ImageTaskFetch(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"task_id":"task_api_image"`)
}

func TestFinishCanvasImageTaskStoresSuccessfulRelayResponse(t *testing.T) {
	setupCanvasImageTaskTestDB(t)
	recorder := httptest.NewRecorder()
	recorder.WriteHeader(http.StatusOK)
	_, err := recorder.WriteString(`{"data":[{"url":"https://example.com/image.png"}]}`)
	require.NoError(t, err)

	task := &model.Task{TaskID: "task_ok", UserId: 1, Status: model.TaskStatusInProgress}
	require.NoError(t, task.Insert())

	finishCanvasImageTask(task, 12, recorder)

	reloaded, exists, err := model.GetByTaskId(1, "task_ok")
	require.NoError(t, err)
	require.True(t, exists)
	require.EqualValues(t, model.TaskStatusSuccess, reloaded.Status)
	require.Equal(t, "100%", reloaded.Progress)
	require.Equal(t, 12, reloaded.ChannelId)
	require.JSONEq(t, `{"data":[{"url":"https://example.com/image.png"}]}`, string(reloaded.Data))
	require.Empty(t, reloaded.FailReason)
}

func TestFinishCanvasImageTaskDoesNotOverwriteTimedOutTask(t *testing.T) {
	setupCanvasImageTaskTestDB(t)
	task := &model.Task{TaskID: "task_timeout_race", UserId: 1, Status: model.TaskStatusInProgress, Progress: "10%"}
	require.NoError(t, task.Insert())

	timedOut, exists, err := model.GetByTaskId(1, task.TaskID)
	require.NoError(t, err)
	require.True(t, exists)
	timedOut.Status = model.TaskStatusFailure
	timedOut.Progress = "100%"
	timedOut.FailReason = "image generation timed out"
	won, err := timedOut.UpdateWithStatus(model.TaskStatusInProgress)
	require.NoError(t, err)
	require.True(t, won)

	recorder := httptest.NewRecorder()
	recorder.WriteHeader(http.StatusOK)
	_, err = recorder.WriteString(`{"data":[{"url":"https://example.com/late.png"}]}`)
	require.NoError(t, err)
	finishCanvasImageTask(task, 12, recorder)

	reloaded, exists, err := model.GetByTaskId(1, task.TaskID)
	require.NoError(t, err)
	require.True(t, exists)
	require.EqualValues(t, model.TaskStatusFailure, reloaded.Status)
	require.Equal(t, "image generation timed out", reloaded.FailReason)
}

func TestExecuteCanvasImageRelayRoutesEditTasks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-1"))
	require.NoError(t, writer.WriteField("prompt", "edit"))
	part, err := writer.CreateFormFile("image", "source.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("fake image"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	relayReq := canvasImageTaskRelayRequest{
		Action: canvasImageTaskActionEdits,
		Body:   body.Bytes(),
		Header: http.Header{"Content-Type": []string{writer.FormDataContentType()}},
	}

	recorder, _ := executeCanvasImageRelayWithHandler(relayReq, func(c *gin.Context) {
		imageCount := 0
		if form, err := c.MultipartForm(); err == nil && form != nil {
			imageCount = len(form.File["image"])
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "path": c.Request.URL.Path, "imageCount": imageCount})
	})

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"ok":true,"path":"/canvas/v1/images/edits","imageCount":1}`, recorder.Body.String())
}

func TestExecuteAPIImageRelayUsesRegularRequestPath(t *testing.T) {
	relayReq := canvasImageTaskRelayRequest{
		Action:      canvasImageTaskActionGenerations,
		RelayPrefix: apiImageTaskRelayPrefix,
		Body:        []byte(`{"model":"gpt-image-1","prompt":"test"}`),
		Header:      http.Header{"Content-Type": []string{"application/json"}},
	}

	recorder, _ := executeCanvasImageRelayWithHandler(relayReq, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"path": c.Request.URL.Path})
	})

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"path":"/v1/images/generations"}`, recorder.Body.String())
}

func TestExecuteCanvasImageRelayPropagatesTaskCancellation(t *testing.T) {
	requestContext, cancel := context.WithCancel(context.Background())
	cancel()

	relayReq := canvasImageTaskRelayRequest{
		Action:  canvasImageTaskActionGenerations,
		Body:    []byte(`{"model":"grok-imagine-image","prompt":"test"}`),
		Context: requestContext,
	}
	contextCanceled := false

	executeCanvasImageRelayWithHandler(relayReq, func(c *gin.Context) {
		contextCanceled = errors.Is(c.Request.Context().Err(), context.Canceled)
		c.Status(http.StatusGatewayTimeout)
	})

	require.True(t, contextCanceled)
}

func TestRunCanvasImageTaskRelayMarksBlockedTaskFailedAfterTimeout(t *testing.T) {
	setupCanvasImageTaskTestDB(t)
	task := &model.Task{
		TaskID:   "task_blocked_timeout",
		UserId:   1,
		Platform: constant.TaskPlatformImage,
		Status:   model.TaskStatusQueued,
		Progress: "0%",
	}
	require.NoError(t, task.Insert())

	relayReq := canvasImageTaskRelayRequest{
		TaskID: task.TaskID,
		Keys: map[string]any{
			string(constant.ContextKeyUserId): 1,
		},
	}
	runCanvasImageTaskRelayWithExecutor(relayReq, 20*time.Millisecond, func(req canvasImageTaskRelayRequest) (*httptest.ResponseRecorder, int) {
		<-req.Context.Done()
		recorder := httptest.NewRecorder()
		recorder.WriteHeader(http.StatusGatewayTimeout)
		return recorder, 0
	})

	reloaded, exists, err := model.GetByTaskId(1, task.TaskID)
	require.NoError(t, err)
	require.True(t, exists)
	require.EqualValues(t, model.TaskStatusFailure, reloaded.Status)
	require.Equal(t, "100%", reloaded.Progress)
	require.Equal(t, "image generation timed out", reloaded.FailReason)
	require.NotZero(t, reloaded.FinishTime)
}

func TestNormalizeCanvasImageTaskActionAcceptsShortEditAction(t *testing.T) {
	require.Equal(t, canvasImageTaskActionEdits, normalizeCanvasImageTaskAction("edits"))
	require.Equal(t, canvasImageTaskActionEdits, normalizeCanvasImageTaskAction("images/edits"))
	require.Equal(t, canvasImageTaskActionGenerations, normalizeCanvasImageTaskAction(""))
	_, ok := parseImageTaskAction("unsupported")
	require.False(t, ok)
}

func TestImageTaskRelayRawQueryDropsControlAction(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/images/tasks?action=edits&response_format=b64_json", nil)

	require.Equal(t, "response_format=b64_json", imageTaskRelayRawQuery(ctx))
}

func TestAPIImageTaskGroupIgnoresUntrustedQueryOverride(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/images/tasks?group=untrusted", nil)
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "token-group")

	require.Equal(t, "token-group", imageTaskGroup(ctx, apiImageTaskRelayPrefix))
	require.Equal(t, "untrusted", imageTaskGroup(ctx, canvasImageTaskRelayPrefix))
}

func TestImageTaskContentMaxAgeUsesRemainingRetention(t *testing.T) {
	previous := common.GetImageTaskDataRetentionHours()
	common.SetImageTaskDataRetentionHours(1)
	t.Cleanup(func() { common.SetImageTaskDataRetentionHours(previous) })

	task := &model.Task{FinishTime: 1000}
	require.EqualValues(t, 3000, imageTaskContentMaxAge(task, 1600))
}

func setupCanvasImageTaskTestDB(t *testing.T) {
	t.Helper()

	oldDB := model.DB
	oldUsingSQLite := common.UsingSQLite
	t.Cleanup(func() {
		model.DB = oldDB
		common.UsingSQLite = oldUsingSQLite
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	common.UsingSQLite = true
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.Task{}))
}

func setupCanvasRequestArchiveTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := model.DB
	oldSecret := common.CryptoSecret
	oldRedisEnabled := common.RedisEnabled
	oldUsingSQLite := common.UsingSQLite
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "canvas-request-archive.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	model.DB = db
	common.RedisEnabled = false
	common.UsingSQLite = true
	t.Setenv("CRYPTO_SECRET", "stable-canvas-request-archive-test-secret")
	common.CryptoSecret = "stable-canvas-request-archive-test-secret"
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.UserSubscription{},
		&model.RequestArchiveConfig{}, &model.RequestArchiveTarget{},
		&model.RequestArchiveJob{}, &model.RequestArchiveQueueState{},
	))
	require.NoError(t, db.Create(&model.User{
		Id: 1, Username: "canvas-user", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", GroupId: 1,
		Email: "canvas@example.com",
	}).Error)
	require.NoError(t, model.EnsureRequestArchiveDefaults())
	service.InvalidateRequestArchiveConfig()
	config, err := service.GetRequestArchiveConfig(context.Background())
	require.NoError(t, err)
	_, err = service.SaveRequestArchiveConfig(context.Background(), service.RequestArchiveUpdateRequest{
		ExpectedConfigVersion: config.ConfigVersion,
		Enabled:               true,
		ActiveTargetId:        "canvas-archive",
		RetentionDays:         30,
		WorkerCount:           1,
		QueueCapacity:         16,
		MaxBodyBytes:          model.RequestArchiveDefaultMaxBodyBytes,
		QueueMaxBytes:         model.RequestArchiveDefaultQueueMaxBytes,
		Targets: []service.RequestArchiveUpdateTarget{{
			Id: "canvas-archive", Name: "Canvas 归档", Type: model.RequestArchiveTargetLocal,
			Enabled: true, LocalPath: requestArchiveControllerTestLocalPath(t, "archive"),
		}},
	}, 1)
	require.NoError(t, err)

	t.Cleanup(func() {
		service.InvalidateRequestArchiveConfig()
		common.CryptoSecret = oldSecret
		common.RedisEnabled = oldRedisEnabled
		common.UsingSQLite = oldUsingSQLite
		model.DB = oldDB
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func requireCanvasArchivedBody(
	t *testing.T,
	db *gorm.DB,
	original []byte,
	wantPath string,
	wantContentType string,
) {
	t.Helper()
	var jobs []model.RequestArchiveJob
	require.NoError(t, db.Order("id ASC").Find(&jobs).Error)
	require.Len(t, jobs, 1, "Canvas 注入分组及后续审计不得重复归档")
	job := &jobs[0]
	require.Equal(t, http.MethodPost, job.Method)
	require.Equal(t, wantPath, job.Path)
	require.Equal(t, wantContentType, job.ContentType)
	require.NotContains(t, job.Path, "credential")
	plaintext, err := service.DecryptRequestArchivePayload(job)
	require.NoError(t, err)
	require.Equal(t, original, plaintext)
}
