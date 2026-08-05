package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

var userRateLimitTestSequence atomic.Uint64

func uniqueUserRateLimitTestMark(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("USER-TEST-%s-%d", t.Name(), userRateLimitTestSequence.Add(1))
}

func TestUserRateLimitUsesAuthenticatedUserInsteadOfClientIP(t *testing.T) {
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
	})

	gin.SetMode(gin.TestMode)
	mark := uniqueUserRateLimitTestMark(t)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-User") == "second" {
			c.Set("id", 2)
		} else {
			c.Set("id", 1)
		}
	})
	router.Use(userRateLimitFactory(1, 60, mark))
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := func(ip string, user string) int {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = ip + ":12345"
		req.Header.Set("X-Test-User", user)
		router.ServeHTTP(recorder, req)
		return recorder.Code
	}

	require.Equal(t, http.StatusNoContent, request("192.0.2.1", "first"))
	require.Equal(t, http.StatusTooManyRequests, request("198.51.100.2", "first"))
	require.Equal(t, http.StatusNoContent, request("198.51.100.2", "second"))
}

func TestUserRateLimitRejectsNonPositiveMaximumWithoutRedisAccess(t *testing.T) {
	previousRedisEnabled := common.RedisEnabled
	previousRDB := common.RDB
	common.RedisEnabled = true
	common.RDB = nil
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRDB
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 1)
	})
	router.Use(userRateLimitFactory(0, 60, uniqueUserRateLimitTestMark(t)))
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
}

func TestUserRateLimitRejectsOutOfRangeDurationWithoutRedisAccess(t *testing.T) {
	previousRedisEnabled := common.RedisEnabled
	previousRDB := common.RDB
	common.RedisEnabled = true
	common.RDB = nil
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRDB
	})

	gin.SetMode(gin.TestMode)
	for _, duration := range []int64{0, -1, maxUserRateLimitDurationSeconds + 1} {
		t.Run(fmt.Sprintf("duration_%d", duration), func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("id", 1)
			})
			router.Use(userRateLimitFactory(1, duration, uniqueUserRateLimitTestMark(t)))
			router.GET("/", func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
			require.Equal(t, http.StatusTooManyRequests, recorder.Code)
		})
	}
}

func TestUserRateLimitExpirationCoversConfiguredDuration(t *testing.T) {
	configuredExpirationSeconds := int64(common.RateLimitKeyExpirationDuration / time.Second)
	durations := []int64{
		1,
		configuredExpirationSeconds,
		configuredExpirationSeconds + 1,
		maxUserRateLimitDurationSeconds,
	}

	for _, duration := range durations {
		t.Run(fmt.Sprintf("duration_%d", duration), func(t *testing.T) {
			expectedRedisExpiration := duration
			if configuredExpirationSeconds > expectedRedisExpiration {
				expectedRedisExpiration = configuredExpirationSeconds
			}
			redisExpiration := userRateLimitRedisExpirationSeconds(duration)
			require.Equal(t, expectedRedisExpiration, redisExpiration)

			expectedMemoryExpiration := time.Duration(duration) * time.Second
			if common.RateLimitKeyExpirationDuration > expectedMemoryExpiration {
				expectedMemoryExpiration = common.RateLimitKeyExpirationDuration
			}
			memoryExpiration := userRateLimitMemoryExpiration(duration)
			require.Equal(t, expectedMemoryExpiration, memoryExpiration)
		})
	}
}

func TestUserRateLimitMemorySharesBudgetAcrossFactories(t *testing.T) {
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
	})

	gin.SetMode(gin.TestMode)
	mark := uniqueUserRateLimitTestMark(t)
	newRouter := func(limiter gin.HandlerFunc) *gin.Engine {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("id", 1)
		})
		router.Use(limiter)
		router.GET("/", func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		})
		return router
	}

	firstRouter := newRouter(userRateLimitFactory(1, 60, mark))
	secondRouter := newRouter(userRateLimitFactory(1, 60, mark))
	request := func(router http.Handler) int {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
		return recorder.Code
	}

	require.Equal(t, http.StatusNoContent, request(firstRouter))
	require.Equal(t, http.StatusTooManyRequests, request(secondRouter))
}
