package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useRateLimitMiniRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()

	previousRedisEnabled := common.RedisEnabled
	previousRedisClient := common.RDB
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	require.NoError(t, redisClient.Ping(context.Background()).Err())

	common.RedisEnabled = true
	common.RDB = redisClient
	t.Cleanup(func() {
		_ = redisClient.Close()
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRedisClient
	})

	return redisServer, redisClient
}

func TestChannelAdminBypassIsScopedToAuthenticatedWrites(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useRateLimitMiniRedis(t)
	previousEnabled := common.GlobalApiRateLimitEnable
	previousLimit := common.GlobalApiRateLimitNum
	previousDuration := common.GlobalApiRateLimitDuration
	previousClassifier := classifyDashboardCredentialForRateLimit
	common.GlobalApiRateLimitEnable = true
	common.GlobalApiRateLimitNum = 1
	common.GlobalApiRateLimitDuration = 30
	classifyDashboardCredentialForRateLimit = func(*gin.Context) (*model.UserBase, service.AuthIdentity, dashboardCredentialKind, error) {
		return &model.UserBase{Username: "admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}, service.AuthIdentity{}, dashboardCredentialInternal, nil
	}
	t.Cleanup(func() {
		common.GlobalApiRateLimitEnable = previousEnabled
		common.GlobalApiRateLimitNum = previousLimit
		common.GlobalApiRateLimitDuration = previousDuration
		classifyDashboardCredentialForRateLimit = previousClassifier
	})

	router := gin.New()
	router.Use(GlobalAPIRateLimitWithChannelAdminBypass())
	router.Any("/*path", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	assert.Equal(t, http.StatusNoContent, performRateLimitMethod(router, http.MethodPut, "/api/channel/", "192.0.2.90:1").Code)
	assert.Equal(t, http.StatusNoContent, performRateLimitMethod(router, http.MethodPut, "/api/channel/", "192.0.2.90:1").Code)
	assert.Equal(t, http.StatusNoContent, performRateLimitMethod(router, http.MethodPut, "/api/other", "192.0.2.90:1").Code)
	assert.Equal(t, http.StatusTooManyRequests, performRateLimitMethod(router, http.MethodPut, "/api/other", "192.0.2.90:1").Code)
}

func TestChannelAdminBypassCoversManagementWritesButProtectsKeyRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useRateLimitMiniRedis(t)
	previousEnabled := common.GlobalApiRateLimitEnable
	previousLimit := common.GlobalApiRateLimitNum
	previousDuration := common.GlobalApiRateLimitDuration
	previousClassifier := classifyDashboardCredentialForRateLimit
	common.GlobalApiRateLimitEnable = true
	common.GlobalApiRateLimitNum = 1
	common.GlobalApiRateLimitDuration = 30
	classifyDashboardCredentialForRateLimit = func(*gin.Context) (*model.UserBase, service.AuthIdentity, dashboardCredentialKind, error) {
		return &model.UserBase{Username: "admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}, service.AuthIdentity{}, dashboardCredentialInternal, nil
	}
	t.Cleanup(func() {
		common.GlobalApiRateLimitEnable = previousEnabled
		common.GlobalApiRateLimitNum = previousLimit
		common.GlobalApiRateLimitDuration = previousDuration
		classifyDashboardCredentialForRateLimit = previousClassifier
	})

	router := gin.New()
	router.Use(GlobalAPIRateLimitWithChannelAdminBypass())
	router.Any("/*path", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	bypassed := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/channel"},
		{http.MethodPut, "/api/channel/"},
		{http.MethodPost, "/api/channel/status/batch"},
		{http.MethodPost, "/api/channel/42/status"},
		{http.MethodDelete, "/api/channel/42"},
		{http.MethodDelete, "/api/channel/disabled"},
		{http.MethodPost, "/api/channel/tag/disabled"},
		{http.MethodPost, "/api/channel/tag/enabled"},
		{http.MethodPut, "/api/channel/tag"},
		{http.MethodPost, "/api/channel/batch"},
		{http.MethodPost, "/api/channel/batch/tag"},
		{http.MethodPost, "/api/channel/fix"},
		{http.MethodPost, "/api/channel/fetch_models"},
		{http.MethodPost, "/api/channel/42/codex/refresh"},
		{http.MethodPost, "/api/channel/42/codex/usage/reset"},
		{http.MethodPost, "/api/channel/ollama/pull"},
		{http.MethodPost, "/api/channel/ollama/pull/stream"},
		{http.MethodDelete, "/api/channel/ollama/delete"},
		{http.MethodPost, "/api/channel/copy/42"},
		{http.MethodPost, "/api/channel/multi_key/manage"},
		{http.MethodPost, "/api/channel/upstream_updates/apply"},
		{http.MethodPost, "/api/channel/upstream_updates/apply_all"},
		{http.MethodPost, "/api/channel/upstream_updates/detect"},
		{http.MethodPost, "/api/channel/upstream_updates/detect_all"},
	}
	for index, route := range bypassed {
		t.Run("bypass "+route.method+" "+route.path, func(t *testing.T) {
			remoteAddr := "192.0.2." + strconv.Itoa(100+index) + ":1"
			assert.Equal(t, http.StatusNoContent, performRateLimitMethod(router, route.method, route.path, remoteAddr).Code)
			assert.Equal(t, http.StatusNoContent, performRateLimitMethod(router, route.method, route.path, remoteAddr).Code)
		})
	}

	protected := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/channel/42/key"},
		{http.MethodPost, "/api/channel/42/key/extra"},
		{http.MethodGet, "/api/channel/42"},
		{http.MethodGet, "/api/channel/42/codex/usage"},
		{http.MethodDelete, "/api/channel/tag"},
		{http.MethodDelete, "/api/channel/unknown"},
		{http.MethodPost, "/api/other"},
	}
	for index, route := range protected {
		t.Run("protect "+route.method+" "+route.path, func(t *testing.T) {
			remoteAddr := "198.51.100." + strconv.Itoa(100+index) + ":1"
			assert.Equal(t, http.StatusNoContent, performRateLimitMethod(router, route.method, route.path, remoteAddr).Code)
			assert.Equal(t, http.StatusTooManyRequests, performRateLimitMethod(router, route.method, route.path, remoteAddr).Code)
		})
	}
}

func TestChannelAdminBypassRejectsUnprivilegedCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useRateLimitMiniRedis(t)
	previousEnabled := common.GlobalApiRateLimitEnable
	previousLimit := common.GlobalApiRateLimitNum
	previousDuration := common.GlobalApiRateLimitDuration
	previousClassifier := classifyDashboardCredentialForRateLimit
	common.GlobalApiRateLimitEnable = true
	common.GlobalApiRateLimitNum = 1
	common.GlobalApiRateLimitDuration = 30
	t.Cleanup(func() {
		common.GlobalApiRateLimitEnable = previousEnabled
		common.GlobalApiRateLimitNum = previousLimit
		common.GlobalApiRateLimitDuration = previousDuration
		classifyDashboardCredentialForRateLimit = previousClassifier
	})

	testCases := []struct {
		name       string
		classifier func(*gin.Context) (*model.UserBase, service.AuthIdentity, dashboardCredentialKind, error)
	}{
		{
			name: "anonymous",
			classifier: func(*gin.Context) (*model.UserBase, service.AuthIdentity, dashboardCredentialKind, error) {
				return nil, service.AuthIdentity{}, dashboardCredentialUnmatched, nil
			},
		},
		{
			name: "ordinary user",
			classifier: func(*gin.Context) (*model.UserBase, service.AuthIdentity, dashboardCredentialKind, error) {
				return &model.UserBase{Username: "user", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}, service.AuthIdentity{}, dashboardCredentialInternal, nil
			},
		},
		{
			name: "invalid credential",
			classifier: func(*gin.Context) (*model.UserBase, service.AuthIdentity, dashboardCredentialKind, error) {
				return nil, service.AuthIdentity{}, dashboardCredentialUnmatched, assert.AnError
			},
		},
	}

	for index, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			classifyDashboardCredentialForRateLimit = testCase.classifier
			router := gin.New()
			router.Use(GlobalAPIRateLimitWithChannelAdminBypass())
			router.Any("/*path", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			remoteAddr := "203.0.113." + strconv.Itoa(100+index) + ":1"

			assert.Equal(t, http.StatusNoContent, performRateLimitMethod(router, http.MethodPut, "/api/channel", remoteAddr).Code)
			assert.Equal(t, http.StatusTooManyRequests, performRateLimitMethod(router, http.MethodPut, "/api/channel", remoteAddr).Code)
		})
	}
}

func performRateLimitRequest(router http.Handler, path string, remoteAddr string) *httptest.ResponseRecorder {
	return performRateLimitMethod(router, http.MethodGet, path, remoteAddr)
}

func performRateLimitMethod(router http.Handler, method string, path string, remoteAddr string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, nil)
	request.RemoteAddr = remoteAddr
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestRedisIPRateLimiterThresholdTTLAndNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer, _ := useRateLimitMiniRedis(t)

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.GET("/limited", rateLimitFactory(2, 37, "TEST"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	remoteAddr := "192.0.2.10:12345"
	legacyKey := "rateLimit:TEST192.0.2.10"
	_, err := redisServer.Push(legacyKey, "legacy-list-entry")
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/limited", remoteAddr).Code)
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/limited", remoteAddr).Code)
	limitedResponse := performRateLimitRequest(router, "/limited", remoteAddr)
	assert.Equal(t, http.StatusTooManyRequests, limitedResponse.Code)
	assert.Equal(t, "37", limitedResponse.Header().Get("Retry-After"))

	key := redisIPRateLimitKey("TEST", "192.0.2.10")
	count, err := redisServer.Get(key)
	require.NoError(t, err)
	assert.Equal(t, "3", count)
	assert.Equal(t, 37*time.Second, redisServer.TTL(key))
	assert.True(t, redisServer.Exists(legacyKey), "the v2 counter must not touch an old list key")
}

func TestRedisUserRateLimiterUsesSharedFixedWindow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer, _ := useRateLimitMiniRedis(t)

	router := gin.New()
	router.GET(
		"/limited",
		func(c *gin.Context) { c.Set("id", 42) },
		userRateLimitFactory(1, 23, "USER"),
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)

	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/limited", "192.0.2.20:12345").Code)
	assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/limited", "198.51.100.20:12345").Code)

	key := redisUserRateLimitKey("USER", 42)
	assert.True(t, redisServer.Exists(key))
	assert.Equal(t, 23*time.Second, redisServer.TTL(key))
}

func TestRedisEmailVerificationRateLimiterPreservesResponseAndTTL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer, _ := useRateLimitMiniRedis(t)

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.GET("/verify", EmailVerificationRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	remoteAddr := "192.0.2.30:12345"
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/verify", remoteAddr).Code)
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/verify", remoteAddr).Code)
	response := performRateLimitRequest(router, "/verify", remoteAddr)
	assert.Equal(t, http.StatusTooManyRequests, response.Code)
	assert.JSONEq(t, `{"success":false,"message":"发送过于频繁，请等待 30 秒后再试"}`, response.Body.String())

	key := redisIPRateLimitKey(EmailVerificationRateLimitMark, "192.0.2.30")
	assert.True(t, redisServer.Exists(key))
	assert.Equal(t, time.Duration(EmailVerificationDuration)*time.Second, redisServer.TTL(key))
}

func TestRedisFixedWindowIsAtomicUnderConcurrency(t *testing.T) {
	redisServer, _ := useRateLimitMiniRedis(t)
	const (
		requestCount = 20
		maximumCount = 7
		duration     = int64(41)
	)
	key := redisIPRateLimitKey("CONCURRENT", "192.0.2.40")

	var allowedCount atomic.Int64
	errorsFound := make(chan error, requestCount)
	var waitGroup sync.WaitGroup
	waitGroup.Add(requestCount)
	for range requestCount {
		go func() {
			defer waitGroup.Done()
			allowed, _, _, err := redisFixedWindowTake(context.Background(), key, maximumCount, duration)
			if err != nil {
				errorsFound <- err
				return
			}
			if allowed {
				allowedCount.Add(1)
			}
		}()
	}
	waitGroup.Wait()
	close(errorsFound)
	for err := range errorsFound {
		require.NoError(t, err)
	}

	assert.Equal(t, int64(maximumCount), allowedCount.Load())
	count, err := redisServer.Get(key)
	require.NoError(t, err)
	assert.Equal(t, "20", count)
	assert.Equal(t, time.Duration(duration)*time.Second, redisServer.TTL(key))
}

func TestRedisFixedWindowResetsAtBoundary(t *testing.T) {
	redisServer, _ := useRateLimitMiniRedis(t)
	const duration = int64(10)
	key := redisIPRateLimitKey("BOUNDARY", "192.0.2.50")

	for range 2 {
		allowed, _, _, err := redisFixedWindowTake(context.Background(), key, 2, duration)
		require.NoError(t, err)
		assert.True(t, allowed)
	}
	allowed, _, _, err := redisFixedWindowTake(context.Background(), key, 2, duration)
	require.NoError(t, err)
	assert.False(t, allowed)

	// This reset is intentional fixed-window behavior. A client can consume one
	// full allowance immediately before and another immediately after a boundary.
	redisServer.FastForward(time.Duration(duration) * time.Second)
	for range 2 {
		allowed, _, _, err = redisFixedWindowTake(context.Background(), key, 2, duration)
		require.NoError(t, err)
		assert.True(t, allowed)
	}
}

func TestRedisFixedWindowRepairsCounterWithoutTTL(t *testing.T) {
	redisServer, _ := useRateLimitMiniRedis(t)
	const duration = int64(29)
	key := redisIPRateLimitKey("MISSING-TTL", "192.0.2.51")
	redisServer.Set(key, "5")

	allowed, count, ttl, err := redisFixedWindowTake(context.Background(), key, 3, duration)
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Equal(t, int64(6), count)
	assert.Equal(t, duration, ttl)
	assert.Equal(t, time.Duration(duration)*time.Second, redisServer.TTL(key))

	redisServer.FastForward(time.Duration(duration) * time.Second)
	assert.False(t, redisServer.Exists(key), "a recovered counter must not remain permanently rate-limited")
}

func TestRedisFailurePolicies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, redisClient := useRateLimitMiniRedis(t)
	require.NoError(t, redisClient.Close())

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.GET("/ip", rateLimitFactory(1, 30, "FAIL-IP"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.GET(
		"/user",
		func(c *gin.Context) { c.Set("id", 7) },
		userRateLimitFactory(1, 30, "FAIL-USER"),
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)
	router.GET("/email", EmailVerificationRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	ipResponse := performRateLimitRequest(router, "/ip", "192.0.2.60:12345")
	assert.Equal(t, http.StatusInternalServerError, ipResponse.Code)
	assert.Empty(t, ipResponse.Body.String())
	userResponse := performRateLimitRequest(router, "/user", "192.0.2.61:12345")
	assert.Equal(t, http.StatusInternalServerError, userResponse.Code)
	assert.Empty(t, userResponse.Body.String())
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/email", "192.0.2.62:12345").Code)
}

func TestGlobalWebRateLimitBypassesOnlyCheckerApprovedRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer, _ := useRateLimitMiniRedis(t)
	previousEnabled := common.GlobalWebRateLimitEnable
	previousLimit := common.GlobalWebRateLimitNum
	previousDuration := common.GlobalWebRateLimitDuration
	common.GlobalWebRateLimitEnable = true
	common.GlobalWebRateLimitNum = 1
	common.GlobalWebRateLimitDuration = 30
	t.Cleanup(func() {
		common.GlobalWebRateLimitEnable = previousEnabled
		common.GlobalWebRateLimitNum = previousLimit
		common.GlobalWebRateLimitDuration = previousDuration
	})

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.Use(GlobalWebRateLimitWithAssetChecker(func(request *http.Request) bool {
		return request.Method == http.MethodGet && request.URL.Path == "/assets/app.js"
	}))
	router.Any("/*path", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	remoteAddr := "192.0.2.80:12345"
	for range 3 {
		assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/assets/app.js", remoteAddr).Code)
	}
	assert.False(t, redisServer.Exists(redisIPRateLimitKey("GW", "192.0.2.80")))
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/dashboard", remoteAddr).Code)
	assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/dashboard", remoteAddr).Code)
}
