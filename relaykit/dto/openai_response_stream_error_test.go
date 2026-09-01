package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesStreamResponseGetOpenAIError(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantMessage string
		wantType    string
		wantCode    string
		wantParam   string
	}{
		{
			name:        "nested response error",
			body:        `{"type":"response.failed","response":{"error":{"message":"nested failure","type":"invalid_request_error","code":"nested_code"}}}`,
			wantMessage: "nested failure",
			wantType:    "invalid_request_error",
			wantCode:    "nested_code",
		},
		{
			name:        "top level error object",
			body:        `{"type":"response.error","error":{"message":"top-level failure","type":"server_error","code":"top_level_code"}}`,
			wantMessage: "top-level failure",
			wantType:    "server_error",
			wantCode:    "top_level_code",
		},
		{
			name:        "official direct error fields",
			body:        `{"type":"error","message":"direct failure","code":"direct_code","param":"input"}`,
			wantMessage: "direct failure",
			wantType:    "upstream_error",
			wantCode:    "direct_code",
			wantParam:   "input",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var event ResponsesStreamResponse
			require.NoError(t, json.Unmarshal([]byte(test.body), &event))

			openAIError := event.GetOpenAIError()
			require.NotNil(t, openAIError)
			assert.Equal(t, test.wantMessage, openAIError.Message)
			assert.Equal(t, test.wantType, openAIError.Type)
			assert.Equal(t, test.wantCode, openAIError.Code)
			assert.Equal(t, test.wantParam, openAIError.Param)
		})
	}
}
