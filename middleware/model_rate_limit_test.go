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
		admissionRule:   rule,
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
		admissionRule:   rule,
	}
	router.Use(memoryRateLimitHandler(requestContext))
	router.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/test", nil))

	assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
	assert.True(t, inMemoryRateLimiter.Allow(totalKey, rule.totalMaxCount, 60), "success rejection must not consume total quota")
}

func TestMemoryRateLimitHandlerRecordsSuccessForFinalUsingGroupFromCapturedSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	inMemoryRateLimiter.Init(time.Minute)
	restoreModelRequestRateLimitSnapshot(t)
	require.NoError(t, setting.UpdateModelRequestRateLimitOptions(map[string]string{
		"ModelRequestRateLimitCount":        "0",
		"ModelRequestRateLimitSuccessCount": "100",
		"ModelRequestRateLimitGroup":        `{"codex-final":[0,1]}`,
		"ModelRequestRateLimitUserGroup":    `{}`,
	}))

	const userID = 987654
	snapshot := setting.GetModelRequestRateLimitSnapshot()
	requestContext := &modelRequestRateLimitContext{
		snapshot:        snapshot,
		durationSeconds: 60,
		userGroup:       "default",
		admissionGroup:  "default",
		admissionRule:   buildModelRequestRateLimitRule(snapshot, "default", "default"),
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
	assert.Equal(t, modelRequestRateLimitRule{name: "request_group", scope: "request_group:codex-final", totalMaxCount: 0, successMaxCount: 1}, requestContext.successRule)
	finalKey := buildModelRequestRateLimitMemoryKey("success", "request_group:codex-final", "987654")
	require.False(t, inMemoryRateLimiter.Allow(finalKey, 1, 60), "captured-generation success counter should be full")
	globalKey := buildModelRequestRateLimitMemoryKey("success", "global", "987654")
	require.True(t, inMemoryRateLimiter.Allow(globalKey, 100, 60), "success must not also be counted against the admission rule")
}
