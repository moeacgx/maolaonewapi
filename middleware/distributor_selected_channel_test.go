package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSetupContextForSelectedChannelOverwritesRetrySnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	first := &model.Channel{
		Id: 11, Name: "first", Key: "first-key", Tag: common.GetPointer("batch-a"),
	}
	second := &model.Channel{
		Id: 22, Name: "second", Key: "second-key", Tag: common.GetPointer("batch-b"),
	}
	t.Cleanup(func() { releaseChannelConcurrencyForContext(c) })

	require.Nil(t, SetupContextForSelectedChannel(c, first, "gpt-test"))
	selected, ok := common.GetContextKeyType[*model.Channel](c, constant.ContextKeySelectedChannel)
	require.True(t, ok)
	require.Same(t, first, selected)

	require.Nil(t, SetupContextForSelectedChannel(c, second, "gpt-test"))
	selected, ok = common.GetContextKeyType[*model.Channel](c, constant.ContextKeySelectedChannel)
	require.True(t, ok)
	require.Same(t, second, selected)
	require.Equal(t, "batch-b", *selected.Tag)
}
