package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSelectedChannelContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	return ctx
}

func newMultiKeyChannel(id int, keys string, statusList map[int]int) *model.Channel {
	return &model.Channel{
		Id:  id,
		Key: keys,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:         true,
			MultiKeyStatusList: statusList,
		},
	}
}

func TestSetupContextForSelectedChannelFallsBackWhenPreferredKeyBecomesDisabled(t *testing.T) {
	channel := newMultiKeyChannel(1001, "fallback\npreferred", map[int]int{
		0: common.ChannelStatusEnabled,
		1: common.ChannelStatusEnabled,
	})
	_, _, validationErr := channel.GetKeyByIndex(1)
	require.Nil(t, validationErr)

	channel.ChannelInfo.MultiKeyStatusList[1] = common.ChannelStatusManuallyDisabled
	ctx := newSelectedChannelContext()
	common.SetContextKey(ctx, constant.ContextKeyChannelPreferredMultiKeyIndex, 1)

	require.Nil(t, SetupContextForSelectedChannel(ctx, channel, "test-model"))
	assert.Equal(t, "fallback", common.GetContextKeyString(ctx, constant.ContextKeyChannelKey))
	assert.Equal(t, 0, common.GetContextKeyInt(ctx, constant.ContextKeyChannelMultiKeyIndex))
	_, exists := common.GetContextKey(ctx, constant.ContextKeyChannelPreferredMultiKeyIndex)
	assert.False(t, exists)
}

func TestSetupContextForSelectedChannelFallsBackFromOutOfRangePreference(t *testing.T) {
	channel := newMultiKeyChannel(1002, "first\nsecond", nil)
	ctx := newSelectedChannelContext()
	common.SetContextKey(ctx, constant.ContextKeyChannelPreferredMultiKeyIndex, 99)

	require.Nil(t, SetupContextForSelectedChannel(ctx, channel, "test-model"))
	assert.Equal(t, "first", common.GetContextKeyString(ctx, constant.ContextKeyChannelKey))
	assert.Equal(t, 0, common.GetContextKeyInt(ctx, constant.ContextKeyChannelMultiKeyIndex))
}

func TestSetupContextForSelectedChannelReturnsFallbackErrorWhenAllKeysUnusable(t *testing.T) {
	channel := newMultiKeyChannel(1003, "first\nsecond", map[int]int{
		0: common.ChannelStatusManuallyDisabled,
		1: common.ChannelStatusManuallyDisabled,
	})
	ctx := newSelectedChannelContext()
	common.SetContextKey(ctx, constant.ContextKeyChannelPreferredMultiKeyIndex, 1)

	apiErr := SetupContextForSelectedChannel(ctx, channel, "test-model")
	require.NotNil(t, apiErr)
	assert.Equal(t, "no enabled keys", apiErr.Error())
	_, exists := common.GetContextKey(ctx, constant.ContextKeyChannelPreferredMultiKeyIndex)
	assert.False(t, exists)
}

func TestSetupContextForSelectedChannelConsumesPreferredMarkerBetweenSelections(t *testing.T) {
	ctx := newSelectedChannelContext()
	common.SetContextKey(ctx, constant.ContextKeyChannelPreferredMultiKeyIndex, 1)

	first := newMultiKeyChannel(1004, "first-0\nfirst-1", nil)
	require.Nil(t, SetupContextForSelectedChannel(ctx, first, "test-model"))
	assert.Equal(t, "first-1", common.GetContextKeyString(ctx, constant.ContextKeyChannelKey))
	_, exists := common.GetContextKey(ctx, constant.ContextKeyChannelPreferredMultiKeyIndex)
	require.False(t, exists)

	second := newMultiKeyChannel(1005, "second-0\nsecond-1", nil)
	require.Nil(t, SetupContextForSelectedChannel(ctx, second, "test-model"))
	assert.Equal(t, "second-0", common.GetContextKeyString(ctx, constant.ContextKeyChannelKey))
	assert.Equal(t, 0, common.GetContextKeyInt(ctx, constant.ContextKeyChannelMultiKeyIndex))
}
