package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestGetEndpointTypesByChannelTypeAtlasCloudImageOnly(t *testing.T) {
	for _, modelName := range []string{"seedream-3.0", "grok-imagine-image", "gpt-image-1.5", "gpt-image-2"} {
		t.Run(modelName, func(t *testing.T) {
			got := GetEndpointTypesByChannelType(constant.ChannelTypeAtlasCloud, modelName)
			require.Equal(t, []constant.EndpointType{constant.EndpointTypeImageGeneration}, got)
		})
	}
}

func TestGetEndpointTypesByChannelTypeAtlasCloudVideoOnly(t *testing.T) {
	got := GetEndpointTypesByChannelType(constant.ChannelTypeAtlasCloud, "grok-imagine-video-1.5")
	require.Equal(t, []constant.EndpointType{constant.EndpointTypeOpenAIVideo}, got)
}

func TestGetEndpointTypesByChannelTypeXaiVideoPrependsOpenAIVideo(t *testing.T) {
	got := GetEndpointTypesByChannelType(constant.ChannelTypeXai, "grok-imagine-video")
	require.Equal(t, []constant.EndpointType{constant.EndpointTypeOpenAIVideo, constant.EndpointTypeOpenAI, constant.EndpointTypeOpenAIResponse}, got)
}
