package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvictChannelAffinityBindingForFailureRemovesOnlyHitBinding(t *testing.T) {
	cacheKeySuffix := fmt.Sprintf("failure-eviction-%d", time.Now().UnixNano())
	cacheKey := channelAffinityCacheNamespace + ":" + cacheKeySuffix
	t.Cleanup(func() {
		_, _ = getChannelAffinityCache().DeleteMany([]string{cacheKey})
		_, _ = getChannelAffinityBindingCache().DeleteMany([]string{cacheKeySuffix})
	})

	binding := channelAffinityBinding{Version: 3, ChannelID: 17, Revision: "before-failure"}
	require.NoError(t, getChannelAffinityCache().SetWithTTL(cacheKey, binding.ChannelID, time.Minute))
	require.NoError(t, getChannelAffinityBindingCache().SetWithTTL(cacheKeySuffix, binding, time.Minute))

	ctx := affinityJSONContext(`{}`)
	setChannelAffinityContext(ctx, channelAffinityMeta{CacheKey: cacheKey, CacheKeySuffix: cacheKeySuffix, TTLSeconds: 60})
	ctx.Set(ginKeyChannelAffinityCandidate, channelAffinityCandidate{Binding: binding, BindingCache: true})
	MarkChannelAffinityUsed(ctx, "default", binding.ChannelID)

	require.True(t, EvictChannelAffinityBindingForFailure(ctx, binding.ChannelID))
	_, found, err := getChannelAffinityCache().Get(cacheKeySuffix)
	require.NoError(t, err)
	assert.False(t, found)
	_, found, err = getChannelAffinityBindingCache().Get(cacheKeySuffix)
	require.NoError(t, err)
	assert.False(t, found)
	assert.False(t, ShouldSkipRetryAfterChannelAffinityFailure(ctx))
}

func TestEvictChannelAffinityBindingForFailurePreservesRetryRule(t *testing.T) {
	cacheKeySuffix := fmt.Sprintf("failure-eviction-retry-rule-%d", time.Now().UnixNano())
	cacheKey := channelAffinityCacheNamespace + ":" + cacheKeySuffix
	t.Cleanup(func() {
		_, _ = getChannelAffinityCache().DeleteMany([]string{cacheKey})
		_, _ = getChannelAffinityBindingCache().DeleteMany([]string{cacheKeySuffix})
	})

	binding := channelAffinityBinding{Version: 3, ChannelID: 17, Revision: "retry-rule"}
	require.NoError(t, getChannelAffinityCache().SetWithTTL(cacheKey, binding.ChannelID, time.Minute))
	require.NoError(t, getChannelAffinityBindingCache().SetWithTTL(cacheKeySuffix, binding, time.Minute))
	ctx := affinityJSONContext(`{}`)
	setChannelAffinityContext(ctx, channelAffinityMeta{CacheKey: cacheKey, CacheKeySuffix: cacheKeySuffix, TTLSeconds: 60, SkipRetry: true})
	ctx.Set(ginKeyChannelAffinityCandidate, channelAffinityCandidate{Binding: binding, BindingCache: true})
	MarkChannelAffinityUsed(ctx, "default", binding.ChannelID)

	require.True(t, ShouldSkipRetryAfterChannelAffinityFailure(ctx))
	require.True(t, EvictChannelAffinityBindingForFailure(ctx, binding.ChannelID))
	assert.True(t, ShouldSkipRetryAfterChannelAffinityFailure(ctx))
}

func TestEvictChannelAffinityBindingForFailurePreservesConcurrentReplacement(t *testing.T) {
	cacheKeySuffix := fmt.Sprintf("failure-eviction-replacement-%d", time.Now().UnixNano())
	cacheKey := channelAffinityCacheNamespace + ":" + cacheKeySuffix
	t.Cleanup(func() {
		_, _ = getChannelAffinityCache().DeleteMany([]string{cacheKey})
		_, _ = getChannelAffinityBindingCache().DeleteMany([]string{cacheKeySuffix})
	})

	oldBinding := channelAffinityBinding{Version: 3, ChannelID: 17, Revision: "old"}
	newBinding := channelAffinityBinding{Version: 3, ChannelID: 18, Revision: "new"}
	require.NoError(t, getChannelAffinityCache().SetWithTTL(cacheKey, oldBinding.ChannelID, time.Minute))
	require.NoError(t, getChannelAffinityBindingCache().SetWithTTL(cacheKeySuffix, oldBinding, time.Minute))

	ctx := affinityJSONContext(`{}`)
	setChannelAffinityContext(ctx, channelAffinityMeta{CacheKey: cacheKey, CacheKeySuffix: cacheKeySuffix, TTLSeconds: 60})
	ctx.Set(ginKeyChannelAffinityCandidate, channelAffinityCandidate{Binding: oldBinding, BindingCache: true})
	MarkChannelAffinityUsed(ctx, "default", oldBinding.ChannelID)
	require.NoError(t, getChannelAffinityBindingCache().SetWithTTL(cacheKeySuffix, newBinding, time.Minute))
	require.NoError(t, getChannelAffinityCache().SetWithTTL(cacheKey, newBinding.ChannelID, time.Minute))

	assert.False(t, EvictChannelAffinityBindingForFailure(ctx, oldBinding.ChannelID))
	binding, found, err := getChannelAffinityBindingCache().Get(cacheKeySuffix)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, newBinding, binding)
}

func TestRecordChannelAffinitySkipsFailedRequest(t *testing.T) {
	setting := operation_setting.GetChannelAffinitySetting()
	original := *setting
	t.Cleanup(func() { *setting = original })
	setting.Enabled = true

	cacheKeySuffix := fmt.Sprintf("failed-record-%d", time.Now().UnixNano())
	cacheKey := channelAffinityCacheNamespace + ":" + cacheKeySuffix
	t.Cleanup(func() {
		_, _ = getChannelAffinityCache().DeleteMany([]string{cacheKey})
		_, _ = getChannelAffinityBindingCache().DeleteMany([]string{cacheKeySuffix})
	})

	ctx := affinityJSONContext(`{}`)
	setChannelAffinityContext(ctx, channelAffinityMeta{CacheKey: cacheKey, CacheKeySuffix: cacheKeySuffix, TTLSeconds: 60})
	MarkChannelAffinityFailure(ctx)
	RecordChannelAffinity(ctx, 17)

	_, found, err := getChannelAffinityCache().Get(cacheKeySuffix)
	require.NoError(t, err)
	assert.False(t, found)
}
