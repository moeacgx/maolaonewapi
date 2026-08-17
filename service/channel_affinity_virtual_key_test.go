package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func affinityJSONContext(body string) *gin.Context {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx
}

func TestPromptCacheAffinityExplicitValuePrecedesVirtualSession(t *testing.T) {
	ctx := affinityJSONContext(`{"prompt_cache_key":"explicit-cache"}`)
	ctx.Request.Header.Set("Session_id", "virtual-session")
	value := extractChannelAffinityValue(ctx, operation_setting.ChannelAffinityKeySource{Type: "gjson", Path: "prompt_cache_key"})
	require.Equal(t, "explicit-cache", value)
}

func TestPromptCacheAffinityFallsBackForNullOrEmptyExplicitValue(t *testing.T) {
	for _, body := range []string{`{"prompt_cache_key":null}`, `{"prompt_cache_key":""}`} {
		ctx := affinityJSONContext(body)
		ctx.Request.Header.Set("X-Claude-Code-Session-Id", " stable-session ")
		value := extractChannelAffinityValue(ctx, operation_setting.ChannelAffinityKeySource{Type: "gjson", Path: "prompt_cache_key"})
		require.Equal(t, "stable-session", value)
	}
}

func TestPromptCacheAffinityNormalizesClaudeMetadataSession(t *testing.T) {
	ctx := affinityJSONContext(`{"metadata":{"user_id":"user_42_session_claude-session"}}`)
	value := extractChannelAffinityValue(ctx, operation_setting.ChannelAffinityKeySource{Type: "gjson", Path: "prompt_cache_key"})
	require.Equal(t, "claude-session", value)
}

func TestPromptCacheAffinityRejectsStructuredMetadataIdentities(t *testing.T) {
	for _, body := range []string{
		`{"metadata":{"session_id":{"value":"secret"}}}`,
		`{"metadata":{"session_id":["secret"]}}`,
		`{"metadata":{"session_id":123}}`,
		`{"metadata":{"session_id":true}}`,
	} {
		ctx := affinityJSONContext(body)
		value := extractChannelAffinityValue(ctx, operation_setting.ChannelAffinityKeySource{Type: "gjson", Path: "prompt_cache_key"})
		require.Empty(t, value)
	}
}

func TestAffinityCacheKeyHashesRawIdentity(t *testing.T) {
	raw := "session-secret-value"
	key := buildChannelAffinityCacheKeySuffix(operation_setting.ChannelAffinityRule{}, "model", "group", raw)
	require.NotContains(t, key, raw)
	require.Contains(t, key, affinityCacheKeyComponent(raw))
}

func TestAffinityAdminInfoDoesNotExposeRawIdentity(t *testing.T) {
	ctx := affinityJSONContext(`{}`)
	setChannelAffinityContext(ctx, channelAffinityMeta{
		RuleName: "rule", UsingGroup: "default", ModelName: "gpt-5",
		KeyHint: "raw-secret-session", KeyFingerprint: "a1b2c3d4",
	})
	MarkChannelAffinityUsed(ctx, "default", 7)
	admin := map[string]interface{}{}
	AppendChannelAffinityAdminInfo(ctx, admin)
	require.NotContains(t, admin["channel_affinity"], "key_hint")
	require.NotContains(t, fmt.Sprint(admin), "raw-secret-session")
}

func TestRecordChannelAffinityDoesNotBindIndexFromDifferentChannel(t *testing.T) {
	setting := operation_setting.GetChannelAffinitySetting()
	original := *setting
	t.Cleanup(func() { *setting = original })
	setting.Enabled = true
	setting.SwitchOnSuccess = false

	cacheKey := "record-mismatch-test"
	_, _ = getChannelAffinityBindingCache().DeleteMany([]string{cacheKey})
	t.Cleanup(func() { _, _ = getChannelAffinityBindingCache().DeleteMany([]string{cacheKey}) })
	ctx := affinityJSONContext(`{}`)
	setChannelAffinityContext(ctx, channelAffinityMeta{
		CacheKey:       cacheKey,
		CacheKeySuffix: cacheKey,
		TTLSeconds:     60,
		BindMultiKey:   true,
	})
	common.SetContextKey(ctx, constant.ContextKeyChannelId, 2)
	common.SetContextKey(ctx, constant.ContextKeyChannelIsMultiKey, true)
	common.SetContextKey(ctx, constant.ContextKeyChannelMultiKeyIndex, 3)

	RecordChannelAffinity(ctx, 1)
	binding, found, err := getChannelAffinityBindingCache().Get(cacheKey)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, 1, binding.ChannelID)
	require.False(t, binding.BindMultiKey)
}
