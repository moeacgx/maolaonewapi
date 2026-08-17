package controller

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func withTaskAuditRules(t *testing.T, rules []setting.SensitiveRule) {
	t.Helper()
	oldEnabled := setting.CheckSensitiveEnabled
	oldPromptEnabled := setting.CheckSensitiveOnPromptEnabled
	oldRules := setting.SensitiveRules
	oldConfigured := setting.SensitiveRulesConfigured
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
		setting.SensitiveRulesConfigured = oldConfigured
		setting.SensitiveRuleChannelIds = oldChannelIds
		setting.SensitiveWords = oldWords
	})
}

func newTaskAuditContext(body string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c
}

func taskAuditStoredBody(t *testing.T, c *gin.Context) string {
	t.Helper()
	storage, err := common.GetBodyStorage(c)
	require.NoError(t, err)
	body, err := storage.Bytes()
	require.NoError(t, err)
	_, err = storage.Seek(0, io.SeekStart)
	require.NoError(t, err)
	return string(body)
}

func TestPrepareSelectedRouteTaskRequestBlocksOnlyActualTaskChannel(t *testing.T) {
	withTaskAuditRules(t, []setting.SensitiveRule{{
		ID: "task-channel", Enabled: true, Action: setting.SensitiveRuleActionBlock,
		Keywords: []string{"blocked phrase"}, TargetType: setting.SensitiveRuleTargetChannels, ChannelIds: []int{20},
	}})
	original := []byte(`{"prompt":"blocked phrase"}`)
	c := newTaskAuditContext(string(original))
	common.SetContextKey(c, constant.ContextKeyChannelId, 10)
	common.SetContextKey(c, constant.ContextKeySelectedChannel, &model.Channel{Id: 10})
	require.Nil(t, prepareSelectedRouteTaskRequest(c, types.RelayFormatTask, original))

	common.SetContextKey(c, constant.ContextKeyChannelId, 20)
	common.SetContextKey(c, constant.ContextKeySelectedChannel, &model.Channel{Id: 20})
	taskErr := prepareSelectedRouteTaskRequest(c, types.RelayFormatTask, original)
	require.NotNil(t, taskErr)
	require.True(t, taskErr.LocalError)
	require.Equal(t, string(types.ErrorCodeSensitiveWordsDetected), taskErr.Code)
}

func TestPrepareSelectedRouteTaskRequestRestoresBodyBeforeRetryMask(t *testing.T) {
	withTaskAuditRules(t, []setting.SensitiveRule{
		{ID: "first", Enabled: true, Action: setting.SensitiveRuleActionMask, Replacement: "[FIRST]", Keywords: []string{"route-secret"}, TargetType: setting.SensitiveRuleTargetChannels, ChannelIds: []int{10}},
		{ID: "second", Enabled: true, Action: setting.SensitiveRuleActionMask, Replacement: "[SECOND]", Keywords: []string{"route-secret"}, TargetType: setting.SensitiveRuleTargetChannels, ChannelIds: []int{20}},
	})
	original := []byte(`{"prompt":"route-secret"}`)
	c := newTaskAuditContext(string(original))
	common.SetContextKey(c, constant.ContextKeyChannelId, 10)
	common.SetContextKey(c, constant.ContextKeySelectedChannel, &model.Channel{Id: 10})
	require.Nil(t, prepareSelectedRouteTaskRequest(c, types.RelayFormatTask, original))
	require.JSONEq(t, `{"prompt":"[FIRST]"}`, taskAuditStoredBody(t, c))

	common.SetContextKey(c, constant.ContextKeyChannelId, 20)
	common.SetContextKey(c, constant.ContextKeySelectedChannel, &model.Channel{Id: 20})
	require.Nil(t, prepareSelectedRouteTaskRequest(c, types.RelayFormatTask, original))
	require.JSONEq(t, `{"prompt":"[SECOND]"}`, taskAuditStoredBody(t, c))
}

func TestPrepareSelectedRouteTaskRequestMasksMidjourneyByFullChannelTag(t *testing.T) {
	withTaskAuditRules(t, []setting.SensitiveRule{{
		ID: "mj-tag", Enabled: true, Action: setting.SensitiveRuleActionMask, Replacement: "[MJ]",
		Keywords: []string{"mj-secret"}, TargetType: setting.SensitiveRuleTargetChannelTags, ChannelTags: []string{"mj-guarded"},
	}})
	original := []byte(`{"prompt":"mj-secret"}`)
	c := newTaskAuditContext(string(original))
	common.SetContextKey(c, constant.ContextKeyChannelId, 30)
	common.SetContextKey(c, constant.ContextKeySelectedChannel, &model.Channel{Id: 30, Tag: common.GetPointer("mj-guarded")})
	require.Nil(t, prepareSelectedRouteTaskRequest(c, types.RelayFormatMjProxy, original))
	require.JSONEq(t, `{"prompt":"[MJ]"}`, taskAuditStoredBody(t, c))
}
