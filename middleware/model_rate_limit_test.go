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

func TestBuildModelRequestRateLimitRuleUsesMostSpecificOverride(t *testing.T) {
	restoreModelRequestRateLimitSnapshot(t)
	require.NoError(t, setting.UpdateModelRequestRateLimitOptions(map[string]string{
		"ModelRequestRateLimitCount":        "10",
		"ModelRequestRateLimitSuccessCount": "20",
		"ModelRequestRateLimitGroup":        `{"codex":[30,40]}`,
		"ModelRequestRateLimitUserGroup":    `{"vip":{"global":[50,60],"groups":{"codex":[70,80]}}}`,
	}))
	snapshot := setting.GetModelRequestRateLimitSnapshot()
	assert.Equal(t, modelRequestRateLimitRule{name: "user_group_request_group", scope: "user_group:vip:request_group:codex", totalMaxCount: 70, successMaxCount: 80}, buildModelRequestRateLimitRule(snapshot, "vip", "codex"))
	assert.Equal(t, []modelRequestRateLimitRule{
		{name: "global", scope: "global", totalMaxCount: 10, successMaxCount: 20},
		{name: "request_group", scope: "request_group:codex", totalMaxCount: 30, successMaxCount: 40},
		{name: "user_group", scope: "user_group:vip", totalMaxCount: 50, successMaxCount: 60},
		{name: "user_group_request_group", scope: "user_group:vip:request_group:codex", totalMaxCount: 70, successMaxCount: 80},
	}, buildModelRequestRateLimitRules(snapshot, "vip", "codex"))
	assert.Equal(t, modelRequestRateLimitRule{name: "user_group", scope: "user_group:vip", totalMaxCount: 50, successMaxCount: 60}, buildModelRequestRateLimitRule(snapshot, "vip", "other"))
	assert.Equal(t, modelRequestRateLimitRule{name: "request_group", scope: "request_group:codex", totalMaxCount: 30, successMaxCount: 40}, buildModelRequestRateLimitRule(snapshot, "other", "codex"))
	assert.Equal(t, modelRequestRateLimitRule{name: "global", scope: "global", totalMaxCount: 10, successMaxCount: 20}, buildModelRequestRateLimitRule(snapshot, "other", "other"))
}

func TestBuildModelRequestRateLimitKeysEscapeScopes(t *testing.T) {
	assert.Equal(t, "rateLimit:model_request:v2:success:user_group%3Avip%3Aa%2Frequest_group%3Acodex+plus:user:42", buildModelRequestRateLimitKey("success", "user_group:vip:a/request_group:codex plus", "42"))
	assert.Equal(t, "MRRL:v2:total:user_group%3Avip%3Aa%2Frequest_group%3Acodex+plus:user:42", buildModelRequestRateLimitMemoryKey("total", "user_group:vip:a/request_group:codex plus", "42"))
}

func TestMemoryRateLimitHandlerChecksSuccessWithoutConsumingIt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	inMemoryRateLimiter.Init(time.Minute)
	userID := "424242"
	rule := modelRequestRateLimitRule{name: "global", scope: "global", totalMaxCount: 0, successMaxCount: 1}
	successKey := buildModelRequestRateLimitMemoryKey("success", rule.scope, userID)
	require.True(t, inMemoryRateLimiter.Request(successKey, rule.successMaxCount, 60))

	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", 424242); c.Next() })
	requestContext := &modelRequestRateLimitContext{
		snapshot:        setting.GetModelRequestRateLimitSnapshot(),
		durationSeconds: 60,
		admissionRules:  []modelRequestRateLimitRule{rule},
	}
	router.Use(memoryRateLimitHandler(requestContext))
	router.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/test", nil))
	assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
}

func TestMemoryRateLimitHandlerDoesNotConsumeTotalWhenSuccessLimitRejects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	inMemoryRateLimiter.Init(time.Minute)
	userID := "424243"
	rule := modelRequestRateLimitRule{name: "global", scope: "global", totalMaxCount: 1, successMaxCount: 1}
	successKey := buildModelRequestRateLimitMemoryKey("success", rule.scope, userID)
	totalKey := buildModelRequestRateLimitMemoryKey("total", rule.scope, userID)
	require.True(t, inMemoryRateLimiter.Request(successKey, rule.successMaxCount, 60))

	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", 424243); c.Next() })
	requestContext := &modelRequestRateLimitContext{
		snapshot:        setting.GetModelRequestRateLimitSnapshot(),
		durationSeconds: 60,
		admissionRules:  []modelRequestRateLimitRule{rule},
	}
	router.Use(memoryRateLimitHandler(requestContext))
	router.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/test", nil))

	assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
	assert.True(t, inMemoryRateLimiter.Allow(totalKey, rule.totalMaxCount, 60), "success rejection must not consume total quota")
}

func TestMemoryRateLimitHandlerRecordsAdmissionScopesAndEnforcesFinalGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	inMemoryRateLimiter.Init(time.Minute)
	restoreModelRequestRateLimitSnapshot(t)
	require.NoError(t, setting.UpdateModelRequestRateLimitOptions(map[string]string{
		"ModelRequestRateLimitCount":        "0",
		"ModelRequestRateLimitSuccessCount": "2",
		"ModelRequestRateLimitGroup":        `{"codex-final":[0,1]}`,
		"ModelRequestRateLimitUserGroup":    `{}`,
	}))

	const userID = 987654
	const userIDString = "987654"
	snapshot := setting.GetModelRequestRateLimitSnapshot()
	requestContext := &modelRequestRateLimitContext{
		snapshot:        snapshot,
		durationSeconds: 60,
		userGroup:       "default",
		admissionGroup:  "default",
		admissionRules:  buildModelRequestRateLimitRules(snapshot, "default", "default"),
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", userID)
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
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
	assert.Equal(t, "codex-final", requestContext.finalUsingGroup)
	assert.Equal(t, []modelRequestRateLimitRule{
		{name: "global", scope: "global", totalMaxCount: 0, successMaxCount: 2},
		{name: "request_group", scope: "request_group:codex-final", totalMaxCount: 0, successMaxCount: 1},
	}, requestContext.successRules)

	finalKey := buildModelRequestRateLimitMemoryKey("success", "request_group:codex-final", userIDString)
	require.False(t, inMemoryRateLimiter.Allow(finalKey, 1, 60), "final resolved group should receive the successful request")

	finalAdmissionContext := &modelRequestRateLimitContext{
		snapshot:        snapshot,
		durationSeconds: 60,
		userGroup:       "default",
		admissionGroup:  "codex-final",
		admissionRules:  buildModelRequestRateLimitRules(snapshot, "default", "codex-final"),
	}
	finalRouter := gin.New()
	finalRouter.Use(func(c *gin.Context) {
		c.Set("id", userID)
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "codex-final")
		c.Next()
	})
	finalRouter.Use(memoryRateLimitHandler(finalAdmissionContext))
	finalRouter.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })
	finalRecorder := httptest.NewRecorder()
	finalRouter.ServeHTTP(finalRecorder, httptest.NewRequest(http.MethodGet, "/test", nil))
	require.Equal(t, http.StatusTooManyRequests, finalRecorder.Code)

	globalKey := buildModelRequestRateLimitMemoryKey("success", "global", userIDString)
	require.True(t, inMemoryRateLimiter.Request(globalKey, 2, 60), "rejected final-group admission must not record another global success")
	require.False(t, inMemoryRateLimiter.Allow(globalKey, 2, 60), "admission global scope should retain the first successful request")
}

func TestMemoryRateLimitHandlerEnforcesAutoGroupAdmissionWithoutPartialAccounting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	inMemoryRateLimiter.Init(time.Minute)
	restoreModelRequestRateLimitSnapshot(t)
	require.NoError(t, setting.UpdateModelRequestRateLimitOptions(map[string]string{
		"ModelRequestRateLimitCount":        "1",
		"ModelRequestRateLimitSuccessCount": "1",
		"ModelRequestRateLimitGroup":        `{"auto":[1,1]}`,
		"ModelRequestRateLimitUserGroup":    `{"default":{"global":[1,1],"groups":{"auto":[1,1]}}}`,
	}))

	const userID = 987655
	const userIDString = "987655"
	snapshot := setting.GetModelRequestRateLimitSnapshot()
	rules := buildModelRequestRateLimitRules(snapshot, "default", "auto")
	require.Equal(t, []string{"global", "request_group", "user_group", "user_group_request_group"}, []string{rules[0].name, rules[1].name, rules[2].name, rules[3].name})
	blockedKey := buildModelRequestRateLimitMemoryKey("success", "request_group:auto", userIDString)
	require.True(t, inMemoryRateLimiter.Request(blockedKey, 1, 60))

	requestContext := &modelRequestRateLimitContext{
		snapshot:        snapshot,
		durationSeconds: 60,
		userGroup:       "default",
		admissionGroup:  "auto",
		admissionRules:  rules,
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
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/test", nil))
	require.Equal(t, http.StatusTooManyRequests, recorder.Code)

	globalSuccessKey := buildModelRequestRateLimitMemoryKey("success", "global", userIDString)
	assert.True(t, inMemoryRateLimiter.Allow(globalSuccessKey, 1, 60), "rejected admission must not record success")
	globalTotalKey := buildModelRequestRateLimitMemoryKey("total", "global", userIDString)
	assert.True(t, inMemoryRateLimiter.Allow(globalTotalKey, 1, 60), "all success scopes should be checked before total consumption")
}

func TestMemoryRateLimitHandlerDeduplicatesEqualSuccessScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	inMemoryRateLimiter.Init(time.Minute)
	const userID = 987656
	const userIDString = "987656"
	rule := modelRequestRateLimitRule{name: "global", scope: "global", totalMaxCount: 0, successMaxCount: 2}
	requestContext := &modelRequestRateLimitContext{
		snapshot:        setting.GetModelRequestRateLimitSnapshot(),
		durationSeconds: 60,
		admissionRules:  []modelRequestRateLimitRule{rule},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", userID); c.Next() })
	router.Use(memoryRateLimitHandler(requestContext))
	router.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/test", nil))
	require.Equal(t, http.StatusOK, recorder.Code)

	key := buildModelRequestRateLimitMemoryKey("success", "global", userIDString)
	require.True(t, inMemoryRateLimiter.Allow(key, 2, 60), "admission and final global scopes should record only once")
	require.True(t, inMemoryRateLimiter.Request(key, 2, 60))
	require.False(t, inMemoryRateLimiter.Allow(key, 2, 60))
}

func TestRedisRateLimitHandlerRecordsAdmissionScopesAndEnforcesFinalGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer, _ := useRateLimitMiniRedis(t)
	restoreModelRequestRateLimitSnapshot(t)
	require.NoError(t, setting.UpdateModelRequestRateLimitOptions(map[string]string{
		"ModelRequestRateLimitCount":        "0",
		"ModelRequestRateLimitSuccessCount": "2",
		"ModelRequestRateLimitGroup":        `{"auto":[0,2],"codex-final":[0,1]}`,
		"ModelRequestRateLimitUserGroup":    `{"default":{"global":[0,2],"groups":{"auto":[0,2]}}}`,
	}))

	const userID = 987658
	const userIDString = "987658"
	snapshot := setting.GetModelRequestRateLimitSnapshot()
	requestContext := &modelRequestRateLimitContext{
		snapshot:        snapshot,
		durationSeconds: 60,
		userGroup:       "default",
		admissionGroup:  "auto",
		admissionRules:  buildModelRequestRateLimitRules(snapshot, "default", "auto"),
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
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "codex-final")
		c.Status(http.StatusOK)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/test", nil))
	require.Equal(t, http.StatusOK, recorder.Code)

	for _, scope := range []string{
		"global",
		"user_group:default",
		"request_group:auto",
		"user_group:default:request_group:auto",
		"request_group:codex-final",
	} {
		key := buildModelRequestRateLimitKey("success", scope, userIDString)
		values, err := redisServer.List(key)
		if err != nil {
			t.Fatalf("scope %q list missing: %v (keys=%v)", scope, err, redisServer.Keys())
		}
		assert.Len(t, values, 1, "scope %q should receive one successful request", scope)
	}

	finalAdmissionContext := &modelRequestRateLimitContext{
		snapshot:        snapshot,
		durationSeconds: 60,
		userGroup:       "default",
		admissionGroup:  "codex-final",
		admissionRules:  buildModelRequestRateLimitRules(snapshot, "default", "codex-final"),
	}
	finalRouter := gin.New()
	finalRouter.Use(func(c *gin.Context) {
		c.Set("id", userID)
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "codex-final")
		c.Next()
	})
	finalRouter.Use(redisRateLimitHandler(finalAdmissionContext))
	finalRouter.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })
	finalRecorder := httptest.NewRecorder()
	finalRouter.ServeHTTP(finalRecorder, httptest.NewRequest(http.MethodGet, "/test", nil))
	require.Equal(t, http.StatusTooManyRequests, finalRecorder.Code)
	globalKey := buildModelRequestRateLimitKey("success", "global", userIDString)
	values, err := redisServer.List(globalKey)
	require.NoError(t, err)
	assert.Len(t, values, 1, "rejected final-group admission must not record success")
}

func TestRedisRateLimitHandlerDeduplicatesEqualSuccessScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer, _ := useRateLimitMiniRedis(t)
	const userID = 987657
	const userIDString = "987657"
	rule := modelRequestRateLimitRule{name: "global", scope: "global", totalMaxCount: 0, successMaxCount: 2}
	requestContext := &modelRequestRateLimitContext{
		snapshot:        setting.GetModelRequestRateLimitSnapshot(),
		durationSeconds: 60,
		admissionRules:  []modelRequestRateLimitRule{rule},
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
	assert.Len(t, values, 1, "admission and final global scopes should record only once")
}

func TestMemoryModelRateLimitDoesNotPartiallyConsumeTotals(t *testing.T) {
	inMemoryRateLimiter.Init(time.Minute)
	const userID = "987659"
	rules := []modelRequestRateLimitRule{
		{name: "global", scope: "global", totalMaxCount: 1},
		{name: "request_group", scope: "request_group:auto", totalMaxCount: 1},
	}
	laterKey := buildModelRequestRateLimitMemoryKey("total", rules[1].scope, userID)
	require.True(t, inMemoryRateLimiter.Request(laterKey, rules[1].totalMaxCount, 60))

	rejectedRule, allowed := checkMemoryModelRequestRateLimit(userID, 60, rules)
	require.False(t, allowed)
	assert.Equal(t, "request_group", rejectedRule.name)
	earlierKey := buildModelRequestRateLimitMemoryKey("total", rules[0].scope, userID)
	assert.True(t, inMemoryRateLimiter.Allow(earlierKey, rules[0].totalMaxCount, 60), "later rejection must not consume earlier total")
}

func TestRedisModelRateLimitDoesNotPartiallyConsumeTotals(t *testing.T) {
	_, redisClient := useRateLimitMiniRedis(t)
	const userID = "987660"
	const duration = int64(60)
	rules := []modelRequestRateLimitRule{
		{name: "global", scope: "global", totalMaxCount: 1},
		{name: "request_group", scope: "request_group:auto", totalMaxCount: 1},
	}
	laterKey := buildModelRequestRateLimitKey("total", rules[1].scope, userID)
	_, err := redisClient.HSet(context.Background(), laterKey, "tokens", 0, "last_time", time.Now().Unix()).Result()
	require.NoError(t, err)

	rejectedRule, allowed, err := checkRedisModelRequestRateLimit(context.Background(), redisClient, userID, duration, rules)
	require.NoError(t, err)
	require.False(t, allowed)
	assert.Equal(t, "request_group", rejectedRule.name)
	earlierKey := buildModelRequestRateLimitKey("total", rules[0].scope, userID)
	exists, err := redisClient.Exists(context.Background(), earlierKey).Result()
	require.NoError(t, err)
	assert.Zero(t, exists, "later rejection must not consume earlier total")
}
