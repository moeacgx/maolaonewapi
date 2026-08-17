package vertex

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/require"
)

func TestGeminiGetRequestURLOnlyTrimsReasoningSuffixesFromGeminiReasoningModels(t *testing.T) {
	settings := model_setting.GetGeminiSettings()
	originalEnabled := settings.ThinkingAdapterEnabled
	settings.ThinkingAdapterEnabled = true
	t.Cleanup(func() {
		settings.ThinkingAdapterEnabled = originalEnabled
	})

	tests := []struct {
		model     string
		wantModel string
	}{
		{model: "gemini-2.5-flash-low", wantModel: "gemini-2.5-flash"},
		{model: "gemini-2.5-flash-thinking-1024", wantModel: "gemini-2.5-flash"},
		{model: "gemini-2.5-flash-thinking", wantModel: "gemini-2.5-flash"},
		{model: "gemini-2.5-flash-nothinking", wantModel: "gemini-2.5-flash"},
		{model: "gemini-3-flash-preview-ultra", wantModel: "gemini-3-flash-preview"},
		{model: "qwen3-thinking-1024", wantModel: "qwen3-thinking-1024"},
		{model: "qwen3-thinking", wantModel: "qwen3-thinking"},
		{model: "qwen3-nothinking", wantModel: "qwen3-nothinking"},
		{model: "qwen3-max", wantModel: "qwen3-max"},
		{model: "qwen-max", wantModel: "qwen-max"},
		{model: "qwen-vl-max", wantModel: "qwen-vl-max"},
		{model: "custom-vision-ultra", wantModel: "custom-vision-ultra"},
		{model: "gemini-3.1-flash-image-ultra", wantModel: "gemini-3.1-flash-image-ultra"},
	}

	for _, test := range tests {
		t.Run(test.model, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				OriginModelName: test.model,
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelBaseUrl:    "https://aiplatform.googleapis.com",
					ApiVersion:        "global",
					ApiKey:            "test-key",
					UpstreamModelName: test.model,
					ChannelOtherSettings: dto.ChannelOtherSettings{
						VertexKeyType: dto.VertexKeyTypeAPIKey,
					},
				},
			}
			adaptor := &Adaptor{RequestMode: RequestModeGemini}

			url, err := adaptor.GetRequestURL(info)

			require.NoError(t, err)
			require.NotEmpty(t, url)
			require.Equal(t, test.wantModel, info.UpstreamModelName)
		})
	}
}
