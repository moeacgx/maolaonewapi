package service

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestIsUpstreamCyberPolicyPayloadUsesExactStructuredPaths(t *testing.T) {
	require.True(t, IsUpstreamCyberPolicyPayload([]byte(`{"error":{"code":"cyber_policy","message":"blocked"}}`)))
	require.True(t, IsUpstreamCyberPolicyPayload([]byte(`{"type":"response.failed","response":{"error":{"code":"cyber_policy"}}}`)))

	require.False(t, IsUpstreamCyberPolicyPayload([]byte(`{"message":"cyber_policy"}`)))
	require.False(t, IsUpstreamCyberPolicyPayload([]byte(`{"error":{"message":"cyber_policy"}}`)))
	require.False(t, IsUpstreamCyberPolicyPayload([]byte(`{"metadata":{"error":{"code":"cyber_policy"}}}`)))
	require.False(t, IsUpstreamCyberPolicyPayload([]byte(`{"response":{"code":"cyber_policy"}}`)))
	require.False(t, IsUpstreamCyberPolicyPayload([]byte(`not-json cyber_policy`)))
}

func TestIsUpstreamCyberPolicyErrorDoesNotInspectMessage(t *testing.T) {
	structured := types.WithOpenAIError(types.OpenAIError{
		Message: "blocked", Type: "invalid_request_error", Code: "cyber_policy",
	}, 400)
	require.True(t, IsUpstreamCyberPolicyError(structured))

	messageOnly := types.NewOpenAIError(errors.New("upstream failure"), types.ErrorCodeBadResponseStatusCode, 400)
	messageOnly.SetMessage("upstream said cyber_policy")
	require.False(t, IsUpstreamCyberPolicyError(messageOnly))
}

func TestUpstreamPolicyEventWithoutCryptoStoresOnlyHashAndMetadata(t *testing.T) {
	db := setupPromptAuditServiceTest(t, false, false, nil)
	oldSecret := common.CryptoSecret
	t.Setenv("CRYPTO_SECRET", "")
	common.CryptoSecret = ""
	t.Cleanup(func() { common.CryptoSecret = oldSecret })
	InvalidatePromptAuditConfig()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Set(common.RequestIdKey, "req-upstream-policy")
	common.SetContextKey(c, constant.ContextKeyUserId, 12)
	common.SetContextKey(c, constant.ContextKeyTokenId, 34)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-test")
	snapshot, err := BuildPromptAuditTextSnapshot(PromptAuditRequest{
		RequestId: "req-upstream-policy", UserId: 12, TokenId: 34,
		Endpoint: "/v1/responses", Protocol: "openai_responses", Model: "gpt-test",
	}, "正文绝不能在无密钥时保存")
	require.NoError(t, err)
	SetSecurityAuditRequestSnapshot(c, snapshot)

	require.True(t, RecordUpstreamPolicyPayload(c,
		[]byte(`{"type":"response.failed","response":{"error":{"code":"cyber_policy"}}}`),
		"response_stream"))
	// 同一 HTTP/SSE 请求即使跨流式解析和控制器错误转换，也只能生成一条事件。
	require.True(t, RecordUpstreamPolicyPayload(c,
		[]byte(`{"error":{"code":"cyber_policy"}}`), "response"))
	require.True(t, RecordUpstreamPolicyError(c, types.WithOpenAIError(types.OpenAIError{
		Message: "blocked", Type: "invalid_request_error", Code: "cyber_policy",
	}, 400), "response"))
	require.True(t, RecordUpstreamPolicyCode(c, "cyber_policy", "task_response"))

	var events []model.PromptAuditEvent
	require.NoError(t, db.Where("request_id = ?", "req-upstream-policy").Find(&events).Error)
	require.Len(t, events, 1)
	event := events[0]
	require.Equal(t, PromptAuditSourceUpstreamPolicy, event.Source)
	require.Equal(t, "response_stream", event.Stage)
	require.Equal(t, "cyber_policy", event.ErrorCode)
	require.NotEmpty(t, event.PromptHash)
	require.Equal(t, len([]rune("正文绝不能在无密钥时保存")), event.PromptLength)
	require.False(t, event.PromptAvailable)
	require.Empty(t, event.PromptCiphertext)
	require.Empty(t, event.RedactedPreview)
}

func TestResponseAuditEventUsesFinalRetryGroupMetadata(t *testing.T) {
	db := setupPromptAuditServiceTest(t, false, false, nil)
	require.NoError(t, db.AutoMigrate(&model.Group{}, &model.GroupAlias{}))
	finalGroup := model.Group{
		Code: "beta", Name: "最终渠道分组", Ratio: 1, Status: model.GroupStatusActive,
	}
	require.NoError(t, db.Create(&finalGroup).Error)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Set(common.RequestIdKey, "req-final-retry-group")
	common.SetContextKey(c, constant.ContextKeyUserGroupId, 111)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "alpha")
	common.SetContextKey(c, constant.ContextKeySelectedChannelGroup, "beta")
	snapshot, err := BuildPromptAuditTextSnapshot(PromptAuditRequest{
		RequestId: "req-final-retry-group", GroupId: 111, GroupName: "alpha",
		Endpoint: "/v1/responses", Protocol: "openai_responses",
	}, "触发最终分组上游策略")
	require.NoError(t, err)
	SetSecurityAuditRequestSnapshot(c, snapshot)

	require.True(t, RecordUpstreamPolicyPayload(c,
		[]byte(`{"error":{"code":"cyber_policy"}}`), "response"))

	var event model.PromptAuditEvent
	require.NoError(t, db.First(&event, "request_id = ?", "req-final-retry-group").Error)
	require.Equal(t, finalGroup.Id, event.GroupId)
	require.Equal(t, finalGroup.Name, event.GroupName)
}

func TestSensitiveWordEventWithCryptoCanDecryptOriginalPrompt(t *testing.T) {
	db := setupPromptAuditServiceTest(t, false, false, nil)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Set(common.RequestIdKey, "req-sensitive-event")
	snapshot, err := BuildPromptAuditTextSnapshot(PromptAuditRequest{
		RequestId: "req-sensitive-event", Endpoint: "/v1/chat/completions",
		Protocol: "openai_chat_completions",
	}, "包含应当拦截的测试关键词")
	require.NoError(t, err)
	SetSecurityAuditRequestSnapshot(c, snapshot)

	RecordSensitiveWordAuditEvent(c, "request", []SensitiveFilterMatch{{
		RuleID: "rule-1", RuleName: "测试规则", Action: "block", Keyword: "不得入库",
	}}, nil)

	var event model.PromptAuditEvent
	require.NoError(t, db.First(&event, "request_id = ?", "req-sensitive-event").Error)
	require.Equal(t, PromptAuditSourceSensitiveWord, event.Source)
	require.Equal(t, "request", event.Stage)
	require.True(t, event.PromptAvailable)
	require.NotEmpty(t, event.PromptCiphertext)
	require.NotContains(t, string(event.PromptCiphertext), "测试关键词")
	detail, err := GetPromptAuditEventDetail(event.Id)
	require.NoError(t, err)
	require.Equal(t, "包含应当拦截的测试关键词", detail.FullPrompt)
	require.Equal(t, []string{"rule:rule-1"}, detail.MatchedScanners)
}

func TestRealtimeSensitiveWordEventsAreRecordedPerFrame(t *testing.T) {
	db := setupPromptAuditServiceTest(t, false, false, nil)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
	c.Set(common.RequestIdKey, "req-realtime-sensitive")
	matches := []SensitiveFilterMatch{{
		RuleID: "rule-1", RuleName: "测试规则", Action: "block", Keyword: "不得入库",
	}}
	prompts := []string{"第一帧命中同一规则", "第二帧命中同一规则"}

	for _, prompt := range prompts {
		snapshot, err := BuildPromptAuditTextSnapshot(PromptAuditRequest{
			RequestId: "req-realtime-sensitive", Endpoint: "/v1/realtime", Protocol: "openai_realtime",
		}, prompt)
		require.NoError(t, err)
		RecordSensitiveWordAuditEvent(c, "realtime_request", matches, &snapshot)
	}

	var events []model.PromptAuditEvent
	require.NoError(t, db.Where("request_id = ? AND source = ?", "req-realtime-sensitive", PromptAuditSourceSensitiveWord).
		Order("id ASC").Find(&events).Error)
	require.Len(t, events, 2)
	for index, event := range events {
		detail, err := GetPromptAuditEventDetail(event.Id)
		require.NoError(t, err)
		require.Equal(t, prompts[index], detail.FullPrompt)
	}
}

func TestRealtimeUpstreamPolicyEventsAreRecordedPerFrame(t *testing.T) {
	db := setupPromptAuditServiceTest(t, false, false, nil)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
	c.Set(common.RequestIdKey, "req-realtime-policy")
	prompts := []string{"第一轮上游策略拒绝", "第二轮上游策略拒绝"}

	for _, prompt := range prompts {
		snapshot, err := BuildPromptAuditTextSnapshot(PromptAuditRequest{
			RequestId: "req-realtime-policy", Endpoint: "/v1/realtime", Protocol: "openai_realtime",
		}, prompt)
		require.NoError(t, err)
		SetSecurityAuditRequestSnapshot(c, snapshot)
		require.True(t, RecordUpstreamPolicyPayload(c,
			[]byte(`{"error":{"code":"cyber_policy"}}`), "realtime_response"))
	}

	var events []model.PromptAuditEvent
	require.NoError(t, db.Where("request_id = ? AND source = ?", "req-realtime-policy", PromptAuditSourceUpstreamPolicy).
		Order("id ASC").Find(&events).Error)
	require.Len(t, events, 2)
	for index, event := range events {
		detail, err := GetPromptAuditEventDetail(event.Id)
		require.NoError(t, err)
		require.Equal(t, prompts[index], detail.FullPrompt)
	}
}
