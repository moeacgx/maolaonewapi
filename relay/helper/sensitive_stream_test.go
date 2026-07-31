package helper

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setSensitiveStreamTestChannel(c *gin.Context, channelID int) {
	common.SetContextKey(c, constant.ContextKeyChannelId, channelID)
	common.SetContextKey(c, constant.ContextKeySelectedChannel, &model.Channel{
		Id: channelID, GroupDetails: make([]model.GroupReference, 0),
	})
}

func withSensitiveStreamRules(t *testing.T, rules []setting.SensitiveRule) {
	t.Helper()
	oldEnabled := setting.CheckSensitiveEnabled
	oldRules := setting.SensitiveRules
	oldRulesConfigured := setting.SensitiveRulesConfigured
	oldChannelIds := setting.SensitiveRuleChannelIds
	oldWords := setting.SensitiveWords
	setting.CheckSensitiveEnabled = true
	setting.SensitiveRules = rules
	setting.SensitiveRulesConfigured = true
	setting.SensitiveRuleChannelIds = []int{1}
	setting.SensitiveWords = nil
	t.Cleanup(func() {
		setting.CheckSensitiveEnabled = oldEnabled
		setting.SensitiveRules = oldRules
		setting.SensitiveRulesConfigured = oldRulesConfigured
		setting.SensitiveRuleChannelIds = oldChannelIds
		setting.SensitiveWords = oldWords
	})
}

func TestStringDataSendsErrorEventAndStopsAfterSensitiveBlock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldEnabled := setting.CheckSensitiveEnabled
	oldRules := setting.SensitiveRules
	oldRulesConfigured := setting.SensitiveRulesConfigured
	oldChannelIds := setting.SensitiveRuleChannelIds
	oldWords := setting.SensitiveWords
	setting.CheckSensitiveEnabled = true
	setting.SensitiveRules = []setting.SensitiveRule{
		{
			ID:       "response-block",
			Name:     "Response Block",
			Enabled:  true,
			Action:   setting.SensitiveRuleActionBlock,
			Scope:    setting.SensitiveRuleScopeResponse,
			Keywords: []string{"secret"},
		},
	}
	setting.SensitiveRulesConfigured = true
	setting.SensitiveRuleChannelIds = []int{1}
	setting.SensitiveWords = nil
	t.Cleanup(func() {
		setting.CheckSensitiveEnabled = oldEnabled
		setting.SensitiveRules = oldRules
		setting.SensitiveRulesConfigured = oldRulesConfigured
		setting.SensitiveRuleChannelIds = oldChannelIds
		setting.SensitiveWords = oldWords
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(common.RequestIdKey, "stream-sensitive-1")
	setSensitiveStreamTestChannel(c, 1)

	err := StringData(c, `{"choices":[{"delta":{"content":"secret"}}]}`)
	require.NoError(t, err)
	Done(c)

	body := recorder.Body.String()
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, body, "event: error")
	assert.NotContains(t, body, "sensitive_words_detected")
	assert.Contains(t, body, "内容审计命中风险规则")
	assert.Contains(t, body, "request id:")
	assert.NotContains(t, body, "metadata")
	assert.True(t, c.GetBool("sensitive_response_stream_blocked"))
	assert.False(t, strings.Contains(body, "[DONE]"))
}

func TestStringDataBlocksSensitiveKeywordAcrossStreamChunks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldEnabled := setting.CheckSensitiveEnabled
	oldRules := setting.SensitiveRules
	oldRulesConfigured := setting.SensitiveRulesConfigured
	oldChannelIds := setting.SensitiveRuleChannelIds
	oldWords := setting.SensitiveWords
	setting.CheckSensitiveEnabled = true
	setting.SensitiveRules = []setting.SensitiveRule{
		{
			ID:       "response-block",
			Name:     "Response Block",
			Enabled:  true,
			Action:   setting.SensitiveRuleActionBlock,
			Scope:    setting.SensitiveRuleScopeResponse,
			Keywords: []string{"啦啦队"},
		},
	}
	setting.SensitiveRulesConfigured = true
	setting.SensitiveRuleChannelIds = []int{1}
	setting.SensitiveWords = nil
	t.Cleanup(func() {
		setting.CheckSensitiveEnabled = oldEnabled
		setting.SensitiveRules = oldRules
		setting.SensitiveRulesConfigured = oldRulesConfigured
		setting.SensitiveRuleChannelIds = oldChannelIds
		setting.SensitiveWords = oldWords
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	setSensitiveStreamTestChannel(c, 1)

	require.NoError(t, StringData(c, `{"choices":[{"delta":{"content":"啦"}}]}`))
	require.NoError(t, StringData(c, `{"choices":[{"delta":{"content":"啦"}}]}`))
	require.NoError(t, StringData(c, `{"choices":[{"delta":{"content":"队"}}]}`))
	Done(c)

	body := recorder.Body.String()
	assert.Contains(t, body, "event: error")
	assert.Contains(t, body, "内容审计命中风险规则")
	assert.NotContains(t, body, "sensitive_words_detected")
	assert.True(t, c.GetBool("sensitive_response_stream_blocked"))
	assert.NotContains(t, body, `"content":"啦"`)
	assert.NotContains(t, body, `"content":"队"`)
	assert.False(t, strings.Contains(body, "[DONE]"))
}

func TestStringDataDoesNotBlockEmbeddedKeywordAcrossStreamChunks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withSensitiveStreamRules(t, []setting.SensitiveRule{{
		ID: "response-block", Name: "Response Block", Enabled: true,
		Action: setting.SensitiveRuleActionBlock, Scope: setting.SensitiveRuleScopeResponse,
		Keywords: []string{"Master Key"},
	}})

	tests := []struct {
		name   string
		chunks []string
	}{
		{name: "left boundary", chunks: []string{"Web", "Master Key"}},
		{name: "right boundary", chunks: []string{"Master Key", "word"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			setSensitiveStreamTestChannel(c, 1)

			for _, chunk := range test.chunks {
				require.NoError(t, StringData(c, `{"choices":[{"delta":{"content":"`+chunk+`"}}]}`))
			}
			Done(c)

			body := recorder.Body.String()
			assert.NotContains(t, body, "event: error")
			assert.Contains(t, body, "[DONE]")
			assert.False(t, c.GetBool("sensitive_response_stream_blocked"))
		})
	}
}

func TestStringDataBlocksKeywordCompletedAtStreamEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withSensitiveStreamRules(t, []setting.SensitiveRule{{
		ID: "response-block", Name: "Response Block", Enabled: true,
		Action: setting.SensitiveRuleActionBlock, Scope: setting.SensitiveRuleScopeResponse,
		Keywords: []string{"Master Key"},
	}})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	setSensitiveStreamTestChannel(c, 1)

	require.NoError(t, StringData(c, `{"choices":[{"delta":{"content":"Master Ke"}}]}`))
	require.NoError(t, StringData(c, `{"choices":[{"delta":{"content":"y"}}]}`))
	Done(c)

	body := recorder.Body.String()
	assert.Contains(t, body, "event: error")
	assert.NotContains(t, body, "Master Ke")
	assert.NotContains(t, body, `"content":"y"`)
	assert.NotContains(t, body, "[DONE]")
	assert.True(t, c.GetBool("sensitive_response_stream_blocked"))
}

func TestStringDataMasksKeywordAcrossStreamChunks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withSensitiveStreamRules(t, []setting.SensitiveRule{{
		ID: "response-mask", Name: "Response Mask", Enabled: true,
		Action: setting.SensitiveRuleActionMask, Scope: setting.SensitiveRuleScopeResponse,
		Replacement: "[MASK]", Keywords: []string{"Master Key"},
	}})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	setSensitiveStreamTestChannel(c, 1)

	require.NoError(t, StringData(c, `{"choices":[{"delta":{"content":"Master Ke"}}]}`))
	require.NoError(t, StringData(c, `{"choices":[{"delta":{"content":"y"}}]}`))
	Done(c)

	body := recorder.Body.String()
	assert.Contains(t, body, `"content":"[MASK]"`)
	assert.Contains(t, body, `"content":""`)
	assert.NotContains(t, body, "Master Ke")
	assert.Contains(t, body, "[DONE]")
	assert.False(t, c.GetBool("sensitive_response_stream_blocked"))
}

func TestFilteredEventDataPreservesBufferedEventLines(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withSensitiveStreamRules(t, []setting.SensitiveRule{{
		ID: "response-mask", Name: "Response Mask", Enabled: true,
		Action: setting.SensitiveRuleActionMask, Scope: setting.SensitiveRuleScopeResponse,
		Replacement: "[MASK]", Keywords: []string{"Master Key"},
	}})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	setSensitiveStreamTestChannel(c, 1)

	blocked, err := writeFilteredEventData(c, "event: first.delta\n", `{"delta":"Master Ke"}`)
	require.NoError(t, err)
	require.False(t, blocked)
	blocked, err = writeFilteredEventData(c, "event: second.delta\n", `{"delta":"y"}`)
	require.NoError(t, err)
	require.False(t, blocked)
	Done(c)

	body := recorder.Body.String()
	assert.Equal(t, 1, strings.Count(body, "event: first.delta"))
	assert.Equal(t, 1, strings.Count(body, "event: second.delta"))
	assert.Contains(t, body, `"delta":"[MASK]"`)
	assert.Contains(t, body, `"delta":""`)
}

func TestStringDataDoesNotDelaySafeStreamChunks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldEnabled := setting.CheckSensitiveEnabled
	oldRules := setting.SensitiveRules
	oldRulesConfigured := setting.SensitiveRulesConfigured
	oldChannelIds := setting.SensitiveRuleChannelIds
	oldWords := setting.SensitiveWords
	setting.CheckSensitiveEnabled = true
	setting.SensitiveRules = []setting.SensitiveRule{
		{
			ID:       "response-block",
			Name:     "Response Block",
			Enabled:  true,
			Action:   setting.SensitiveRuleActionBlock,
			Scope:    setting.SensitiveRuleScopeResponse,
			Keywords: []string{"啦啦队"},
		},
	}
	setting.SensitiveRulesConfigured = true
	setting.SensitiveRuleChannelIds = []int{1}
	setting.SensitiveWords = nil
	t.Cleanup(func() {
		setting.CheckSensitiveEnabled = oldEnabled
		setting.SensitiveRules = oldRules
		setting.SensitiveRulesConfigured = oldRulesConfigured
		setting.SensitiveRuleChannelIds = oldChannelIds
		setting.SensitiveWords = oldWords
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	setSensitiveStreamTestChannel(c, 1)

	require.NoError(t, StringData(c, `{"choices":[{"delta":{"content":"安全"}}]}`))
	bodyAfterChunk := recorder.Body.String()
	assert.Contains(t, bodyAfterChunk, `"content":"安全"`)
	assert.False(t, strings.Contains(bodyAfterChunk, "[DONE]"))

	Done(c)

	body := recorder.Body.String()
	assert.Contains(t, body, `"content":"安全"`)
	assert.Contains(t, body, "[DONE]")
	assert.False(t, c.GetBool("sensitive_response_stream_blocked"))
}
