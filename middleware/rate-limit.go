package middleware

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

var timeFormat = "2006-01-02T15:04:05.000Z"

var inMemoryRateLimiter common.InMemoryRateLimiter

var userInMemoryRateLimiters sync.Map

// 同时保证 duration*time.Second 与 duration*1000 不溢出；
// time.Duration 的纳秒范围是其中更严格的边界。
const maxUserRateLimitDurationSeconds int64 = (1<<63 - 1) / int64(time.Second)

var defNext = func(c *gin.Context) {
	c.Next()
}

func redisRateLimiter(c *gin.Context, maxRequestNum int, duration int64, mark string) {
	ctx := context.Background()
	rdb := common.RDB
	key := "rateLimit:" + mark + c.ClientIP()
	listLength, err := rdb.LLen(ctx, key).Result()
	if err != nil {
		fmt.Println(err.Error())
		c.Status(http.StatusInternalServerError)
		c.Abort()
		return
	}
	if listLength < int64(maxRequestNum) {
		rdb.LPush(ctx, key, time.Now().Format(timeFormat))
		rdb.Expire(ctx, key, common.RateLimitKeyExpirationDuration)
	} else {
		oldTimeStr, _ := rdb.LIndex(ctx, key, -1).Result()
		oldTime, err := time.Parse(timeFormat, oldTimeStr)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			c.Abort()
			return
		}
		nowTimeStr := time.Now().Format(timeFormat)
		nowTime, err := time.Parse(timeFormat, nowTimeStr)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			c.Abort()
			return
		}
		// time.Since will return negative number!
		// See: https://stackoverflow.com/questions/50970900/why-is-time-since-returning-negative-durations-on-windows
		if int64(nowTime.Sub(oldTime).Seconds()) < duration {
			rdb.Expire(ctx, key, common.RateLimitKeyExpirationDuration)
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		} else {
			rdb.LPush(ctx, key, time.Now().Format(timeFormat))
			rdb.LTrim(ctx, key, 0, int64(maxRequestNum-1))
			rdb.Expire(ctx, key, common.RateLimitKeyExpirationDuration)
		}
	}
}

func memoryRateLimiter(c *gin.Context, maxRequestNum int, duration int64, mark string) {
	key := mark + c.ClientIP()
	if !inMemoryRateLimiter.Request(key, maxRequestNum, duration) {
		c.Status(http.StatusTooManyRequests)
		c.Abort()
		return
	}
}

func rateLimitFactory(maxRequestNum int, duration int64, mark string) func(c *gin.Context) {
	if common.RedisEnabled {
		return func(c *gin.Context) {
			redisRateLimiter(c, maxRequestNum, duration, mark)
		}
	} else {
		// It's safe to call multi times.
		inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
		return func(c *gin.Context) {
			memoryRateLimiter(c, maxRequestNum, duration, mark)
		}
	}
}

func GlobalWebRateLimit() func(c *gin.Context) {
	return globalWebRateLimit(nil)
}

// GlobalWebRateLimitWithAssetChecker 为已经接入静态文件中间件的网页路由提供
// 文件存在性判断。这样只有确实会被静态文件中间件处理的资源才跳过限流，
// 不存在的 .js/.css 路径仍然受到网页限流保护。
func GlobalWebRateLimitWithAssetChecker(checker func(*http.Request) bool) func(c *gin.Context) {
	return globalWebRateLimit(checker)
}

func globalWebRateLimit(assetChecker func(*http.Request) bool) func(c *gin.Context) {
	if common.GlobalWebRateLimitEnable {
		limiter := rateLimitFactory(common.GlobalWebRateLimitNum, common.GlobalWebRateLimitDuration, "GW")
		return func(c *gin.Context) {
			if assetChecker != nil {
				if c != nil && c.Request != nil && assetChecker(c.Request) {
					c.Next()
					return
				}
			} else if isStaticWebAssetRequest(c) {
				c.Next()
				return
			}
			limiter(c)
		}
	}
	return defNext
}

func isStaticWebAssetRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		return false
	}

	requestPath := strings.ToLower(c.Request.URL.Path)
	if strings.HasPrefix(requestPath, "/assets/") {
		return true
	}

	switch path.Ext(requestPath) {
	case ".avif", ".css", ".gif", ".ico", ".jpeg", ".jpg", ".js", ".json", ".map",
		".mp3", ".mp4", ".ogg", ".png", ".svg", ".txt", ".wasm", ".wav", ".webm",
		".webmanifest", ".webp", ".woff", ".woff2":
		return true
	default:
		return false
	}
}

func GlobalAPIRateLimit() func(c *gin.Context) {
	if common.GlobalApiRateLimitEnable {
		limiter := rateLimitFactory(common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration, "GA")
		return func(c *gin.Context) {
			// 安全审计是 Root-only 管理接口。详情查看、删除预览等操作
			// 不能因为普通 API 全局限流而返回 429；RootAuth 仍在路由组中执行。
			if c != nil && c.Request != nil && c.Request.URL != nil {
				requestPath := c.Request.URL.Path
				if requestPath == "/api/security-audit" ||
					strings.HasPrefix(requestPath, "/api/security-audit/") {
					c.Next()
					return
				}
			}
			limiter(c)
		}
	}
	return defNext
}

func CriticalRateLimit() func(c *gin.Context) {
	if common.CriticalRateLimitEnable {
		return rateLimitFactory(common.CriticalRateLimitNum, common.CriticalRateLimitDuration, "CT")
	}
	return defNext
}

// UserCriticalRateLimit 按认证用户限流，避免同一账号轮换代理 IP 绕过关键操作保护。
// 必须放在 UserAuth、AdminAuth 或 RootAuth 之后使用。
func UserCriticalRateLimit() func(c *gin.Context) {
	if common.CriticalRateLimitEnable {
		return userRateLimitFactory(common.CriticalRateLimitNum, common.CriticalRateLimitDuration, "UCT")
	}
	return defNext
}

func DownloadRateLimit() func(c *gin.Context) {
	return rateLimitFactory(common.DownloadRateLimitNum, common.DownloadRateLimitDuration, "DW")
}

func UploadRateLimit() func(c *gin.Context) {
	return rateLimitFactory(common.UploadRateLimitNum, common.UploadRateLimitDuration, "UP")
}

// userRateLimitFactory 按认证用户 ID 限流，必须放在认证中间件之后使用。
func userRateLimitFactory(maxRequestNum int, duration int64, mark string) func(c *gin.Context) {
	if maxRequestNum <= 0 || duration <= 0 || duration > maxUserRateLimitDurationSeconds {
		return func(c *gin.Context) {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
		}
	}
	if common.RedisEnabled {
		return func(c *gin.Context) {
			userId := c.GetInt("id")
			if userId == 0 {
				c.Status(http.StatusUnauthorized)
				c.Abort()
				return
			}
			key := fmt.Sprintf("rateLimit:%s:user:%d", mark, userId)
			userRedisRateLimiter(c, maxRequestNum, duration, key)
		}
	}
	limiter := getUserInMemoryRateLimiter(mark, duration)
	return func(c *gin.Context) {
		userId := c.GetInt("id")
		if userId == 0 {
			c.Status(http.StatusUnauthorized)
			c.Abort()
			return
		}
		key := fmt.Sprintf("%s:user:%d", mark, userId)
		if !limiter.Request(key, maxRequestNum, duration) {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}
	}
}

type userInMemoryRateLimiterPolicy struct {
	mark       string
	duration   int64
	expiration time.Duration
}

func getUserInMemoryRateLimiter(mark string, duration int64) *common.InMemoryRateLimiter {
	expiration := userRateLimitMemoryExpiration(duration)
	policy := userInMemoryRateLimiterPolicy{
		mark:       mark,
		duration:   duration,
		expiration: expiration,
	}
	limiter, _ := userInMemoryRateLimiters.LoadOrStore(policy, &common.InMemoryRateLimiter{})
	typedLimiter := limiter.(*common.InMemoryRateLimiter)
	// Init 内部有双重检查，同一策略并发创建时仍只会初始化一次。
	typedLimiter.Init(expiration)
	return typedLimiter
}

func userRateLimitMemoryExpiration(duration int64) time.Duration {
	expiration := time.Duration(duration) * time.Second
	if common.RateLimitKeyExpirationDuration > expiration {
		expiration = common.RateLimitKeyExpirationDuration
	}
	return expiration
}

func userRateLimitRedisExpirationSeconds(duration int64) int64 {
	expirationSeconds := int64(common.RateLimitKeyExpirationDuration / time.Second)
	if duration > expirationSeconds {
		expirationSeconds = duration
	}
	return expirationSeconds
}

const userRedisRateLimitScript = `
local max_requests = tonumber(ARGV[1])
local duration_ms = tonumber(ARGV[2])
local now_ms = tonumber(ARGV[3])
local expiration_seconds = tonumber(ARGV[4])

if max_requests == nil or max_requests <= 0 then
    return 0
end

local list_length = redis.call("LLEN", KEYS[1])

if list_length < max_requests then
    redis.call("LPUSH", KEYS[1], now_ms)
    redis.call("EXPIRE", KEYS[1], expiration_seconds)
    return 1
end

local oldest_ms = tonumber(redis.call("LINDEX", KEYS[1], -1))
if oldest_ms == nil then
    redis.call("DEL", KEYS[1])
    redis.call("LPUSH", KEYS[1], now_ms)
    redis.call("EXPIRE", KEYS[1], expiration_seconds)
    return 1
end

if now_ms - oldest_ms < duration_ms then
    redis.call("EXPIRE", KEYS[1], expiration_seconds)
    return 0
end

redis.call("LPUSH", KEYS[1], now_ms)
redis.call("LTRIM", KEYS[1], 0, max_requests - 1)
redis.call("EXPIRE", KEYS[1], expiration_seconds)
return 1
`

// userRedisRateLimiter 使用单条 Lua 脚本完成检查与写入，避免并发突发绕过限制。
func userRedisRateLimiter(c *gin.Context, maxRequestNum int, duration int64, key string) {
	allowed, err := common.RDB.Eval(
		context.Background(),
		userRedisRateLimitScript,
		[]string{key},
		maxRequestNum,
		duration*1000,
		time.Now().UnixMilli(),
		userRateLimitRedisExpirationSeconds(duration),
	).Int()
	if err != nil {
		fmt.Println(err.Error())
		c.Status(http.StatusInternalServerError)
		c.Abort()
		return
	}
	if allowed != 1 {
		c.Status(http.StatusTooManyRequests)
		c.Abort()
	}
}

// SearchRateLimit returns a per-user rate limiter for search endpoints.
// Configurable via SEARCH_RATE_LIMIT_ENABLE / SEARCH_RATE_LIMIT / SEARCH_RATE_LIMIT_DURATION.
func SearchRateLimit() func(c *gin.Context) {
	if !common.SearchRateLimitEnable {
		return defNext
	}
	return userRateLimitFactory(common.SearchRateLimitNum, common.SearchRateLimitDuration, "SR")
}
