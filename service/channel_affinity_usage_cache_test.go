package service

import (
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/samber/hot"
	"github.com/stretchr/testify/require"
)

func buildChannelAffinityStatsContextForTest(ruleName, usingGroup, keyFP string) *gin.Context {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	setChannelAffinityContext(ctx, channelAffinityMeta{
		CacheKey:       fmt.Sprintf("test:%s:%s:%s", ruleName, usingGroup, keyFP),
		TTLSeconds:     600,
		RuleName:       ruleName,
		UsingGroup:     usingGroup,
		KeyFingerprint: keyFP,
	})
	return ctx
}

func isolateChannelAffinityUsageCacheForTest(t *testing.T) {
	t.Helper()

	previousCache := channelAffinityUsageCacheStatsCache
	previousInitialized := previousCache != nil

	channelAffinityUsageCacheStatsCache = cachex.NewHybridCache[ChannelAffinityUsageCacheCounters](cachex.HybridCacheConfig[ChannelAffinityUsageCacheCounters]{
		Namespace:    cachex.Namespace(channelAffinityUsageCacheStatsNamespace),
		RedisEnabled: func() bool { return false },
		Memory: func() *hot.HotCache[string, ChannelAffinityUsageCacheCounters] {
			return hot.NewHotCache[string, ChannelAffinityUsageCacheCounters](hot.LRU, 8).Build()
		},
	})
	channelAffinityUsageCacheStatsOnce = sync.Once{}
	channelAffinityUsageCacheStatsOnce.Do(func() {})

	t.Cleanup(func() {
		channelAffinityUsageCacheStatsCache = previousCache
		channelAffinityUsageCacheStatsOnce = sync.Once{}
		if previousInitialized {
			channelAffinityUsageCacheStatsOnce.Do(func() {})
		}
	})
}

func TestObserveChannelAffinityUsageCacheByRelayFormat_ClaudeMode(t *testing.T) {
	ruleName := fmt.Sprintf("rule_%d", time.Now().UnixNano())
	usingGroup := "default"
	keyFP := fmt.Sprintf("fp_%d", time.Now().UnixNano())
	ctx := buildChannelAffinityStatsContextForTest(ruleName, usingGroup, keyFP)
	t.Cleanup(func() {
		_, _ = getChannelAffinityUsageCacheStatsCache().DeleteMany([]string{channelAffinityUsageCacheEntryKey(ruleName, usingGroup, keyFP)})
	})

	usage := &dto.Usage{
		PromptTokens:     100,
		CompletionTokens: 40,
		TotalTokens:      140,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 30,
		},
	}

	ObserveChannelAffinityUsageCacheByRelayFormat(ctx, usage, types.RelayFormatClaude)
	stats := GetChannelAffinityUsageCacheStats(ruleName, usingGroup, keyFP)

	require.EqualValues(t, 1, stats.Total)
	require.EqualValues(t, 1, stats.Hit)
	require.EqualValues(t, 100, stats.PromptTokens)
	require.EqualValues(t, 40, stats.CompletionTokens)
	require.EqualValues(t, 140, stats.TotalTokens)
	require.EqualValues(t, 30, stats.CachedTokens)
	require.Equal(t, cacheTokenRateModeCachedOverPromptPlusCached, stats.CachedTokenRateMode)
}

func TestObserveChannelAffinityUsageCacheByRelayFormat_MixedMode(t *testing.T) {
	isolateChannelAffinityUsageCacheForTest(t)
	ruleName := t.Name()
	usingGroup := "default"
	keyFP := "mixed-mode"
	ctx := buildChannelAffinityStatsContextForTest(ruleName, usingGroup, keyFP)
	t.Cleanup(func() {
		_, _ = getChannelAffinityUsageCacheStatsCache().DeleteMany([]string{channelAffinityUsageCacheEntryKey(ruleName, usingGroup, keyFP)})
	})

	openAIUsage := &dto.Usage{
		PromptTokens: 100,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 10,
		},
	}
	claudeUsage := &dto.Usage{
		PromptTokens: 80,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 20,
		},
	}

	ObserveChannelAffinityUsageCacheByRelayFormat(ctx, openAIUsage, types.RelayFormatOpenAI)
	ObserveChannelAffinityUsageCacheByRelayFormat(ctx, claudeUsage, types.RelayFormatClaude)
	stats := GetChannelAffinityUsageCacheStats(ruleName, usingGroup, keyFP)

	require.EqualValues(t, 2, stats.Total)
	require.EqualValues(t, 2, stats.Hit)
	require.EqualValues(t, 180, stats.PromptTokens)
	require.EqualValues(t, 30, stats.CachedTokens)
	require.Equal(t, cacheTokenRateModeMixed, stats.CachedTokenRateMode)
}

func TestObserveChannelAffinityUsageCacheByRelayFormat_UnsupportedModeKeepsEmpty(t *testing.T) {
	isolateChannelAffinityUsageCacheForTest(t)
	ruleName := t.Name()
	usingGroup := "default"
	keyFP := "unsupported-mode"
	ctx := buildChannelAffinityStatsContextForTest(ruleName, usingGroup, keyFP)
	t.Cleanup(func() {
		_, _ = getChannelAffinityUsageCacheStatsCache().DeleteMany([]string{channelAffinityUsageCacheEntryKey(ruleName, usingGroup, keyFP)})
	})

	usage := &dto.Usage{
		PromptTokens: 100,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 25,
		},
	}

	ObserveChannelAffinityUsageCacheByRelayFormat(ctx, usage, types.RelayFormatGemini)
	stats := GetChannelAffinityUsageCacheStats(ruleName, usingGroup, keyFP)

	require.EqualValues(t, 1, stats.Total)
	require.EqualValues(t, 1, stats.Hit)
	require.EqualValues(t, 25, stats.CachedTokens)
	require.Equal(t, "", stats.CachedTokenRateMode)
}
