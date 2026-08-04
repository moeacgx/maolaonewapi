package dto

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGeneralOpenAIRequestPreserveExplicitZeroValues(t *testing.T) {
	raw := []byte(`{
		"model":"gpt-4.1",
		"stream":false,
		"max_tokens":0,
		"max_completion_tokens":0,
		"top_p":0,
		"top_k":0,
		"n":0,
		"frequency_penalty":0,
		"presence_penalty":0,
		"seed":0,
		"logprobs":false,
		"top_logprobs":0,
		"dimensions":0,
		"return_images":false,
		"return_related_questions":false
	}`)

	var req GeneralOpenAIRequest
	err := common.Unmarshal(raw, &req)
	require.NoError(t, err)

	encoded, err := common.Marshal(req)
	require.NoError(t, err)

	require.True(t, gjson.GetBytes(encoded, "stream").Exists())
	require.True(t, gjson.GetBytes(encoded, "max_tokens").Exists())
	require.True(t, gjson.GetBytes(encoded, "max_completion_tokens").Exists())
	require.True(t, gjson.GetBytes(encoded, "top_p").Exists())
	require.True(t, gjson.GetBytes(encoded, "top_k").Exists())
	require.True(t, gjson.GetBytes(encoded, "n").Exists())
	require.True(t, gjson.GetBytes(encoded, "frequency_penalty").Exists())
	require.True(t, gjson.GetBytes(encoded, "presence_penalty").Exists())
	require.True(t, gjson.GetBytes(encoded, "seed").Exists())
	require.True(t, gjson.GetBytes(encoded, "logprobs").Exists())
	require.True(t, gjson.GetBytes(encoded, "top_logprobs").Exists())
	require.True(t, gjson.GetBytes(encoded, "dimensions").Exists())
	require.True(t, gjson.GetBytes(encoded, "return_images").Exists())
	require.True(t, gjson.GetBytes(encoded, "return_related_questions").Exists())
}

func TestGeneralOpenAIRequestPreservesQwenThinkingBudget(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{model: "qwen-plus", want: true},
		{model: "provider/QwQ-32B", want: true},
		{model: "gpt-4.1", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			req := GeneralOpenAIRequest{Model: tt.model, ThinkingBudget: json.RawMessage(`0`)}
			encoded, err := common.Marshal(req)
			require.NoError(t, err)
			value := gjson.GetBytes(encoded, "thinking_budget")
			require.Equal(t, tt.want, value.Exists())
			if tt.want {
				require.Equal(t, int64(0), value.Int())
			}
		})
	}
}

func TestOpenAIResponsesRequestPreserveExplicitZeroValues(t *testing.T) {
	raw := []byte(`{
		"model":"gpt-4.1",
		"max_output_tokens":0,
		"max_tool_calls":0,
		"stream":false,
		"top_p":0,
		"client_metadata":{"thread_id":"thread-123","turn_id":"turn-123"}
	}`)

	var req OpenAIResponsesRequest
	err := common.Unmarshal(raw, &req)
	require.NoError(t, err)

	encoded, err := common.Marshal(req)
	require.NoError(t, err)

	require.True(t, gjson.GetBytes(encoded, "max_output_tokens").Exists())
	require.True(t, gjson.GetBytes(encoded, "max_tool_calls").Exists())
	require.True(t, gjson.GetBytes(encoded, "stream").Exists())
	require.True(t, gjson.GetBytes(encoded, "top_p").Exists())
	require.Equal(t, "thread-123", gjson.GetBytes(encoded, "client_metadata.thread_id").String())
	require.Equal(t, "turn-123", gjson.GetBytes(encoded, "client_metadata.turn_id").String())
}

func TestOpenAIResponsesRequestPreservesQwenThinkingBudget(t *testing.T) {
	for _, tt := range []struct {
		model string
		want  bool
	}{
		{model: "Qwen/Qwen3-235B-A22B", want: true},
		{model: "deepseek-r1", want: false},
	} {
		t.Run(tt.model, func(t *testing.T) {
			req := OpenAIResponsesRequest{Model: tt.model, ThinkingBudget: json.RawMessage(`128`)}
			encoded, err := common.Marshal(req)
			require.NoError(t, err)
			require.Equal(t, tt.want, gjson.GetBytes(encoded, "thinking_budget").Exists())
		})
	}
}
