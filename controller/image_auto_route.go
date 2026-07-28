package controller

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// autoRouteImageRequest 将误用 chat/responses 端点的图片模型请求改写为
// OpenAI 图片生成请求。改写发生在网关内部，不会向客户端返回重定向。
func autoRouteImageRequest(c *gin.Context, relayFormat types.RelayFormat) (types.RelayFormat, error) {
	if c == nil || c.Request == nil || (relayFormat != types.RelayFormatOpenAI && relayFormat != types.RelayFormatOpenAIResponses) {
		return relayFormat, nil
	}
	if !strings.HasPrefix(c.Request.Header.Get("Content-Type"), "application/json") {
		return relayFormat, nil
	}

	var raw map[string]json.RawMessage
	if err := common.UnmarshalBodyReusable(c, &raw); err != nil {
		// 让原有请求校验返回与未改写请求相同的错误。
		return relayFormat, nil
	}
	modelName := rawString(raw["model"])
	if modelName == "" {
		modelName = common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
	}
	if !isImageAutoRouteModel(modelName) {
		return relayFormat, nil
	}

	imagePath := imageGenerationPath(c.Request.URL.Path)
	if imagePath == "" {
		return relayFormat, nil
	}
	if prompt := imagePromptFromRequest(raw); prompt != "" {
		raw["prompt"], _ = common.Marshal(prompt)
		if err := replaceRequestBody(c, raw); err != nil {
			return relayFormat, err
		}
	}

	c.Request.URL.Path = imagePath
	c.Request.URL.RawPath = ""
	return types.RelayFormatOpenAIImage, nil
}

func isImageAutoRouteModel(modelName string) bool {
	if common.IsImageGenerationModel(modelName) {
		return true
	}
	for _, endpointType := range model.GetModelSupportEndpointTypes(modelName) {
		if endpointType == constant.EndpointTypeImageGeneration {
			return true
		}
	}
	return false
}

func imageGenerationPath(path string) string {
	switch {
	case strings.HasPrefix(path, "/v1/chat/completions"):
		return "/v1/images/generations"
	case strings.HasPrefix(path, "/v1/responses") && !strings.HasPrefix(path, "/v1/responses/compact"):
		return "/v1/images/generations"
	case strings.HasPrefix(path, "/canvas/v1/chat/completions"):
		return "/canvas/v1/images/generations"
	default:
		return ""
	}
}

func replaceRequestBody(c *gin.Context, raw map[string]json.RawMessage) error {
	body, err := common.Marshal(raw)
	if err != nil {
		return err
	}
	storage, err := common.CreateBodyStorage(body)
	if err != nil {
		return err
	}
	if previous, exists := c.Get(common.KeyBodyStorage); exists {
		if previousStorage, ok := previous.(common.BodyStorage); ok {
			_ = previousStorage.Close()
		}
	}
	c.Set(common.KeyBodyStorage, storage)
	c.Request.Body = io.NopCloser(storage)
	c.Request.ContentLength = int64(len(body))
	return nil
}

func rawString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value string
	if common.Unmarshal(raw, &value) == nil {
		return strings.TrimSpace(value)
	}
	return ""
}

func imagePromptFromRequest(raw map[string]json.RawMessage) string {
	if prompt := imagePromptValue(raw["prompt"]); prompt != "" {
		return prompt
	}
	for _, key := range []string{"input", "messages", "instructions"} {
		if prompt := imagePromptValue(raw[key]); prompt != "" {
			return prompt
		}
	}
	return ""
}

func imagePromptValue(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value any
	if err := common.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return strings.TrimSpace(imagePromptValueAny(value))
}

func imagePromptValueAny(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(imagePromptValueAny(item)); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		for _, key := range []string{"text", "content", "input_text", "value", "parts"} {
			if item, ok := typed[key]; ok {
				if text := strings.TrimSpace(imagePromptValueAny(item)); text != "" {
					return text
				}
			}
		}
	}
	return ""
}
