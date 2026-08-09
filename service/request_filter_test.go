package service

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func withRequestFilterRules(t *testing.T, rules []setting.SensitiveRule) {
	t.Helper()
	oldEnabled := setting.CheckSensitiveEnabled
	oldPromptEnabled := setting.CheckSensitiveOnPromptEnabled
	oldRules := setting.SensitiveRules
	oldRulesConfigured := setting.SensitiveRulesConfigured
	oldChannelIds := setting.SensitiveRuleChannelIds
	oldWords := setting.SensitiveWords
	setting.CheckSensitiveEnabled = true
	setting.CheckSensitiveOnPromptEnabled = true
	setting.SensitiveRules = rules
	setting.SensitiveRulesConfigured = false
	setting.SensitiveRuleChannelIds = nil
	setting.SensitiveWords = nil
	t.Cleanup(func() {
		setting.CheckSensitiveEnabled = oldEnabled
		setting.CheckSensitiveOnPromptEnabled = oldPromptEnabled
		setting.SensitiveRules = oldRules
		setting.SensitiveRulesConfigured = oldRulesConfigured
		setting.SensitiveRuleChannelIds = oldChannelIds
		setting.SensitiveWords = oldWords
	})
}

func newJSONFilterContext(t *testing.T, body string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	return c
}

func TestSensitiveKeywordSmartBoundaryAvoidsEmbeddedEnglishPhrase(t *testing.T) {
	filter := newSensitiveTextFilter([]setting.SensitiveRule{
		{
			ID: "english-block", Name: "English block", Enabled: true,
			Action: setting.SensitiveRuleActionBlock, Keywords: []string{"Master Key"},
		},
		{
			ID: "chinese-block", Name: "Chinese block", Enabled: true,
			Action: setting.SensitiveRuleActionBlock, Keywords: []string{"敏感词"},
		},
	})

	require.Empty(t, filter.blockMatches("Bing Webmaster Keyword Research"))
	require.Len(t, filter.blockMatches("Use the Master Key now"), 1)
	require.Len(t, filter.blockMatches("包含敏感词内容"), 1)
}

func TestSensitiveKeywordSmartBoundaryAppliesToMaskRanges(t *testing.T) {
	filter := newSensitiveTextFilter([]setting.SensitiveRule{{
		ID: "english-mask", Name: "English mask", Enabled: true,
		Action: setting.SensitiveRuleActionMask, Replacement: "[MASK]",
		Keywords: []string{"Master Key"},
	}})

	updated, matches, changed := filter.maskText(
		"Webmaster Keyword / Master Key / Master Keywordization",
	)

	require.True(t, changed)
	require.Equal(t, "Webmaster Keyword / [MASK] / Master Keywordization", updated)
	require.Len(t, matches, 1)
	require.Equal(t, "Master Key", matches[0].Keyword)
}

func TestSensitiveKeywordSmartBoundaryUsesStreamEndContext(t *testing.T) {
	filter := newSensitiveTextFilter([]setting.SensitiveRule{{
		ID: "stream-boundary", Name: "Stream boundary", Enabled: true,
		Action: setting.SensitiveRuleActionBlock, Keywords: []string{"Master Key"},
	}})

	require.Empty(t, filter.blockMatchesWithEnd("WebMaster Key", true))
	require.Empty(t, filter.blockMatchesWithEnd("Master Key", false))
	require.Empty(t, filter.blockMatchesWithEnd("Master Keyword", true))
	require.Len(t, filter.blockMatchesWithEnd("Master Key ", false), 1)
	require.Len(t, filter.blockMatchesWithEnd("Master Key", true), 1)
}

func storedBody(t *testing.T, c *gin.Context) string {
	t.Helper()
	storage, err := common.GetBodyStorage(c)
	require.NoError(t, err)
	body, err := storage.Bytes()
	require.NoError(t, err)
	_, err = storage.Seek(0, io.SeekStart)
	require.NoError(t, err)
	return string(body)
}

func setFilterChannelIds(ids ...int) {
	setting.SensitiveRuleChannelIds = ids
}

func withSensitivePrefillGroups(t *testing.T, groups ...model.PrefillGroup) {
	t.Helper()
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PrefillGroup{}))
	model.DB = db
	t.Cleanup(func() {
		sensitiveWordPrefillGroupCache.Lock()
		sensitiveWordPrefillGroupCache.loadedAt = time.Time{}
		sensitiveWordPrefillGroupCache.groups = nil
		sensitiveWordPrefillGroupCache.Unlock()
		model.DB = oldDB
	})
	sensitiveWordPrefillGroupCache.Lock()
	sensitiveWordPrefillGroupCache.loadedAt = time.Time{}
	sensitiveWordPrefillGroupCache.groups = nil
	sensitiveWordPrefillGroupCache.Unlock()

	for i := range groups {
		require.NoError(t, db.Create(&groups[i]).Error)
	}
}

func sensitiveRuleIDs(rules []setting.SensitiveRule) []string {
	ids := make([]string, 0, len(rules))
	for _, rule := range rules {
		ids = append(ids, rule.ID)
	}
	return ids
}

func TestSensitiveFilterClientErrorsHideInternalClassification(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(common.RequestIdKey, "request-sensitive-1")

	httpErr := NewSensitiveFilterAPIError(c)
	require.Equal(t, http.StatusForbidden, httpErr.StatusCode)
	require.Equal(t, types.ErrorCodeSensitiveWordsDetected, httpErr.GetErrorCode())
	require.True(t, types.IsSkipRetryError(httpErr))
	clientError := SensitiveFilterClientOpenAIError(httpErr)
	require.Equal(t, "内容审计命中风险规则，请调整输入后重试 (request id: request-sensitive-1)", clientError.Message)
	require.Nil(t, clientError.Code)
	require.Empty(t, clientError.Metadata)

	var streamBody struct {
		Error types.OpenAIError `json:"error"`
	}
	var httpBody struct {
		Error types.OpenAIError `json:"error"`
	}
	httpResponseBody, httpStatusCode := SensitiveFilterOpenAIErrorResponse(c)
	require.Equal(t, http.StatusForbidden, httpStatusCode)
	require.NoError(t, common.Unmarshal(httpResponseBody, &httpBody))
	require.Equal(t, "内容审计命中风险规则，请调整输入后重试 (request id: request-sensitive-1)", httpBody.Error.Message)
	require.Nil(t, httpBody.Error.Code)
	require.Empty(t, httpBody.Error.Metadata)

	require.NoError(t, common.Unmarshal(SensitiveFilterSSEOpenAIErrorBody(c), &streamBody))
	require.Equal(t, "内容审计命中风险规则，请调整输入后重试 (request id: request-sensitive-1)", streamBody.Error.Message)
	require.Nil(t, streamBody.Error.Code)
	require.Empty(t, streamBody.Error.Metadata)

	require.Equal(t, "内容审计命中风险规则，请调整输入后重试 (request id: request-sensitive-1)", SensitiveFilterRealtimeMessage(c))
}

func TestSensitiveFilterOpenAIErrorResponseUsesClientStatusReplacement(t *testing.T) {
	require.NoError(t, common.UpdateErrorMessageReplacementRules(
		`[{"status_code":403,"match":"内容审计命中风险规则","mode":"contains","replace_status_code":429,"replace":"请求过于频繁，请稍后重试"}]`,
	))
	t.Cleanup(func() {
		require.NoError(t, common.UpdateErrorMessageReplacementRules(`[]`))
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body, statusCode := SensitiveFilterOpenAIErrorResponse(c)
	require.Equal(t, http.StatusTooManyRequests, statusCode)

	var response struct {
		Error types.OpenAIError `json:"error"`
	}
	require.NoError(t, common.Unmarshal(body, &response))
	require.Equal(t, "请求过于频繁，请稍后重试", response.Error.Message)
}

func TestApplySensitiveFilterToRequestBodyBlocksBeforeMasking(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{
		{
			ID:          "mask",
			Name:        "Mask",
			Enabled:     true,
			Action:      setting.SensitiveRuleActionMask,
			Replacement: "[MASK]",
			Keywords:    []string{"secret"},
		},
		{
			ID:       "block",
			Name:     "Block",
			Enabled:  true,
			Action:   setting.SensitiveRuleActionBlock,
			Keywords: []string{"secret"},
		},
	})
	setFilterChannelIds(1)

	c := newJSONFilterContext(t, `{"model":"gpt-test","messages":[{"role":"user","content":"secret"}]}`)
	common.SetContextKey(c, constant.ContextKeyChannelId, 1)

	result, err := ApplySensitiveFilterToRequestBody(c, types.RelayFormatOpenAI)
	require.NoError(t, err)

	assert.True(t, result.Blocked)
	assert.False(t, result.Mutated)
	require.Len(t, result.Matches, 1)
	assert.Equal(t, setting.SensitiveRuleActionBlock, result.Matches[0].Action)
	assert.Contains(t, storedBody(t, c), "secret")
}

func TestApplySensitiveFilterToRealtimeRequestFrameMasksBeforeUpstream(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{{
		ID: "realtime-mask", Name: "Realtime Mask", Enabled: true,
		Action: setting.SensitiveRuleActionMask, Scope: setting.SensitiveRuleScopeRequest,
		Replacement: "[MASK]", Keywords: []string{"frame-secret"},
	}})
	setFilterChannelIds(1)
	c := newJSONFilterContext(t, `{}`)
	c.Request.URL.Path = "/v1/realtime"
	common.SetContextKey(c, constant.ContextKeyChannelId, 1)

	result, rewritten, err := ApplySensitiveFilterToRealtimeRequestFrame(c, []byte(
		`{"type":"session.update","session":{"instructions":"frame-secret","input_audio_format":"pcm16"}}`,
	))
	require.NoError(t, err)
	require.True(t, result.Mutated)
	require.False(t, result.Blocked)
	require.Contains(t, string(rewritten), "[MASK]")
	require.NotContains(t, string(rewritten), "frame-secret")
}

func TestApplySensitiveFilterToRealtimeResponseFrameBlocksDelta(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{{
		ID: "realtime-block", Name: "Realtime Block", Enabled: true,
		Action: setting.SensitiveRuleActionBlock, Scope: setting.SensitiveRuleScopeResponse,
		Keywords: []string{"unsafe-delta"},
	}})
	setFilterChannelIds(1)
	c := newJSONFilterContext(t, `{}`)
	c.Request.URL.Path = "/v1/realtime"
	common.SetContextKey(c, constant.ContextKeyChannelId, 1)
	original := []byte(`{"type":"response.output_text.delta","delta":"unsafe-delta"}`)

	result, rewritten, err := ApplySensitiveFilterToRealtimeResponseFrame(c, original)
	require.NoError(t, err)
	require.True(t, result.Blocked)
	require.False(t, result.Mutated)
	require.Equal(t, original, rewritten)
	require.True(t, IsContentPolicyRejected(c))
}

func TestApplySensitiveFilterToRequestBodyMasksPromptFields(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{
		{
			ID:          "mask",
			Name:        "Mask",
			Enabled:     true,
			Action:      setting.SensitiveRuleActionMask,
			Replacement: "[MASK]",
			Keywords:    []string{"Secret", "词"},
		},
	})
	setFilterChannelIds(1)

	tests := []struct {
		name        string
		format      types.RelayFormat
		body        string
		wantPresent []string
		wantAbsent  []string
	}{
		{
			name:   "openai chat",
			format: types.RelayFormatOpenAI,
			body: `{
				"model":"gpt-test",
				"messages":[
					{"role":"user","content":"Secret text"},
					{"role":"user","content":[{"type":"text","text":"包含词"}]}
				],
				"prompt":"Secret prompt",
				"metadata":{"note":"do-not-touch"}
			}`,
			wantPresent: []string{"[MASK] text", "包含[MASK]", "[MASK] prompt", "do-not-touch"},
			wantAbsent:  []string{"Secret text", "包含词", "Secret prompt"},
		},
		{
			name:   "responses",
			format: types.RelayFormatOpenAIResponses,
			body: `{
				"model":"gpt-test",
				"instructions":"Secret instructions",
				"input":[{"role":"user","content":[{"type":"input_text","text":"Secret input"}]}],
				"metadata":{"note":"do-not-touch"}
			}`,
			wantPresent: []string{"[MASK] instructions", "[MASK] input", "do-not-touch"},
			wantAbsent:  []string{"Secret instructions", "Secret input"},
		},
		{
			name:   "claude",
			format: types.RelayFormatClaude,
			body: `{
				"model":"claude-test",
				"system":"Secret system",
				"messages":[{"role":"user","content":[{"type":"text","text":"Secret message"}]}]
			}`,
			wantPresent: []string{"[MASK] system", "[MASK] message"},
			wantAbsent:  []string{"Secret system", "Secret message"},
		},
		{
			name:   "gemini",
			format: types.RelayFormatGemini,
			body: `{
				"systemInstruction":{"parts":[{"text":"Secret system"}]},
				"contents":[{"role":"user","parts":[{"text":"Secret message"}]}]
			}`,
			wantPresent: []string{"[MASK] system", "[MASK] message"},
			wantAbsent:  []string{"Secret system", "Secret message"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newJSONFilterContext(t, tt.body)
			common.SetContextKey(c, constant.ContextKeyChannelId, 1)

			result, err := ApplySensitiveFilterToRequestBody(c, tt.format)
			require.NoError(t, err)

			assert.False(t, result.Blocked)
			assert.True(t, result.Mutated)
			body := storedBody(t, c)
			for _, want := range tt.wantPresent {
				assert.Contains(t, body, want)
			}
			for _, want := range tt.wantAbsent {
				assert.NotContains(t, body, want)
			}
		})
	}
}

func TestApplySensitiveFilterToRequestBodySkipsWhenNoChannelsConfigured(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{
		{
			ID:       "block",
			Name:     "Block",
			Enabled:  true,
			Action:   setting.SensitiveRuleActionBlock,
			Keywords: []string{"secret"},
		},
	})
	setFilterChannelIds()

	c := newJSONFilterContext(t, `{"model":"gpt-test","messages":[{"role":"user","content":"secret"}]}`)
	common.SetContextKey(c, constant.ContextKeyChannelId, 10)

	result, err := ApplySensitiveFilterToRequestBody(c, types.RelayFormatOpenAI)
	require.NoError(t, err)

	assert.False(t, result.Blocked)
	assert.False(t, result.Mutated)
	assert.Contains(t, storedBody(t, c), "secret")
}

func TestApplySensitiveFilterToRequestBodySkipsWhenChannelNotSelected(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{
		{
			ID:       "block",
			Name:     "Block",
			Enabled:  true,
			Action:   setting.SensitiveRuleActionBlock,
			Keywords: []string{"secret"},
		},
	})
	setFilterChannelIds(10, 20)

	c := newJSONFilterContext(t, `{"model":"gpt-test","messages":[{"role":"user","content":"secret"}]}`)
	common.SetContextKey(c, constant.ContextKeyChannelId, 30)

	result, err := ApplySensitiveFilterToRequestBody(c, types.RelayFormatOpenAI)
	require.NoError(t, err)

	assert.False(t, result.Blocked)
	assert.False(t, result.Mutated)
	assert.Contains(t, storedBody(t, c), "secret")
}

func TestApplySensitiveFilterToRequestBodyBlocksWhenChannelSelected(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{
		{
			ID:       "block",
			Name:     "Block",
			Enabled:  true,
			Action:   setting.SensitiveRuleActionBlock,
			Keywords: []string{"secret"},
		},
	})
	setFilterChannelIds(10, 20)

	c := newJSONFilterContext(t, `{"model":"gpt-test","messages":[{"role":"user","content":"secret"}]}`)
	common.SetContextKey(c, constant.ContextKeyChannelId, 20)

	result, err := ApplySensitiveFilterToRequestBody(c, types.RelayFormatOpenAI)
	require.NoError(t, err)

	assert.True(t, result.Blocked)
	assert.False(t, result.Mutated)
}

func TestApplySensitiveFilterToRequestBodyMasksWhenChannelSelected(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{
		{
			ID:          "mask",
			Name:        "Mask",
			Enabled:     true,
			Action:      setting.SensitiveRuleActionMask,
			Replacement: "[MASK]",
			Keywords:    []string{"secret"},
		},
	})
	setFilterChannelIds(10, 20)

	c := newJSONFilterContext(t, `{"model":"gpt-test","messages":[{"role":"user","content":"secret"}]}`)
	common.SetContextKey(c, constant.ContextKeyChannelId, 10)

	result, err := ApplySensitiveFilterToRequestBody(c, types.RelayFormatOpenAI)
	require.NoError(t, err)

	assert.False(t, result.Blocked)
	assert.True(t, result.Mutated)
	body := storedBody(t, c)
	assert.Contains(t, body, "[MASK]")
	assert.NotContains(t, body, "secret")
}

func TestApplySensitiveFilterToRequestBodySkipsResponseOnlyRules(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{
		{
			ID:       "response",
			Name:     "Response",
			Enabled:  true,
			Action:   setting.SensitiveRuleActionBlock,
			Scope:    setting.SensitiveRuleScopeResponse,
			Keywords: []string{"secret"},
		},
	})
	setFilterChannelIds(1)

	c := newJSONFilterContext(t, `{"model":"gpt-test","messages":[{"role":"user","content":"secret"}]}`)
	common.SetContextKey(c, constant.ContextKeyChannelId, 1)

	result, err := ApplySensitiveFilterToRequestBody(c, types.RelayFormatOpenAI)
	require.NoError(t, err)

	assert.False(t, result.Blocked)
	assert.False(t, result.Mutated)
	assert.Contains(t, storedBody(t, c), "secret")
}

func TestApplySensitiveFilterToRequestBodySkipsConfiguredResponseOnlyLegacyRule(t *testing.T) {
	withRequestFilterRules(t, nil)
	setting.SensitiveWords = []string{"secret"}
	err := setting.UpdateSensitiveRulesByJSONString(`{
		"rules": [
			{
				"id": "legacy-sensitive-words",
				"name": "Legacy sensitive words",
				"enabled": true,
				"action": "block",
				"scope": "response",
				"keywords": ["secret"]
			}
		]
	}`)
	require.NoError(t, err)
	setFilterChannelIds(1)

	c := newJSONFilterContext(t, `{"model":"gpt-test","messages":[{"role":"user","content":"secret"}]}`)
	common.SetContextKey(c, constant.ContextKeyChannelId, 1)

	result, err := ApplySensitiveFilterToRequestBody(c, types.RelayFormatOpenAI)
	require.NoError(t, err)

	assert.False(t, result.Blocked)
	assert.False(t, result.Mutated)
	assert.Contains(t, storedBody(t, c), "secret")
}

func TestApplySensitiveFilterToRequestBodyAppliesBothScopeRules(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{
		{
			ID:          "both",
			Name:        "Both",
			Enabled:     true,
			Action:      setting.SensitiveRuleActionMask,
			Scope:       setting.SensitiveRuleScopeBoth,
			Replacement: "[MASK]",
			Keywords:    []string{"secret"},
		},
	})
	setFilterChannelIds(1)

	c := newJSONFilterContext(t, `{"model":"gpt-test","messages":[{"role":"user","content":"secret"}]}`)
	common.SetContextKey(c, constant.ContextKeyChannelId, 1)

	result, err := ApplySensitiveFilterToRequestBody(c, types.RelayFormatOpenAI)
	require.NoError(t, err)

	assert.False(t, result.Blocked)
	assert.True(t, result.Mutated)
	body := storedBody(t, c)
	assert.Contains(t, body, "[MASK]")
	assert.NotContains(t, body, "secret")
}

func TestApplySensitiveFilterToRequestBodyExpandsSensitivePrefillGroupRefs(t *testing.T) {
	withSensitivePrefillGroups(t, model.PrefillGroup{
		Id:    10,
		Name:  "Blocked Words",
		Type:  "sensitive_word",
		Items: model.JSONValue(`["group-secret","重复","GROUP-SECRET"]`),
	})
	withRequestFilterRules(t, []setting.SensitiveRule{
		{
			ID:        "group-only",
			Name:      "Group Only",
			Enabled:   true,
			Action:    setting.SensitiveRuleActionBlock,
			Scope:     setting.SensitiveRuleScopeRequest,
			GroupRefs: []string{"10"},
		},
	})
	setFilterChannelIds(1)

	c := newJSONFilterContext(t, `{"model":"gpt-test","messages":[{"role":"user","content":"hello group-secret"}]}`)
	common.SetContextKey(c, constant.ContextKeyChannelId, 1)

	result, err := ApplySensitiveFilterToRequestBody(c, types.RelayFormatOpenAI)
	require.NoError(t, err)

	assert.True(t, result.Blocked)
	require.Len(t, result.Matches, 1)
	assert.Equal(t, "group-secret", result.Matches[0].Keyword)
}

func TestApplySensitiveFilterToResponseBodyMasksJSONText(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{
		{
			ID:          "response-mask",
			Name:        "Response Mask",
			Enabled:     true,
			Action:      setting.SensitiveRuleActionMask,
			Scope:       setting.SensitiveRuleScopeResponse,
			Replacement: "[MASK]",
			Keywords:    []string{"Secret", "中文"},
		},
	})
	setFilterChannelIds(1)
	c := newJSONFilterContext(t, `{}`)
	common.SetContextKey(c, constant.ContextKeyChannelId, 1)
	body := []byte(`{
		"id":"chatcmpl-secret",
		"model":"gpt-secret",
		"choices":[{"message":{"content":"Secret 中文"}}],
		"metadata":{"note":"Secret"},
		"usage":{"prompt_tokens":1}
	}`)

	result, filtered, err := ApplySensitiveFilterToResponseBody(c, "application/json", body)
	require.NoError(t, err)

	assert.False(t, result.Blocked)
	assert.True(t, result.Mutated)
	bodyText := string(filtered)
	assert.Contains(t, bodyText, "[MASK] [MASK]")
	assert.Contains(t, bodyText, "chatcmpl-secret")
	assert.Contains(t, bodyText, "gpt-secret")
	assert.Contains(t, bodyText, `"note":"Secret"`)
	assert.NotContains(t, bodyText, "Secret 中文")
}

func TestApplySensitiveFilterToResponseBodyBlocksBeforeMasking(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{
		{
			ID:          "mask",
			Name:        "Mask",
			Enabled:     true,
			Action:      setting.SensitiveRuleActionMask,
			Scope:       setting.SensitiveRuleScopeResponse,
			Replacement: "[MASK]",
			Keywords:    []string{"secret"},
		},
		{
			ID:       "block",
			Name:     "Block",
			Enabled:  true,
			Action:   setting.SensitiveRuleActionBlock,
			Scope:    setting.SensitiveRuleScopeResponse,
			Keywords: []string{"secret"},
		},
	})
	setFilterChannelIds(1)
	c := newJSONFilterContext(t, `{}`)
	common.SetContextKey(c, constant.ContextKeyChannelId, 1)

	result, filtered, err := ApplySensitiveFilterToResponseBody(c, "application/json", []byte(`{"choices":[{"message":{"content":"secret"}}]}`))
	require.NoError(t, err)

	assert.True(t, result.Blocked)
	assert.False(t, result.Mutated)
	assert.Contains(t, string(filtered), "secret")
	assert.True(t, IsContentPolicyRejected(c))
	assert.False(t, shouldRecordRelaySuccess(c))
}

func TestIOCopyBytesGracefullyReturnsForbiddenForBlockedResponse(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{{
		ID: "response-block", Name: "Response Block", Enabled: true,
		Action: setting.SensitiveRuleActionBlock, Scope: setting.SensitiveRuleScopeResponse,
		Keywords: []string{"secret"},
	}})
	setFilterChannelIds(1)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{}`))
	c.Set(common.RequestIdKey, "response-sensitive-1")
	common.SetContextKey(c, constant.ContextKeyChannelId, 1)
	upstream := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}

	IOCopyBytesGracefully(c, upstream, []byte(`{"choices":[{"message":{"content":"secret"}}]}`))

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "内容审计命中风险规则")
	require.NotContains(t, recorder.Body.String(), string(types.ErrorCodeSensitiveWordsDetected))
	require.NotContains(t, recorder.Body.String(), "metadata")
	require.NotContains(t, recorder.Body.String(), "secret")
}

func TestApplySensitiveFilterToResponseBodySkipsRequestOnlyRules(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{
		{
			ID:       "request",
			Name:     "Request",
			Enabled:  true,
			Action:   setting.SensitiveRuleActionBlock,
			Scope:    setting.SensitiveRuleScopeRequest,
			Keywords: []string{"secret"},
		},
	})
	setFilterChannelIds(1)
	c := newJSONFilterContext(t, `{}`)
	common.SetContextKey(c, constant.ContextKeyChannelId, 1)

	result, filtered, err := ApplySensitiveFilterToResponseBody(c, "application/json", []byte(`{"choices":[{"message":{"content":"secret"}}]}`))
	require.NoError(t, err)

	assert.False(t, result.Blocked)
	assert.False(t, result.Mutated)
	assert.Contains(t, string(filtered), "secret")
}

func TestApplySensitiveFilterToStreamDataMasksAndBlocks(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{
		{
			ID:          "mask",
			Name:        "Mask",
			Enabled:     true,
			Action:      setting.SensitiveRuleActionMask,
			Scope:       setting.SensitiveRuleScopeBoth,
			Replacement: "[MASK]",
			Keywords:    []string{"secret"},
		},
		{
			ID:       "block",
			Name:     "Block",
			Enabled:  true,
			Action:   setting.SensitiveRuleActionBlock,
			Scope:    setting.SensitiveRuleScopeResponse,
			Keywords: []string{"forbidden"},
		},
	})
	setFilterChannelIds(1)
	c := newJSONFilterContext(t, `{}`)
	common.SetContextKey(c, constant.ContextKeyChannelId, 1)

	result, filtered, err := ApplySensitiveFilterToStreamData(c, `{"choices":[{"delta":{"content":"Secret "}}]}`)
	require.NoError(t, err)
	assert.False(t, result.Blocked)
	assert.True(t, result.Mutated)
	assert.Contains(t, filtered, "[MASK]")
	assert.NotContains(t, filtered, "Secret")

	result, filtered, err = ApplySensitiveFilterToStreamData(c, `{"choices":[{"delta":{"content":"forbidden "}}]}`)
	require.NoError(t, err)
	assert.True(t, result.Blocked)
	assert.Equal(t, `{"choices":[{"delta":{"content":"forbidden "}}]}`, filtered)
	assert.True(t, IsContentPolicyRejected(c))
}

func TestResetSensitiveStreamDataForRetry(t *testing.T) {
	c := newJSONFilterContext(t, `{}`)
	buffer := &sensitiveStreamDelayBuffer{
		queue: []sensitiveStreamChunk{{data: `{type:response.created}`}},
	}
	c.Set(sensitiveStreamDelayBufferContextKey, buffer)
	c.Set("sensitive_response_stream_blocked", true)

	ResetSensitiveStreamDataForRetry(c)

	require.Nil(t, getSensitiveStreamDelayBuffer(c))
	require.False(t, c.GetBool("sensitive_response_stream_blocked"))
}

func TestSensitiveStreamDelayBufferHoldsWholeAtomicBatch(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{{
		ID: "response-block", Name: "Response Block", Enabled: true,
		Action: setting.SensitiveRuleActionBlock, Scope: setting.SensitiveRuleScopeResponse,
		Keywords: []string{"Master Key"},
	}})
	setFilterChannelIds(1)
	c := newJSONFilterContext(t, `{}`)
	common.SetContextKey(c, constant.ContextKeyChannelId, 1)

	result, err := ApplySensitiveFilterToStreamDataBatchForSend(c, []SensitiveStreamDataItem{
		{Data: `{"type":"response.created","response":{"id":"resp_1"}}`, EventLine: "event: response.created\n"},
		{Data: `{"type":"response.output_text.delta","delta":"Master","item_id":"msg_1"}`, EventLine: "event: response.output_text.delta\n"},
	})
	require.NoError(t, err)
	require.True(t, result.Held)
	require.Empty(t, result.Items)

	result, err = FlushSensitiveStreamDataForSend(c)
	require.NoError(t, err)
	require.Len(t, result.Items, 2)
	require.Contains(t, result.Items[0].Data, "response.created")
	require.Contains(t, result.Items[1].Data, "response.output_text.delta")
}

func TestSensitiveStreamDelayBufferDoesNotHoldEarlierBatch(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{{
		ID: "response-block", Name: "Response Block", Enabled: true,
		Action: setting.SensitiveRuleActionBlock, Scope: setting.SensitiveRuleScopeResponse,
		Keywords: []string{"Master Key"},
	}})
	setFilterChannelIds(1)
	c := newJSONFilterContext(t, `{}`)
	common.SetContextKey(c, constant.ContextKeyChannelId, 1)

	result, err := ApplySensitiveFilterToStreamDataBatchForSend(c, []SensitiveStreamDataItem{{
		Data: `{"type":"response.created","response":{"id":"resp_1"}}`, EventLine: "event: response.created\n",
	}})
	require.NoError(t, err)
	require.False(t, result.Held)
	require.Len(t, result.Items, 1)

	result, err = ApplySensitiveFilterToStreamDataBatchForSend(c, []SensitiveStreamDataItem{{
		Data: `{"type":"response.output_text.delta","delta":"Master"}`, EventLine: "event: response.output_text.delta\n",
	}})
	require.NoError(t, err)
	require.True(t, result.Held)
	require.Empty(t, result.Items)
}

func TestSensitiveStreamDelayBufferEmitsSafeTextBeforeAtomicPrefix(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{{
		ID: "response-block", Name: "Response Block", Enabled: true,
		Action: setting.SensitiveRuleActionBlock, Scope: setting.SensitiveRuleScopeResponse,
		Keywords: []string{"Master Key"},
	}})
	setFilterChannelIds(1)
	c := newJSONFilterContext(t, `{}`)
	common.SetContextKey(c, constant.ContextKeyChannelId, 1)

	result, err := ApplySensitiveFilterToStreamDataBatchForSend(c, []SensitiveStreamDataItem{
		{Data: `{"type":"response.output_text.delta","delta":"safe "}`},
		{Data: `{"type":"response.in_progress"}`},
		{Data: `{"type":"response.output_text.delta","delta":"Master"}`},
	})
	require.NoError(t, err)
	require.True(t, result.Held)
	require.Len(t, result.Items, 1)
	require.Contains(t, result.Items[0].Data, `"delta":"safe "`)
}

func TestSelectSensitiveRulesForRouteMatchesExplicitChannelAndTagTargets(t *testing.T) {
	rules := []setting.SensitiveRule{
		{ID: "channel-20", Enabled: true, TargetType: setting.SensitiveRuleTargetChannels, ChannelIds: []int{20}},
		{ID: "channel-30", Enabled: true, TargetType: setting.SensitiveRuleTargetChannels, ChannelIds: []int{30}},
		{ID: "tag-a", Enabled: true, TargetType: setting.SensitiveRuleTargetChannelTags, ChannelTags: []string{"batch-a"}},
		{ID: "tag-b", Enabled: true, TargetType: setting.SensitiveRuleTargetChannelTags, ChannelTags: []string{"batch-b"}},
	}

	selected := selectSensitiveRulesForRoute(rules, sensitiveRuleRouteScope{
		channelId:               20,
		channelTag:              "batch-a",
		channelTagKnown:         true,
		channelEligible:         true,
		channelEligibilityKnown: true,
	}, setting.GetSensitivePolicySnapshot())
	require.ElementsMatch(t, []string{"channel-20", "tag-a"}, sensitiveRuleIDs(selected))

	selected = selectSensitiveRulesForRoute(rules, sensitiveRuleRouteScope{
		channelId:               99,
		channelTag:              "batch-z",
		channelTagKnown:         true,
		channelEligible:         true,
		channelEligibilityKnown: true,
	}, setting.GetSensitivePolicySnapshot())
	require.Empty(t, selected)
}

func TestSelectSensitiveRulesForRouteMatchesAllAndBusinessGroupTargets(t *testing.T) {
	rules := []setting.SensitiveRule{
		{ID: "all", Enabled: true, TargetType: setting.SensitiveRuleTargetAll},
		{ID: "group-a", Enabled: true, TargetType: setting.SensitiveRuleTargetGroups, GroupCodes: []string{"group-a"}},
		{ID: "group-b", Enabled: true, TargetType: setting.SensitiveRuleTargetGroups, GroupCodes: []string{"group-b"}},
	}
	route := sensitiveRuleRouteScope{
		channelId:          10,
		channelGroupsKnown: true,
		channelGroupCodes:  []string{"group-a"},
	}
	selected := selectSensitiveRulesForRoute(rules, route, setting.SensitivePolicySnapshot{})
	assert.Equal(t, []string{"all", "group-a"}, sensitiveRuleIDs(selected))

	before := sensitiveRuleRouteScope{
		before:              true,
		candidateGroupCodes: []string{"group-b"},
	}
	selected = selectSensitiveRulesForRoute(rules, before, setting.SensitivePolicySnapshot{})
	assert.Equal(t, []string{"all", "group-b"}, sensitiveRuleIDs(selected))
}

func TestSelectSensitiveRulesForRouteMatchesCombinedTargetsWithOrSemantics(t *testing.T) {
	rules := []setting.SensitiveRule{{
		ID:         "routes",
		Enabled:    true,
		TargetType: setting.SensitiveRuleTargetRoutes,
		ChannelIds: []int{20},
		GroupCodes: []string{"group-a"},
	}}

	selected := selectSensitiveRulesForRoute(rules, sensitiveRuleRouteScope{
		channelId:          20,
		channelGroupsKnown: true,
		channelGroupCodes:  []string{"group-z"},
	}, setting.SensitivePolicySnapshot{})
	require.Equal(t, []string{"routes"}, sensitiveRuleIDs(selected))

	selected = selectSensitiveRulesForRoute(rules, sensitiveRuleRouteScope{
		channelId:          30,
		channelGroupsKnown: true,
		channelGroupCodes:  []string{"group-a"},
	}, setting.SensitivePolicySnapshot{})
	require.Equal(t, []string{"routes"}, sensitiveRuleIDs(selected))

	selected = selectSensitiveRulesForRoute(rules, sensitiveRuleRouteScope{
		channelId:          30,
		channelGroupsKnown: true,
		channelGroupCodes:  []string{"group-z"},
	}, setting.SensitivePolicySnapshot{})
	require.Empty(t, selected)

	selected = selectSensitiveRulesForRoute(rules, sensitiveRuleRouteScope{
		before:              true,
		candidateGroupCodes: []string{"group-a"},
	}, setting.SensitivePolicySnapshot{})
	require.Equal(t, []string{"routes"}, sensitiveRuleIDs(selected))

	selected = selectSensitiveRulesForRoute(rules, sensitiveRuleRouteScope{
		before:              true,
		candidateGroupCodes: []string{"group-z"},
	}, setting.SensitivePolicySnapshot{})
	require.Empty(t, selected)
}

func TestSelectSensitiveRulesBeforeDistributionSkipsKnownUnavailableFixedChannel(t *testing.T) {
	rules := []setting.SensitiveRule{
		{ID: "channel", Enabled: true, TargetType: setting.SensitiveRuleTargetChannels, ChannelIds: []int{20}},
		{ID: "tag", Enabled: true, TargetType: setting.SensitiveRuleTargetChannelTags, ChannelTags: []string{"batch-a"}},
	}

	selected := selectSensitiveRulesForRoute(rules, sensitiveRuleRouteScope{
		channelId:               20,
		channelTag:              "batch-a",
		channelTagKnown:         true,
		channelEligible:         false,
		channelEligibilityKnown: true,
		before:                  true,
	}, setting.GetSensitivePolicySnapshot())
	require.Empty(t, selected)
}

func TestSelectSensitiveRulesBeforeDistributionDefersUncertainChannelScope(t *testing.T) {
	rules := []setting.SensitiveRule{
		{ID: "candidate", Enabled: true, TargetType: setting.SensitiveRuleTargetChannels, ChannelIds: []int{10}},
		{ID: "tag", Enabled: true, TargetType: setting.SensitiveRuleTargetChannelTags, ChannelTags: []string{"batch-a"}},
		{ID: "group", Enabled: true, TargetType: setting.SensitiveRuleTargetGroups, GroupCodes: []string{"group-a"}},
		{ID: "all", Enabled: true, TargetType: setting.SensitiveRuleTargetAll},
	}

	selected := selectSensitiveRulesForRoute(rules, sensitiveRuleRouteScope{
		unknownCandidateGroups: true,
		before:                 true,
	}, setting.GetSensitivePolicySnapshot())
	require.ElementsMatch(t, []string{"all"}, sensitiveRuleIDs(selected))

	selected = selectSensitiveRulesForRoute(rules, sensitiveRuleRouteScope{
		candidateGroupCodes: []string{"group-a"},
		before:              true,
	}, setting.GetSensitivePolicySnapshot())
	require.ElementsMatch(t, []string{"all", "group"}, sensitiveRuleIDs(selected))
}

func TestSensitiveFilterDoesNotPrecheckUncertainChannelRule(t *testing.T) {
	withRequestFilterRules(t, []setting.SensitiveRule{{
		ID:         "claude-channel",
		Enabled:    true,
		TargetType: setting.SensitiveRuleTargetChannels,
		ChannelIds: []int{20},
		Action:     setting.SensitiveRuleActionBlock,
		Keywords:   []string{"blocked phrase"},
	}})

	c := newJSONFilterContext(t, `{"model":"claude-fable-5","messages":[{"role":"user","content":"blocked phrase"}]}`)
	result, err := ApplySensitiveFilterToRequestBodyBeforeDistribution(c, types.RelayFormatOpenAI, "claude-fable-5", "auto")
	require.NoError(t, err)
	require.False(t, result.Blocked)
	require.False(t, c.GetBool(sensitiveRequestPrecheckedContextKey))

	common.SetContextKey(c, constant.ContextKeyChannelId, 20)
	common.SetContextKey(c, constant.ContextKeySelectedChannel, &model.Channel{
		Id: 20, Status: common.ChannelStatusEnabled, GroupDetails: []model.GroupReference{},
	})
	result, err = ApplySensitiveFilterToRequestBody(c, types.RelayFormatOpenAI)
	require.NoError(t, err)
	require.True(t, result.Blocked)
}

func TestSelectSensitiveRulesBeforeDistributionPreservesLegacyDynamicScope(t *testing.T) {
	oldChannelIds := setting.SensitiveRuleChannelIds
	setting.SensitiveRuleChannelIds = []int{999}
	t.Cleanup(func() { setting.SensitiveRuleChannelIds = oldChannelIds })
	rules := []setting.SensitiveRule{{ID: "legacy", Enabled: true}}

	selected := selectSensitiveRulesForRoute(rules, sensitiveRuleRouteScope{
		candidateGroupCodes: []string{"default"},
		modelName:           "gpt-test",
		before:              true,
	}, setting.GetSensitivePolicySnapshot())
	require.Equal(t, []string{"legacy"}, sensitiveRuleIDs(selected))
}

func TestSelectSensitiveRulesBeforeDistributionMatchesFixedChannelExactly(t *testing.T) {
	c := newJSONFilterContext(t, `{}`)
	common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, "20")
	common.SetContextKey(c, constant.ContextKeySelectedChannel, &model.Channel{
		Id: 20, Status: common.ChannelStatusEnabled, Tag: common.GetPointer("batch-a"),
	})
	rules := []setting.SensitiveRule{
		{ID: "fixed", Enabled: true, TargetType: setting.SensitiveRuleTargetChannels, ChannelIds: []int{20}},
		{ID: "other", Enabled: true, TargetType: setting.SensitiveRuleTargetChannels, ChannelIds: []int{30}},
	}

	selected := selectSensitiveRulesBeforeDistribution(c, rules, setting.GetSensitivePolicySnapshot(), "gpt-test", "")
	require.Equal(t, []string{"fixed"}, sensitiveRuleIDs(selected))
}

func TestSelectSensitiveRulesForSelectedRouteUsesActualChannelTagsAcrossRetries(t *testing.T) {
	c := newJSONFilterContext(t, `{}`)
	common.SetContextKey(c, constant.ContextKeyChannelId, 20)
	common.SetContextKey(c, constant.ContextKeySelectedChannel, &model.Channel{Id: 20, Tag: common.GetPointer("batch-b")})
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "alpha,beta")
	common.SetContextKey(c, constant.ContextKeyTokenGroupIds, []int{1, 2})
	rules := []setting.SensitiveRule{
		{ID: "channel", Enabled: true, TargetType: setting.SensitiveRuleTargetChannels, ChannelIds: []int{20}},
		{ID: "retry-channel", Enabled: true, TargetType: setting.SensitiveRuleTargetChannels, ChannelIds: []int{30}},
		{ID: "batch-a", Enabled: true, TargetType: setting.SensitiveRuleTargetChannelTags, ChannelTags: []string{"batch-a"}},
		{ID: "batch-b", Enabled: true, TargetType: setting.SensitiveRuleTargetChannelTags, ChannelTags: []string{"batch-b"}},
	}

	selected := selectSensitiveRulesForSelectedRoute(c, rules, setting.GetSensitivePolicySnapshot())
	require.ElementsMatch(t, []string{"channel", "batch-b"}, sensitiveRuleIDs(selected))

	common.SetContextKey(c, constant.ContextKeyChannelId, 30)
	common.SetContextKey(c, constant.ContextKeySelectedChannel, &model.Channel{Id: 30, Tag: common.GetPointer("batch-a")})
	selected = selectSensitiveRulesForSelectedRoute(c, rules, setting.GetSensitivePolicySnapshot())
	require.ElementsMatch(t, []string{"retry-channel", "batch-a"}, sensitiveRuleIDs(selected))
}

func TestSelectSensitiveRulesForSelectedRouteDoesNotUseUserRouteOrTokenGroupsAsTags(t *testing.T) {
	c := newJSONFilterContext(t, `{}`)
	common.SetContextKey(c, constant.ContextKeyChannelId, 20)
	common.SetContextKey(c, constant.ContextKeySelectedChannel, &model.Channel{Id: 20, GroupIds: []int{9}, Tag: common.GetPointer("actual-channel-group")})
	common.SetContextKey(c, constant.ContextKeyUserGroupId, 2)
	common.SetContextKey(c, constant.ContextKeyTokenGroupIds, []int{2})
	rules := []setting.SensitiveRule{
		{ID: "user-token-group", Enabled: true, TargetType: setting.SensitiveRuleTargetChannelTags, ChannelTags: []string{"2", "9"}},
		{ID: "actual-channel-group", Enabled: true, TargetType: setting.SensitiveRuleTargetChannelTags, ChannelTags: []string{"actual-channel-group"}},
	}

	selected := selectSensitiveRulesForSelectedRoute(c, rules, setting.GetSensitivePolicySnapshot())
	require.Equal(t, []string{"actual-channel-group"}, sensitiveRuleIDs(selected))
}
