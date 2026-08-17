package controller

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var canvasTestPNG = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}

func setupCanvasControllerDB(t *testing.T) {
	t.Helper()
	previousDB := model.DB
	previousType := common.MainDatabaseType()
	previousRedis := common.RedisEnabled
	dsn := fmt.Sprintf("file:canvas-controller-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Task{}))
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
		common.RedisEnabled = previousRedis
	})
}

func createCanvasTestUser(t *testing.T, username, group string) *model.User {
	t.Helper()
	user := &model.User{
		Username: username, Password: "not-used", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: group, AuthVersion: 1,
		AffCode: "canvas-aff-" + username,
	}
	require.NoError(t, model.DB.Create(user).Error)
	return user
}

func withCanvasGroupSettings(t *testing.T) {
	t.Helper()
	previousGroups := setting.UserUsableGroups2JSONString()
	previousRatios := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1}`))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousRatios))
	})
}

func TestCanvasPrepareRequestRequiresAuthorizedSelectedGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	missingRecorder := httptest.NewRecorder()
	missing, _ := gin.CreateTestContext(missingRecorder)
	missing.Request = httptest.NewRequest(http.MethodGet, "/canvas/v1/models", nil)
	CanvasPrepareRequest(missing)
	assert.Equal(t, http.StatusBadRequest, missingRecorder.Code)

	setupCanvasControllerDB(t)
	withCanvasGroupSettings(t)
	user := createCanvasTestUser(t, "canvas-group-user", "default")
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default"}`))
	deniedRecorder := httptest.NewRecorder()
	denied, _ := gin.CreateTestContext(deniedRecorder)
	denied.Request = httptest.NewRequest(http.MethodGet, "/canvas/v1/models?group=vip", nil)
	denied.Set("id", user.Id)
	CanvasPrepareRequest(denied)
	assert.Equal(t, http.StatusForbidden, deniedRecorder.Code)
}

func TestCanvasJSONInjectionOverridesClientGroupAndEnforcesGlobalBound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousLimit := constant.MaxRequestBodyMB
	constant.MaxRequestBodyMB = 1
	t.Cleanup(func() { constant.MaxRequestBodyMB = previousLimit })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/canvas/v1/images/tasks", strings.NewReader(`{"group":"evil","model":"gpt-image-1"}`))
	ctx.Request.Header.Set("Content-Type", gin.MIMEJSON)
	require.NoError(t, injectCanvasGroup(ctx, "vip"))
	storage, err := common.GetBodyStorage(ctx)
	require.NoError(t, err)
	body, err := storage.Bytes()
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(body, &payload))
	assert.Equal(t, "vip", payload["group"])
	common.CleanupBodyStorage(ctx)

	oversized, _ := gin.CreateTestContext(httptest.NewRecorder())
	oversized.Request = httptest.NewRequest(http.MethodPost, "/canvas/v1/images/tasks", strings.NewReader(`{"prompt":"`+strings.Repeat("x", 1<<20)+`"}`))
	oversized.Request.Header.Set("Content-Type", gin.MIMEJSON)
	assert.ErrorIs(t, injectCanvasGroup(oversized, "default"), common.ErrRequestBodyTooLarge)
	common.CleanupBodyStorage(oversized)
}

func TestCanvasMultipartRejectsUnsafeFilenameTypeAndPartCount(t *testing.T) {
	build := func(filename, contentType string, fields int) (*gin.Context, func()) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		for i := 0; i < fields; i++ {
			require.NoError(t, writer.WriteField(fmt.Sprintf("field-%d", i), "value"))
		}
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", `form-data; name="image"; filename="`+filename+`"`)
		header.Set("Content-Type", contentType)
		part, err := writer.CreatePart(header)
		require.NoError(t, err)
		_, err = part.Write(canvasTestPNG)
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodPost, "/canvas/v1/images/tasks", bytes.NewReader(body.Bytes()))
		ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
		return ctx, func() { common.CleanupBodyStorage(ctx) }
	}
	valid, cleanupValid := build("safe.png", "image/png", 0)
	require.NoError(t, injectCanvasGroup(valid, "vip"))
	require.NoError(t, valid.Request.ParseMultipartForm(1<<20))
	assert.Equal(t, "vip", valid.Request.FormValue("group"))
	require.Len(t, valid.Request.MultipartForm.File["image"], 1)
	assert.Equal(t, "safe.png", valid.Request.MultipartForm.File["image"][0].Filename)
	cleanupValid()

	unsafe, cleanupUnsafe := build("../secret.png", "image/png", 0)
	assert.Error(t, injectCanvasGroup(unsafe, "default"))
	cleanupUnsafe()

	badType, cleanupType := build("safe.svg", "image/svg+xml", 0)
	assert.Error(t, injectCanvasGroup(badType, "default"))
	cleanupType()

	tooMany, cleanupParts := build("safe.png", "image/png", canvasMultipartMaxParts)
	assert.Error(t, injectCanvasGroup(tooMany, "default"), "server group plus bounded client parts must not permit an extra file part")
	cleanupParts()
}

func TestImageTaskFetchEnforcesOwnerAndPlatform(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCanvasControllerDB(t)
	task := &model.Task{TaskID: "task-owner-platform", UserId: 1, Platform: constant.TaskPlatformCanvasImage, Status: model.TaskStatusQueued}
	require.NoError(t, task.Insert())

	for _, test := range []struct {
		name     string
		userID   int
		platform constant.TaskPlatform
	}{
		{name: "other user", userID: 2, platform: constant.TaskPlatformCanvasImage},
		{name: "other platform", userID: 1, platform: constant.TaskPlatformImage},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Set("id", test.userID)
			ctx.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
			fetchImageTask(ctx, test.platform, "/v1")
			assert.Equal(t, http.StatusNotFound, recorder.Code)
		})
	}
}

func TestImageTaskContentExpiryIndexTypeAndSecurityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCanvasControllerDB(t)
	previousRetention := common.GetImageTaskDataRetentionHours()
	common.SetImageTaskDataRetentionHours(1)
	t.Cleanup(func() { common.SetImageTaskDataRetentionHours(previousRetention) })
	encoded := base64.StdEncoding.EncodeToString(canvasTestPNG)
	task := &model.Task{
		TaskID: "task-content", UserId: 1, Platform: constant.TaskPlatformCanvasImage,
		Status: model.TaskStatusSuccess, FinishTime: time.Now().Unix(),
		Data: []byte(`{"data":[{"b64_json":"` + encoded + `"}]}`),
	}
	require.NoError(t, task.Insert())
	response := buildImageTaskResponse(task, canvasImageTaskRelayPrefix)
	result, ok := response["result"].(gin.H)
	require.True(t, ok)
	items, ok := result["data"].([]gin.H)
	require.True(t, ok)
	require.Len(t, items, 1)
	assert.Equal(t, "/canvas/v1/images/tasks/task-content/content/0", items[0]["url"])

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Params = gin.Params{{Key: "task_id", Value: task.TaskID}, {Key: "index", Value: "0"}}
	CanvasImageTaskContent(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "image/png", recorder.Header().Get("Content-Type"))
	assert.Contains(t, recorder.Header().Get("Cache-Control"), "private, max-age=")
	assert.Equal(t, "default-src 'none'; sandbox", recorder.Header().Get("Content-Security-Policy"))
	assert.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))

	missing := httptest.NewRecorder()
	missingCtx, _ := gin.CreateTestContext(missing)
	missingCtx.Set("id", 1)
	missingCtx.Params = gin.Params{{Key: "task_id", Value: task.TaskID}, {Key: "index", Value: "1"}}
	CanvasImageTaskContent(missingCtx)
	assert.Equal(t, http.StatusNotFound, missing.Code)

	require.NoError(t, model.DB.Model(&model.Task{}).Where("id = ?", task.ID).Update("finish_time", time.Now().Add(-2*time.Hour).Unix()).Error)
	expired := httptest.NewRecorder()
	expiredCtx, _ := gin.CreateTestContext(expired)
	expiredCtx.Set("id", 1)
	expiredCtx.Params = gin.Params{{Key: "task_id", Value: task.TaskID}, {Key: "index", Value: "0"}}
	CanvasImageTaskContent(expiredCtx)
	assert.Equal(t, http.StatusGone, expired.Code)

	_, _, err := decodeImageData("data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte("<svg/>")))
	assert.Error(t, err)
}

func TestImageTaskCompletionCASAndErrorRedaction(t *testing.T) {
	setupCanvasControllerDB(t)
	stored := &model.Task{TaskID: "task-cas", UserId: 1, Platform: constant.TaskPlatformImage, Status: model.TaskStatusInProgress, Progress: "10%"}
	require.NoError(t, stored.Insert())
	loser := *stored
	require.NoError(t, model.DB.Model(&model.Task{}).Where("id = ?", stored.ID).Updates(map[string]any{
		"status": model.TaskStatusFailure, "progress": "100%", "fail_reason": "image generation timed out",
	}).Error)
	success := httptest.NewRecorder()
	success.Code = http.StatusOK
	_, _ = success.Write([]byte(`{"data":[{"b64_json":"result"}]}`))
	assert.False(t, finishImageTask(&loser, 3, success))
	var after model.Task
	require.NoError(t, model.DB.First(&after, stored.ID).Error)
	assert.EqualValues(t, model.TaskStatusFailure, after.Status)
	assert.Equal(t, "image generation timed out", after.FailReason)

	failed := &model.Task{TaskID: "task-redacted", UserId: 1, Platform: constant.TaskPlatformImage, Status: model.TaskStatusInProgress}
	require.NoError(t, failed.Insert())
	upstream := httptest.NewRecorder()
	upstream.Code = http.StatusBadGateway
	_, _ = upstream.Write([]byte(`{"error":{"message":"secret upstream key sk-sensitive and host internal.local"}}`))
	finishImageTask(failed, 0, upstream)
	after = model.Task{}
	require.NoError(t, model.DB.First(&after, failed.ID).Error)
	assert.Equal(t, "image generation service is temporarily unavailable", after.FailReason)
	assert.Empty(t, after.Data)
	assert.NotContains(t, buildImageTaskResponse(&after, "/v1")["error"], "sensitive")
}
func TestImageTaskRelayBoundsOversizedUpstreamResponse(t *testing.T) {
	setupCanvasControllerDB(t)
	previousLimit := constant.MaxFileDownloadMB
	constant.MaxFileDownloadMB = 1
	t.Cleanup(func() { constant.MaxFileDownloadMB = previousLimit })

	limit := maxImageTaskContentBytes()
	capture := newBoundedImageTaskResponseRecorder(limit)
	upstream := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(strings.Repeat("x", int(limit)+1)))
	})
	upstream.ServeHTTP(capture, httptest.NewRequest(http.MethodGet, "/oversized", nil))
	require.True(t, capture.overflow)
	require.EqualValues(t, limit, capture.Body.Len())

	task := &model.Task{TaskID: "task-oversized-response", UserId: 1, Platform: constant.TaskPlatformImage, Status: model.TaskStatusQueued}
	require.NoError(t, task.Insert())
	previousExecutor := imageTaskRelayExecutor
	imageTaskRelayExecutor = func(imageTaskRelayRequest) imageTaskRelayResult {
		return imageTaskRelayResult{Recorder: capture.ResponseRecorder, ResponseOverflow: capture.overflow}
	}
	t.Cleanup(func() { imageTaskRelayExecutor = previousExecutor })

	runImageTaskRelay(imageTaskRelayRequest{TaskID: task.TaskID, UserID: task.UserId, Platform: task.Platform})
	var stored model.Task
	require.NoError(t, model.DB.Where("task_id = ?", task.TaskID).First(&stored).Error)
	assert.EqualValues(t, model.TaskStatusFailure, stored.Status)
	assert.Equal(t, imageTaskResponseTooLargeReason, stored.FailReason)
	assert.Empty(t, stored.Data)
}

func TestImageTaskSubmitReturnsExactAcceptedSchema(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCanvasControllerDB(t)
	previousExecutor := imageTaskRelayExecutor
	done := make(chan imageTaskRelayRequest, 1)
	imageTaskRelayExecutor = func(request imageTaskRelayRequest) imageTaskRelayResult {
		recorder := httptest.NewRecorder()
		recorder.Code = http.StatusOK
		_, _ = recorder.Write([]byte(`{"data":[]}`))
		done <- request
		return imageTaskRelayResult{Recorder: recorder}
	}
	t.Cleanup(func() { imageTaskRelayExecutor = previousExecutor })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/tasks?action=images/generations", strings.NewReader(`{"model":"gpt-image-1"}`))
	ctx.Request.Header.Set("Content-Type", gin.MIMEJSON)
	ctx.Set("id", 9)
	ctx.Set("username", "canvas-submit-user")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
	ImageTaskSubmit(ctx)
	require.Equal(t, http.StatusAccepted, recorder.Code)
	var response map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Len(t, response, 2)
	assert.Equal(t, "queued", response["status"])
	assert.NotEmpty(t, response["task_id"])
	var replay imageTaskRelayRequest
	select {
	case replay = <-done:
	case <-time.After(time.Second):
		t.Fatal("background relay did not start")
	}
	taskID, ok := response["task_id"].(string)
	require.True(t, ok)
	assert.Equal(t, taskID, replay.Keys[common.RequestIdKey])
	assert.Equal(t, taskID, replay.Header.Get(common.RequestIdKey))
	assert.Equal(t, true, replay.Keys[imageTaskAsyncContextKey])
	require.Eventually(t, func() bool {
		var task model.Task
		return model.DB.Where("task_id = ?", taskID).First(&task).Error == nil && task.Status == model.TaskStatusSuccess
	}, time.Second, 10*time.Millisecond)
	common.CleanupBodyStorage(ctx)
}

func TestImageTaskHeaderAggregateBound(t *testing.T) {
	header := http.Header{"X-Large": {strings.Repeat("x", imageTaskMaxHeaderBytes)}}
	_, err := sanitizedImageTaskHeaders(header)
	assert.Error(t, err)
}

type imageTaskCountingReader struct {
	reads int
}

func (reader *imageTaskCountingReader) Read([]byte) (int, error) {
	reader.reads++
	return 0, io.EOF
}

func resetImageTaskAdmissionTestState() {
	imageTaskAdmissionState.Lock()
	defer imageTaskAdmissionState.Unlock()
	imageTaskAdmissionState.windows = make(map[string][]time.Time)
}

func TestImageTaskAdmissionRejectsBeforeBodyOrTaskAllocation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCanvasControllerDB(t)
	resetImageTaskAdmissionTestState()
	t.Cleanup(resetImageTaskAdmissionTestState)
	for _, user := range []*model.User{
		{Id: 44, Username: "admission-user", AffCode: "admission-user-aff", Status: common.UserStatusEnabled},
		{Id: 45, Username: "admission-token", AffCode: "admission-token-aff", Status: common.UserStatusEnabled},
	} {
		require.NoError(t, model.DB.Create(user).Error)
	}

	for i := range imageTaskAdmissionUserActive {
		task := &model.Task{
			TaskID: fmt.Sprintf("active-admission-%d", i), UserId: 44,
			Platform: constant.TaskPlatformImage, Status: model.TaskStatusInProgress,
			PrivateData: model.TaskPrivateData{TokenId: 55},
		}
		require.NoError(t, task.Insert())
	}
	body := &imageTaskCountingReader{}
	called := false
	router := gin.New()
	router.POST("/v1/images/tasks",
		func(c *gin.Context) { c.Set("id", 44); c.Set("token_id", 55); c.Next() },
		ImageTaskAdmissionGuard(),
		func(c *gin.Context) { called = true; _, _ = io.ReadAll(c.Request.Body) },
	)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/images/tasks", body)
	router.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
	assert.False(t, called)
	assert.Zero(t, body.reads)
	var count int64
	require.NoError(t, model.DB.Model(&model.Task{}).Where("user_id = ?", 44).Count(&count).Error)
	assert.EqualValues(t, imageTaskAdmissionUserActive, count)
	for i := range imageTaskAdmissionTokenActive {
		task := &model.Task{
			TaskID: fmt.Sprintf("active-token-admission-%d", i), UserId: 45,
			Platform: constant.TaskPlatformImage, Status: model.TaskStatusInProgress,
			PrivateData: model.TaskPrivateData{TokenId: 56},
		}
		require.NoError(t, task.Insert())
	}
	tokenBody := &imageTaskCountingReader{}
	tokenCalled := false
	tokenRouter := gin.New()
	tokenRouter.POST("/v1/images/tasks",
		func(c *gin.Context) { c.Set("id", 45); c.Set("token_id", 56); c.Next() },
		ImageTaskAdmissionGuard(),
		func(c *gin.Context) { tokenCalled = true; _, _ = io.ReadAll(c.Request.Body) },
	)
	tokenRecorder := httptest.NewRecorder()
	tokenRouter.ServeHTTP(tokenRecorder, httptest.NewRequest(http.MethodPost, "/v1/images/tasks", tokenBody))
	assert.Equal(t, http.StatusTooManyRequests, tokenRecorder.Code)
	assert.False(t, tokenCalled)
	assert.Zero(t, tokenBody.reads)
}

func TestImageTaskAdmissionRateRejectsBeforeBodyAllocation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCanvasControllerDB(t)
	resetImageTaskAdmissionTestState()
	t.Cleanup(resetImageTaskAdmissionTestState)
	imageTaskAdmissionState.Lock()
	entries := make([]time.Time, imageTaskAdmissionUserRate)
	for i := range entries {
		entries[i] = time.Now()
	}
	imageTaskAdmissionState.windows["user:66"] = entries
	imageTaskAdmissionState.Unlock()

	body := &imageTaskCountingReader{}
	router := gin.New()
	router.POST("/canvas/v1/images/tasks",
		func(c *gin.Context) { c.Set("id", 66); c.Next() },
		ImageTaskAdmissionGuard(),
		func(c *gin.Context) { _, _ = io.ReadAll(c.Request.Body) },
	)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/canvas/v1/images/tasks", body))
	assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
	assert.Zero(t, body.reads)
}
