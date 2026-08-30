package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelJSONIncludesConcurrencyLimit(t *testing.T) {
	limit := 12
	data, err := common.Marshal(&Channel{Id: 7, ConcurrencyLimit: &limit})
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(data, &payload))
	assert.Equal(t, float64(12), payload["concurrency_limit"])
}

func TestChannelConcurrencyZeroAndNilAreUnlimited(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	resetChannelConcurrencyForTest()
	zero := 0
	for _, channel := range []*Channel{
		{Id: 1, ConcurrencyLimit: &zero},
		{Id: 2},
	} {
		_, acquired := TryAcquireChannelConcurrencyLease(channel)
		require.True(t, acquired)
		_, acquired = TryAcquireChannelConcurrencyLease(channel)
		require.True(t, acquired)
	}
}

func TestCachedSelectorSkipsFullHigherPriorityChannel(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	resetChannelConcurrencyForTest()
	oldMemory := common.MemoryCacheEnabled
	oldGroups, oldChannels := group2model2channels, channelsIDM
	t.Cleanup(func() {
		common.MemoryCacheEnabled = oldMemory
		group2model2channels, channelsIDM = oldGroups, oldChannels
		resetChannelConcurrencyForTest()
	})

	common.MemoryCacheEnabled = true
	limit := 1
	high := &Channel{Id: 10, Priority: common.GetPointer[int64](10), Weight: common.GetPointer[uint](0), ConcurrencyLimit: &limit}
	low := &Channel{Id: 11, Priority: common.GetPointer[int64](1), Weight: common.GetPointer[uint](0), ConcurrencyLimit: &limit}
	channelsIDM = map[int]*Channel{10: high, 11: low}
	group2model2channels = map[string]map[string][]int{"default": {"gpt-test": {10, 11}}}
	_, acquired := TryAcquireChannelConcurrencyLease(high)
	require.True(t, acquired)

	selected, err := GetRandomSatisfiedChannelWithSelectionExclusions("default", "gpt-test", 0, "", ChannelSelectionExclusions{})
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, 11, selected.Id)
}

func TestCachedSelectorReturnsDistinctErrorWhenAllChannelsFull(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	resetChannelConcurrencyForTest()
	oldMemory := common.MemoryCacheEnabled
	oldGroups, oldChannels := group2model2channels, channelsIDM
	t.Cleanup(func() {
		common.MemoryCacheEnabled = oldMemory
		group2model2channels, channelsIDM = oldGroups, oldChannels
		resetChannelConcurrencyForTest()
	})

	common.MemoryCacheEnabled = true
	limit := 1
	channelsIDM = map[int]*Channel{
		20: {Id: 20, Priority: common.GetPointer[int64](1), Weight: common.GetPointer[uint](0), ConcurrencyLimit: &limit},
		21: {Id: 21, Priority: common.GetPointer[int64](0), Weight: common.GetPointer[uint](0), ConcurrencyLimit: &limit},
	}
	group2model2channels = map[string]map[string][]int{"default": {"gpt-test": {20, 21}}}
	_, acquired := TryAcquireChannelConcurrencyLease(channelsIDM[20])
	require.True(t, acquired)
	_, acquired = TryAcquireChannelConcurrencyLease(channelsIDM[21])
	require.True(t, acquired)

	selected, err := GetRandomSatisfiedChannel("default", "gpt-test", 0, "")
	require.Nil(t, selected)
	assert.ErrorIs(t, err, ErrChannelConcurrencyLimitReached)
}

func TestCachedSelectorKeepsNilWhenAllCandidatesAreExcluded(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	resetChannelConcurrencyForTest()
	oldMemory := common.MemoryCacheEnabled
	oldGroups, oldChannels := group2model2channels, channelsIDM
	t.Cleanup(func() {
		common.MemoryCacheEnabled = oldMemory
		group2model2channels, channelsIDM = oldGroups, oldChannels
		resetChannelConcurrencyForTest()
	})

	common.MemoryCacheEnabled = true
	limit := 1
	channelsIDM = map[int]*Channel{30: {Id: 30, Priority: common.GetPointer[int64](1), ConcurrencyLimit: &limit}}
	group2model2channels = map[string]map[string][]int{"default": {"gpt-test": {30}}}

	selected, err := GetRandomSatisfiedChannelWithSelectionExclusions(
		"default", "gpt-test", 0, "", ChannelSelectionExclusions{ChannelIDs: map[int]struct{}{30: {}}},
	)
	assert.Nil(t, selected)
	assert.NoError(t, err)
}

func TestChannelConcurrencyIsSharedAcrossRedisClients(t *testing.T) {
	redisServer, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(redisServer.Close)

	clientA := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	clientB := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		_ = clientA.Close()
		_ = clientB.Close()
	})

	oldRedisEnabled, oldRedisClient := common.RedisEnabled, common.RDB
	common.RedisEnabled = true
	common.RDB = clientA
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		common.RDB = oldRedisClient
		resetChannelConcurrencyForTest()
	})

	limit := 18
	channel := &Channel{Id: 990010, ConcurrencyLimit: &limit}
	leasesA := make([]*ChannelConcurrencyLease, 0, 10)
	for range 10 {
		lease, acquired := TryAcquireChannelConcurrencyLease(channel)
		require.True(t, acquired)
		leasesA = append(leasesA, lease)
	}

	common.RDB = clientB
	for range 8 {
		lease, acquired := TryAcquireChannelConcurrencyLease(channel)
		require.True(t, acquired)
		require.NotNil(t, lease)
	}
	lease, acquired := TryAcquireChannelConcurrencyLease(channel)
	assert.False(t, acquired, "two containers must share the configured limit")
	assert.Nil(t, lease)

	require.True(t, ReleaseChannelConcurrencyLease(leasesA[0]))
	lease, acquired = TryAcquireChannelConcurrencyLease(channel)
	require.True(t, acquired)
	require.NotNil(t, lease)
	assert.False(t, IsChannelConcurrencyAvailable(channel), "the replacement lease restores the shared count")

	redisServer.FastForward(ChannelConcurrencyLeaseTTL + time.Second)
	assert.True(t, IsChannelConcurrencyAvailable(channel), "expired leases must be cleaned before counting")
}

func TestChannelConcurrencyRedisFailureFailsClosedForFiniteLimit(t *testing.T) {
	redisServer, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	oldRedisEnabled, oldRedisClient := common.RedisEnabled, common.RDB
	common.RedisEnabled = true
	common.RDB = client
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		common.RDB = oldRedisClient
		resetChannelConcurrencyForTest()
	})

	limit := 1
	channel := &Channel{Id: 990011, ConcurrencyLimit: &limit}
	redisServer.Close()

	lease, acquired := TryAcquireChannelConcurrencyLease(channel)
	assert.False(t, acquired)
	assert.Nil(t, lease)
	assert.False(t, IsChannelConcurrencyAvailable(channel))
}

func TestChannelConcurrencyLeaseCanBeRenewedBeforeExpiry(t *testing.T) {
	redisServer, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(redisServer.Close)

	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	oldRedisEnabled, oldRedisClient := common.RedisEnabled, common.RDB
	common.RedisEnabled = true
	common.RDB = client
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		common.RDB = oldRedisClient
		resetChannelConcurrencyForTest()
	})

	limit := 1
	channel := &Channel{Id: 990013, ConcurrencyLimit: &limit}
	lease, acquired := TryAcquireChannelConcurrencyLease(channel)
	require.True(t, acquired)

	redisServer.FastForward(ChannelConcurrencyLeaseTTL - time.Second)
	require.True(t, RenewChannelConcurrencyLease(lease))
	redisServer.FastForward(ChannelConcurrencyLeaseTTL - time.Second)
	assert.False(t, IsChannelConcurrencyAvailable(channel), "renewal must keep the request slot occupied")
	require.True(t, ReleaseChannelConcurrencyLease(lease))
}

func TestChannelConcurrencyReleaseRequiresMatchingLeaseToken(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	resetChannelConcurrencyForTest()

	limit := 1
	channel := &Channel{Id: 990012, ConcurrencyLimit: &limit}
	lease, acquired := TryAcquireChannelConcurrencyLease(channel)
	require.True(t, acquired)

	assert.False(t, ReleaseChannelConcurrencyLease(&ChannelConcurrencyLease{
		ChannelID: channel.Id,
		Token:     "another-request",
	}))
	assert.False(t, IsChannelConcurrencyAvailable(channel))
	require.True(t, ReleaseChannelConcurrencyLease(lease))
	assert.True(t, IsChannelConcurrencyAvailable(channel))
}
