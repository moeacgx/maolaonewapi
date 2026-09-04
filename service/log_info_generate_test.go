package service

import (
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGenerateTextOtherInfoIncludesUpstreamResponseModelName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		OriginModelName: "requested-model",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "mapped-model",
		},
		UpstreamResponseModelName: "provider-actual",
	}

	other := GenerateTextOtherInfo(c, info, 1, 1, 1, 0, 0, 0, 1)

	require.Equal(t, "provider-actual", other["upstream_response_model_name"])
}
