package channel

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	common2 "github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type downstreamRequestIDTestAdaptor struct {
	url string
}

func (a *downstreamRequestIDTestAdaptor) Init(info *relaycommon.RelayInfo) {}
func (a *downstreamRequestIDTestAdaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return a.url, nil
}
func (a *downstreamRequestIDTestAdaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	req.Set("Authorization", "Bearer test")
	req.Set("Content-Type", "application/json")
	return nil
}
func (a *downstreamRequestIDTestAdaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return request, nil
}
func (a *downstreamRequestIDTestAdaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return request, nil
}
func (a *downstreamRequestIDTestAdaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}
func (a *downstreamRequestIDTestAdaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, nil
}
func (a *downstreamRequestIDTestAdaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return request, nil
}
func (a *downstreamRequestIDTestAdaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return request, nil
}
func (a *downstreamRequestIDTestAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return DoApiRequest(a, c, info, requestBody)
}
func (a *downstreamRequestIDTestAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	return nil, nil
}
func (a *downstreamRequestIDTestAdaptor) GetModelList() []string { return nil }
func (a *downstreamRequestIDTestAdaptor) GetChannelName() string { return "downstream-request-id-test" }
func (a *downstreamRequestIDTestAdaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return request, nil
}
func (a *downstreamRequestIDTestAdaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return request, nil
}

func TestDoApiRequestForwardsDownstreamRequestIDForOpenAICompatibleRoutes(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	testCases := []struct {
		name        string
		path        string
		relayMode   int
		relayFormat types.RelayFormat
		stream      bool
	}{
		{name: "chat completions", path: "/v1/chat/completions", relayMode: relayconstant.RelayModeChatCompletions, relayFormat: types.RelayFormatOpenAI},
		{name: "chat completions stream", path: "/v1/chat/completions", relayMode: relayconstant.RelayModeChatCompletions, relayFormat: types.RelayFormatOpenAI, stream: true},
		{name: "responses", path: "/v1/responses", relayMode: relayconstant.RelayModeResponses, relayFormat: types.RelayFormatOpenAIResponses},
		{name: "responses stream", path: "/v1/responses", relayMode: relayconstant.RelayModeResponses, relayFormat: types.RelayFormatOpenAIResponses, stream: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			gotHeader := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotHeader <- r.Header.Get(downstreamRequestIDHeader)
				if testCase.stream {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = w.Write([]byte("data: [DONE]\n\n"))
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"resp_test"}`))
			}))
			defer server.Close()

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, testCase.path, strings.NewReader(`{}`))
			ctx.Request.Header.Set("Content-Type", "application/json")
			ctx.Set(common2.RequestIdKey, "newapi-request-id-123")

			info := &relaycommon.RelayInfo{
				RelayMode:   testCase.relayMode,
				RelayFormat: testCase.relayFormat,
				IsStream:    testCase.stream,
				DisablePing: true,
				ChannelMeta: &relaycommon.ChannelMeta{},
			}

			resp, err := DoApiRequest(&downstreamRequestIDTestAdaptor{url: server.URL}, ctx, info, strings.NewReader(`{}`))
			require.NoError(t, err)
			require.NotNil(t, resp)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, "newapi-request-id-123", <-gotHeader)
		})
	}
}

func TestDoApiRequestDownstreamRequestIDUsesOnlyContextValue(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	testCases := []struct {
		name           string
		contextID      string
		headerOverride map[string]any
		wantHeader     string
	}{
		{
			name: "empty context removes client passthrough",
			headerOverride: map[string]any{
				"*": "",
			},
		},
		{
			name:      "context overrides static channel header",
			contextID: "context-request-id",
			headerOverride: map[string]any{
				"X-Downstream-Request-ID": "static-channel-value",
			},
			wantHeader: "context-request-id",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			gotHeader := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotHeader <- r.Header.Get(downstreamRequestIDHeader)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"resp_test"}`))
			}))
			defer server.Close()

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
			ctx.Request.Header.Set("Content-Type", "application/json")
			ctx.Request.Header.Set(downstreamRequestIDHeader, "client-spoofed-value")
			ctx.Request.Header.Set(common2.RequestIdKey, "client-oneapi-request-id")
			if testCase.contextID != "" {
				ctx.Set(common2.RequestIdKey, testCase.contextID)
			}

			info := &relaycommon.RelayInfo{
				RelayMode:   relayconstant.RelayModeChatCompletions,
				RelayFormat: types.RelayFormatOpenAI,
				ChannelMeta: &relaycommon.ChannelMeta{HeadersOverride: testCase.headerOverride},
			}

			resp, err := DoApiRequest(&downstreamRequestIDTestAdaptor{url: server.URL}, ctx, info, strings.NewReader(`{}`))
			require.NoError(t, err)
			require.NotNil(t, resp)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, testCase.wantHeader, <-gotHeader)
		})
	}
}
