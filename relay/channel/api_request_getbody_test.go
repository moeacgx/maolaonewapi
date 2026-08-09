package channel

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	rootcommon "github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestApplyUpstreamBodyMetadataSetsReplayableGetBody(t *testing.T) {
	payload := []byte(`{"model":"test","messages":[{"role":"user","content":"hi"}]}`)
	body, closer, err := relaycommon.NewOutboundJSONBody(payload)
	require.NoError(t, err)
	defer closer.Close()

	req, err := http.NewRequest(http.MethodPost, "https://example.com/v1/chat/completions", body)
	require.NoError(t, err)
	require.Nil(t, req.GetBody)
	require.Zero(t, req.ContentLength)

	ApplyUpstreamBodyMetadata(req, req.Body)
	require.Nil(t, req.GetBody, "metadata must come from the original replayable body, not NewRequest's wrapper")

	ApplyUpstreamBodyMetadata(req, body)
	require.EqualValues(t, len(payload), req.ContentLength)
	require.NotNil(t, req.GetBody)

	first, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.Equal(t, payload, first)

	replay, err := req.GetBody()
	require.NoError(t, err)
	got, err := io.ReadAll(replay)
	require.NoError(t, err)
	require.NoError(t, replay.Close())
	require.Equal(t, payload, got)
}

func TestApplyUpstreamBodyMetadataKeepsNativeGetBody(t *testing.T) {
	nativeBody := bytes.NewReader([]byte("native"))
	req, err := http.NewRequest(http.MethodPost, "https://example.com", nativeBody)
	require.NoError(t, err)
	require.NotNil(t, req.GetBody)
	originalGetBody := req.GetBody

	ApplyUpstreamBodyMetadata(req, nativeBody)
	require.EqualValues(t, 6, req.ContentLength)
	require.NotNil(t, req.GetBody)

	replay, err := req.GetBody()
	require.NoError(t, err)
	got, err := io.ReadAll(replay)
	require.NoError(t, err)
	require.NoError(t, replay.Close())
	require.Equal(t, "native", string(got))

	replay, err = originalGetBody()
	require.NoError(t, err)
	require.NoError(t, replay.Close())
}

func TestApplyUpstreamBodyMetadataHidesRawBodyStorageCloser(t *testing.T) {
	payload := []byte(`{"raw":true}`)
	storage, err := rootcommon.CreateBodyStorage(payload)
	require.NoError(t, err)
	defer storage.Close()

	req, err := http.NewRequest(http.MethodPost, "https://example.com", storage)
	require.NoError(t, err)
	_, exposesStorageBeforeApply := req.Body.(rootcommon.BodyStorage)
	require.True(t, exposesStorageBeforeApply)

	ApplyUpstreamBodyMetadata(req, storage)
	_, exposesStorageAfterApply := req.Body.(rootcommon.BodyStorage)
	require.False(t, exposesStorageAfterApply)
	require.EqualValues(t, len(payload), req.ContentLength)
	require.NotNil(t, req.GetBody)
	require.NoError(t, req.Body.Close())

	replay, err := req.GetBody()
	require.NoError(t, err, "closing request body must not close shared storage")
	got, err := io.ReadAll(replay)
	require.NoError(t, err)
	require.NoError(t, replay.Close())
	require.Equal(t, payload, got)
}

func TestDoRequestKeepsUpstreamRedirectResponse(t *testing.T) {
	service.InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", io.NopCloser(bytes.NewReader(nil)))

	req, err := http.NewRequest(http.MethodPost, server.URL+"/redirect", http.NoBody)
	require.NoError(t, err)
	resp, err := doRequest(c, req, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.Equal(t, "/final", resp.Header.Get("Location"))
}
