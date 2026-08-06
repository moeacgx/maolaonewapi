package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func cyberPolicyConversationTestContext(userId int, body string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyUserId, userId)
	return c
}

func TestCyberPolicyConversationCacheUsesScopedNamespace(t *testing.T) {
	require.Equal(t, "new-api:cyber_policy_conversation:v2", cyberPolicyConversationCacheNamespace)
	key := "conversation-key"
	legacyKey := cachex.Namespace("new-api:cyber_policy_conversation:v1").FullKey(key)
	currentKey := getCyberPolicyConversationCache().FullKey(key)
	require.Equal(t, "new-api:cyber_policy_conversation:v2:conversation-key", currentKey)
	require.NotEqual(t, legacyKey, currentKey, "旧 v1 标记不得被当前缓存读取")
}

func TestCyberPolicyConversationLookupPropagatesBodyLimitError(t *testing.T) {
	originalMaxRequestBodyMB := constant.MaxRequestBodyMB
	constant.MaxRequestBodyMB = 1
	t.Cleanup(func() {
		constant.MaxRequestBodyMB = originalMaxRequestBodyMB
	})

	c := cyberPolicyConversationTestContext(11, strings.Repeat("x", (1<<20)+1))
	blocked, err := IsCyberPolicyConversationBlocked(c, &PromptAuditConfig{
		CyberPolicyConversationBlockEnabled: true,
		UpstreamPolicyTargetType:            PromptAuditUpstreamPolicyTargetAll,
	})

	require.False(t, blocked)
	require.Error(t, err)
	require.True(t, common.IsRequestBodyTooLargeError(err))
}

func TestCyberPolicyConversationBlockUsesStableIdentityAndUserBoundary(t *testing.T) {
	require.NoError(t, getCyberPolicyConversationCache().Purge())
	blocked := cyberPolicyConversationTestContext(11, `{"prompt_cache_key":"conversation-a"}`)
	require.True(t, MarkCyberPolicyConversationBlocked(blocked, 1))

	same := cyberPolicyConversationTestContext(11, `{"prompt_cache_key":"conversation-a"}`)
	got, err := isCyberPolicyConversationMarked(same)
	require.NoError(t, err)
	require.True(t, got)

	otherUser := cyberPolicyConversationTestContext(22, `{"prompt_cache_key":"conversation-a"}`)
	got, err = isCyberPolicyConversationMarked(otherUser)
	require.NoError(t, err)
	require.False(t, got, "相同会话标识不得跨用户拦截")

	otherConversation := cyberPolicyConversationTestContext(11, `{"prompt_cache_key":"conversation-b"}`)
	got, err = isCyberPolicyConversationMarked(otherConversation)
	require.NoError(t, err)
	require.False(t, got)
}

func TestCyberPolicyConversationBlockSupportsStableHeadersOnly(t *testing.T) {
	require.NoError(t, getCyberPolicyConversationCache().Purge())
	withHeader := cyberPolicyConversationTestContext(11, `{}`)
	withHeader.Request.Header.Set("X-Codex-Session-Id", "codex-session")
	require.True(t, MarkCyberPolicyConversationBlocked(withHeader, 1))

	sameHeader := cyberPolicyConversationTestContext(11, `{"input":"changed next turn"}`)
	sameHeader.Request.Header.Set("X-Codex-Session-Id", "codex-session")
	got, err := isCyberPolicyConversationMarked(sameHeader)
	require.NoError(t, err)
	require.True(t, got)

	withoutStableIdentity := cyberPolicyConversationTestContext(
		11, `{"previous_response_id":"changes-each-turn","input":"hello"}`,
	)
	require.False(t, MarkCyberPolicyConversationBlocked(withoutStableIdentity, 1))
	got, err = isCyberPolicyConversationMarked(withoutStableIdentity)
	require.NoError(t, err)
	require.False(t, got, "不稳定响应 ID 或正文不得用于推测会话")
}

func TestCyberPolicyConversationIdentityAllowsBodylessWebSocketUpgrade(t *testing.T) {
	c := cyberPolicyConversationTestContext(11, `{}`)
	c.Request.Body = nil

	blocked, err := isCyberPolicyConversationMarked(c)
	require.NoError(t, err)
	require.False(t, blocked)
	require.False(t, MarkCyberPolicyConversationBlocked(c, 1))
}
