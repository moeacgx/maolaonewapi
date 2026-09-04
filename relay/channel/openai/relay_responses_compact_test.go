package openai

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOaiResponsesCompactionHandlerStoresUpstreamResponseModelName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatOpenAIResponsesCompaction,
		OriginModelName: "requested-model",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "mapped-model",
		},
	}
	body := []byte(`{"id":"resp_compact","object":"response.compaction","model":"provider-actual","usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)

	_, apiErr := OaiResponsesCompactionHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
	})

	require.Nil(t, apiErr)
	require.Equal(t, "provider-actual", info.UpstreamResponseModelName)
}
