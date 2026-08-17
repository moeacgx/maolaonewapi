package channel

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoRequestReturnsUpstreamRedirectWithoutFollowing(t *testing.T) {
	service.InitHttpClient()
	gin.SetMode(gin.TestMode)
	sharedClient := service.GetHttpClient()
	require.NotNil(t, sharedClient)
	require.NotNil(t, sharedClient.CheckRedirect)
	originalRedirectPolicy := reflect.ValueOf(sharedClient.CheckRedirect).Pointer()

	var targetRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetRequests.Add(1)
		w.WriteHeader(http.StatusTeapot)
	}))
	defer target.Close()

	const responseBody = "redirect response"
	tests := []int{
		http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusSeeOther,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect,
	}

	for _, statusCode := range tests {
		t.Run(http.StatusText(statusCode), func(t *testing.T) {
			targetRequests.Store(0)
			var sourceRequests atomic.Int32
			type sourceResult struct {
				body []byte
				err  error
			}
			sourceResultCh := make(chan sourceResult, 1)
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sourceRequests.Add(1)
				body, err := io.ReadAll(r.Body)
				sourceResultCh <- sourceResult{body: body, err: err}
				w.Header().Set("Location", target.URL+"/redirect-target")
				w.WriteHeader(statusCode)
				_, _ = io.WriteString(w, responseBody)
			}))
			defer source.Close()

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/relay", nil)

			req, err := http.NewRequest(http.MethodPost, source.URL, bytes.NewReader([]byte("request body")))
			require.NoError(t, err)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

			resp, err := doRequest(ctx, req, info)
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			gotSource := <-sourceResultCh
			require.NoError(t, gotSource.err)

			assert.Equal(t, statusCode, resp.StatusCode)
			assert.Equal(t, target.URL+"/redirect-target", resp.Header.Get("Location"))
			assert.Equal(t, responseBody, string(body))
			assert.Equal(t, []byte("request body"), gotSource.body)
			assert.EqualValues(t, 1, sourceRequests.Load())
			assert.Zero(t, targetRequests.Load())
		})
	}

	assert.Equal(t, originalRedirectPolicy, reflect.ValueOf(sharedClient.CheckRedirect).Pointer(), "the cached client must not be mutated")
}

func TestRedirectBoundaryKeepsLocationInternalAndSanitizesClientError(t *testing.T) {
	service.InitHttpClient()
	gin.SetMode(gin.TestMode)

	var targetRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetRequests.Add(1)
		w.WriteHeader(http.StatusTeapot)
	}))
	defer target.Close()

	internalLocation := target.URL + "/internal/control?credential=upstream-secret"
	redirectBody := `{"error":{"message":"Location=/internal/control; group=private; key=raw-routing-key"}}`
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", internalLocation)
		w.Header().Set("X-Internal-Route", "group=private,key=raw-routing-key")
		w.WriteHeader(http.StatusTemporaryRedirect)
		_, _ = io.WriteString(w, redirectBody)
	}))
	defer source.Close()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req, err := http.NewRequest(http.MethodPost, source.URL, bytes.NewReader([]byte("request body")))
	require.NoError(t, err)

	resp, err := doRequest(ctx, req, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}})
	require.NoError(t, err)
	require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
	require.Equal(t, internalLocation, resp.Header.Get("Location"), "the relay keeps the upstream redirect available for internal error handling")
	require.Equal(t, "group=private,key=raw-routing-key", resp.Header.Get("X-Internal-Route"))
	require.Zero(t, targetRequests.Load(), "the relay must not follow the upstream redirect")

	relayErr := service.RelayErrorHandler(ctx.Request.Context(), resp, false)
	require.NotNil(t, relayErr)
	require.Equal(t, http.StatusTemporaryRedirect, relayErr.StatusCode)
	ctx.JSON(relayErr.StatusCode, gin.H{"error": relayErr.ToOpenAIError()})
	require.Equal(t, http.StatusTemporaryRedirect, recorder.Code)
	require.Empty(t, recorder.Header().Get("Location"))

	clientBody := recorder.Body.String()
	require.NotContains(t, clientBody, internalLocation)
	require.NotContains(t, clientBody, target.URL)
	require.NotContains(t, clientBody, "upstream-secret")
	require.NotContains(t, clientBody, "raw-routing-key")
	require.NotContains(t, clientBody, "internal redirect policy details")
	require.NotContains(t, clientBody, "X-Internal-Route")
}
