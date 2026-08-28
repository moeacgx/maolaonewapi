package channel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRelayRequestPathsCancelUpstreamHTTPTransport(t *testing.T) {
	service.InitHttpClient()

	tests := []struct {
		name   string
		invoke func(*gin.Context, *relaycommon.RelayInfo, string) (*http.Response, error)
	}{
		{
			name: "api",
			invoke: func(c *gin.Context, info *relaycommon.RelayInfo, target string) (*http.Response, error) {
				return DoApiRequest(&downstreamRequestIDTestAdaptor{url: target}, c, info, strings.NewReader(`{}`))
			},
		},
		{
			name: "form",
			invoke: func(c *gin.Context, info *relaycommon.RelayInfo, target string) (*http.Response, error) {
				return DoFormRequest(&downstreamRequestIDTestAdaptor{url: target}, c, info, strings.NewReader("field=value"))
			},
		},
		{
			name: "task",
			invoke: func(c *gin.Context, info *relaycommon.RelayInfo, target string) (*http.Response, error) {
				return DoTaskApiRequest(&stubTaskAdaptor{baseURL: target}, c, info, strings.NewReader(`{}`))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			upstreamStarted := make(chan struct{})
			upstreamCanceled := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
				close(upstreamStarted)
				<-r.Context().Done()
				close(upstreamCanceled)
			}))
			defer server.Close()

			requestContext, cancel := context.WithCancel(context.Background())
			defer cancel()
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`)).WithContext(requestContext)
			info := &relaycommon.RelayInfo{
				RelayMode:   relayconstant.RelayModeChatCompletions,
				RelayFormat: types.RelayFormatOpenAI,
				DisablePing: true,
				ChannelMeta: &relaycommon.ChannelMeta{},
			}

			type requestResult struct {
				resp *http.Response
				err  error
			}
			result := make(chan requestResult, 1)
			go func() {
				resp, err := test.invoke(c, info, server.URL)
				result <- requestResult{resp: resp, err: err}
			}()

			select {
			case <-upstreamStarted:
			case <-time.After(2 * time.Second):
				t.Fatal("upstream request did not start")
			}

			var outcome requestResult
			select {
			case outcome = <-result:
				require.NoError(t, outcome.err)
				require.NotNil(t, outcome.resp)
			case <-time.After(2 * time.Second):
				t.Fatal("relay request did not receive upstream headers")
			}
			cancel()

			select {
			case <-upstreamCanceled:
			case <-time.After(2 * time.Second):
				t.Fatal("upstream request context was not canceled")
			}

			require.NoError(t, outcome.resp.Body.Close())
		})
	}
}
