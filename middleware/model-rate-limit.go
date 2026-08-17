package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const (
	ModelRequestRateLimitCountMark        = "MRRL"
	ModelRequestRateLimitSuccessCountMark = "MRRLS"
	modelRateLimitTimeFormat              = "2006-01-02T15:04:05.000Z"
)
const modelRequestRateLimitTotalScript = `
local now = redis.call('TIME')
local nowInSeconds = tonumber(now[1])
local requested = tonumber(ARGV[1])
local rate = tonumber(ARGV[2])
local capacity = tonumber(ARGV[3])
local bucket = redis.call('HMGET', KEYS[1], 'tokens', 'last_time')
local tokens = tonumber(bucket[1])
local lastTime = tonumber(bucket[2])

if not tokens or not lastTime then
    tokens = capacity
    lastTime = nowInSeconds
else
    local elapsed = nowInSeconds - lastTime
    tokens = math.min(capacity, tokens + elapsed * rate)
    lastTime = nowInSeconds
end

if tokens < requested then
    return 0
end

tokens = tokens - requested
redis.call('HMSET', KEYS[1], 'tokens', tokens, 'last_time', lastTime)
return 1
`

var modelRequestRateLimitTotalRedisScript = redis.NewScript(modelRequestRateLimitTotalScript)

type modelRequestRateLimitRule struct {
	name            string
	scope           string
	totalMaxCount   int
	successMaxCount int
}

type modelRequestRateLimitContext struct {
	snapshot        *setting.ModelRequestRateLimitSnapshot
	durationSeconds int64
	userGroup       string
	admissionRule   modelRequestRateLimitRule
	finalRule       modelRequestRateLimitRule
}

// 检查Redis中的请求限制
func checkRedisRateLimit(ctx context.Context, rdb *redis.Client, key string, maxCount int, duration int64) (bool, error) {
	// 如果maxCount为0，表示不限制
	if maxCount == 0 {
		return true, nil
	}

	// 获取当前计数
	length, err := rdb.LLen(ctx, key).Result()
	if err != nil {
		return false, err
	}

	// 如果未达到限制，允许请求
	if length < int64(maxCount) {
		return true, nil
	}

	// 检查时间窗口
	oldTimeStr, _ := rdb.LIndex(ctx, key, -1).Result()
	oldTime, err := time.Parse(modelRateLimitTimeFormat, oldTimeStr)
	if err != nil {
		return false, err
	}

	nowTimeStr := time.Now().UTC().Format(modelRateLimitTimeFormat)
	nowTime, err := time.Parse(modelRateLimitTimeFormat, nowTimeStr)
	if err != nil {
		return false, err
	}
	// 如果在时间窗口内已达到限制，拒绝请求
	subTime := nowTime.Sub(oldTime).Seconds()
	if int64(subTime) < duration {
		rdb.Expire(ctx, key, time.Duration(duration)*time.Second)
		return false, nil
	}

	return true, nil
}

// 记录Redis请求
func recordRedisRequest(ctx context.Context, rdb *redis.Client, key string, maxCount int, duration int64) {
	// 如果maxCount为0，不记录请求
	if maxCount == 0 {
		return
	}

	now := time.Now().UTC().Format(modelRateLimitTimeFormat)
	rdb.LPush(ctx, key, now)
	rdb.LTrim(ctx, key, 0, int64(maxCount-1))
	rdb.Expire(ctx, key, time.Duration(duration)*time.Second)
}

func buildModelRequestRateLimitRule(snapshot *setting.ModelRequestRateLimitSnapshot, userGroup, group string) modelRequestRateLimitRule {
	if total, success, found := snapshot.GetUserGroupRateLimit(userGroup, group); found {
		return modelRequestRateLimitRule{name: "user_group_request_group", scope: fmt.Sprintf("user_group:%s:request_group:%s", userGroup, group), totalMaxCount: total, successMaxCount: success}
	}
	if total, success, found := snapshot.GetGroupRateLimit(group); found {
		return modelRequestRateLimitRule{name: "request_group", scope: fmt.Sprintf("request_group:%s", group), totalMaxCount: total, successMaxCount: success}
	}
	if total, success, found := snapshot.GetUserGroupGlobalRateLimit(userGroup); found {
		return modelRequestRateLimitRule{name: "user_group", scope: fmt.Sprintf("user_group:%s", userGroup), totalMaxCount: total, successMaxCount: success}
	}
	total, success := snapshot.GlobalRateLimit()
	return modelRequestRateLimitRule{name: "global", scope: "global", totalMaxCount: total, successMaxCount: success}
}

func escapeModelRequestRateLimitScope(scope string) string {
	return url.QueryEscape(scope)
}

func buildModelRequestRateLimitKey(limitType, scope, userId string) string {
	return fmt.Sprintf("rateLimit:model_request:v2:%s:%s:user:%s", limitType, escapeModelRequestRateLimitScope(scope), userId)
}

func buildModelRequestRateLimitMemoryKey(limitType, scope, userId string) string {
	return fmt.Sprintf("%s:v2:%s:%s:user:%s", ModelRequestRateLimitCountMark, limitType, escapeModelRequestRateLimitScope(scope), userId)
}

func currentModelRequestRateLimitGroup(c *gin.Context) string {
	group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if group == "" {
		group = common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
	}
	if group == "" {
		group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	}
	return group
}

func (requestContext *modelRequestRateLimitContext) resolveFinalRule(c *gin.Context) (modelRequestRateLimitRule, bool) {
	finalGroup := currentModelRequestRateLimitGroup(c)
	requestContext.finalRule = buildModelRequestRateLimitRule(requestContext.snapshot, requestContext.userGroup, finalGroup)
	return requestContext.finalRule, requestContext.finalRule.scope != requestContext.admissionRule.scope
}

func requestRedisModelRateLimitTotal(ctx context.Context, rdb *redis.Client, userId string, duration int64, rule modelRequestRateLimitRule) (bool, error) {
	if rule.totalMaxCount == 0 {
		return true, nil
	}

	key := buildModelRequestRateLimitKey("total", rule.scope, userId)
	allowed, err := modelRequestRateLimitTotalRedisScript.Run(
		ctx,
		rdb,
		[]string{key},
		duration,
		int64(rule.totalMaxCount),
		int64(rule.totalMaxCount)*duration,
	).Int()
	if err != nil {
		return false, err
	}
	return allowed == 1, nil
}

func checkRedisModelRequestRateLimit(ctx context.Context, rdb *redis.Client, userId string, duration int64, rule modelRequestRateLimitRule) (bool, error) {
	successKey := buildModelRequestRateLimitKey("success", rule.scope, userId)
	allowed, err := checkRedisRateLimit(ctx, rdb, successKey, rule.successMaxCount, duration)
	if err != nil || !allowed {
		return allowed, err
	}
	return requestRedisModelRateLimitTotal(ctx, rdb, userId, duration, rule)
}

func checkMemoryModelRequestRateLimit(userId string, duration int64, rule modelRequestRateLimitRule) bool {
	successKey := buildModelRequestRateLimitMemoryKey("success", rule.scope, userId)
	if !inMemoryRateLimiter.Allow(successKey, rule.successMaxCount, duration) {
		return false
	}
	if rule.totalMaxCount == 0 {
		return true
	}
	return inMemoryRateLimiter.Request(
		buildModelRequestRateLimitMemoryKey("total", rule.scope, userId),
		rule.totalMaxCount,
		duration,
	)
}

// Redis限流处理器
func redisRateLimitHandler(requestContext *modelRequestRateLimitContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		userId := strconv.Itoa(c.GetInt("id"))
		ctx := context.Background()
		rdb := common.RDB

		allowed, err := checkRedisModelRequestRateLimit(ctx, rdb, userId, requestContext.durationSeconds, requestContext.admissionRule)
		if err != nil {
			fmt.Println("检查模型请求限流失败:", err.Error())
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
			return
		}
		if !allowed {
			abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("您已达到请求数限制：%d分钟内最多请求%d次", requestContext.snapshot.DurationMinutes(), requestContext.admissionRule.successMaxCount))
			return
		}

		c.Next()
		if c.Writer.Status() < 400 {
			admissionKey := buildModelRequestRateLimitKey("success", requestContext.admissionRule.scope, userId)
			recordRedisRequest(ctx, rdb, admissionKey, requestContext.admissionRule.successMaxCount, requestContext.durationSeconds)
			if finalRule, differs := requestContext.resolveFinalRule(c); differs {
				finalKey := buildModelRequestRateLimitKey("success", finalRule.scope, userId)
				recordRedisRequest(ctx, rdb, finalKey, finalRule.successMaxCount, requestContext.durationSeconds)
			}
		}
	}
}

// 内存限流处理器
func memoryRateLimitHandler(requestContext *modelRequestRateLimitContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		userId := strconv.Itoa(c.GetInt("id"))
		if !checkMemoryModelRequestRateLimit(userId, requestContext.durationSeconds, requestContext.admissionRule) {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}

		c.Next()
		if c.Writer.Status() < 400 {
			admissionKey := buildModelRequestRateLimitMemoryKey("success", requestContext.admissionRule.scope, userId)
			inMemoryRateLimiter.Request(admissionKey, requestContext.admissionRule.successMaxCount, requestContext.durationSeconds)
			if finalRule, differs := requestContext.resolveFinalRule(c); differs {
				finalKey := buildModelRequestRateLimitMemoryKey("success", finalRule.scope, userId)
				inMemoryRateLimiter.Request(finalKey, finalRule.successMaxCount, requestContext.durationSeconds)
			}
		}
	}
}

// ModelRequestRateLimit 模型请求限流中间件
func ModelRequestRateLimit() func(c *gin.Context) {
	inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
	return func(c *gin.Context) {
		snapshot := setting.GetModelRequestRateLimitSnapshot()
		if !snapshot.Enabled() {
			c.Next()
			return
		}

		userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
		admissionGroup := currentModelRequestRateLimitGroup(c)
		requestContext := &modelRequestRateLimitContext{
			snapshot:        snapshot,
			durationSeconds: int64(snapshot.DurationMinutes()) * 60,
			userGroup:       userGroup,
			admissionRule:   buildModelRequestRateLimitRule(snapshot, userGroup, admissionGroup),
		}
		if common.RedisEnabled {
			redisRateLimitHandler(requestContext)(c)
		} else {
			memoryRateLimitHandler(requestContext)(c)
		}
	}
}
