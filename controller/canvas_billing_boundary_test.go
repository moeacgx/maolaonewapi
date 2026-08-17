package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useCanvasBoundaryRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	server := miniredis.RunT(t)
	previousEnabled := common.RedisEnabled
	previousClient := common.RDB
	common.RedisEnabled = true
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	require.NoError(t, common.RDB.Ping(context.Background()).Err())
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RedisEnabled = previousEnabled
		common.RDB = previousClient
	})
	return server
}

func issueCanvasBoundaryCookie(t *testing.T, identity service.AuthIdentity) *http.Cookie {
	t.Helper()
	accessToken, _, err := service.IssueAccessToken(identity)
	require.NoError(t, err)
	engine := gin.New()
	engine.GET("/api/user/self/groups", middleware.UserSessionAuth(), middleware.IssueCanvasSessionCookie(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/api/user/self/groups", nil)
	request.Header.Set("Authorization", "Bearer "+accessToken)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusNoContent, recorder.Code)
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == middleware.CanvasSessionCookieName {
			return cookie
		}
	}
	t.Fatal("Canvas session cookie was not issued")
	return nil
}

func assertCanvasBoundaryHasNoTokenState(t *testing.T, server *miniredis.Miniredis) {
	t.Helper()
	var tokenRows int64
	require.NoError(t, model.DB.Model(&model.Token{}).Count(&tokenRows).Error)
	assert.Zero(t, tokenRows)
	for _, key := range server.Keys() {
		assert.False(t, strings.HasPrefix(key, "token:"), "unexpected token cache key %q", key)
	}
}

func TestCanvasPreparedAsyncReplayBillsWalletWithoutTokenState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCanvasControllerDB(t)
	withCanvasGroupSettings(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Token{}, &model.UserSession{}))
	server := useCanvasBoundaryRedis(t)
	previousSecret := common.SessionSecret
	common.SessionSecret = "canvas-billing-boundary-secret"
	t.Cleanup(func() { common.SessionSecret = previousSecret })

	user := &model.User{
		Username: "canvas-billing-user", Password: "not-used", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, Quota: 1_000,
		AffCode: "canvas-billing-user-aff",
	}
	user.SetSetting(dto.UserSetting{BillingPreference: "wallet_only"})
	require.NoError(t, model.DB.Create(user).Error)
	now := time.Now().Unix()
	identity := service.AuthIdentity{UserID: user.Id, SessionID: "canvas-billing-session", UserAuthVersion: user.AuthVersion, SessionVersion: 1}
	require.NoError(t, model.DB.Create(&model.UserSession{
		SID: identity.SessionID, UserID: user.Id, Version: identity.SessionVersion,
		UserAuthVersion: user.AuthVersion, Status: model.UserSessionStatusActive,
		RefreshHash: strings.Repeat("a", 64), LoginMethod: "password",
		CreatedAt: now, LastActiveAt: now, ExpiresAt: now + 3600,
	}).Error)
	cookie := issueCanvasBoundaryCookie(t, identity)

	var preparedKeys map[string]any
	var preparedInfo *relaycommon.RelayInfo
	prepareEngine := gin.New()
	prepareEngine.Use(middleware.CanvasOriginGuard(), middleware.BodyStorageCleanup())
	prepareEngine.POST("/canvas/v1/billing-boundary", middleware.UserSessionAuth(), CanvasPrepareRequest, func(c *gin.Context) {
		var err error
		preparedInfo, err = relaycommon.GenRelayInfo(c, relaytypes.RelayFormatOpenAIImage, nil, nil)
		require.NoError(t, err)
		preparedKeys = cloneImageTaskKeys(c.Keys)
		c.Status(http.StatusNoContent)
	})
	prepareRequest := httptest.NewRequest(http.MethodPost, "/canvas/v1/billing-boundary?group=default", strings.NewReader(`{"model":"priced-canvas-model"}`))
	prepareRequest.Header.Set("Origin", middleware.CanvasConfiguredOrigin())
	prepareRequest.Header.Set("Content-Type", gin.MIMEJSON)
	prepareRequest.AddCookie(cookie)
	prepareRecorder := httptest.NewRecorder()
	prepareEngine.ServeHTTP(prepareRecorder, prepareRequest)
	require.Equal(t, http.StatusNoContent, prepareRecorder.Code)
	require.NotNil(t, preparedInfo)
	assert.True(t, preparedInfo.TokenQuotaExempt)
	assert.Zero(t, preparedInfo.TokenId)
	assert.True(t, preparedInfo.IsCanvas)
	assert.False(t, preparedInfo.IsPlayground)
	assert.Equal(t, "/canvas/v1/billing-boundary?group=default", preparedInfo.OriginalRequestURLPath)
	assert.Equal(t, "/v1/billing-boundary?group=default", preparedInfo.RequestURLPath)

	task := &model.Task{
		TaskID: "canvas-billing-async", UserId: user.Id, Platform: constant.TaskPlatformCanvasImage,
		Group: "default", Status: model.TaskStatusQueued, Progress: "0%", SubmitTime: now,
		Properties: model.Properties{OriginModelName: "priced-canvas-model"},
	}
	require.NoError(t, task.Insert())
	preparedKeys[imageTaskAsyncContextKey] = true
	preparedKeys[common.RequestIdKey] = task.TaskID

	previousExecutor := imageTaskRelayExecutor
	t.Cleanup(func() { imageTaskRelayExecutor = previousExecutor })
	var replayInfo *relaycommon.RelayInfo
	var billingErr *relaytypes.NewAPIError
	imageTaskRelayExecutor = func(request imageTaskRelayRequest) imageTaskRelayResult {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodPost, "/canvas/v1/images/generations?group=default", nil)
		for key, value := range request.Keys {
			ctx.Set(key, value)
		}
		common.SetContextKey(ctx, constant.ContextKeyOriginalModel, "priced-canvas-model")
		var err error
		replayInfo, err = relaycommon.GenRelayInfo(ctx, relaytypes.RelayFormatOpenAIImage, nil, nil)
		require.NoError(t, err)
		applyImageTaskAsyncPreConsume(ctx, replayInfo)
		var billing *service.BillingSession
		billing, billingErr = service.NewBillingSession(ctx, replayInfo, 100)
		if billingErr == nil {
			require.NoError(t, billing.Reserve(160))
			require.NoError(t, billing.Settle(120))
		}
		recorder := httptest.NewRecorder()
		recorder.Code = http.StatusOK
		_, _ = recorder.WriteString(`{"data":[]}`)
		return imageTaskRelayResult{Recorder: recorder}
	}
	runImageTaskRelay(imageTaskRelayRequest{
		TaskID: task.TaskID, UserID: user.Id, Platform: constant.TaskPlatformCanvasImage,
		Keys: preparedKeys,
	})

	require.Nil(t, billingErr)
	require.NotNil(t, replayInfo)
	assert.True(t, replayInfo.TokenQuotaExempt)
	assert.True(t, replayInfo.ForcePreConsume)
	assert.True(t, replayInfo.IsCanvas)
	assert.False(t, replayInfo.IsPlayground)
	assert.Equal(t, "/canvas/v1/images/generations?group=default", replayInfo.OriginalRequestURLPath)
	assert.Equal(t, "/v1/images/generations?group=default", replayInfo.RequestURLPath)
	var quota int
	require.Eventually(t, func() bool {
		var loadErr error
		quota, loadErr = model.GetUserQuota(user.Id, false)
		return loadErr == nil && quota == 880
	}, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, 880, quota)
	assertCanvasBoundaryHasNoTokenState(t, server)
}

func TestTokenAuthenticatedImageReplayCannotInheritCanvasExemption(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCanvasControllerDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Token{}))
	user := createCanvasTestUser(t, "token-image-zero", "default")
	require.NoError(t, model.DB.Model(user).Update("quota", 1_000).Error)
	now := time.Now().Unix()
	task := &model.Task{
		TaskID: "token-image-zero", UserId: user.Id, Platform: constant.TaskPlatformImage,
		Group: "default", Status: model.TaskStatusQueued, Progress: "0%", SubmitTime: now,
		PrivateData: model.TaskPrivateData{TokenId: 0},
	}
	require.NoError(t, task.Insert())

	previousExecutor := imageTaskRelayExecutor
	t.Cleanup(func() { imageTaskRelayExecutor = previousExecutor })
	var replayInfo *relaycommon.RelayInfo
	var billingErr *relaytypes.NewAPIError
	imageTaskRelayExecutor = func(request imageTaskRelayRequest) imageTaskRelayResult {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
		for key, value := range request.Keys {
			ctx.Set(key, value)
		}
		common.SetContextKey(ctx, constant.ContextKeyUserId, user.Id)
		common.SetContextKey(ctx, constant.ContextKeyOriginalModel, "priced-image-model")
		common.SetContextKey(ctx, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
		var err error
		replayInfo, err = relaycommon.GenRelayInfo(ctx, relaytypes.RelayFormatOpenAIImage, nil, nil)
		require.NoError(t, err)
		applyImageTaskAsyncPreConsume(ctx, replayInfo)
		_, billingErr = service.NewBillingSession(ctx, replayInfo, 50)
		recorder := httptest.NewRecorder()
		recorder.Code = http.StatusForbidden
		return imageTaskRelayResult{Recorder: recorder}
	}
	runImageTaskRelay(imageTaskRelayRequest{
		TaskID: task.TaskID, UserID: user.Id, Platform: constant.TaskPlatformImage,
		Keys: map[string]any{
			string(constant.ContextKeyTokenQuotaExempt): true,
			string(constant.ContextKeyCanvasTrusted):    true,
			imageTaskAsyncContextKey:                    true,
		},
	})

	require.NotNil(t, replayInfo)
	assert.False(t, replayInfo.TokenQuotaExempt)
	assert.False(t, replayInfo.IsCanvas)
	assert.Equal(t, "/v1/images/generations", replayInfo.RequestURLPath)
	assert.True(t, replayInfo.ForcePreConsume)
	require.NotNil(t, billingErr)
	assert.Equal(t, http.StatusForbidden, billingErr.StatusCode)
	quota, err := model.GetUserQuota(user.Id, false)
	require.NoError(t, err)
	assert.Equal(t, 1_000, quota)
}
