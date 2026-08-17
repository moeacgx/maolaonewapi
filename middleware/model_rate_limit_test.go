package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func restoreModelRequestRateLimitSnapshot(t *testing.T) {
	t.Helper()
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
}

func TestModelRedisRateLimitUsesUTCRegardlessOfLocalTimezone(t *testing.T) {
	redisServer, redisClient := useRateLimitMiniRedis(t)
	previousLocation := time.Local
	time.Local = time.FixedZone("test-utc-plus-eight", 8*60*60)
	t.Cleanup(func() { time.Local = previousLocation })

	ctx := context.Background()
	recordKey := "rateLimit:model-utc-record"
	recordRedisRequest(ctx, redisClient, recordKey, 2, 60)
	recorded, err := redisClient.LIndex(ctx, recordKey, 0).Result()
	require.NoError(t, err)
	recordedAt, err := time.Parse(modelRateLimitTimeFormat, recorded)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().UTC(), recordedAt, 2*time.Second)

	checkKey := "rateLimit:model-utc-check"
	withinWindow := time.Now().UTC().Add(-30 * time.Second).Format(modelRateLimitTimeFormat)
	_, err = redisServer.Push(checkKey, withinWindow, withinWindow)
	require.NoError(t, err)
	allowed, err := checkRedisRateLimit(ctx, redisClient, checkKey, 2, 60)
	require.NoError(t, err)
	assert.False(t, allowed, "an existing UTC timestamp inside the window must remain limited on a non-UTC host")
}

func TestBuildModelRequestRateLimitRuleUsesMostSpecificPriority(t *testing.T) {
	restoreModelRequestRateLimitSnapshot(t)
	require.NoError(t, setting.UpdateModelRequestRateLimitOptions(map[string]string{
		"ModelRequestRateLimitCount":        "10",
		"ModelRequestRateLimitSuccessCount": "20",
		"ModelRequestRateLimitGroup":        `{"codex":[30,40]}`,
		"ModelRequestRateLimitUserGroup":    `{"vip":{"global":[50,60],"groups":{"codex":[70,80]}}}`,
	}))
	snapshot := setting.GetModelRequestRateLimitSnapshot()
	assert.Equal(t, modelRequestRateLimitRule{name: "user_group_request_group", scope: "user_group:vip:request_group:codex", totalMaxCount: 70, successMaxCount: 80}, buildModelRequestRateLimitRule(snapshot, "vip", "codex"))
	assert.Equal(t, modelRequestRateLimitRule{name: "request_group", scope: "request_group:codex", totalMaxCount: 30, successMaxCount: 40}, buildModelRequestRateLimitRule(snapshot, "other", "codex"))
	assert.Equal(t, modelRequestRateLimitRule{name: "user_group", scope: "user_group:vip", totalMaxCount: 50, successMaxCount: 60}, buildModelRequestRateLimitRule(snapshot, "vip", "other"))
	assert.Equal(t, modelRequestRateLimitRule{name: "global", scope: "global", totalMaxCount: 10, successMaxCount: 20}, buildModelRequestRateLimitRule(snapshot, "other", "other"))
}

func TestBuildModelRequestRateLimitRulePrefersGroupOverUserGroup(t *testing.T) {
	restoreModelRequestRateLimitSnapshot(t)
	require.NoError(t, setting.UpdateModelRequestRateLimitOptions(map[string]string{
		"ModelRequestRateLimitCount":        "1",
		"ModelRequestRateLimitSuccessCount": "1",
		"ModelRequestRateLimitGroup":        `{"auto":[3,3]}`,
		"ModelRequestRateLimitUserGroup":    `{"vip":{"global":[2,2]}}`,
	}))
	snapshot := setting.GetModelRequestRateLimitSnapshot()
	assert.Equal(t, modelRequestRateLimitRule{name: "request_group", scope: "request_group:auto", totalMaxCount: 3, successMaxCount: 3}, buildModelRequestRateLimitRule(snapshot, "vip", "auto"))
}

func TestBuildModelRequestRateLimitKeysEscapeScopes(t *testing.T) {
	assert.Equal(t, "rateLimit:model_request:v2:success:user_group%3Avip%3Aa%2Frequest_group%3Acodex+plus:user:42", buildModelRequestRateLimitKey("success", "user_group:vip:a/request_group:codex plus", "42"))
	assert.Equal(t, "MRRL:v2:total:user_group%3Avip%3Aa%2Frequest_group%3Acodex+plus:user:42", buildModelRequestRateLimitMemoryKey("total", "user_group:vip:a/request_group:codex plus", "42"))
}

func TestMemoryMostSpecificGroupOverridesGlobalAndRecordsAdmission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	inMemoryRateLimiter.Init(time.Minute)
	restoreModelRequestRateLimitSnapshot(t)
	require.NoError(t, setting.UpdateModelRequestRateLimitOptions(map[string]string{
		"ModelRequestRateLimitCount":        "1",
		"ModelRequestRateLimitSuccessCount": "1",
		"ModelRequestRateLimitGroup":        `{"auto":[2,2]}`,
		"ModelRequestRateLimitUserGroup":    `{}`,
	}))

	const userID = 987651
	const userIDString = "987651"
	snapshot := setting.GetModelRequestRateLimitSnapshot()
	requestContext := &modelRequestRateLimitContext{
		snapshot:        snapshot,
		durationSeconds: 60,
		userGroup:       "default",
		admissionRule:   buildModelRequestRateLimitRule(snapshot, "default", "auto"),
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", userID)
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "auto")
		c.Next()
	})
	router.Use(memoryRateLimitHandler(requestContext))
	router.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	for _, expectedStatus := range []int{http.StatusOK, http.StatusOK, http.StatusTooManyRequests} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/test", nil))
		assert.Equal(t, expectedStatus, recorder.Code)
	}

	groupSuccessKey := buildModelRequestRateLimitMemoryKey("success", "request_group:auto", userIDString)
	assert.False(t, inMemoryRateLimiter.Allow(groupSuccessKey, 2, 60), "two successful admissions should consume the selected group success quota")
	groupTotalKey := buildModelRequestRateLimitMemoryKey("total", "request_group:auto", userIDString)
	assert.False(t, inMemoryRateLimiter.Allow(groupTotalKey, 2, 60), "three admission attempts should leave the selected group total quota exhausted")
	globalSuccessKey := buildModelRequestRateLimitMemoryKey("success", "global", userIDString)
	assert.True(t, inMemoryRateLimiter.Allow(globalSuccessKey, 1, 60), "the broader global success quota must not be enforced or recorded")
	globalTotalKey := buildModelRequestRateLimitMemoryKey("total", "global", userIDString)
	assert.True(t, inMemoryRateLimiter.Allow(globalTotalKey, 1, 60), "the broader global total quota must not be enforced or recorded")
}

func TestMemoryRecordsAdmissionAndFinalResolvedScopesFromSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	inMemoryRateLimiter.Init(time.Minute)
	restoreModelRequestRateLimitSnapshot(t)
	require.NoError(t, setting.UpdateModelRequestRateLimitOptions(map[string]string{
		"ModelRequestRateLimitCount":        "0",
		"ModelRequestRateLimitSuccessCount": "5",
		"ModelRequestRateLimitGroup":        `{"auto":[0,2],"codex-final":[0,1]}`,
		"ModelRequestRateLimitUserGroup":    `{}`,
	}))

	const userID = 987652
	const userIDString = "987652"
	snapshot := setting.GetModelRequestRateLimitSnapshot()
	requestContext := &modelRequestRateLimitContext{
		snapshot:        snapshot,
		durationSeconds: 60,
		userGroup:       "default",
		admissionRule:   buildModelRequestRateLimitRule(snapshot, "default", "auto"),
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", userID)
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "auto")
		c.Next()
	})
	router.Use(memoryRateLimitHandler(requestContext))
	router.GET("/test", func(c *gin.Context) {
		require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(`{}`))
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "codex-final")
		c.Status(http.StatusOK)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/test", nil))
	require.Equal(t, http.StatusOK, recorder.Code)

	assert.Same(t, snapshot, requestContext.snapshot)
	assert.Equal(t, modelRequestRateLimitRule{name: "request_group", scope: "request_group:codex-final", totalMaxCount: 0, successMaxCount: 1}, requestContext.finalRule)
	admissionKey := buildModelRequestRateLimitMemoryKey("success", "request_group:auto", userIDString)
	assert.True(t, inMemoryRateLimiter.Allow(admissionKey, 2, 60))
	assert.True(t, inMemoryRateLimiter.Request(admissionKey, 2, 60))
	assert.False(t, inMemoryRateLimiter.Allow(admissionKey, 2, 60), "the admission scope should receive one successful request")
	finalKey := buildModelRequestRateLimitMemoryKey("success", "request_group:codex-final", userIDString)
	assert.False(t, inMemoryRateLimiter.Allow(finalKey, 1, 60), "the distinct final resolved scope should receive one successful request")
	globalKey := buildModelRequestRateLimitMemoryKey("success", "global", userIDString)
	assert.True(t, inMemoryRateLimiter.Allow(globalKey, 5, 60), "unselected broader scopes must remain untouched")
}

func TestMemoryRecordsEqualAdmissionAndFinalScopeOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	inMemoryRateLimiter.Init(time.Minute)
	const userID = 987653
	const userIDString = "987653"
	rule := modelRequestRateLimitRule{name: "global", scope: "global", totalMaxCount: 0, successMaxCount: 2}
	requestContext := &modelRequestRateLimitContext{
		snapshot:        setting.GetModelRequestRateLimitSnapshot(),
		durationSeconds: 60,
		admissionRule:   rule,
	}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", userID); c.Next() })
	router.Use(memoryRateLimitHandler(requestContext))
	router.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/test", nil))
	require.Equal(t, http.StatusOK, recorder.Code)

	key := buildModelRequestRateLimitMemoryKey("success", "global", userIDString)
	assert.True(t, inMemoryRateLimiter.Allow(key, 2, 60))
	assert.True(t, inMemoryRateLimiter.Request(key, 2, 60))
	assert.False(t, inMemoryRateLimiter.Allow(key, 2, 60), "equal admission and final scopes must be recorded once")
}

func TestRedisMostSpecificGroupOverridesGlobalAndRecordsAdmission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer, redisClient := useRateLimitMiniRedis(t)
	restoreModelRequestRateLimitSnapshot(t)
	require.NoError(t, setting.UpdateModelRequestRateLimitOptions(map[string]string{
		"ModelRequestRateLimitCount":        "1",
		"ModelRequestRateLimitSuccessCount": "1",
		"ModelRequestRateLimitGroup":        `{"auto":[2,2]}`,
		"ModelRequestRateLimitUserGroup":    `{}`,
	}))

	const userID = 987654
	const userIDString = "987654"
	snapshot := setting.GetModelRequestRateLimitSnapshot()
	requestContext := &modelRequestRateLimitContext{
		snapshot:        snapshot,
		durationSeconds: 60,
		userGroup:       "default",
		admissionRule:   buildModelRequestRateLimitRule(snapshot, "default", "auto"),
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", userID)
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "auto")
		c.Next()
	})
	router.Use(redisRateLimitHandler(requestContext))
	router.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	for _, expectedStatus := range []int{http.StatusOK, http.StatusOK, http.StatusTooManyRequests} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/test", nil))
		assert.Equal(t, expectedStatus, recorder.Code)
	}

	groupSuccessKey := buildModelRequestRateLimitKey("success", "request_group:auto", userIDString)
	values, err := redisServer.List(groupSuccessKey)
	require.NoError(t, err)
	assert.Len(t, values, 2, "only successful admissions should be recorded on the selected group")
	groupTotalKey := buildModelRequestRateLimitKey("total", "request_group:auto", userIDString)
	tokens, err := redisClient.HGet(context.Background(), groupTotalKey, "tokens").Int()
	require.NoError(t, err)
	assert.Zero(t, tokens)
	for _, key := range []string{
		buildModelRequestRateLimitKey("success", "global", userIDString),
		buildModelRequestRateLimitKey("total", "global", userIDString),
	} {
		exists, err := redisClient.Exists(context.Background(), key).Result()
		require.NoError(t, err)
		assert.Zero(t, exists, "broader global quota %q must not be enforced or recorded", key)
	}
}

func TestRedisRecordsAdmissionAndFinalResolvedScopesFromSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer, redisClient := useRateLimitMiniRedis(t)
	restoreModelRequestRateLimitSnapshot(t)
	require.NoError(t, setting.UpdateModelRequestRateLimitOptions(map[string]string{
		"ModelRequestRateLimitCount":        "0",
		"ModelRequestRateLimitSuccessCount": "5",
		"ModelRequestRateLimitGroup":        `{"auto":[0,2],"codex-final":[0,1]}`,
		"ModelRequestRateLimitUserGroup":    `{}`,
	}))

	const userID = 987655
	const userIDString = "987655"
	snapshot := setting.GetModelRequestRateLimitSnapshot()
	requestContext := &modelRequestRateLimitContext{
		snapshot:        snapshot,
		durationSeconds: 60,
		userGroup:       "default",
		admissionRule:   buildModelRequestRateLimitRule(snapshot, "default", "auto"),
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", userID)
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "auto")
		c.Next()
	})
	router.Use(redisRateLimitHandler(requestContext))
	router.GET("/test", func(c *gin.Context) {
		require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(`{}`))
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "codex-final")
		c.Status(http.StatusOK)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/test", nil))
	require.Equal(t, http.StatusOK, recorder.Code)

	assert.Same(t, snapshot, requestContext.snapshot)
	assert.Equal(t, modelRequestRateLimitRule{name: "request_group", scope: "request_group:codex-final", totalMaxCount: 0, successMaxCount: 1}, requestContext.finalRule)
	for _, scope := range []string{"request_group:auto", "request_group:codex-final"} {
		key := buildModelRequestRateLimitKey("success", scope, userIDString)
		values, err := redisServer.List(key)
		require.NoError(t, err)
		assert.Len(t, values, 1, "scope %q should receive the successful request", scope)
	}
	globalKey := buildModelRequestRateLimitKey("success", "global", userIDString)
	exists, err := redisClient.Exists(context.Background(), globalKey).Result()
	require.NoError(t, err)
	assert.Zero(t, exists, "unselected broader scopes must remain untouched")
}

func TestRedisRecordsEqualAdmissionAndFinalScopeOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer, _ := useRateLimitMiniRedis(t)
	const userID = 987656
	const userIDString = "987656"
	rule := modelRequestRateLimitRule{name: "global", scope: "global", totalMaxCount: 0, successMaxCount: 2}
	requestContext := &modelRequestRateLimitContext{
		snapshot:        setting.GetModelRequestRateLimitSnapshot(),
		durationSeconds: 60,
		admissionRule:   rule,
	}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", userID); c.Next() })
	router.Use(redisRateLimitHandler(requestContext))
	router.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/test", nil))
	require.Equal(t, http.StatusOK, recorder.Code)

	key := buildModelRequestRateLimitKey("success", "global", userIDString)
	values, err := redisServer.List(key)
	require.NoError(t, err)
	assert.Len(t, values, 1, "equal admission and final scopes must be recorded once")
}
