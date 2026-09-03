package middleware

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelSelectionConcurrencyErrorMessageUsesRequestLanguage(t *testing.T) {
	require.NoError(t, i18n.Init())

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/v1/models", nil)
	ctx.Request.Header.Set("Accept-Language", "zh-CN")

	assert.Equal(t, "当前渠道并发已达到上限，请稍后重试", channelSelectionErrorMessage(ctx, model.ErrChannelConcurrencyLimitReached))
	assert.Equal(t, "当前渠道并发已达到上限，请稍后重试", channelSelectionErrorMessage(ctx, fmt.Errorf("wrapped: %w", model.ErrChannelConcurrencyLimitReached)))

	ctx.Request.Header.Set("Accept-Language", "en")
	assert.Equal(t, "The channel concurrency limit has been reached, please try again later", channelSelectionErrorMessage(ctx, model.ErrChannelConcurrencyLimitReached))
}

func TestSetupContextForSelectedChannelReleasesConcurrencyWhenKeySelectionFails(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	limit := 1
	channel := &model.Channel{
		Id:               990001,
		Key:              "disabled-key",
		ConcurrencyLimit: &limit,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:         true,
			MultiKeyStatusList: map[int]int{0: common.ChannelStatusAutoDisabled},
			MultiKeyMode:       constant.MultiKeyModeRandom,
		},
	}
	err := SetupContextForSelectedChannel(c, channel, "test-model")
	require.Error(t, err)
	assert.True(t, model.IsChannelConcurrencyAvailable(channel))
}

func TestReleaseChannelConcurrencyForContextIsIdempotentAndOwnsOnlyContextSlot(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	limit := 1
	channel := &model.Channel{Id: 990002, Key: "test-key", ConcurrencyLimit: &limit}

	require.Nil(t, SetupContextForSelectedChannel(ctx, channel, "test-model"))
	ReleaseChannelConcurrencyForContext(ctx)
	lease, acquired := model.TryAcquireChannelConcurrencyLease(channel)
	require.True(t, acquired)

	ReleaseChannelConcurrencyForContext(ctx)
	require.False(t, model.IsChannelConcurrencyAvailable(channel), "a second release must not consume another request's slot")
	require.True(t, model.ReleaseChannelConcurrencyLease(lease))
}
