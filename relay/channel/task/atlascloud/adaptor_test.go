package atlascloud

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBuildRequestBodyMapsImageURLAndMetadata(t *testing.T) {
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:    "kling-v2.0",
		Prompt:   "waves",
		Images:   []string{"https://example.com/image.png"},
		Duration: 5,
		Metadata: map[string]interface{}{"negative_prompt": "blur"},
	})
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "kling-v2.0"}}

	body, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	require.NoError(t, err)
	data, err := io.ReadAll(body)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(data, &payload))
	require.Equal(t, "kling-v2.0", payload["model"])
	require.Equal(t, "waves", payload["prompt"])
	require.Equal(t, "https://example.com/image.png", payload["image_url"])
	require.Equal(t, float64(5), payload["duration"])
	require.Equal(t, "blur", payload["negative_prompt"])
}

func TestBuildRequestBodyUsesMappedGrokVideoModel(t *testing.T) {
	c := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:  "grok-imagine-video-1.5",
		Prompt: "waves",
	})
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "xai/grok-imagine-video-v1.5/text-to-video"}}

	body, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	require.NoError(t, err)
	data, err := io.ReadAll(body)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(data, &payload))
	require.Equal(t, "xai/grok-imagine-video-v1.5/text-to-video", payload["model"])
}

func TestParseTaskResultCompleted(t *testing.T) {
	taskInfo, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{"data":{"id":"p1","status":"completed","outputs":["https://example.com/v.mp4"]}}`))
	require.NoError(t, err)

	require.Equal(t, model.TaskStatusSuccess, taskInfo.Status)
	require.Equal(t, "https://example.com/v.mp4", taskInfo.Url)
	require.Equal(t, "100%", taskInfo.Progress)
}

func TestParseTaskResultFailed(t *testing.T) {
	taskInfo, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{"data":{"id":"p1","status":"failed","error":"blocked"}}`))
	require.NoError(t, err)

	require.Equal(t, model.TaskStatusFailure, taskInfo.Status)
	require.Equal(t, "blocked", taskInfo.Reason)
}
