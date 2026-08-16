package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/limiter"
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
	admissionGroup  string
	admissionRule   modelRequestRateLimitRule
	finalUsingGroup string
	successRule     modelRequestRateLimitRule
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
	if total, success, found := snapshot.GetUserGroupGlobalRateLimit(userGroup); found {
		return modelRequestRateLimitRule{name: "user_group", scope: fmt.Sprintf("user_group:%s", userGroup), totalMaxCount: total, successMaxCount: success}
	}
	if total, success, found := snapshot.GetGroupRateLimit(group); found {
		return modelRequestRateLimitRule{name: "request_group", scope: fmt.Sprintf("request_group:%s", group), totalMaxCount: total, successMaxCount: success}
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

func (requestContext *modelRequestRateLimitContext) resolveSuccessRule(c *gin.Context) modelRequestRateLimitRule {
	requestContext.finalUsingGroup = currentModelRequestRateLimitGroup(c)
	requestContext.successRule = buildModelRequestRateLimitRule(requestContext.snapshot, requestContext.userGroup, requestContext.finalUsingGroup)
	return requestContext.successRule
}

func checkRedisModelRequestRateLimit(ctx context.Context, rdb *redis.Client, userId string, duration int64, rule modelRequestRateLimitRule) (bool, error) {
	successKey := buildModelRequestRateLimitKey("success", rule.scope, userId)
	allowed, err := checkRedisRateLimit(ctx, rdb, successKey, rule.successMaxCount, duration)
	if err != nil || !allowed {
		return allowed, err
	}
	if rule.totalMaxCount == 0 {
		return true, nil
	}
	totalKey := buildModelRequestRateLimitKey("total", rule.scope, userId)
	tokenBucket := limiter.New(ctx, rdb)
	return tokenBucket.Allow(ctx, totalKey, limiter.WithCapacity(int64(rule.totalMaxCount)*duration), limiter.WithRate(int64(rule.totalMaxCount)), limiter.WithRequested(duration))
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
			successRule := requestContext.resolveSuccessRule(c)
			successKey := buildModelRequestRateLimitKey("success", successRule.scope, userId)
			recordRedisRequest(ctx, rdb, successKey, successRule.successMaxCount, requestContext.durationSeconds)
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
			successRule := requestContext.resolveSuccessRule(c)
			successKey := buildModelRequestRateLimitMemoryKey("success", successRule.scope, userId)
			inMemoryRateLimiter.Request(successKey, successRule.successMaxCount, requestContext.durationSeconds)
		}
	}
}

func checkMemoryModelRequestRateLimit(userId string, duration int64, rule modelRequestRateLimitRule) bool {
	successKey := buildModelRequestRateLimitMemoryKey("success", rule.scope, userId)
	if !inMemoryRateLimiter.Allow(successKey, rule.successMaxCount, duration) {
		return false
	}
	if rule.totalMaxCount == 0 {
		return true
	}
	totalKey := buildModelRequestRateLimitMemoryKey("total", rule.scope, userId)
	return inMemoryRateLimiter.Request(totalKey, rule.totalMaxCount, duration)
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
			admissionGroup:  admissionGroup,
			admissionRule:   buildModelRequestRateLimitRule(snapshot, userGroup, admissionGroup),
		}
		if common.RedisEnabled {
			redisRateLimitHandler(requestContext)(c)
		} else {
			memoryRateLimitHandler(requestContext)(c)
		}
	}
}
