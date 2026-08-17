package router

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type securityAuditPermissionResponse struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
}

func TestSecurityAuditAdminRoutesAreRootOnly(t *testing.T) {
	root, admin := setupSecurityAuditRouterTestDB(t)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	routes := []struct {
		method string
		path   string
		call   string
	}{
		{http.MethodGet, "/api/security-audit/config", "/api/security-audit/config"},
		{http.MethodPut, "/api/security-audit/config", "/api/security-audit/config"},
		{http.MethodGet, "/api/security-audit/builtin-policy", "/api/security-audit/builtin-policy"},
		{http.MethodGet, "/api/security-audit/builtin-policy/channels", "/api/security-audit/builtin-policy/channels"},
		{http.MethodGet, "/api/security-audit/builtin-policy/channel-tags", "/api/security-audit/builtin-policy/channel-tags"},
		{http.MethodGet, "/api/security-audit/builtin-policy/groups", "/api/security-audit/builtin-policy/groups"},
		{http.MethodPut, "/api/security-audit/builtin-policy", "/api/security-audit/builtin-policy"},
		{http.MethodPost, "/api/security-audit/endpoints/probe", "/api/security-audit/endpoints/probe"},
		{http.MethodGet, "/api/security-audit/runtime", "/api/security-audit/runtime"},
		{http.MethodGet, "/api/security-audit/request-archive/config", "/api/security-audit/request-archive/config"},
		{http.MethodPut, "/api/security-audit/request-archive/config", "/api/security-audit/request-archive/config"},
		{http.MethodPost, "/api/security-audit/request-archive/targets/probe", "/api/security-audit/request-archive/targets/probe"},
		{http.MethodGet, "/api/security-audit/request-archive/runtime", "/api/security-audit/request-archive/runtime"},
		{http.MethodGet, "/api/security-audit/events", "/api/security-audit/events"},
		{http.MethodGet, "/api/security-audit/events/:id", "/api/security-audit/events/1"},
		{http.MethodDelete, "/api/security-audit/events/:id", "/api/security-audit/events/1"},
		{http.MethodPost, "/api/security-audit/events/batch-delete", "/api/security-audit/events/batch-delete"},
		{http.MethodPost, "/api/security-audit/events/delete-preview", "/api/security-audit/events/delete-preview"},
		{http.MethodPost, "/api/security-audit/events/delete-by-filter", "/api/security-audit/events/delete-by-filter"},
	}
	registered := make(map[string]struct{})
	for _, route := range engine.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}
	_, legacyChannelGroupsRegistered := registered[http.MethodGet+" /api/security-audit/builtin-policy/channel-groups"]
	require.False(t, legacyChannelGroupsRegistered, "不应保留未发布的错误渠道分组接口")

	adminAuthorization := securityAuditAuthorization(t, admin.Id)
	for _, route := range routes {
		_, ok := registered[route.method+" "+route.path]
		require.True(t, ok, "%s %s 未注册", route.method, route.path)

		unauthenticated := httptest.NewRecorder()
		engine.ServeHTTP(unauthenticated, httptest.NewRequest(route.method, route.call, nil))
		require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
		require.Contains(t, unauthenticated.Header().Get("Cache-Control"), "no-store")

		request := httptest.NewRequest(route.method, route.call, nil)
		request.Header.Set("Authorization", adminAuthorization)
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusForbidden, recorder.Code)
		require.Contains(t, recorder.Header().Get("Cache-Control"), "no-store")
		var response securityAuditPermissionResponse
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		require.False(t, response.Success, "%s %s 不应允许普通管理员", route.method, route.call)
		require.Equal(t, "AUTH_INSUFFICIENT_PRIVILEGE", response.Code)
	}

	rootAuthorization := securityAuditAuthorization(t, root.Id)
	request := httptest.NewRequest(http.MethodGet, "/api/security-audit/config", nil)
	request.Header.Set("Authorization", rootAuthorization)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Header().Get("Cache-Control"), "no-store")
	var response securityAuditPermissionResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
}

func TestRequestArchiveProbeDoesNotRequireSecureVerification(t *testing.T) {
	root, _ := setupSecurityAuditRouterTestDB(t)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/security-audit/request-archive/targets/probe",
		strings.NewReader("{"),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", securityAuditAuthorization(t, root.Id))
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Header().Get("Cache-Control"), "no-store")
	var response struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.False(t, response.Success)
	require.Equal(t, "request_archive_invalid_request", response.Code)
}

func TestSecurityAuditConfigurationSavesDoNotRequireSecureVerification(t *testing.T) {
	tests := []struct {
		name string
		path string
		code string
	}{
		{name: "builtin policy", path: "/api/security-audit/builtin-policy", code: "security_audit_builtin_policy_invalid_request"},
		{name: "request archive", path: "/api/security-audit/request-archive/config", code: "request_archive_invalid_request"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, _ := setupSecurityAuditRouterTestDB(t)
			gin.SetMode(gin.TestMode)
			engine := gin.New()
			SetApiRouter(engine)

			request := httptest.NewRequest(http.MethodPut, test.path, strings.NewReader("{"))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", securityAuditAuthorization(t, root.Id))
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Contains(t, recorder.Header().Get("Cache-Control"), "no-store")
			var response struct {
				Success bool   `json:"success"`
				Code    string `json:"code"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.False(t, response.Success)
			require.Equal(t, test.code, response.Code)
		})
	}
}

func TestSecurityAuditRoutesDoNotRequireSecureVerification(t *testing.T) {
	root, _ := setupSecurityAuditRouterTestDB(t)
	oldGlobalRateLimitEnabled := common.GlobalApiRateLimitEnable
	common.GlobalApiRateLimitEnable = false
	t.Cleanup(func() {
		common.GlobalApiRateLimitEnable = oldGlobalRateLimitEnabled
	})
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	routes := []struct {
		method string
		path   string
	}{
		{method: http.MethodPut, path: "/api/security-audit/config"},
		{method: http.MethodPost, path: "/api/security-audit/endpoints/probe"},
		{method: http.MethodGet, path: "/api/security-audit/events/1"},
		{method: http.MethodDelete, path: "/api/security-audit/events/1"},
		{method: http.MethodPost, path: "/api/security-audit/events/batch-delete"},
		{method: http.MethodPost, path: "/api/security-audit/events/delete-preview"},
		{method: http.MethodPost, path: "/api/security-audit/events/delete-by-filter"},
	}
	rootAuthorization := securityAuditAuthorization(t, root.Id)
	for _, route := range routes {
		request := httptest.NewRequest(route.method, route.path, nil)
		request.Header.Set("Authorization", rootAuthorization)
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)

		require.NotEqual(t, http.StatusForbidden, recorder.Code, "%s %s", route.method, route.path)
		require.Contains(t, recorder.Header().Get("Cache-Control"), "no-store")
		var response struct {
			Success bool   `json:"success"`
			Code    string `json:"code"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		require.NotEqual(t, "VERIFICATION_REQUIRED", response.Code)
	}

}

func TestSecurityAuditRoutesApplyNoStoreBeforeGlobalRateLimitAndRootAuth(t *testing.T) {
	root, _ := setupSecurityAuditRouterTestDB(t)
	oldGlobalRateLimitEnabled := common.GlobalApiRateLimitEnable
	oldGlobalRateLimitNum := common.GlobalApiRateLimitNum
	oldGlobalRateLimitDuration := common.GlobalApiRateLimitDuration
	common.GlobalApiRateLimitEnable = true
	common.GlobalApiRateLimitNum = 1
	common.GlobalApiRateLimitDuration = 3600
	t.Cleanup(func() {
		common.GlobalApiRateLimitEnable = oldGlobalRateLimitEnabled
		common.GlobalApiRateLimitNum = oldGlobalRateLimitNum
		common.GlobalApiRateLimitDuration = oldGlobalRateLimitDuration
	})

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)
	const clientAddress = "203.0.113.77:12345"

	unauthorized := httptest.NewRequest(http.MethodPut, "/api/security-audit/config", nil)
	unauthorized.RemoteAddr = clientAddress
	unauthorizedRecorder := httptest.NewRecorder()
	engine.ServeHTTP(unauthorizedRecorder, unauthorized)
	require.Equal(t, http.StatusUnauthorized, unauthorizedRecorder.Code,
		"the first limiter pass must still reach mandatory RootAuth")
	require.Contains(t, unauthorizedRecorder.Header().Get("Cache-Control"), "no-store")

	rateLimited := httptest.NewRequest(http.MethodPut, "/api/security-audit/config", nil)
	rateLimited.RemoteAddr = clientAddress
	rateLimited.Header.Set("Authorization", securityAuditAuthorization(t, root.Id))
	rateLimitedRecorder := httptest.NewRecorder()
	engine.ServeHTTP(rateLimitedRecorder, rateLimited)
	require.Equal(t, http.StatusTooManyRequests, rateLimitedRecorder.Code,
		"the audit subtree must invoke the global API limiter exactly once per request")
	require.Contains(t, rateLimitedRecorder.Header().Get("Cache-Control"), "no-store")
	require.NotEmpty(t, rateLimitedRecorder.Header().Get("Retry-After"))
}

func TestSecurityAuditChannelOptionsContainOnlyRealChannels(t *testing.T) {
	root, _ := setupSecurityAuditRouterTestDB(t)
	baseURL := "https://must-not-be-returned.example.com"
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: 601, Name: "Default URL Channel", Key: "must-not-be-returned", BaseURL: &baseURL,
		Models: "must-not-be-returned", Group: "user-route-group", Status: 1, Type: 1,
		Tag: common.GetPointer("audit-channel-batch"),
	}).Error)
	_, err := model.GetAllChannelOptions()
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	request := httptest.NewRequest(http.MethodGet, "/api/security-audit/builtin-policy/channels", nil)
	request.Header.Set("Authorization", securityAuditAuthorization(t, root.Id))
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var response struct {
		Success bool                     `json:"success"`
		Data    []map[string]interface{} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Len(t, response.Data, 1)
	fields := make([]string, 0, len(response.Data[0]))
	for field := range response.Data[0] {
		fields = append(fields, field)
	}
	require.ElementsMatch(t, []string{"id", "name", "status", "type", "tag"}, fields)
	require.Equal(t, "Default URL Channel", response.Data[0]["name"])
	require.Equal(t, "audit-channel-batch", response.Data[0]["tag"])

	tagRequest := httptest.NewRequest(http.MethodGet, "/api/security-audit/builtin-policy/channel-tags", nil)
	tagRequest.Header.Set("Authorization", securityAuditAuthorization(t, root.Id))
	tagRecorder := httptest.NewRecorder()
	engine.ServeHTTP(tagRecorder, tagRequest)
	require.Equal(t, http.StatusOK, tagRecorder.Code)
	var tagResponse struct {
		Success bool                     `json:"success"`
		Data    []model.ChannelTagOption `json:"data"`
	}
	require.NoError(t, common.Unmarshal(tagRecorder.Body.Bytes(), &tagResponse))
	require.True(t, tagResponse.Success)
	require.Equal(t, []model.ChannelTagOption{{Tag: "audit-channel-batch", ChannelCount: 1}}, tagResponse.Data)
}

func securityAuditAuthorization(t *testing.T, userID int) string {
	t.Helper()
	bundle, err := service.CreateLoginSession(userID, "password", "127.0.0.1", "security-audit-router-test")
	require.NoError(t, err)
	return "Bearer " + bundle.AccessToken
}

func setupSecurityAuditRouterTestDB(t *testing.T) (*model.User, *model.User) {
	t.Helper()
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldRedis := common.RedisEnabled
	oldMainDatabaseType, oldLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()
	oldSessionSecret := common.SessionSecret
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "security-audit-router.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.UserSession{}, &model.Channel{}, &model.PromptAuditConfig{}, &model.PromptAuditEndpoint{},
		&model.PromptAuditJob{}, &model.PromptAuditEvent{}, &model.PromptAuditQueueState{},
		&model.RequestArchiveConfig{}, &model.RequestArchiveTarget{}, &model.RequestArchiveJob{}, &model.RequestArchiveQueueState{},
	))
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.SessionSecret = "security-audit-router-test-secret"
	require.NoError(t, model.EnsurePromptAuditDefaults())
	require.NoError(t, model.EnsureRequestArchiveDefaults())
	service.InvalidatePromptAuditConfig()
	service.InvalidateRequestArchiveConfig()
	root := &model.User{Id: 501, Username: "audit-root", Password: "password123", DisplayName: "Audit Root",
		Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "audit-root-aff"}
	admin := &model.User{Id: 502, Username: "audit-admin", Password: "password123", DisplayName: "Audit Admin",
		Role: common.RoleAdminUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "audit-admin-aff"}
	require.NoError(t, db.Create(root).Error)
	require.NoError(t, db.Create(admin).Error)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.RedisEnabled = oldRedis
		common.SetDatabaseTypes(oldMainDatabaseType, oldLogDatabaseType)
		common.SessionSecret = oldSessionSecret
		service.InvalidatePromptAuditConfig()
		service.InvalidateRequestArchiveConfig()
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	return root, admin
}
