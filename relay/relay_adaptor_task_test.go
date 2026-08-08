package relay

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

func TestGetTaskAdaptorSupportsXAI(t *testing.T) {
	platform := constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeXai))
	adaptor := GetTaskAdaptor(platform)
	if adaptor == nil {
		t.Fatal("GetTaskAdaptor(xAI) = nil")
	}
	if adaptor.GetChannelName() != "xai" {
		t.Fatalf("channel name = %q, want xai", adaptor.GetChannelName())
	}
}

func TestTaskModel2DtoUsesAtlasCloudProviderDisplayPlatform(t *testing.T) {
	platform := constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeAtlasCloud))
	tests := []struct {
		name      string
		modelName string
		want      string
	}{
		{name: "grok", modelName: "grok-imagine-video", want: "xAI"},
		{name: "xai upstream", modelName: "xai/grok-imagine-video/text-to-video", want: "xAI"},
		{name: "openai image", modelName: "gpt-image-1", want: "OpenAI"},
		{name: "openai upstream", modelName: "openai/gpt-image-1/text-to-image", want: "OpenAI"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			task := &model.Task{
				Platform: platform,
				Properties: model.Properties{
					OriginModelName: test.modelName,
				},
			}
			got := TaskModel2Dto(task)
			if got.DisplayPlatform != test.want {
				t.Fatalf("display platform = %q, want %q", got.DisplayPlatform, test.want)
			}
		})
	}
}
