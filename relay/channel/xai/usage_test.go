package xai

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXAIHandlerMapsCacheUsageForBilling(t *testing.T) {
	tests := []struct {
		name         string
		usage        string
		cached       int
		cachePresent bool
	}{
		{
			name:         "standard cached tokens take precedence",
			usage:        `"prompt_tokens_details":{"cached_tokens":6},"prompt_cache_hit_tokens":9`,
			cached:       6,
			cachePresent: true,
		},
		{
			name:         "prompt cache hit alias is mapped",
			usage:        `"prompt_cache_hit_tokens":7`,
			cached:       7,
			cachePresent: true,
		},
		{
			name:         "missing cache fields remain compatible",
			usage:        ``,
			cached:       0,
			cachePresent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			usageFields := ""
			if tt.usage != "" {
				usageFields = "," + tt.usage
			}
			body := []byte(`{"id":"chatcmpl-test","object":"chat.completion","model":"grok-test","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12` + usageFields + `}}`)
			info := &relaycommon.RelayInfo{
				RelayFormat: types.RelayFormatOpenAI,
				ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeXai, UpstreamModelName: "grok-test"},
			}

			usage, apiErr := xAIHandler(c, info, &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(body)),
			})

			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, tt.cached, usage.PromptTokensDetails.CachedTokens)
			assert.Equal(t, tt.cachePresent, usage.PromptTokensDetails.HasCachedTokens)
		})
	}
}

func TestXAIStreamHandlerPreservesFinalUsageCacheFields(t *testing.T) {
	tests := []struct {
		name         string
		usage        string
		cached       int
		cachePresent bool
	}{
		{
			name:         "standard cached tokens take precedence",
			usage:        `"prompt_tokens_details":{"cached_tokens":4},"prompt_cache_hit_tokens":9`,
			cached:       4,
			cachePresent: true,
		},
		{
			name:         "prompt cache hit alias is mapped",
			usage:        `"prompt_cache_hit_tokens":5`,
			cached:       5,
			cachePresent: true,
		},
		{
			name:         "missing cache fields remain compatible",
			usage:        "",
			cached:       0,
			cachePresent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{
				RelayFormat:        types.RelayFormatOpenAI,
				ShouldIncludeUsage: true,
				ChannelMeta:        &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeXai, UpstreamModelName: "grok-test"},
			}
			usageFields := ""
			if tt.usage != "" {
				usageFields = "," + tt.usage
			}
			body := "data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"grok-test\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"}}]}\n\n" +
				"data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"grok-test\",\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2,\"total_tokens\":12" + usageFields + "}}\n\n" +
				"data: [DONE]\n\n"

			usage, apiErr := xAIStreamHandler(c, info, &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(body)),
			})

			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, tt.cached, usage.PromptTokensDetails.CachedTokens)
			assert.Equal(t, tt.cachePresent, usage.PromptTokensDetails.HasCachedTokens)
			assert.Equal(t, 12, usage.TotalTokens)
			if tt.cachePresent {
				assert.Contains(t, recorder.Body.String(), `"usage"`)
			}
		})
	}
}

func TestXAIStreamHandlerMergesDetailOnlyUsageChunk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		RelayFormat:        types.RelayFormatOpenAI,
		ShouldIncludeUsage: true,
		ChannelMeta:        &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeXai, UpstreamModelName: "grok-test"},
	}
	body := "data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"model\":\"grok-test\",\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":20,\"total_tokens\":120}}\n\n" +
		"data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"model\":\"grok-test\",\"choices\":[],\"usage\":{\"prompt_tokens_details\":{\"cached_tokens\":80}}}\n\n" +
		"data: [DONE]\n\n"

	usage, apiErr := xAIStreamHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 100, usage.PromptTokens)
	assert.Equal(t, 120, usage.TotalTokens)
	assert.Equal(t, 80, usage.PromptTokensDetails.CachedTokens)
}

func TestNormalizeXAIUsageBoundsCacheTokens(t *testing.T) {
	tests := []struct {
		name   string
		usage  string
		cached int
	}{
		{name: "negative cache", usage: `{"prompt_tokens":100,"prompt_tokens_details":{"cached_tokens":-1}}`, cached: 0},
		{name: "cache exceeds prompt", usage: `{"prompt_tokens":100,"prompt_tokens_details":{"cached_tokens":200}}`, cached: 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var usage dto.Usage
			require.NoError(t, common.UnmarshalJsonStr(tt.usage, &usage))
			normalizeXAIUsage(&usage)
			assert.Equal(t, tt.cached, usage.PromptTokensDetails.CachedTokens)
		})
	}
}
