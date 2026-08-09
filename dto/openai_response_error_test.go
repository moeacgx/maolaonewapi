package dto

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestResponsesStreamResponseGetOpenAIErrorSupportsOfficialShapes(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{
			name:    "response failed without error type",
			payload: `{"type":"response.failed","response":{"error":{"code":"server_error","message":"Selected model is at capacity. Please try a different model."}}}`,
		},
		{
			name:    "top level error fields",
			payload: `{"type":"error","code":"server_error","message":"Selected model is at capacity. Please try a different model."}`,
		},
		{
			name:    "top level nested error",
			payload: `{"type":"error","error":{"code":"server_error","message":"Selected model is at capacity. Please try a different model."}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var event ResponsesStreamResponse
			require.NoError(t, common.Unmarshal([]byte(tt.payload), &event))

			openAIError := event.GetOpenAIError()
			require.NotNil(t, openAIError)
			require.Equal(t, "upstream_error", openAIError.Type)
			require.Equal(t, "server_error", openAIError.Code)
			require.Contains(t, openAIError.Message, "Selected model is at capacity")
		})
	}
}

func TestStreamResponseGetOpenAIErrorIgnoresNormalEvents(t *testing.T) {
	responsesEvent := &ResponsesStreamResponse{Type: "response.created"}
	require.Nil(t, responsesEvent.GetOpenAIError())

	chatEvent := &ChatCompletionsStreamResponse{}
	require.Nil(t, chatEvent.GetOpenAIError())
}
