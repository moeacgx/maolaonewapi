package vertex

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRequestURLDynamicRouteTrimsGeminiThinkingSuffix(t *testing.T) {
	settings := model_setting.GetGeminiSettings()
	originalEnabled := settings.ThinkingAdapterEnabled
	settings.ThinkingAdapterEnabled = false
	t.Cleanup(func() {
		settings.ThinkingAdapterEnabled = originalEnabled
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3.7-flash",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://aiplatform.googleapis.com",
			ApiVersion:        "global",
			ApiKey:            "test-key",
			UpstreamModelName: "gemini-3.7-flash-low",
			ChannelOtherSettings: dto.ChannelOtherSettings{
				VertexKeyType: dto.VertexKeyTypeAPIKey,
			},
			IsDynamicModelRouted: true,
		},
	}

	url, err := (&Adaptor{RequestMode: RequestModeGemini}).GetRequestURL(info)

	require.NoError(t, err)
	assert.Contains(t, url, "gemini-3.7-flash")
	assert.NotContains(t, url, "gemini-3.7-flash-low")
	assert.Equal(t, "gemini-3.7-flash", info.UpstreamModelName)
}
