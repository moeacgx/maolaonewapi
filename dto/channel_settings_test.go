package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelSettingsValidateHTTPTransport(t *testing.T) {
	require.NoError(t, (&ChannelSettings{}).ValidateHTTPTransport())
	require.NoError(t, (&ChannelSettings{HTTPProtocol: "AUTO"}).ValidateHTTPTransport())
	require.NoError(t, (&ChannelSettings{HTTPProtocol: HTTPProtocolHTTP1}).ValidateHTTPTransport())
	require.NoError(t, (&ChannelSettings{HTTP2ConnectionShards: MaxHTTP2ConnectionShards}).ValidateHTTPTransport())

	tests := []struct {
		name    string
		setting ChannelSettings
		wantErr string
	}{
		{
			name:    "invalid protocol",
			setting: ChannelSettings{HTTPProtocol: "http3"},
			wantErr: "http_protocol",
		},
		{
			name:    "negative shards",
			setting: ChannelSettings{HTTP2ConnectionShards: -1},
			wantErr: "http2_connection_shards",
		},
		{
			name:    "too many shards",
			setting: ChannelSettings{HTTP2ConnectionShards: MaxHTTP2ConnectionShards + 1},
			wantErr: "http2_connection_shards",
		},
		{
			name:    "http1 cannot shard",
			setting: ChannelSettings{HTTPProtocol: HTTPProtocolHTTP1, HTTP2ConnectionShards: 2},
			wantErr: "http2_connection_shards",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.setting.ValidateHTTPTransport()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
