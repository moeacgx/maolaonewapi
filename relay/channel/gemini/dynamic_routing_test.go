package gemini

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRequestURLDynamicRoutePreservesGeminiThinkingSuffix(t *testing.T) {
	settings := model_setting.GetGeminiSettings()
	originalEnabled := settings.ThinkingAdapterEnabled
	settings.ThinkingAdapterEnabled = true
	t.Cleanup(func() {
		settings.ThinkingAdapterEnabled = originalEnabled
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "public-gemini-model",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:       "https://generativelanguage.googleapis.com",
			ApiVersion:           "v1beta",
			ApiKey:               "test-key",
			UpstreamModelName:    "gemini-2.5-pro-high",
			IsDynamicModelRouted: true,
		},
	}

	url, err := (&Adaptor{}).GetRequestURL(info)

	require.NoError(t, err)
	assert.Contains(t, url, "/models/gemini-2.5-pro-high:")
	assert.Equal(t, "gemini-2.5-pro-high", info.UpstreamModelName)
}
