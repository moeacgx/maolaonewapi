package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
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

func TestCyberPolicyConversationLookupPropagatesBodyLimitError(t *testing.T) {
	originalMaxRequestBodyMB := constant.MaxRequestBodyMB
	constant.MaxRequestBodyMB = 1
	t.Cleanup(func() {
		constant.MaxRequestBodyMB = originalMaxRequestBodyMB
	})

	c := cyberPolicyConversationTestContext(11, strings.Repeat("x", (1<<20)+1))
	blocked, err := IsCyberPolicyConversationBlocked(c)

	require.False(t, blocked)
	require.Error(t, err)
	require.True(t, common.IsRequestBodyTooLargeError(err))
}

func TestCyberPolicyConversationBlockUsesStableIdentityAndUserBoundary(t *testing.T) {
	require.NoError(t, getCyberPolicyConversationCache().Purge())
	blocked := cyberPolicyConversationTestContext(11, `{"prompt_cache_key":"conversation-a"}`)
	require.True(t, MarkCyberPolicyConversationBlocked(blocked, 1))

	same := cyberPolicyConversationTestContext(11, `{"prompt_cache_key":"conversation-a"}`)
	got, err := IsCyberPolicyConversationBlocked(same)
	require.NoError(t, err)
	require.True(t, got)

	otherUser := cyberPolicyConversationTestContext(22, `{"prompt_cache_key":"conversation-a"}`)
	got, err = IsCyberPolicyConversationBlocked(otherUser)
	require.NoError(t, err)
	require.False(t, got, "相同会话标识不得跨用户拦截")

	otherConversation := cyberPolicyConversationTestContext(11, `{"prompt_cache_key":"conversation-b"}`)
	got, err = IsCyberPolicyConversationBlocked(otherConversation)
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
	got, err := IsCyberPolicyConversationBlocked(sameHeader)
	require.NoError(t, err)
	require.True(t, got)

	withoutStableIdentity := cyberPolicyConversationTestContext(
		11, `{"previous_response_id":"changes-each-turn","input":"hello"}`,
	)
	require.False(t, MarkCyberPolicyConversationBlocked(withoutStableIdentity, 1))
	got, err = IsCyberPolicyConversationBlocked(withoutStableIdentity)
	require.NoError(t, err)
	require.False(t, got, "不稳定响应 ID 或正文不得用于推测会话")
}
