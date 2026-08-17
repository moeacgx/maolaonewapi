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
const modelRequestRateLimitBatchScript = `
local now = redis.call('TIME')
local nowInSeconds = tonumber(now[1])
local rejected = 0
local states = {}

for index, key in ipairs(KEYS) do
    local argumentIndex = (index - 1) * 3
    local requested = tonumber(ARGV[argumentIndex + 1])
    local rate = tonumber(ARGV[argumentIndex + 2])
    local capacity = tonumber(ARGV[argumentIndex + 3])
    local bucket = redis.call('HMGET', key, 'tokens', 'last_time')
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
        if rejected == 0 then
            rejected = index
        end
    else
        tokens = tokens - requested
    end
    states[index] = {key, tokens, lastTime}
end

if rejected ~= 0 then
    return rejected
end

for index, state in ipairs(states) do
    redis.call('HMSET', state[1], 'tokens', state[2], 'last_time', state[3])
end
return 0
`

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
	admissionRules  []modelRequestRateLimitRule
	finalUsingGroup string
	successRules    []modelRequestRateLimitRule
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

func buildModelRequestRateLimitRules(snapshot *setting.ModelRequestRateLimitSnapshot, userGroup, group string) []modelRequestRateLimitRule {
	total, success := snapshot.GlobalRateLimit()
	rules := make([]modelRequestRateLimitRule, 0, 4)
	rules = append(rules, modelRequestRateLimitRule{name: "global", scope: "global", totalMaxCount: total, successMaxCount: success})
	if total, success, found := snapshot.GetGroupRateLimit(group); found {
		rules = append(rules, modelRequestRateLimitRule{name: "request_group", scope: fmt.Sprintf("request_group:%s", group), totalMaxCount: total, successMaxCount: success})
	}
	if total, success, found := snapshot.GetUserGroupGlobalRateLimit(userGroup); found {
		rules = append(rules, modelRequestRateLimitRule{name: "user_group", scope: fmt.Sprintf("user_group:%s", userGroup), totalMaxCount: total, successMaxCount: success})
	}
	if total, success, found := snapshot.GetUserGroupRateLimit(userGroup, group); found {
		rules = append(rules, modelRequestRateLimitRule{name: "user_group_request_group", scope: fmt.Sprintf("user_group:%s:request_group:%s", userGroup, group), totalMaxCount: total, successMaxCount: success})
	}
	return rules
}

func buildModelRequestRateLimitRule(snapshot *setting.ModelRequestRateLimitSnapshot, userGroup, group string) modelRequestRateLimitRule {
	rules := buildModelRequestRateLimitRules(snapshot, userGroup, group)
	return rules[len(rules)-1]
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

func (requestContext *modelRequestRateLimitContext) resolveSuccessRules(c *gin.Context) []modelRequestRateLimitRule {
	requestContext.finalUsingGroup = currentModelRequestRateLimitGroup(c)
	requestContext.successRules = append([]modelRequestRateLimitRule(nil), requestContext.admissionRules...)
	seen := make(map[string]struct{}, len(requestContext.successRules))
	for _, rule := range requestContext.successRules {
		seen[rule.scope] = struct{}{}
	}
	for _, rule := range buildModelRequestRateLimitRules(requestContext.snapshot, requestContext.userGroup, requestContext.finalUsingGroup) {
		if _, exists := seen[rule.scope]; exists {
			continue
		}
		seen[rule.scope] = struct{}{}
		requestContext.successRules = append(requestContext.successRules, rule)
	}
	return requestContext.successRules
}

func checkRedisModelRequestRateLimit(ctx context.Context, rdb *redis.Client, userId string, duration int64, rules []modelRequestRateLimitRule) (modelRequestRateLimitRule, bool, error) {
	for _, rule := range rules {
		successKey := buildModelRequestRateLimitKey("success", rule.scope, userId)
		allowed, err := checkRedisRateLimit(ctx, rdb, successKey, rule.successMaxCount, duration)
		if err != nil || !allowed {
			return rule, allowed, err
		}
	}

	totalRules := make([]modelRequestRateLimitRule, 0, len(rules))
	keys := make([]string, 0, len(rules))
	args := make([]interface{}, 0, len(rules)*3)
	for _, rule := range rules {
		if rule.totalMaxCount == 0 {
			continue
		}
		totalRules = append(totalRules, rule)
		keys = append(keys, buildModelRequestRateLimitKey("total", rule.scope, userId))
		args = append(args, duration, int64(rule.totalMaxCount), int64(rule.totalMaxCount)*duration)
	}
	if len(keys) == 0 {
		return modelRequestRateLimitRule{}, true, nil
	}

	rejectedIndex, err := rdb.Eval(ctx, modelRequestRateLimitBatchScript, keys, args...).Int()
	if err != nil {
		return modelRequestRateLimitRule{}, false, err
	}
	if rejectedIndex > 0 {
		return totalRules[rejectedIndex-1], false, nil
	}
	return modelRequestRateLimitRule{}, true, nil
}

func checkMemoryModelRequestRateLimit(userId string, duration int64, rules []modelRequestRateLimitRule) (modelRequestRateLimitRule, bool) {
	for _, rule := range rules {
		successKey := buildModelRequestRateLimitMemoryKey("success", rule.scope, userId)
		if !inMemoryRateLimiter.Allow(successKey, rule.successMaxCount, duration) {
			return rule, false
		}
	}
	totalRules := make([]modelRequestRateLimitRule, 0, len(rules))
	totalRequests := make([]common.RateLimitBatchRequest, 0, len(rules))
	for _, rule := range rules {
		if rule.totalMaxCount == 0 {
			continue
		}
		totalRules = append(totalRules, rule)
		totalRequests = append(totalRequests, common.RateLimitBatchRequest{
			Key:           buildModelRequestRateLimitMemoryKey("total", rule.scope, userId),
			MaxRequestNum: rule.totalMaxCount,
			Duration:      duration,
		})
	}
	if rejectedIndex, allowed := inMemoryRateLimiter.RequestBatch(totalRequests); !allowed {
		return totalRules[rejectedIndex], false
	}
	return modelRequestRateLimitRule{}, true
}

// Redis限流处理器
func redisRateLimitHandler(requestContext *modelRequestRateLimitContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		userId := strconv.Itoa(c.GetInt("id"))
		ctx := context.Background()
		rdb := common.RDB

		rejectedRule, allowed, err := checkRedisModelRequestRateLimit(ctx, rdb, userId, requestContext.durationSeconds, requestContext.admissionRules)
		if err != nil {
			fmt.Println("检查模型请求限流失败:", err.Error())
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
			return
		}
		if !allowed {
			abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("您已达到请求数限制：%d分钟内最多请求%d次", requestContext.snapshot.DurationMinutes(), rejectedRule.successMaxCount))
			return
		}

		c.Next()
		if c.Writer.Status() < 400 {
			for _, successRule := range requestContext.resolveSuccessRules(c) {
				successKey := buildModelRequestRateLimitKey("success", successRule.scope, userId)
				recordRedisRequest(ctx, rdb, successKey, successRule.successMaxCount, requestContext.durationSeconds)
			}
		}
	}
}

// 内存限流处理器
func memoryRateLimitHandler(requestContext *modelRequestRateLimitContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		userId := strconv.Itoa(c.GetInt("id"))
		if _, allowed := checkMemoryModelRequestRateLimit(userId, requestContext.durationSeconds, requestContext.admissionRules); !allowed {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}

		c.Next()
		if c.Writer.Status() < 400 {
			for _, successRule := range requestContext.resolveSuccessRules(c) {
				successKey := buildModelRequestRateLimitMemoryKey("success", successRule.scope, userId)
				inMemoryRateLimiter.Request(successKey, successRule.successMaxCount, requestContext.durationSeconds)
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
			admissionGroup:  admissionGroup,
			admissionRules:  buildModelRequestRateLimitRules(snapshot, userGroup, admissionGroup),
		}
		if common.RedisEnabled {
			redisRateLimitHandler(requestContext)(c)
		} else {
			memoryRateLimitHandler(requestContext)(c)
		}
	}
}
