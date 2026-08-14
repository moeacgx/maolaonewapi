package helper

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelMappedHelperResponsesUsesMappedTargetWithoutCompactEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	ctx.Set("model_mapping", `{"gpt-5.5-openai-compact":"gpt-5.5"}`)
	request := &dto.OpenAIResponsesRequest{Model: "gpt-5.5-openai-compact"}
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeResponses,
		OriginModelName: "gpt-5.5-openai-compact",
	}

	err := ModelMappedHelper(ctx, info, request)

	require.NoError(t, err)
	require.Equal(t, relayconstant.RelayModeResponses, info.RelayMode)
	require.Equal(t, "gpt-5.5-openai-compact", info.OriginModelName)
	require.Equal(t, "gpt-5.5", info.UpstreamModelName)
	require.Equal(t, "gpt-5.5", request.Model)
}

func TestModelMappedHelperResponsesCompactExactAliasMappingDowngradesToResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	ctx.Set("model_mapping", `{"gpt-5.5-openai-compact":"gpt-5.5"}`)
	request := &dto.OpenAIResponsesCompactionRequest{Model: "gpt-5.5-openai-compact"}
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeResponsesCompact,
		RequestURLPath:  "/v1/responses/compact",
		OriginModelName: "gpt-5.5-openai-compact",
	}

	err := ModelMappedHelper(ctx, info, request)

	require.NoError(t, err)
	require.Equal(t, relayconstant.RelayModeResponses, info.RelayMode)
	require.Equal(t, "/v1/responses", info.RequestURLPath)
	require.Equal(t, "gpt-5.5-openai-compact", info.OriginModelName)
	require.Equal(t, "gpt-5.5", info.UpstreamModelName)
	require.Equal(t, "gpt-5.5", request.Model)
}

func TestModelMappedHelperResponsesCompactWithoutExactMappingKeepsCompactRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	request := &dto.OpenAIResponsesCompactionRequest{Model: "gpt-5.5-openai-compact"}
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeResponsesCompact,
		RequestURLPath:  "/v1/responses/compact",
		OriginModelName: "gpt-5.5-openai-compact",
	}

	err := ModelMappedHelper(ctx, info, request)

	require.NoError(t, err)
	require.Equal(t, relayconstant.RelayModeResponsesCompact, info.RelayMode)
	require.Equal(t, "/v1/responses/compact", info.RequestURLPath)
	require.Equal(t, ratio_setting.WithCompactModelSuffix("gpt-5.5"), info.OriginModelName)
	require.Equal(t, "gpt-5.5", info.UpstreamModelName)
	require.Equal(t, "gpt-5.5", request.Model)
}
