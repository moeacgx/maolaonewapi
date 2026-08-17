package common

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJsonRawMessageToString(t *testing.T) {
	tests := []struct {
		name string
		data json.RawMessage
		want string
	}{
		{
			name: "object",
			data: json.RawMessage(`{"city":"Paris","days":0,"strict":false}`),
			want: `{"city":"Paris","days":0,"strict":false}`,
		},
		{
			name: "string",
			data: json.RawMessage(`"{\"city\":\"Paris\",\"days\":0,\"strict\":false}"`),
			want: `{"city":"Paris","days":0,"strict":false}`,
		},
		{
			name: "null",
			data: json.RawMessage(`null`),
			want: "",
		},
		{
			name: "empty",
			data: nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, JsonRawMessageToString(tt.data))
		})
	}
}

func TestWriteJsonStringBytesPreservesContentWithoutHTMLEscaping(t *testing.T) {
	body := []byte("<tag>&\"\\\n\t中文")
	var encoded strings.Builder
	require.NoError(t, WriteJsonStringBytes(&encoded, body))
	require.NotContains(t, encoded.String(), `\u003c`)
	require.NotContains(t, encoded.String(), `\u0026`)
	var decoded string
	require.NoError(t, Unmarshal([]byte(encoded.String()), &decoded))
	require.Equal(t, string(body), decoded)
}

func TestWriteJsonStringBytesRejectsInvalidUTF8(t *testing.T) {
	var encoded strings.Builder
	require.Error(t, WriteJsonStringBytes(&encoded, []byte{0xff}))
	require.Empty(t, encoded.String())
}
