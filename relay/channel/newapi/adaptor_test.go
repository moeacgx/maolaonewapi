package newapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdaptorAlphaSearchUsesNewAPIEndpointAndBearerAuth(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:      relayconstant.RelayModeAlphaSearch,
		RequestURLPath: "/backend-api/codex/alpha/search",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeNewAPI,
			ChannelBaseUrl: "https://newapi.example/base",
			ApiKey:         "newapi-key",
		},
	}
	adaptor := &Adaptor{}

	requestURL, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://newapi.example/base/v1/alpha/search", requestURL)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/alpha/search", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	headers := http.Header{}
	require.NoError(t, adaptor.SetupRequestHeader(c, &headers, info))
	assert.Equal(t, "Bearer newapi-key", headers.Get("Authorization"))
	assert.Equal(t, "application/json", headers.Get("Content-Type"))
}

func TestAdaptorPreservesResponsesRequestForNewAPIPassthrough(t *testing.T) {
	request := dto.OpenAIResponsesRequest{
		Model:          "gpt-5.1",
		Input:          []byte(`"hello"`),
		ClientMetadata: []byte(`{"thread_id":"thread-1"}`),
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, &relaycommon.RelayInfo{}, request)

	require.NoError(t, err)
	assert.Equal(t, request, converted)
}

func TestAdaptorRoutesOrdinaryRequestPathUnchanged(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayFormat:    types.RelayFormatOpenAIResponses,
		RequestURLPath: "/v1/responses",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeNewAPI,
			ChannelBaseUrl: "https://newapi.example",
		},
	}

	requestURL, err := (&Adaptor{}).GetRequestURL(info)

	require.NoError(t, err)
	assert.Equal(t, "https://newapi.example/v1/responses", requestURL)
}
