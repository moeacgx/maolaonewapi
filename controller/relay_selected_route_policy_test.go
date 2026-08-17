package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const selectedRouteOriginalBody = `{"model":"gpt-test","messages":[{"role":"user","content":"route secret"}]}`

func withSelectedRouteSensitiveRules(t *testing.T, rules ...setting.SensitiveRule) {
	t.Helper()
	previous := setting.GetSensitivePolicySnapshot()
	setting.ReplaceSensitivePolicySnapshot(setting.SensitivePolicySnapshot{
		CheckEnabled:         true,
		CheckOnPromptEnabled: true,
		Rules:                rules,
		RulesConfigured:      true,
	})
	t.Cleanup(func() { setting.ReplaceSensitivePolicySnapshot(previous) })
}

func newSelectedRouteRelayContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(selectedRouteOriginalBody))
	c.Request.Header.Set("Content-Type", "application/json")
	return c
}

func installSelectedRoute(t *testing.T, c *gin.Context, channel *model.Channel, group string) {
	t.Helper()
	common.SetContextKey(c, constant.ContextKeySelectedChannelGroup, group)
	require.Nil(t, middleware.SetupContextForSelectedChannel(c, channel, "gpt-test"))
	setSelectedSecurityAuditRoute(c, channel, group)
}

func selectedRouteChannel(id int, tag string, groups ...string) *model.Channel {
	channel := &model.Channel{
		Id: id, Type: constant.ChannelTypeOpenAI, Key: "sk-test", Name: tag,
		Status: common.ChannelStatusEnabled, Tag: common.GetPointer(tag),
	}
	for index, group := range groups {
		channel.GroupDetails = append(channel.GroupDetails, model.GroupReference{Id: index + 1, Code: group, Name: group})
	}
	return channel
}

func serializedPreparedRequest(t *testing.T, request dto.Request) string {
	t.Helper()
	body, err := common.Marshal(request)
	require.NoError(t, err)
	return string(body)
}

func TestPrepareSelectedRouteRequestAppliesFirstAttemptChannelTagRule(t *testing.T) {
	withSelectedRouteSensitiveRules(t, setting.SensitiveRule{
		ID: "first-tag", Name: "first-tag", Enabled: true,
		Action: setting.SensitiveRuleActionBlock, Scope: setting.SensitiveRuleScopeRequest,
		TargetType: setting.SensitiveRuleTargetChannelTags, ChannelTags: []string{"protected"},
		Keywords: []string{"route secret"},
	})
	c := newSelectedRouteRelayContext()
	channel := selectedRouteChannel(11, "protected", "alpha")
	installSelectedRoute(t, c, channel, "alpha")

	selected, ok := common.GetContextKeyType[*model.Channel](c, constant.ContextKeySelectedChannel)
	require.True(t, ok)
	require.Same(t, channel, selected)
	require.Equal(t, "protected", selected.GetTag())
	selectedByRelay, channelErr := getChannel(c, &relaycommon.RelayInfo{UsingGroup: "alpha"}, nil)
	require.Nil(t, channelErr)
	require.Same(t, channel, selectedByRelay)

	request, apiErr := prepareSelectedRouteRequest(c, types.RelayFormatOpenAI, []byte(selectedRouteOriginalBody))
	require.Nil(t, request)
	require.NotNil(t, apiErr)
	require.Equal(t, types.ErrorCodeSensitiveWordsDetected, apiErr.GetErrorCode())
}

func TestPrepareSelectedRouteRequestBlocksRetryInFinalSelectedGroup(t *testing.T) {
	withSelectedRouteSensitiveRules(t, setting.SensitiveRule{
		ID: "beta-only", Name: "beta-only", Enabled: true,
		Action: setting.SensitiveRuleActionBlock, Scope: setting.SensitiveRuleScopeRequest,
		TargetType: setting.SensitiveRuleTargetGroups, GroupCodes: []string{"beta"},
		Keywords: []string{"route secret"},
	})
	c := newSelectedRouteRelayContext()
	first := selectedRouteChannel(21, "shared", "alpha", "beta")
	installSelectedRoute(t, c, first, "alpha")

	request, apiErr := prepareSelectedRouteRequest(c, types.RelayFormatOpenAI, []byte(selectedRouteOriginalBody))
	require.Nil(t, apiErr)
	require.Contains(t, serializedPreparedRequest(t, request), "route secret")

	retry := selectedRouteChannel(22, "retry", "beta")
	installSelectedRoute(t, c, retry, "beta")
	request, apiErr = prepareSelectedRouteRequest(c, types.RelayFormatOpenAI, []byte(selectedRouteOriginalBody))
	require.Nil(t, request)
	require.NotNil(t, apiErr)
	require.Equal(t, types.ErrorCodeSensitiveWordsDetected, apiErr.GetErrorCode())
}

func TestPrepareSelectedRouteRequestRetryMaskReachesSerializedPayload(t *testing.T) {
	withSelectedRouteSensitiveRules(t, setting.SensitiveRule{
		ID: "retry-mask", Name: "retry-mask", Enabled: true,
		Action: setting.SensitiveRuleActionMask, Scope: setting.SensitiveRuleScopeRequest,
		TargetType: setting.SensitiveRuleTargetChannels, ChannelIds: []int{32},
		Keywords: []string{"route secret"}, Replacement: "[RETRY-MASK]",
	})
	c := newSelectedRouteRelayContext()
	installSelectedRoute(t, c, selectedRouteChannel(31, "first", "alpha"), "alpha")

	request, apiErr := prepareSelectedRouteRequest(c, types.RelayFormatOpenAI, []byte(selectedRouteOriginalBody))
	require.Nil(t, apiErr)
	require.Contains(t, serializedPreparedRequest(t, request), "route secret")

	installSelectedRoute(t, c, selectedRouteChannel(32, "retry", "beta"), "beta")
	request, apiErr = prepareSelectedRouteRequest(c, types.RelayFormatOpenAI, []byte(selectedRouteOriginalBody))
	require.Nil(t, apiErr)
	serialized := serializedPreparedRequest(t, request)
	require.Contains(t, serialized, "[RETRY-MASK]")
	require.NotContains(t, serialized, "route secret")
	storage, err := common.GetBodyStorage(c)
	require.NoError(t, err)
	stored, err := storage.Bytes()
	require.NoError(t, err)
	require.Contains(t, string(stored), "[RETRY-MASK]")
}

func TestPrepareSelectedRouteRequestRetryDoesNotInheritFirstChannelMask(t *testing.T) {
	withSelectedRouteSensitiveRules(t, setting.SensitiveRule{
		ID: "first-mask", Name: "first-mask", Enabled: true,
		Action: setting.SensitiveRuleActionMask, Scope: setting.SensitiveRuleScopeRequest,
		TargetType: setting.SensitiveRuleTargetChannels, ChannelIds: []int{41},
		Keywords: []string{"route secret"}, Replacement: "[FIRST-MASK]",
	})
	c := newSelectedRouteRelayContext()
	installSelectedRoute(t, c, selectedRouteChannel(41, "first", "alpha"), "alpha")

	request, apiErr := prepareSelectedRouteRequest(c, types.RelayFormatOpenAI, []byte(selectedRouteOriginalBody))
	require.Nil(t, apiErr)
	require.Contains(t, serializedPreparedRequest(t, request), "[FIRST-MASK]")

	installSelectedRoute(t, c, selectedRouteChannel(42, "retry", "beta"), "beta")
	request, apiErr = prepareSelectedRouteRequest(c, types.RelayFormatOpenAI, []byte(selectedRouteOriginalBody))
	require.Nil(t, apiErr)
	serialized := serializedPreparedRequest(t, request)
	require.Contains(t, serialized, "route secret")
	require.NotContains(t, serialized, "[FIRST-MASK]")
}
