package gemini

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/require"
)

func TestGetRequestURLOnlyTrimsReasoningSuffixesFromGeminiReasoningModels(t *testing.T) {
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
		{model: "gemini-2.5-pro-high", wantModel: "gemini-2.5-pro"},
		{model: "gemini-2.5-pro-thinking-1024", wantModel: "gemini-2.5-pro"},
		{model: "gemini-2.5-pro-thinking", wantModel: "gemini-2.5-pro"},
		{model: "gemini-2.5-pro-nothinking", wantModel: "gemini-2.5-pro"},
		{model: "gemini-3.1-flash-lite-preview-max", wantModel: "gemini-3.1-flash-lite-preview"},
		{model: "qwen3-thinking-1024", wantModel: "qwen3-thinking-1024"},
		{model: "qwen3-thinking", wantModel: "qwen3-thinking"},
		{model: "qwen3-nothinking", wantModel: "qwen3-nothinking"},
		{model: "qwen3-max", wantModel: "qwen3-max"},
		{model: "qwen-max", wantModel: "qwen-max"},
		{model: "qwen-vl-max", wantModel: "qwen-vl-max"},
		{model: "custom-vision-ultra", wantModel: "custom-vision-ultra"},
		{model: "gemini-3-pro-image-ultra", wantModel: "gemini-3-pro-image-ultra"},
	}

	for _, test := range tests {
		t.Run(test.model, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				OriginModelName: test.model,
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelBaseUrl:    "https://generativelanguage.googleapis.com",
					ApiVersion:        "v1beta",
					ApiKey:            "test-key",
					UpstreamModelName: test.model,
				},
			}

			url, err := (&Adaptor{}).GetRequestURL(info)

			require.NoError(t, err)
			require.NotEmpty(t, url)
			require.Equal(t, test.wantModel, info.UpstreamModelName)
		})
	}
}
