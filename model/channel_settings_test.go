package model

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelValidateSettingsRejectsInvalidProxyAndHTTPTransport(t *testing.T) {
	tests := []struct {
		name    string
		setting dto.ChannelSettings
		wantErr string
	}{
		{
			name:    "valid normalized transport",
			setting: dto.ChannelSettings{Proxy: "socks5://127.0.0.1", HTTPProtocol: dto.HTTPProtocolAuto, HTTP2ConnectionShards: 4},
		},
		{
			name:    "proxy path rejected at save time",
			setting: dto.ChannelSettings{Proxy: "http://127.0.0.1/proxy"},
			wantErr: "invalid channel proxy",
		},
		{
			name:    "http1 with shards rejected",
			setting: dto.ChannelSettings{HTTPProtocol: dto.HTTPProtocolHTTP1, HTTP2ConnectionShards: 2},
			wantErr: "http2_connection_shards",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &Channel{}
			channel.SetSetting(tt.setting)
			err := channel.ValidateSettings()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
