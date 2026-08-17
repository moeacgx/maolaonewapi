package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestParseTokenGroupListRejectsAmbiguousLists(t *testing.T) {
	groups, err := ParseTokenGroupList("vip,default")
	require.NoError(t, err)
	require.Equal(t, []string{"vip", "default"}, groups)

	for _, raw := range []string{"vip,,default", "vip,vip", "vip,auto"} {
		_, err := ParseTokenGroupList(raw)
		require.Error(t, err, raw)
	}
}

func TestGetRequestTokenGroupsScopesAuthorizedOverride(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "vip,default")
	common.SetContextKey(ctx, constant.ContextKeyTokenGroups, []string{"vip", "default"})

	require.Equal(t, []string{"vip", "default"}, GetRequestTokenGroups(ctx, "vip,default"))
	require.Equal(t, []string{"default"}, GetRequestTokenGroups(ctx, "default"))
	require.Nil(t, GetRequestTokenGroups(ctx, "unauthorized"))
}

func TestRelayMaxRetriesUsesGlobalBudgetForOrderedGroups(t *testing.T) {
	original := common.RetryTimes
	common.RetryTimes = 7
	t.Cleanup(func() { common.RetryTimes = original })

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "vip,default")
	common.SetContextKey(ctx, constant.ContextKeyTokenGroups, []string{"vip", "default"})
	param := &RetryParam{Ctx: ctx, TokenGroup: "vip,default", Retry: common.GetPointer(0)}
	require.Equal(t, 7, RelayMaxRetries(param))

	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)
	common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{})
	param.TokenGroup = "auto"
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	require.Equal(t, 7, RelayMaxRetries(param))
}

func TestRetryParamExclusionsDoNotMutateOriginalTokenGroup(t *testing.T) {
	param := &RetryParam{TokenGroup: "vip,default", Retry: common.GetPointer(0)}
	param.ExcludeChannelID(42, false)
	require.Equal(t, "vip,default", param.TokenGroup)
	_, excluded := param.ExcludedChannelIDs[42]
	require.True(t, excluded)
}
