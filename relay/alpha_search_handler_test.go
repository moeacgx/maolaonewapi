package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestBuildAlphaSearchRequestBodyPreservesUnknownFields(t *testing.T) {
	raw := []byte(`{"model":"gpt-5-codex","query":"hello","extra":{"keep":true}}`)

	got, err := buildAlphaSearchRequestBody(raw, "gpt-5-codex", "gpt-5-codex")
	require.NoError(t, err)
	require.Equal(t, raw, got)
}

func TestBuildAlphaSearchRequestBodyRewritesMappedModelOnly(t *testing.T) {
	raw := []byte(`{"model":"gpt-5-codex","query":"hello","extra":{"keep":true}}`)

	got, err := buildAlphaSearchRequestBody(raw, "gpt-5-codex", "gpt-5-upstream")
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"gpt-5-upstream","query":"hello","extra":{"keep":true}}`, string(got))
}

func TestBuildAlphaSearchRequestBodyRejectsEmptyBody(t *testing.T) {
	_, err := buildAlphaSearchRequestBody(nil, "gpt-5-codex", "gpt-5-codex")
	require.ErrorContains(t, err, "empty alpha search request body")
}

func TestBuildAlphaSearchRequestBodyRejectsInvalidMappedJSON(t *testing.T) {
	_, err := buildAlphaSearchRequestBody([]byte(`{`), "gpt-5-codex", "gpt-5-upstream")
	require.Error(t, err)
	require.False(t, common.IsRequestBodyTooLargeError(err))
}
