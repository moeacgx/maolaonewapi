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

func TestSetupContextForSelectedChannelReleasesConcurrencyWhenKeySelectionFails(t *testing.T) {
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
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	limit := 1
	channel := &model.Channel{Id: 990002, Key: "test-key", ConcurrencyLimit: &limit}

	require.Nil(t, SetupContextForSelectedChannel(ctx, channel, "test-model"))
	ReleaseChannelConcurrencyForContext(ctx)
	require.True(t, model.TryAcquireChannelConcurrency(channel))

	ReleaseChannelConcurrencyForContext(ctx)
	require.False(t, model.IsChannelConcurrencyAvailable(channel), "a second release must not consume another request's slot")
	model.ReleaseChannelConcurrency(channel.Id)
}
