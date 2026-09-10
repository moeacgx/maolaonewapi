package gemini

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestConvertOpenAIRequestPlaygroundImagineIncludesTextContents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/pg/chat/completions", nil)

	info := newGeminiImageRelayInfo("gemini-3.1-flash-image")
	converted, err := (&Adaptor{}).ConvertOpenAIRequest(c, info, &dto.GeneralOpenAIRequest{
		Model: "gemini-3.1-flash-image",
		Messages: []dto.Message{
			{Role: "user", Content: "生成一张猴子捕猎的照片"},
		},
	})
	require.NoError(t, err)

	encoded, err := common.Marshal(converted)
	require.NoError(t, err)
	require.Equal(t, "user", gjson.GetBytes(encoded, "contents.0.role").String())
	require.Equal(t, "生成一张猴子捕猎的照片", gjson.GetBytes(encoded, "contents.0.parts.0.text").String())
	require.Equal(t, "TEXT", gjson.GetBytes(encoded, "generationConfig.responseModalities.0").String())
	require.Equal(t, "IMAGE", gjson.GetBytes(encoded, "generationConfig.responseModalities.1").String())
	require.False(t, gjson.GetBytes(encoded, "messages").Exists())
}
