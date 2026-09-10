package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
)

func TestShouldConvertImageRequestForcesGeminiNativeImagineThroughPassThrough(t *testing.T) {
	tests := []struct {
		name string
		info *relaycommon.RelayInfo
		want bool
	}{
		{
			name: "gemini imagine still converts when channel pass-through is on",
			info: &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ApiType:           constant.APITypeGemini,
					UpstreamModelName: "gemini-3-pro-image-preview",
					ChannelSetting:    dto.ChannelSettings{PassThroughBodyEnabled: true},
				},
			},
			want: true,
		},
		{
			name: "vertex imagine still converts when channel pass-through is on",
			info: &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ApiType:           constant.APITypeVertexAi,
					UpstreamModelName: "gemini-2.5-flash-image",
					ChannelSetting:    dto.ChannelSettings{PassThroughBodyEnabled: true},
				},
			},
			want: true,
		},
		{
			name: "openai-compatible image channel keeps pass-through",
			info: &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ApiType:           constant.APITypeOpenAI,
					UpstreamModelName: "gpt-image-2.5-high",
					ChannelSetting:    dto.ChannelSettings{PassThroughBodyEnabled: true},
				},
			},
			want: false,
		},
		{
			name: "gemini imagine converts without pass-through",
			info: &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ApiType:           constant.APITypeGemini,
					UpstreamModelName: "gemini-3-pro-image-preview",
				},
			},
			want: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, shouldConvertImageRequest(test.info))
		})
	}
}
