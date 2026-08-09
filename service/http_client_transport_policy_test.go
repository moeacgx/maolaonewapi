package service

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeHTTPTransportPolicyClampsRuntimeValues(t *testing.T) {
	assert.Equal(t, HTTPTransportPolicy{Protocol: dto.HTTPProtocolAuto, Shards: 1}, NormalizeHTTPTransportPolicy(dto.ChannelSettings{HTTPProtocol: "AUTO"}))
	assert.Equal(t, HTTPTransportPolicy{Protocol: dto.HTTPProtocolHTTP1, Shards: 1}, NormalizeHTTPTransportPolicy(dto.ChannelSettings{HTTPProtocol: "HTTP1", HTTP2ConnectionShards: 8}))
	assert.Equal(t, HTTPTransportPolicy{Protocol: dto.HTTPProtocolAuto, Shards: 1}, NormalizeHTTPTransportPolicy(dto.ChannelSettings{HTTPProtocol: "http3"}))
	assert.Equal(t, HTTPTransportPolicy{Protocol: dto.HTTPProtocolAuto, Shards: 1}, NormalizeHTTPTransportPolicy(dto.ChannelSettings{HTTP2ConnectionShards: -3}))
	assert.Equal(t, HTTPTransportPolicy{Protocol: dto.HTTPProtocolAuto, Shards: dto.MaxHTTP2ConnectionShards}, NormalizeHTTPTransportPolicy(dto.ChannelSettings{HTTP2ConnectionShards: 99}))
}

func TestHTTPClientWithProxySettingsCachesByCanonicalProxyAndPolicy(t *testing.T) {
	ResetProxyClientCache()
	InitHttpClient()

	defaultClient, err := GetHttpClientWithProxySettings("", dto.ChannelSettings{})
	require.NoError(t, err)
	require.Same(t, GetHttpClient(), defaultClient)

	http1Client, err := GetHttpClientWithProxySettings("", dto.ChannelSettings{HTTPProtocol: dto.HTTPProtocolHTTP1})
	require.NoError(t, err)
	require.NotSame(t, defaultClient, http1Client)
	transport, ok := http1Client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.False(t, transport.ForceAttemptHTTP2)
	assert.NotNil(t, transport.TLSNextProto)

	proxyA, err := GetHttpClientWithProxySettings("socks5://127.0.0.1", dto.ChannelSettings{})
	require.NoError(t, err)
	proxyB, err := GetHttpClientWithProxySettings("socks5://127.0.0.1:1080/path?legacy=1", dto.ChannelSettings{})
	require.NoError(t, err)
	require.Same(t, proxyA, proxyB)
}

func TestHTTP2ShardsUseShardedRoundTripper(t *testing.T) {
	client, err := GetHttpClientWithProxySettings("", dto.ChannelSettings{HTTP2ConnectionShards: 3})
	require.NoError(t, err)
	_, ok := client.Transport.(*shardedRoundTripper)
	require.True(t, ok)
}
