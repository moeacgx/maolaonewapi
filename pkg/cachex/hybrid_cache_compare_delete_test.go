package cachex

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/samber/hot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHybridCacheCompareAndDelete(t *testing.T) {
	cache := NewHybridCache(HybridCacheConfig[int]{
		Namespace:  Namespace("compare-delete-test"),
		RedisCodec: IntCodec{},
		RedisEnabled: func() bool {
			return false
		},
		Memory: func() *hot.HotCache[string, int] {
			return hot.NewHotCache[string, int](hot.LRU, 8).Build()
		},
	})

	require.NoError(t, cache.SetWithTTL("binding", 7, time.Minute))

	deleted, err := cache.CompareAndDelete("binding", 8)
	require.NoError(t, err)
	assert.False(t, deleted)
	value, found, err := cache.Get("binding")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, 7, value)

	deleted, err = cache.CompareAndDelete("binding", 7)
	require.NoError(t, err)
	assert.True(t, deleted)
	_, found, err = cache.Get("binding")
	require.NoError(t, err)
	assert.False(t, found)
}

type blockingIntCodec struct {
	mu      sync.Mutex
	calls   int
	entered chan struct{}
	release chan struct{}
}

func (c *blockingIntCodec) Encode(value int) (string, error) {
	c.mu.Lock()
	c.calls++
	block := c.calls == 2
	c.mu.Unlock()
	if block {
		close(c.entered)
		<-c.release
	}
	return strconv.Itoa(value), nil
}

func (c *blockingIntCodec) Decode(value string) (int, error) {
	return strconv.Atoi(value)
}

func TestHybridCacheCompareAndDeleteDoesNotDeleteConcurrentMemoryReplacement(t *testing.T) {
	codec := &blockingIntCodec{entered: make(chan struct{}), release: make(chan struct{})}
	cache := NewHybridCache(HybridCacheConfig[int]{
		Namespace:  Namespace("compare-delete-concurrent-test"),
		RedisCodec: codec,
		RedisEnabled: func() bool {
			return false
		},
		Memory: func() *hot.HotCache[string, int] {
			return hot.NewHotCache[string, int](hot.LRU, 8).Build()
		},
	})
	require.NoError(t, cache.SetWithTTL("binding", 7, time.Minute))

	compareDone := make(chan struct{})
	var deleted bool
	var compareErr error
	go func() {
		deleted, compareErr = cache.CompareAndDelete("binding", 7)
		close(compareDone)
	}()
	<-codec.entered

	setStarted := make(chan struct{})
	setDone := make(chan error, 1)
	go func() {
		close(setStarted)
		setDone <- cache.SetWithTTL("binding", 8, time.Minute)
	}()
	<-setStarted
	close(codec.release)
	<-compareDone
	require.NoError(t, compareErr)
	assert.True(t, deleted)
	require.NoError(t, <-setDone)

	value, found, err := cache.Get("binding")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, 8, value)
}
