package router

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/extension"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupFeatureRouterAuthTest(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousType := common.MainDatabaseType()
	previousRedis := common.RedisEnabled
	previousSecret := common.SessionSecret
	previousManager := extension.DefaultManager

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.Task{}))
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.SessionSecret = "feature-router-auth-test-secret"

	extensionRoot := t.TempDir()
	extension.DefaultManager = extension.NewManager(extensionRoot)
	require.NoError(t, extension.DefaultManager.Scan())

	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
		common.RedisEnabled = previousRedis
		common.SessionSecret = previousSecret
		extension.DefaultManager = previousManager
	})
	return db
}

func issueFeatureRouterSession(t *testing.T, role int, suffix string) (*model.User, string) {
	t.Helper()
	user := &model.User{
		Username:    "feature-router-" + suffix,
		Password:    "password-placeholder",
		Role:        role,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		AffCode:     "feature-router-aff-" + suffix,
	}
	require.NoError(t, model.DB.Create(user).Error)
	now := time.Now().Unix()
	session := &model.UserSession{
		SID:             "feature-router-session-" + suffix,
		UserID:          user.Id,
		Version:         1,
		UserAuthVersion: user.AuthVersion,
		Status:          model.UserSessionStatusActive,
		RefreshHash:     "feature-router-refresh-" + suffix,
		LoginMethod:     "password",
		CreatedAt:       now,
		LastActiveAt:    now,
		ExpiresAt:       now + 3600,
	}
	require.NoError(t, model.CreateUserSession(session))
	token, _, err := service.IssueAccessToken(service.AuthIdentity{
		UserID:          user.Id,
		SessionID:       session.SID,
		UserAuthVersion: session.UserAuthVersion,
		SessionVersion:  session.Version,
	})
	require.NoError(t, err)
	return user, token
}

func serveFeatureRouterRequest(engine *gin.Engine, method, path, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder
}

func TestExtensionsRouteServesAuthenticatedSidebarAndEnforcesRootBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFeatureRouterAuthTest(t)
	_, userToken := issueFeatureRouterSession(t, common.RoleCommonUser, "user")
	_, rootToken := issueFeatureRouterSession(t, common.RoleRootUser, "root")

	engine := gin.New()
	registerExtensionRoutes(engine.Group("/api"))

	authenticated := serveFeatureRouterRequest(engine, http.MethodGet, "/api/extensions/", userToken)
	require.Equal(t, http.StatusOK, authenticated.Code)
	assert.Contains(t, authenticated.Body.String(), `"success":true`)
	assert.NotEqual(t, http.StatusNotFound, authenticated.Code)

	unauthenticated := serveFeatureRouterRequest(engine, http.MethodGet, "/api/extensions/", "")
	assert.Equal(t, http.StatusUnauthorized, unauthenticated.Code)

	nonRoot := serveFeatureRouterRequest(engine, http.MethodGet, "/api/extension-admin/", userToken)
	assert.Equal(t, http.StatusForbidden, nonRoot.Code)
	nonRootEvent := serveFeatureRouterRequest(engine, http.MethodPost, "/api/extensions/orders/notification-events", userToken)
	assert.Equal(t, http.StatusForbidden, nonRootEvent.Code)

	root := serveFeatureRouterRequest(engine, http.MethodGet, "/api/extension-admin/?all=true", rootToken)
	assert.Equal(t, http.StatusOK, root.Code)
}

func TestFeatureRoutesRejectInsufficientAuthorizationBeforeHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFeatureRouterAuthTest(t)
	_, userToken := issueFeatureRouterSession(t, common.RoleCommonUser, "boundary-user")
	engine := gin.New()
	api := engine.Group("/api")
	registerGameRoutes(api)
	registerNotificationRoutes(api)
	registerChannelAnalyticsRoutes(api)

	assert.Equal(t, http.StatusUnauthorized, serveFeatureRouterRequest(engine, http.MethodGet, "/api/game/wallet", "").Code)
	assert.Equal(t, http.StatusForbidden, serveFeatureRouterRequest(engine, http.MethodGet, "/api/game/admin/predictions", userToken).Code)
	assert.Equal(t, http.StatusForbidden, serveFeatureRouterRequest(engine, http.MethodGet, "/api/notification/bots", userToken).Code)
	assert.Equal(t, http.StatusForbidden, serveFeatureRouterRequest(engine, http.MethodGet, "/api/channel-analytics/summary", userToken).Code)
}
func TestNotificationRoutesRejectOversizedBodiesBeforeStorageWork(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFeatureRouterAuthTest(t)
	_, rootToken := issueFeatureRouterSession(t, common.RoleRootUser, "notification-body-limit")
	engine := gin.New()
	registerNotificationRoutes(engine.Group("/api"))
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/notification/bots",
		strings.NewReader(`{"padding":"`+strings.Repeat("x", 64<<10)+`"}`),
	)
	request.Header.Set("Authorization", "Bearer "+rootToken)
	request.Header.Set("Content-Type", gin.MIMEJSON)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
	assert.JSONEq(t, `{"success":false,"message":"通知请求体不能超过 64 KiB"}`, recorder.Body.String())
}

func TestCanvasImageTaskRouteHidesCrossUserTasks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFeatureRouterAuthTest(t)
	owner, _ := issueFeatureRouterSession(t, common.RoleCommonUser, "canvas-owner")
	_, otherToken := issueFeatureRouterSession(t, common.RoleCommonUser, "canvas-other")
	require.NoError(t, model.DB.Create(&model.Task{
		TaskID:   "canvas-owner-only-task",
		Platform: constant.TaskPlatformCanvasImage,
		UserId:   owner.Id,
		Group:    "default",
		Status:   model.TaskStatusQueued,
	}).Error)

	engine := gin.New()
	engine.Use(middleware.CanvasOriginGuard())
	registerCanvasRelayRoutes(engine)
	request := httptest.NewRequest(http.MethodGet, "/canvas/v1/images/tasks/canvas-owner-only-task?group=default", nil)
	request.Header.Set("Origin", middleware.DefaultCanvasOrigin)
	request.Header.Set("Authorization", "Bearer "+otherToken)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusNotFound, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "canvas-owner-only-task")
}

func TestFeatureRoutesRegisterExactlyOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	registerFeatureAPIRoutes(engine.Group("/api"))
	registerCanvasRelayRoutes(engine)

	expected := map[string]int{
		http.MethodGet + " /api/extensions/":                        0,
		http.MethodGet + " /api/game/wallet":                        0,
		http.MethodPost + " /api/game/admin/predictions/:id/settle": 0,
		http.MethodGet + " /api/notification/deliveries":            0,
		http.MethodGet + " /api/channel-analytics/filters/models":   0,
		http.MethodGet + " /canvas/v1/models":                       0,
		http.MethodPost + " /canvas/v1/images/tasks":                0,
		http.MethodPost + " /v1/images/tasks":                       0,
	}
	for _, route := range engine.Routes() {
		key := route.Method + " " + route.Path
		if _, ok := expected[key]; ok {
			expected[key]++
		}
	}
	for route, count := range expected {
		assert.Equal(t, 1, count, route)
	}
}

func TestAsyncImageTaskRouteUsesDedicatedAdmissionInsteadOfGenericModelLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupFeatureRouterAuthTest(t)
	user := &model.User{
		Id: 720001, Username: "async-rate-isolation", Status: common.UserStatusEnabled,
		Group: "default", AffCode: "async-rate-isolation-aff",
	}
	require.NoError(t, db.Create(user).Error)

	original := setting.GetModelRequestRateLimitSnapshot()
	t.Cleanup(func() {
		total, success := original.GlobalRateLimit()
		require.NoError(t, setting.UpdateModelRequestRateLimitOptions(map[string]string{
			"ModelRequestRateLimitEnabled":         strconv.FormatBool(original.Enabled()),
			"ModelRequestRateLimitDurationMinutes": strconv.Itoa(original.DurationMinutes()),
			"ModelRequestRateLimitCount":           strconv.Itoa(total),
			"ModelRequestRateLimitSuccessCount":    strconv.Itoa(success),
			"ModelRequestRateLimitGroup":           original.GroupJSONString(),
			"ModelRequestRateLimitUserGroup":       original.UserGroupJSONString(),
		}))
	})
	require.NoError(t, setting.UpdateModelRequestRateLimitOptions(map[string]string{
		"ModelRequestRateLimitEnabled":         "true",
		"ModelRequestRateLimitDurationMinutes": "1",
		"ModelRequestRateLimitCount":           "1",
		"ModelRequestRateLimitSuccessCount":    "1",
		"ModelRequestRateLimitGroup":           "{}",
		"ModelRequestRateLimitUserGroup":       "{}",
	}))

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("id", user.Id)
		c.Set("token_id", 720001)
		c.Next()
	})
	registerAsyncImageTaskSubmitRoute(engine.Group("/v1/images/tasks"), "", func(c *gin.Context) {
		c.Status(http.StatusAccepted)
	})

	for range 2 {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/images/tasks", nil))
		require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
	}
}
