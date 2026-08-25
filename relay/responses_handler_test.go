package relay

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPinResponsesLiteParallelToolCallsJSON(t *testing.T) {
	tests := []struct {
		name string
		body []byte
	}{
		{
			name: "missing",
			body: []byte(`{"model":"gpt-5-codex","input":"hi"}`),
		},
		{
			name: "true",
			body: []byte(`{"model":"gpt-5-codex","input":"hi","parallel_tool_calls":true}`),
		},
		{
			name: "false",
			body: []byte(`{"model":"gpt-5-codex","input":"hi","parallel_tool_calls":false}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := pinResponsesLiteParallelToolCallsJSON(tt.body)
			require.NoError(t, err)

			value := gjson.GetBytes(out, "parallel_tool_calls")
			require.True(t, value.Exists())
			assert.False(t, value.Bool())
		})
	}
}
