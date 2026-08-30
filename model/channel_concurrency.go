package model

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

// ErrChannelConcurrencyLimitReached means eligible channels exist but all are
// currently occupied. It is distinct from a missing model or group.
var ErrChannelConcurrencyLimitReached = errors.New("channel concurrency limit reached")

const (
	channelConcurrencyRedisKeyPrefix = "new-api:channel-concurrency:"
	// 租约过期后由 Lua 脚本清理，避免实例异常退出永久占用槽位。
	ChannelConcurrencyLeaseTTL            = 2 * time.Minute
	ChannelConcurrencyLeaseRenewInterval  = ChannelConcurrencyLeaseTTL / 3
	channelConcurrencyRedisOperationLimit = time.Second
)

var (
	channelConcurrencyAcquireScript = redis.NewScript(`
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local token = ARGV[2]
local ttl = tonumber(ARGV[3])
local nowParts = redis.call('TIME')
local now = tonumber(nowParts[1]) * 1000 + math.floor(tonumber(nowParts[2]) / 1000)

redis.call('ZREMRANGEBYSCORE', key, '-inf', now)
if redis.call('ZCARD', key) >= limit then
  return 0
end

redis.call('ZADD', key, now + ttl, token)
redis.call('PEXPIRE', key, ttl)
return 1
`)

	channelConcurrencyAvailableScript = redis.NewScript(`
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local nowParts = redis.call('TIME')
local now = tonumber(nowParts[1]) * 1000 + math.floor(tonumber(nowParts[2]) / 1000)

redis.call('ZREMRANGEBYSCORE', key, '-inf', now)
if redis.call('ZCARD', key) < limit then
  return 1
end
return 0
`)

	channelConcurrencyRenewScript = redis.NewScript(`
local key = KEYS[1]
local token = ARGV[1]
local ttl = tonumber(ARGV[2])
local nowParts = redis.call('TIME')
local now = tonumber(nowParts[1]) * 1000 + math.floor(tonumber(nowParts[2]) / 1000)

redis.call('ZREMRANGEBYSCORE', key, '-inf', now)
if redis.call('ZSCORE', key, token) == false then
  return 0
end

redis.call('ZADD', key, now + ttl, token)
redis.call('PEXPIRE', key, ttl)
return 1
`)

	channelConcurrencyReleaseScript = redis.NewScript(`
local key = KEYS[1]
local token = ARGV[1]
if redis.call('ZREM', key, token) == 1 then
  if redis.call('ZCARD', key) == 0 then
    redis.call('DEL', key)
  end
  return 1
end
return 0
`)
)

var channelConcurrency = struct {
	sync.Mutex
	active map[int]map[string]struct{}
}{
	active: make(map[int]map[string]struct{}),
}

// ChannelConcurrencyLease 精确标识一个已占用的渠道槽位。
// Token 只用于匹配 Redis 或本地释放操作，不会发送给客户端。
type ChannelConcurrencyLease struct {
	ChannelID int
	Token     string
}

func IsChannelConcurrencyAvailable(channel *Channel) bool {
	if channel == nil || channel.GetConcurrencyLimit() <= 0 {
		return true
	}

	if common.RedisEnabled {
		available, err := redisChannelConcurrencyAvailable(channel.Id, channel.GetConcurrencyLimit())
		if err != nil {
			// Redis 开启时不能退回进程内计数，否则多容器会再次突破全局上限。
			common.SysError(fmt.Sprintf("channel concurrency Redis check failed for channel #%d: %v", channel.Id, err))
			return false
		}
		return available
	}

	channelConcurrency.Lock()
	defer channelConcurrency.Unlock()
	return len(channelConcurrency.active[channel.Id]) < channel.GetConcurrencyLimit()
}

// TryAcquireChannelConcurrencyLease 原子抢占一个渠道租约。
// Redis 开启时，有限上限统一使用 Redis；Redis 故障时 fail-closed，不回退到进程内计数。
func TryAcquireChannelConcurrencyLease(channel *Channel) (*ChannelConcurrencyLease, bool) {
	if channel == nil || channel.GetConcurrencyLimit() <= 0 {
		return nil, true
	}

	token, err := newChannelConcurrencyToken()
	if err != nil {
		common.SysError(fmt.Sprintf("failed to generate channel concurrency lease: %v", err))
		return nil, false
	}
	lease := &ChannelConcurrencyLease{ChannelID: channel.Id, Token: token}

	if common.RedisEnabled {
		acquired, err := redisChannelConcurrencyAcquire(channel.Id, channel.GetConcurrencyLimit(), token)
		if err != nil {
			common.SysError(fmt.Sprintf("channel concurrency Redis acquire failed for channel #%d: %v", channel.Id, err))
			return nil, false
		}
		if !acquired {
			return nil, false
		}
		return lease, true
	}

	channelConcurrency.Lock()
	defer channelConcurrency.Unlock()
	if len(channelConcurrency.active[channel.Id]) >= channel.GetConcurrencyLimit() {
		return nil, false
	}
	if channelConcurrency.active[channel.Id] == nil {
		channelConcurrency.active[channel.Id] = make(map[string]struct{})
	}
	channelConcurrency.active[channel.Id][token] = struct{}{}
	return lease, true
}

// RenewChannelConcurrencyLease 延长有效租约。Redis 时间是权威时间源，
// 避免容器系统时钟不一致导致租约被提前缩短。
func RenewChannelConcurrencyLease(lease *ChannelConcurrencyLease) bool {
	if lease == nil || lease.ChannelID <= 0 || lease.Token == "" {
		return false
	}
	if common.RedisEnabled {
		renewed, err := redisChannelConcurrencyRenew(lease)
		if err != nil {
			common.SysError(fmt.Sprintf("channel concurrency Redis renew failed for channel #%d: %v", lease.ChannelID, err))
			return false
		}
		return renewed
	}

	channelConcurrency.Lock()
	defer channelConcurrency.Unlock()
	_, ok := channelConcurrency.active[lease.ChannelID][lease.Token]
	return ok
}

// ReleaseChannelConcurrencyLease 精确释放请求持有的租约，不支持只按渠道 ID 释放。
func ReleaseChannelConcurrencyLease(lease *ChannelConcurrencyLease) bool {
	if lease == nil || lease.ChannelID <= 0 || lease.Token == "" {
		return false
	}
	if common.RedisEnabled {
		released, err := redisChannelConcurrencyRelease(lease)
		if err != nil {
			// 释放失败时不伪造本地成功；租约会由 TTL 回收。
			common.SysError(fmt.Sprintf("channel concurrency Redis release failed for channel #%d: %v", lease.ChannelID, err))
			return false
		}
		return released
	}

	channelConcurrency.Lock()
	defer channelConcurrency.Unlock()
	active := channelConcurrency.active[lease.ChannelID]
	if active == nil {
		return false
	}
	if _, ok := active[lease.Token]; !ok {
		return false
	}
	delete(active, lease.Token)
	if len(active) == 0 {
		delete(channelConcurrency.active, lease.ChannelID)
	}
	return true
}

func redisChannelConcurrencyAvailable(channelID int, limit int) (bool, error) {
	if common.RDB == nil {
		return false, errors.New("Redis client is nil")
	}
	ctx, cancel := contextWithChannelConcurrencyTimeout()
	defer cancel()
	result, err := channelConcurrencyAvailableScript.Run(ctx, common.RDB, []string{channelConcurrencyRedisKey(channelID)}, limit).Int64()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func redisChannelConcurrencyAcquire(channelID int, limit int, token string) (bool, error) {
	if common.RDB == nil {
		return false, errors.New("Redis client is nil")
	}
	ctx, cancel := contextWithChannelConcurrencyTimeout()
	defer cancel()
	result, err := channelConcurrencyAcquireScript.Run(
		ctx,
		common.RDB,
		[]string{channelConcurrencyRedisKey(channelID)},
		limit,
		token,
		ChannelConcurrencyLeaseTTL.Milliseconds(),
	).Int64()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func redisChannelConcurrencyRenew(lease *ChannelConcurrencyLease) (bool, error) {
	if common.RDB == nil {
		return false, errors.New("Redis client is nil")
	}
	ctx, cancel := contextWithChannelConcurrencyTimeout()
	defer cancel()
	result, err := channelConcurrencyRenewScript.Run(
		ctx,
		common.RDB,
		[]string{channelConcurrencyRedisKey(lease.ChannelID)},
		lease.Token,
		ChannelConcurrencyLeaseTTL.Milliseconds(),
	).Int64()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func redisChannelConcurrencyRelease(lease *ChannelConcurrencyLease) (bool, error) {
	if common.RDB == nil {
		return false, errors.New("Redis client is nil")
	}
	ctx, cancel := contextWithChannelConcurrencyTimeout()
	defer cancel()
	result, err := channelConcurrencyReleaseScript.Run(
		ctx,
		common.RDB,
		[]string{channelConcurrencyRedisKey(lease.ChannelID)},
		lease.Token,
	).Int64()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func contextWithChannelConcurrencyTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), channelConcurrencyRedisOperationLimit)
}

func channelConcurrencyRedisKey(channelID int) string {
	return fmt.Sprintf("%s%d", channelConcurrencyRedisKeyPrefix, channelID)
}

func newChannelConcurrencyToken() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}

func resetChannelConcurrencyForTest() {
	channelConcurrency.Lock()
	defer channelConcurrency.Unlock()
	channelConcurrency.active = make(map[int]map[string]struct{})
}
