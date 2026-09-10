package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
)

func TestShouldConvertChatRequestForcesGeminiThroughPassThrough(t *testing.T) {
	tests := []struct {
		name string
		info *relaycommon.RelayInfo
		want bool
	}{
		{
			name: "gemini chat still converts when channel pass-through is on",
			info: &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ApiType:           constant.APITypeGemini,
					UpstreamModelName: "gemini-3.1-flash-image",
					ChannelSetting:    dto.ChannelSettings{PassThroughBodyEnabled: true},
				},
			},
			want: true,
		},
		{
			name: "gemini text model still converts when channel pass-through is on",
			info: &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ApiType:           constant.APITypeGemini,
					UpstreamModelName: "gemini-2.5-pro",
					ChannelSetting:    dto.ChannelSettings{PassThroughBodyEnabled: true},
				},
			},
			want: true,
		},
		{
			name: "vertex gemini still converts when channel pass-through is on",
			info: &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ApiType:           constant.APITypeVertexAi,
					UpstreamModelName: "gemini-3.1-flash-image",
					ChannelSetting:    dto.ChannelSettings{PassThroughBodyEnabled: true},
				},
			},
			want: true,
		},
		{
			name: "vertex llama keeps pass-through",
			info: &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ApiType:           constant.APITypeVertexAi,
					UpstreamModelName: "llama-3-70b-maas",
					ChannelSetting:    dto.ChannelSettings{PassThroughBodyEnabled: true},
				},
			},
			want: false,
		},
		{
			name: "openai-compatible chat channel keeps pass-through",
			info: &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ApiType:           constant.APITypeOpenAI,
					UpstreamModelName: "gpt-4o",
					ChannelSetting:    dto.ChannelSettings{PassThroughBodyEnabled: true},
				},
			},
			want: false,
		},
		{
			name: "gemini chat converts without pass-through",
			info: &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ApiType:           constant.APITypeGemini,
					UpstreamModelName: "gemini-3.1-flash-image",
				},
			},
			want: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, shouldConvertChatRequest(test.info))
		})
	}
}
