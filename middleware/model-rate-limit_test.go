package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

func TestBuildModelRequestRateLimitRuleUsesMostSpecificOverride(t *testing.T) {
	originalGroup := setting.ModelRequestRateLimitGroup
	originalUserGroup := setting.ModelRequestRateLimitUserGroup
	originalTotal := setting.ModelRequestRateLimitCount
	originalSuccess := setting.ModelRequestRateLimitSuccessCount
	defer func() {
		setting.ModelRequestRateLimitMutex.Lock()
		setting.ModelRequestRateLimitGroup = originalGroup
		setting.ModelRequestRateLimitUserGroup = originalUserGroup
		setting.ModelRequestRateLimitCount = originalTotal
		setting.ModelRequestRateLimitSuccessCount = originalSuccess
		setting.ModelRequestRateLimitMutex.Unlock()
	}()

	setting.ModelRequestRateLimitCount = 10
	setting.ModelRequestRateLimitSuccessCount = 20
	if err := setting.UpdateModelRequestRateLimitGroupByJSONString(`{"codex":[30,40]}`); err != nil {
		t.Fatalf("failed to set request group limits: %v", err)
	}
	if err := setting.UpdateModelRequestRateLimitUserGroupByJSONString(`{"vip":{"global":[50,60],"groups":{"codex":[70,80]}}}`); err != nil {
		t.Fatalf("failed to set user group limits: %v", err)
	}

	rule := buildModelRequestRateLimitRule("vip", "codex")
	want := modelRequestRateLimitRule{
		name:            "user_group_request_group",
		scope:           "user_group:vip:request_group:codex",
		totalMaxCount:   70,
		successMaxCount: 80,
	}
	if rule != want {
		t.Fatalf("unexpected selected rule, got %#v want %#v", rule, want)
	}
}

func TestBuildModelRequestRateLimitRuleUserGroupGlobalOverridesBaseGlobal(t *testing.T) {
	originalGroup := setting.ModelRequestRateLimitGroup
	originalUserGroup := setting.ModelRequestRateLimitUserGroup
	originalTotal := setting.ModelRequestRateLimitCount
	originalSuccess := setting.ModelRequestRateLimitSuccessCount
	defer func() {
		setting.ModelRequestRateLimitMutex.Lock()
		setting.ModelRequestRateLimitGroup = originalGroup
		setting.ModelRequestRateLimitUserGroup = originalUserGroup
		setting.ModelRequestRateLimitCount = originalTotal
		setting.ModelRequestRateLimitSuccessCount = originalSuccess
		setting.ModelRequestRateLimitMutex.Unlock()
	}()

	setting.ModelRequestRateLimitCount = 10
	setting.ModelRequestRateLimitSuccessCount = 20
	if err := setting.UpdateModelRequestRateLimitGroupByJSONString(`{}`); err != nil {
		t.Fatalf("failed to clear request group limits: %v", err)
	}
	if err := setting.UpdateModelRequestRateLimitUserGroupByJSONString(`{"vip":{"global":[50,60]}}`); err != nil {
		t.Fatalf("failed to set user group limits: %v", err)
	}

	rule := buildModelRequestRateLimitRule("vip", "codex")
	want := modelRequestRateLimitRule{
		name:            "user_group",
		scope:           "user_group:vip",
		totalMaxCount:   50,
		successMaxCount: 60,
	}
	if rule != want {
		t.Fatalf("unexpected selected rule, got %#v want %#v", rule, want)
	}
}

func TestBuildModelRequestRateLimitKeyEscapesScopes(t *testing.T) {
	key := buildModelRequestRateLimitKey("success", "user_group:vip:a/request_group:codex plus", "42")
	if key != "rateLimit:model_request:v2:success:user_group%3Avip%3Aa%2Frequest_group%3Acodex+plus:user:42" {
		t.Fatalf("unexpected escaped redis key: %s", key)
	}

	memoryKey := buildModelRequestRateLimitMemoryKey("total", "user_group:vip:a/request_group:codex plus", "42")
	if memoryKey != "MRRL:v2:total:user_group%3Avip%3Aa%2Frequest_group%3Acodex+plus:user:42" {
		t.Fatalf("unexpected escaped memory key: %s", memoryKey)
	}
}

func TestMemoryRateLimitHandlerBlocksWhenSuccessLimitAlreadyReached(t *testing.T) {
	gin.SetMode(gin.TestMode)
	inMemoryRateLimiter.Init(time.Minute)

	userId := "42"
	rule := modelRequestRateLimitRule{
		name:            "global",
		scope:           "global",
		totalMaxCount:   0,
		successMaxCount: 1,
	}
	successKey := buildModelRequestRateLimitMemoryKey("success", rule.scope, userId)
	if !inMemoryRateLimiter.Request(successKey, rule.successMaxCount, 60) {
		t.Fatal("expected setup request to be recorded")
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 42)
		c.Next()
	})
	router.Use(memoryRateLimitHandler(60, rule))
	router.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429, got %d", recorder.Code)
	}
}

func TestMemoryRateLimitHandlerRecordsSuccessForFinalSelectedGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	inMemoryRateLimiter.Init(time.Minute)

	originalGroup := setting.ModelRequestRateLimitGroup
	defer func() {
		setting.ModelRequestRateLimitMutex.Lock()
		setting.ModelRequestRateLimitGroup = originalGroup
		setting.ModelRequestRateLimitMutex.Unlock()
	}()
	if err := setting.UpdateModelRequestRateLimitGroupByJSONString(`{"codex-final":[0,1]}`); err != nil {
		t.Fatalf("failed to set request group limits: %v", err)
	}

	userId := "987654"
	initialRule := modelRequestRateLimitRule{
		name:            "global",
		scope:           "global",
		totalMaxCount:   0,
		successMaxCount: 100,
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 987654)
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeySelectedChannelGroup, "default")
		c.Next()
	})
	router.Use(memoryRateLimitHandler(60, initialRule))
	router.GET("/test", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeySelectedChannelGroup, "codex-final")
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	finalSuccessKey := buildModelRequestRateLimitMemoryKey("success", "request_group:codex-final", userId)
	if inMemoryRateLimiter.Allow(finalSuccessKey, 1, 60) {
		t.Fatal("expected final request group success counter to be full")
	}
}

func TestModelRequestRateLimitReusesAsyncQuotaRetryAdmission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	inMemoryRateLimiter.Init(time.Minute)

	originalEnabled := setting.ModelRequestRateLimitEnabled
	originalDuration := setting.ModelRequestRateLimitDurationMinutes
	originalTotal := setting.ModelRequestRateLimitCount
	originalSuccess := setting.ModelRequestRateLimitSuccessCount
	originalRedisEnabled := common.RedisEnabled
	defer func() {
		setting.ModelRequestRateLimitEnabled = originalEnabled
		setting.ModelRequestRateLimitDurationMinutes = originalDuration
		setting.ModelRequestRateLimitCount = originalTotal
		setting.ModelRequestRateLimitSuccessCount = originalSuccess
		common.RedisEnabled = originalRedisEnabled
	}()

	setting.ModelRequestRateLimitEnabled = true
	setting.ModelRequestRateLimitDurationMinutes = 1
	setting.ModelRequestRateLimitCount = 1
	setting.ModelRequestRateLimitSuccessCount = 1
	common.RedisEnabled = false

	const userID = 719003
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", userID)
		common.SetContextKey(c, constant.ContextKeyOriginalModel, "quota-retry-model")
		common.SetContextKey(c, constant.ContextKeyAsyncImageTask, true)
		if c.GetHeader("X-Quota-Retry") == "1" {
			common.SetContextKey(c, constant.ContextKeyAsyncImageTaskQuotaSyncRetry, true)
		}
		c.Next()
	})
	router.Use(ModelRequestRateLimit())
	router.GET("/test", func(c *gin.Context) {
		if c.GetHeader("X-Quota-Error") == "1" {
			common.SetContextKey(c, constant.ContextKeyAsyncImageTaskQuotaSync, true)
			common.SetContextKey(c, constant.ContextKeyAsyncImageTaskErrorCode, "query_data_error")
		}
		c.Status(http.StatusOK)
	})

	firstRecorder := httptest.NewRecorder()
	firstRequest := httptest.NewRequest(http.MethodGet, "/test", nil)
	firstRequest.Header.Set("X-Quota-Error", "1")
	router.ServeHTTP(firstRecorder, firstRequest)
	if firstRecorder.Code != http.StatusOK {
		t.Fatalf("expected replaced quota error status 200, got %d", firstRecorder.Code)
	}

	successKey := buildModelRequestRateLimitMemoryKey("success", "global", "719003")
	if !inMemoryRateLimiter.Allow(successKey, 1, 60) {
		t.Fatal("quota sync error must not consume the success limit")
	}

	retryRecorder := httptest.NewRecorder()
	retryRequest := httptest.NewRequest(http.MethodGet, "/test", nil)
	retryRequest.Header.Set("X-Quota-Retry", "1")
	router.ServeHTTP(retryRecorder, retryRequest)
	if retryRecorder.Code != http.StatusOK {
		t.Fatalf("expected admitted quota retry to succeed, got %d", retryRecorder.Code)
	}
	if inMemoryRateLimiter.Allow(successKey, 1, 60) {
		t.Fatal("successful quota retry must record exactly one logical success")
	}

	nextRecorder := httptest.NewRecorder()
	nextRequest := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(nextRecorder, nextRequest)
	if nextRecorder.Code != http.StatusTooManyRequests {
		t.Fatalf("expected a new request to remain limited, got %d", nextRecorder.Code)
	}
}
