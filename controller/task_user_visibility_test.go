package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestTasksToDtoHidesUpstreamModelForUserView(t *testing.T) {
	tasks := []*model.Task{{
		TaskID:   "task-redirect",
		Platform: constant.TaskPlatformImage,
		Properties: model.Properties{
			OriginModelName:   "gpt-5.6-sol",
			UpstreamModelName: "gpt-5.6-sol-wm",
		},
	}}

	items := tasksToDto(tasks, false)
	require.Len(t, items, 1)

	props, ok := items[0].Properties.(model.Properties)
	require.True(t, ok)
	require.Equal(t, "gpt-5.6-sol", props.OriginModelName)
	require.Empty(t, props.UpstreamModelName)
}
