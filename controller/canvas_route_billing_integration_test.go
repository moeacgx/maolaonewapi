package controller

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const canvasRouteBillingModel = "canvas-route-priced-image"

type canvasRouteObservation struct {
	tokenID          int
	tokenQuotaExempt bool
	usingGroup       string
	path             string
	isPlayground     bool
	trafficSource    string
}

func setupCanvasRouteBillingTest(t *testing.T, upstreamURL string) {
	t.Helper()
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousMemoryCache := common.MemoryCacheEnabled
	previousRetryTimes := common.RetryTimes
	previousLogConsume := common.LogConsumeEnabled
	previousSecret := common.SessionSecret
	previousCryptoSecret := common.CryptoSecret
	previousModelPrices := ratio_setting.ModelPrice2JSONString()
	previousModelRatios := ratio_setting.ModelRatio2JSONString()

	setupCanvasControllerDB(t)
	model.LOG_DB = model.DB
	common.MemoryCacheEnabled = true
	common.RetryTimes = 0
	common.LogConsumeEnabled = true
	common.SessionSecret = "canvas-route-billing-secret"
	common.CryptoSecret = "canvas-route-billing-crypto-secret"
	t.Setenv("CRYPTO_SECRET", "canvas-route-billing-crypto-secret")
	require.NoError(t, model.DB.AutoMigrate(
		&model.Token{},
		&model.UserSession{},
		&model.Group{},
		&model.GroupAlias{},
		&model.Channel{},
		&model.Ability{},
		&model.Log{},
		&model.SubscriptionPlan{},
		&model.UserSubscription{},
		&model.SubscriptionPreConsumeRecord{},
		&model.PromptAuditConfig{},
		&model.PromptAuditEndpoint{},
		&model.PromptAuditJob{},
		&model.PromptAuditEvent{},
		&model.PromptAuditQueueState{},
		&model.RequestArchiveConfig{},
		&model.RequestArchiveTarget{},
		&model.RequestArchiveJob{},
		&model.RequestArchiveQueueState{},
	))
	require.NoError(t, model.EnsurePromptAuditDefaults())
	require.NoError(t, model.EnsureRequestArchiveDefaults())
	withCanvasGroupSettings(t)
	require.NoError(t, model.DB.Create(&model.Group{Code: "default", Name: "Default", Ratio: 1, Status: model.GroupStatusActive, CreatedTime: time.Now().Unix(), UpdatedTime: time.Now().Unix()}).Error)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"canvas-route-priced-image":0.0002}`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{}`))

	priority := int64(0)
	weight := uint(100)
	channel := &model.Channel{
		Type: constant.ChannelTypeOpenAI, Key: "route-upstream-key", Status: common.ChannelStatusEnabled,
		Name: "canvas-route-upstream", Weight: &weight, Models: canvasRouteBillingModel,
		Group: "default", Priority: &priority, BaseURL: &upstreamURL,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group: "default", Model: canvasRouteBillingModel, ChannelId: channel.Id,
		Enabled: true, Priority: &priority, Weight: weight,
	}).Error)
	model.InitChannelCache()

	service.InvalidatePromptAuditConfig()
	service.InvalidateRequestArchiveConfig()
	t.Cleanup(func() {
		service.InvalidatePromptAuditConfig()
		service.InvalidateRequestArchiveConfig()
		common.MemoryCacheEnabled = previousMemoryCache
		common.RetryTimes = previousRetryTimes
		common.LogConsumeEnabled = previousLogConsume
		common.SessionSecret = previousSecret
		common.CryptoSecret = previousCryptoSecret
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousModelPrices))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousModelRatios))
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		if previousMemoryCache && previousDB != nil && previousDB.Migrator().HasTable(&model.Channel{}) && previousDB.Migrator().HasTable(&model.Ability{}) {
			model.InitChannelCache()
		}
	})
}

func createCanvasRouteUserSession(t *testing.T, username, billingPreference string, quota int) (*model.User, *http.Cookie) {
	t.Helper()
	user := &model.User{
		Username: username, Password: "not-used", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, Quota: int64(quota),
		AffCode: "canvas-route-" + username,
	}
	user.SetSetting(dto.UserSetting{BillingPreference: billingPreference})
	require.NoError(t, model.DB.Create(user).Error)
	now := time.Now().Unix()
	identity := service.AuthIdentity{
		UserID: user.Id, SessionID: "canvas-route-" + username,
		UserAuthVersion: user.AuthVersion, SessionVersion: 1,
	}
	require.NoError(t, model.DB.Create(&model.UserSession{
		SID: identity.SessionID, UserID: user.Id, Version: identity.SessionVersion,
		UserAuthVersion: identity.UserAuthVersion, Status: model.UserSessionStatusActive,
		RefreshHash: strings.Repeat("b", 64), LoginMethod: "password",
		CreatedAt: now, LastActiveAt: now, ExpiresAt: now + 3600,
	}).Error)
	return user, issueCanvasBoundaryCookie(t, identity)
}

func newCanvasRouteBillingEngine(observed chan<- canvasRouteObservation) *gin.Engine {
	engine := gin.New()
	engine.Use(middleware.CanvasOriginGuard())
	canvas := engine.Group("/canvas/v1")
	canvas.Use(middleware.DecompressRequestMiddleware(), middleware.BodyStorageCleanup(), middleware.StatsMiddleware())
	canvas.Use(middleware.RouteTag("relay"), middleware.SystemPerformanceCheck(), middleware.UserSessionAuth())
	canvas.POST("/images/tasks", ImageTaskAdmissionGuard(), CanvasPrepareRequest, middleware.PromptAudit(), CanvasImageTaskSubmit)
	prepared := canvas.Group("")
	prepared.Use(CanvasPrepareRequest)
	syncRoute := prepared.Group("")
	syncRoute.Use(middleware.ModelRequestRateLimit(), middleware.PromptAudit(), middleware.Distribute())
	if observed != nil {
		syncRoute.Use(func(c *gin.Context) {
			c.Next()
			observed <- canvasRouteObservation{
				tokenID:          c.GetInt("token_id"),
				tokenQuotaExempt: common.GetContextKeyBool(c, constant.ContextKeyTokenQuotaExempt),
				usingGroup:       common.GetContextKeyString(c, constant.ContextKeyUsingGroup),
				path:             c.Request.URL.Path,
				isPlayground:     strings.HasPrefix(c.Request.URL.Path, "/pg/"),
				trafficSource:    common.GetContextKeyString(c, constant.ContextKeyChannelMetricTrafficSource),
			}
		})
	}
	syncRoute.POST("/images/generations", CanvasImageGenerations)

	syncRoute.POST("/chat/completions", CanvasChatCompletions)
	syncRoute.POST("/audio/speech", CanvasAudioSpeech)
	tokenTasks := engine.Group("/v1/images/tasks")
	tokenTasks.Use(middleware.RelayCORS(), middleware.DecompressRequestMiddleware(), middleware.BodyStorageCleanup(), middleware.StatsMiddleware())
	tokenTasks.Use(middleware.RouteTag("relay"), middleware.SystemPerformanceCheck(), middleware.TokenAuth())
	tokenTasks.POST("", ImageTaskAdmissionGuard(), middleware.PromptAudit(), ImageTaskSubmit)
	normalRelay := engine.Group("/v1")
	normalRelay.Use(middleware.RouteTag("relay"), middleware.SystemPerformanceCheck(), middleware.TokenAuth())
	normalRelay.POST("/images/generations", middleware.PromptAudit(), middleware.Distribute(), middleware.ModelRequestRateLimit(), func(c *gin.Context) {
		Relay(c, relaytypes.RelayFormatOpenAIImage)
	})
	return engine
}

func performCanvasRouteImageRequest(engine *gin.Engine, sessionCookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/canvas/v1/images/generations?group=default", strings.NewReader(
		`{"model":"canvas-route-priced-image","prompt":"draw a route","n":1}`,
	))
	request.Header.Set("Origin", middleware.CanvasConfiguredOrigin())
	request.AddCookie(sessionCookie)
	request.Header.Set("Content-Type", gin.MIMEJSON)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder
}

func TestCanvasRouteSyncRelayUsesWalletAndSubscriptionWithoutToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		upstreamCalls.Add(1)
		assert.Equal(t, "/v1/images/generations", request.URL.Path)
		var payload struct {
			Model string `json:"model"`
		}
		if !assert.NoError(t, common.DecodeJson(request.Body, &payload)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		assert.Equal(t, canvasRouteBillingModel, payload.Model)
		assert.Equal(t, "Bearer route-upstream-key", request.Header.Get("Authorization"))
		w.Header().Set("Content-Type", gin.MIMEJSON)
		_, _ = w.Write([]byte(`{"created":1,"data":[{"url":"https://images.example/result.png"}]}`))
	}))
	defer upstream.Close()
	setupCanvasRouteBillingTest(t, upstream.URL)

	wallet, walletCookie := createCanvasRouteUserSession(t, "wallet", "wallet_only", 1_000)
	subscriptionUser, subscriptionCookie := createCanvasRouteUserSession(t, "subscription", "subscription_only", 1_000)
	now := time.Now().Unix()
	plan := &model.SubscriptionPlan{Title: "Canvas route plan", Enabled: true, TotalAmount: 2_000, QuotaResetPeriod: model.SubscriptionResetNever}
	require.NoError(t, model.DB.Create(plan).Error)
	subscription := &model.UserSubscription{
		UserId: subscriptionUser.Id, PlanId: plan.Id, AmountTotal: 2_000,
		Status: "active", StartTime: now - 60, EndTime: now + 3600,
	}
	require.NoError(t, model.DB.Create(subscription).Error)

	observed := make(chan canvasRouteObservation, 2)
	engine := newCanvasRouteBillingEngine(observed)
	wrongOrigin := httptest.NewRequest(http.MethodPost, "/canvas/v1/images/generations?group=default", strings.NewReader(
		`{"model":"canvas-route-priced-image","prompt":"wrong origin","n":1}`,
	))
	wrongOrigin.Header.Set("Origin", "https://canvas.maolaoapi.com.attacker.example")
	wrongOrigin.AddCookie(walletCookie)
	wrongOrigin.Header.Set("Content-Type", gin.MIMEJSON)
	wrongRecorder := httptest.NewRecorder()
	engine.ServeHTTP(wrongRecorder, wrongOrigin)
	assert.Equal(t, http.StatusForbidden, wrongRecorder.Code)

	walletResponse := performCanvasRouteImageRequest(engine, walletCookie)
	require.Equal(t, http.StatusOK, walletResponse.Code, walletResponse.Body.String())
	subscriptionResponse := performCanvasRouteImageRequest(engine, subscriptionCookie)
	require.Equal(t, http.StatusOK, subscriptionResponse.Code, subscriptionResponse.Body.String())

	var storedWallet model.User
	require.NoError(t, model.DB.Select("quota").First(&storedWallet, wallet.Id).Error)
	assert.Equal(t, int64(900), storedWallet.Quota)
	var storedSubscription model.UserSubscription
	require.NoError(t, model.DB.First(&storedSubscription, subscription.Id).Error)
	assert.EqualValues(t, 100, storedSubscription.AmountUsed)
	var storedSubscriptionUser model.User
	require.NoError(t, model.DB.Select("quota").First(&storedSubscriptionUser, subscriptionUser.Id).Error)
	assert.Equal(t, int64(1_000), storedSubscriptionUser.Quota)
	assert.Equal(t, int32(2), upstreamCalls.Load())

	for range 2 {
		marker := <-observed
		assert.Zero(t, marker.tokenID)
		assert.True(t, marker.tokenQuotaExempt)
		assert.Equal(t, "default", marker.usingGroup)
		assert.Equal(t, "/canvas/v1/images/generations", marker.path)
		assert.False(t, marker.isPlayground)
		assert.Equal(t, "canvas", marker.trafficSource)
	}
	var tokenRows int64
	require.NoError(t, model.DB.Model(&model.Token{}).Count(&tokenRows).Error)
	assert.Zero(t, tokenRows)
	var consumeLogs []model.Log
	require.NoError(t, model.LOG_DB.Where("user_id IN ? AND type = ?", []int{wallet.Id, subscriptionUser.Id}, model.LogTypeConsume).Find(&consumeLogs).Error)
	require.Len(t, consumeLogs, 2)
	for _, log := range consumeLogs {
		assert.Zero(t, log.TokenId)
		assert.Equal(t, canvasRouteBillingModel, log.ModelName)
		assert.Equal(t, "default", log.Group)
		assert.Equal(t, 100, log.Quota)
	}
}

func TestCanvasRoutesForwardNormalizedPathsForSyncProtocols(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamPaths := make(chan string, 3)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		assert.NotContains(t, request.URL.Path, "/canvas/v1")
		upstreamPaths <- request.URL.Path
		switch request.URL.Path {
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", gin.MIMEJSON)
			_, _ = w.Write([]byte(`{"id":"chatcmpl-canvas","object":"chat.completion","created":1,"model":"canvas-route-priced-image","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
		case "/v1/images/generations":
			w.Header().Set("Content-Type", gin.MIMEJSON)
			_, _ = w.Write([]byte(`{"created":1,"data":[{"url":"https://images.example/sync.png"}]}`))
		case "/v1/audio/speech":
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("canvas-audio"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	setupCanvasRouteBillingTest(t, upstream.URL)
	_, sessionCookie := createCanvasRouteUserSession(t, "sync-protocols", "wallet_only", 1_000)
	observed := make(chan canvasRouteObservation, 3)
	engine := newCanvasRouteBillingEngine(observed)

	tests := []struct {
		path string
		body string
	}{
		{path: "/canvas/v1/chat/completions?group=default", body: `{"model":"canvas-route-priced-image","messages":[{"role":"user","content":"hello"}]}`},
		{path: "/canvas/v1/images/generations?group=default", body: `{"model":"canvas-route-priced-image","prompt":"draw","n":1}`},
		{path: "/canvas/v1/audio/speech?group=default", body: `{"model":"canvas-route-priced-image","input":"hello","voice":"alloy"}`},
	}
	for _, testCase := range tests {
		request := httptest.NewRequest(http.MethodPost, testCase.path, strings.NewReader(testCase.body))
		request.Header.Set("Origin", middleware.CanvasConfiguredOrigin())
		request.Header.Set("Content-Type", gin.MIMEJSON)
		request.AddCookie(sessionCookie)
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusOK, recorder.Code, testCase.path+": "+recorder.Body.String())
	}

	require.ElementsMatch(t, []string{"/v1/chat/completions", "/v1/images/generations", "/v1/audio/speech"}, []string{<-upstreamPaths, <-upstreamPaths, <-upstreamPaths})
	for range tests {
		marker := <-observed
		assert.True(t, marker.tokenQuotaExempt)
		assert.False(t, marker.isPlayground)
		assert.Equal(t, "canvas", marker.trafficSource)
	}
}
func TestCanvasRouteAsyncSubmissionReplaysAndSettlesWithoutToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamReached := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "/v1/images/generations", request.URL.Path)
		var payload struct {
			Model string `json:"model"`
		}
		if !assert.NoError(t, common.DecodeJson(request.Body, &payload)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		assert.Equal(t, canvasRouteBillingModel, payload.Model)
		upstreamReached <- struct{}{}
		w.Header().Set("Content-Type", gin.MIMEJSON)
		_, _ = w.Write([]byte(`{"created":1,"data":[{"url":"https://images.example/async.png"}]}`))
	}))
	defer upstream.Close()
	setupCanvasRouteBillingTest(t, upstream.URL)
	user, sessionCookie := createCanvasRouteUserSession(t, "async-wallet", "wallet_only", 1_000)
	resetImageTaskAdmissionTestState()

	previousExecutor := imageTaskRelayExecutor
	defer func() { imageTaskRelayExecutor = previousExecutor }()
	replayObserved := make(chan imageTaskRelayRequest, 1)
	replayMetricSource := make(chan string, 1)
	imageTaskRelayExecutor = func(request imageTaskRelayRequest) imageTaskRelayResult {
		replayObserved <- request
		result := executeImageTaskRelay(request)
		replayMetricSource <- result.TrafficSource
		return result
	}

	engine := newCanvasRouteBillingEngine(nil)
	request := httptest.NewRequest(http.MethodPost, "/canvas/v1/images/tasks?action=generations&group=default", strings.NewReader(
		`{"model":"canvas-route-priced-image","prompt":"draw async","n":1}`,
	))
	request.Header.Set("Origin", middleware.CanvasConfiguredOrigin())
	request.AddCookie(sessionCookie)
	request.Header.Set("Content-Type", gin.MIMEJSON)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())

	var accepted struct {
		TaskID string `json:"task_id"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &accepted))
	require.NotEmpty(t, accepted.TaskID)
	select {
	case replay := <-replayObserved:
		assert.Equal(t, constant.TaskPlatform(constant.TaskPlatformCanvasImage), replay.Platform)
		assert.Equal(t, canvasImageTaskRelayPrefix, replay.RelayPrefix)
		assert.Equal(t, canvasImageTaskActionGenerations, replay.Action)
		assert.True(t, replay.Keys[imageTaskAsyncContextKey].(bool))
		assert.True(t, replay.Keys[string(constant.ContextKeyCanvasTrusted)].(bool))
		assert.True(t, replay.Keys[string(constant.ContextKeyTokenQuotaExempt)].(bool))
		assert.Zero(t, replay.Keys[string(constant.ContextKeyTokenId)])
		assert.Equal(t, "default", replay.Keys[string(constant.ContextKeyUsingGroup)])
	case <-time.After(2 * time.Second):
		t.Fatal("async replay did not start")
	}
	select {
	case <-upstreamReached:
	case <-time.After(2 * time.Second):
		t.Fatal("async replay did not reach upstream")
	}
	select {
	case source := <-replayMetricSource:
		assert.Equal(t, "canvas", source)
	case <-time.After(2 * time.Second):
		t.Fatal("async replay did not record Canvas traffic source")
	}

	require.Eventually(t, func() bool {
		var task model.Task
		if err := model.DB.Where("task_id = ?", accepted.TaskID).First(&task).Error; err != nil {

			return false
		}
		return task.Status == model.TaskStatusSuccess && task.Quota == 100
	}, 3*time.Second, 10*time.Millisecond)
	var task model.Task
	require.NoError(t, model.DB.Where("task_id = ?", accepted.TaskID).First(&task).Error)
	assert.Equal(t, constant.TaskPlatform(constant.TaskPlatformCanvasImage), task.Platform)
	assert.Zero(t, task.PrivateData.TokenId)
	assert.Equal(t, "default", task.Group)
	assert.Equal(t, canvasRouteBillingModel, task.Properties.OriginModelName)
	quota, err := model.GetUserQuota(user.Id, false)
	require.NoError(t, err)
	assert.Equal(t, int64(900), quota)
	var tokenRows int64
	require.NoError(t, model.DB.Model(&model.Token{}).Count(&tokenRows).Error)
	assert.Zero(t, tokenRows)
}
func TestCanvasAsyncImageTaskSubmitReAuditsReplayPromptAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	guardCalls := atomic.Int32{}
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		guardCalls.Add(1)
		w.Header().Set("Content-Type", gin.MIMEJSON)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`))
	}))
	defer guard.Close()
	upstreamReached := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamReached <- struct{}{}
		w.Header().Set("Content-Type", gin.MIMEJSON)
		_, _ = w.Write([]byte(`{"created":1,"data":[{"url":"https://images.example/async.png"}]}`))
	}))
	defer upstream.Close()
	setupCanvasRouteBillingTest(t, upstream.URL)
	cfg, _, err := model.LoadPromptAuditConfig()
	require.NoError(t, err)
	cfg.Enabled = true
	cfg.BlockingEnabled = true
	require.NoError(t, model.SavePromptAuditConfig(cfg.ConfigVersion, cfg, []model.PromptAuditEndpoint{{
		Id: "canvas-async-guard", Name: "Canvas Async Guard", Protocol: "openai_compatible",
		BaseUrl: guard.URL, Model: service.PromptAuditDefaultModel,
		TimeoutMs: 1000, InputLimit: service.PromptAuditDefaultInputLimit, Enabled: true,
	}}))
	service.InvalidatePromptAuditConfig()
	user, sessionCookie := createCanvasRouteUserSession(t, "async-audit-block", "wallet_only", 1_000)
	resetImageTaskAdmissionTestState()

	previousExecutor := imageTaskRelayExecutor
	defer func() { imageTaskRelayExecutor = previousExecutor }()
	replayObserved := make(chan imageTaskRelayRequest, 1)
	replayMetricSource := make(chan string, 1)
	imageTaskRelayExecutor = func(request imageTaskRelayRequest) imageTaskRelayResult {
		replayObserved <- request
		result := executeImageTaskRelay(request)
		replayMetricSource <- result.TrafficSource
		return result
	}

	request := httptest.NewRequest(http.MethodPost, "/canvas/v1/images/tasks?action=generations&group=default", strings.NewReader(
		`{"model":"canvas-route-priced-image","prompt":"draw async","n":1}`,
	))
	request.Header.Set("Origin", middleware.CanvasConfiguredOrigin())
	request.AddCookie(sessionCookie)
	request.Header.Set("Content-Type", gin.MIMEJSON)
	recorder := httptest.NewRecorder()
	newCanvasRouteBillingEngine(nil).ServeHTTP(recorder, request)
	require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())

	var accepted struct {
		TaskID string `json:"task_id"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &accepted))
	require.NotEmpty(t, accepted.TaskID)
	select {
	case replay := <-replayObserved:
		assert.Equal(t, constant.TaskPlatform(constant.TaskPlatformCanvasImage), replay.Platform)
		assert.Equal(t, canvasImageTaskRelayPrefix, replay.RelayPrefix)
		assert.Equal(t, canvasImageTaskActionGenerations, replay.Action)
		assert.True(t, replay.Keys[imageTaskAsyncContextKey].(bool))
		assert.True(t, replay.Keys[string(constant.ContextKeyCanvasTrusted)].(bool))
		assert.True(t, replay.Keys[string(constant.ContextKeyTokenQuotaExempt)].(bool))
		assert.Zero(t, replay.Keys[string(constant.ContextKeyTokenId)])
		assert.Equal(t, "default", replay.Keys[string(constant.ContextKeyUsingGroup)])
	case <-time.After(2 * time.Second):
		t.Fatal("async replay did not start")
	}
	require.Eventually(t, func() bool { return guardCalls.Load() == 2 }, 3*time.Second, 10*time.Millisecond)
	select {
	case <-upstreamReached:
	case <-time.After(2 * time.Second):
		t.Fatal("async replay did not reach upstream")
	}
	select {
	case source := <-replayMetricSource:
		assert.Equal(t, "canvas", source)
	case <-time.After(2 * time.Second):
		t.Fatal("async replay did not record Canvas traffic source")
	}

	require.Eventually(t, func() bool {
		var task model.Task
		if err := model.DB.Where("task_id = ?", accepted.TaskID).First(&task).Error; err != nil {
			return false
		}
		return task.Status == model.TaskStatusSuccess && task.Quota == 100
	}, 3*time.Second, 10*time.Millisecond)
	var task model.Task
	require.NoError(t, model.DB.Where("task_id = ?", accepted.TaskID).First(&task).Error)
	assert.Equal(t, constant.TaskPlatform(constant.TaskPlatformCanvasImage), task.Platform)
	assert.Zero(t, task.PrivateData.TokenId)
	assert.Equal(t, "default", task.Group)
	assert.Equal(t, canvasRouteBillingModel, task.Properties.OriginModelName)
	quota, err := model.GetUserQuota(user.Id, false)
	require.NoError(t, err)
	assert.Equal(t, int64(900), quota)
	var tokenRows int64
	require.NoError(t, model.DB.Model(&model.Token{}).Count(&tokenRows).Error)
	assert.Zero(t, tokenRows)
}

func TestCanvasAsyncImageEditTaskReplaysThroughPromptAuditDistributeAndRelay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	guardCalls := atomic.Int32{}
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		guardCalls.Add(1)
		w.Header().Set("Content-Type", gin.MIMEJSON)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`))
	}))
	defer guard.Close()

	type upstreamObservation struct {
		path          string
		authorization string
		model         string
		prompt        string
		group         string
		imageSize     int64
		err           error
	}
	upstreamObserved := make(chan upstreamObservation, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		observation := upstreamObservation{
			path:          request.URL.Path,
			authorization: request.Header.Get("Authorization"),
		}
		if err := request.ParseMultipartForm(64 << 10); err != nil {
			observation.err = err
		} else {
			observation.model = request.FormValue("model")
			observation.prompt = request.FormValue("prompt")
			observation.group = request.FormValue("group")
			if files := request.MultipartForm.File["image"]; len(files) == 1 {
				observation.imageSize = files[0].Size
			}
		}
		upstreamObserved <- observation
		w.Header().Set("Content-Type", gin.MIMEJSON)
		_, _ = w.Write([]byte(`{"created":1,"data":[{"url":"https://images.example/edited.png"}]}`))
	}))
	defer upstream.Close()

	setupCanvasRouteBillingTest(t, upstream.URL)
	cfg, _, err := model.LoadPromptAuditConfig()
	require.NoError(t, err)
	cfg.Enabled = true
	cfg.BlockingEnabled = true
	require.NoError(t, model.SavePromptAuditConfig(cfg.ConfigVersion, cfg, []model.PromptAuditEndpoint{{
		Id: "canvas-async-edit-guard", Name: "Canvas Async Edit Guard", Protocol: "openai_compatible",
		BaseUrl: guard.URL, Model: service.PromptAuditDefaultModel,
		TimeoutMs: 1000, InputLimit: service.PromptAuditDefaultInputLimit, Enabled: true,
	}}))
	service.InvalidatePromptAuditConfig()
	user, sessionCookie := createCanvasRouteUserSession(t, "async-edit-wallet", "wallet_only", 1_000)
	resetImageTaskAdmissionTestState()

	previousExecutor := imageTaskRelayExecutor
	defer func() { imageTaskRelayExecutor = previousExecutor }()
	replayObserved := make(chan imageTaskRelayRequest, 1)
	replayMetricSource := make(chan string, 1)
	imageTaskRelayExecutor = func(request imageTaskRelayRequest) imageTaskRelayResult {
		replayObserved <- request
		result := executeImageTaskRelay(request)
		replayMetricSource <- result.TrafficSource
		return result
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", canvasRouteBillingModel))
	require.NoError(t, writer.WriteField("prompt", "edit async image"))
	imageHeader := make(textproto.MIMEHeader)
	imageHeader.Set("Content-Disposition", `form-data; name="image"; filename="source.png"`)
	imageHeader.Set("Content-Type", "image/png")
	imagePart, err := writer.CreatePart(imageHeader)
	require.NoError(t, err)
	_, err = imagePart.Write(canvasTestPNG)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	request := httptest.NewRequest(http.MethodPost, "/canvas/v1/images/tasks?action=edits&group=default", bytes.NewReader(body.Bytes()))
	request.Header.Set("Origin", middleware.CanvasConfiguredOrigin())
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.AddCookie(sessionCookie)
	recorder := httptest.NewRecorder()
	newCanvasRouteBillingEngine(nil).ServeHTTP(recorder, request)
	require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())

	var accepted struct {
		TaskID string `json:"task_id"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &accepted))
	require.NotEmpty(t, accepted.TaskID)

	select {
	case replay := <-replayObserved:
		assert.Equal(t, constant.TaskPlatform(constant.TaskPlatformCanvasImage), replay.Platform)
		assert.Equal(t, canvasImageTaskRelayPrefix, replay.RelayPrefix)
		assert.Equal(t, canvasImageTaskActionEdits, replay.Action)
		assert.True(t, replay.Keys[imageTaskAsyncContextKey].(bool))
		assert.True(t, replay.Keys[string(constant.ContextKeyCanvasTrusted)].(bool))
		assert.True(t, replay.Keys[string(constant.ContextKeyTokenQuotaExempt)].(bool))
		assert.Equal(t, "default", replay.Keys[string(constant.ContextKeyUsingGroup)])
	case <-time.After(2 * time.Second):
		t.Fatal("async image edit replay did not start")
	}

	require.Eventually(t, func() bool { return guardCalls.Load() == 2 }, 3*time.Second, 10*time.Millisecond)
	select {
	case observation := <-upstreamObserved:
		require.NoError(t, observation.err)
		assert.Equal(t, "/v1/images/edits", observation.path)
		assert.Equal(t, "Bearer route-upstream-key", observation.authorization)
		assert.Equal(t, canvasRouteBillingModel, observation.model)
		assert.Equal(t, "edit async image", observation.prompt)
		assert.Equal(t, "default", observation.group)
		assert.EqualValues(t, len(canvasTestPNG), observation.imageSize)
	case <-time.After(2 * time.Second):
		t.Fatal("async image edit replay did not reach upstream")
	}
	select {
	case source := <-replayMetricSource:
		assert.Equal(t, "canvas", source)
	case <-time.After(2 * time.Second):
		t.Fatal("async image edit replay did not record Canvas traffic source")
	}

	var channel model.Channel
	require.NoError(t, model.DB.Where("name = ?", "canvas-route-upstream").First(&channel).Error)
	require.Eventually(t, func() bool {
		var task model.Task
		if err := model.DB.Where("task_id = ?", accepted.TaskID).First(&task).Error; err != nil {
			return false
		}
		return task.Status == model.TaskStatusSuccess && task.Quota == 100 && task.ChannelId == channel.Id
	}, 3*time.Second, 10*time.Millisecond)

	var task model.Task
	require.NoError(t, model.DB.Where("task_id = ?", accepted.TaskID).First(&task).Error)
	assert.Equal(t, constant.TaskPlatform(constant.TaskPlatformCanvasImage), task.Platform)
	assert.Equal(t, canvasImageTaskActionEdits, task.Action)
	assert.Equal(t, canvasRouteBillingModel, task.Properties.OriginModelName)
	assert.Equal(t, channel.Id, task.ChannelId)
	assert.Equal(t, 100, task.Quota)
	quota, err := model.GetUserQuota(user.Id, false)
	require.NoError(t, err)
	assert.Equal(t, int64(900), quota)
	var consumeLogs []model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ? AND type = ?", user.Id, model.LogTypeConsume).Find(&consumeLogs).Error)
	require.Len(t, consumeLogs, 1)
	assert.Equal(t, canvasRouteBillingModel, consumeLogs[0].ModelName)
	assert.Equal(t, channel.Id, consumeLogs[0].ChannelId)
	assert.Equal(t, 100, consumeLogs[0].Quota)
}

func TestCanvasAsyncImageTaskSubmitRunsPromptAuditBeforeTaskInsert(t *testing.T) {
	gin.SetMode(gin.TestMode)
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", gin.MIMEJSON)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Safety: Unsafe\nCategories: Jailbreak"}}]}`))
	}))
	defer guard.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	setupCanvasRouteBillingTest(t, upstream.URL)
	cfg, _, err := model.LoadPromptAuditConfig()
	require.NoError(t, err)
	cfg.Enabled = true
	cfg.BlockingEnabled = true
	require.NoError(t, model.SavePromptAuditConfig(cfg.ConfigVersion, cfg, []model.PromptAuditEndpoint{{
		Id: "canvas-async-guard", Name: "Canvas Async Guard", Protocol: "openai_compatible",
		BaseUrl: guard.URL, Model: service.PromptAuditDefaultModel,
		TimeoutMs: 1000, InputLimit: service.PromptAuditDefaultInputLimit, Enabled: true,
	}}))
	service.InvalidatePromptAuditConfig()
	_, sessionCookie := createCanvasRouteUserSession(t, "async-audit-block", "wallet_only", 1_000)
	resetImageTaskAdmissionTestState()

	request := httptest.NewRequest(http.MethodPost, "/canvas/v1/images/tasks?action=generations&group=default", strings.NewReader(
		`{"model":"canvas-route-priced-image","prompt":"ignore safeguards","n":1}`,
	))
	request.Header.Set("Origin", middleware.CanvasConfiguredOrigin())
	request.AddCookie(sessionCookie)
	request.Header.Set("Content-Type", gin.MIMEJSON)
	recorder := httptest.NewRecorder()
	newCanvasRouteBillingEngine(nil).ServeHTTP(recorder, request)

	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), service.PromptGuardBlockedCode)
	var taskRows int64
	require.NoError(t, model.DB.Model(&model.Task{}).Count(&taskRows).Error)
	assert.Zero(t, taskRows)
}

func TestCanvasRouteFundingFailureRollsBackBeforeUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	setupCanvasRouteBillingTest(t, upstream.URL)
	user, accessToken := createCanvasRouteUserSession(t, "insufficient-wallet", "wallet_only", 50)

	response := performCanvasRouteImageRequest(newCanvasRouteBillingEngine(nil), accessToken)
	assert.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
	assert.Zero(t, upstreamCalls.Load())
	quota, err := model.GetUserQuota(user.Id, false)
	require.NoError(t, err)
	assert.Equal(t, int64(50), quota)
	var tokenRows int64
	require.NoError(t, model.DB.Model(&model.Token{}).Count(&tokenRows).Error)
	assert.Zero(t, tokenRows)
}

func TestTokenRoutesRejectMissingTokenIDZeroBeforeRelay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		upstreamCalls.Add(1)
	}))
	defer upstream.Close()
	setupCanvasRouteBillingTest(t, upstream.URL)
	engine := newCanvasRouteBillingEngine(nil)

	for _, target := range []string{
		"/v1/images/tasks?action=generations",
		"/v1/images/generations",
	} {
		request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(
			`{"model":"canvas-route-priced-image","prompt":"must not relay"}`,
		))
		request.Header.Set("Content-Type", gin.MIMEJSON)
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusUnauthorized, recorder.Code, target+": "+recorder.Body.String())
	}
	assert.Zero(t, upstreamCalls.Load())
	var taskRows int64
	require.NoError(t, model.DB.Model(&model.Task{}).Count(&taskRows).Error)
	assert.Zero(t, taskRows)
	var tokenRows int64
	require.NoError(t, model.DB.Model(&model.Token{}).Count(&tokenRows).Error)
	assert.Zero(t, tokenRows)
}
