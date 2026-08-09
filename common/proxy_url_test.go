package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseProxyURLRuntimeNormalizesLegacySuffix(t *testing.T) {
	parsedURL, stripped, err := ParseProxyURLRuntime(" SOCKS5://user:pass@example.com/path?x=1#frag ")
	require.NoError(t, err)
	require.NotNil(t, parsedURL)
	assert.True(t, stripped)
	assert.Equal(t, "socks5://user:pass@example.com:1080", parsedURL.String())
}

func TestParseProxyURLStrictRejectsUnsafeOrAmbiguousValues(t *testing.T) {
	for _, rawProxyURL := range []string{
		"http://127.0.0.1:70000",
		"http://127.0.0.1/path",
		"http://127.0.0.1?x=1",
		"http://127.0.0.1#frag",
		"file://127.0.0.1:8080",
		"http://",
	} {
		t.Run(rawProxyURL, func(t *testing.T) {
			_, err := ParseProxyURLStrict(rawProxyURL)
			require.Error(t, err)
		})
	}
}
