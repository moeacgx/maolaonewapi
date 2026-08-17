package middleware

import (
	"encoding/json"
	"io"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/gin-gonic/gin"
)

func KlingRequestConvert() func(c *gin.Context) {
	return func(c *gin.Context) {
		var originalReq map[string]interface{}
		if err := common.UnmarshalBodyReusable(c, &originalReq); err != nil {
			c.Next()
			return
		}

		// Support both model_name and model fields
		model, _ := originalReq["model_name"].(string)
		if model == "" {
			model, _ = originalReq["model"].(string)
		}
		prompt, _ := originalReq["prompt"].(string)

		unifiedReq := map[string]interface{}{
			"model":    model,
			"prompt":   prompt,
			"metadata": originalReq,
		}

		jsonData, err := json.Marshal(unifiedReq)
		if err != nil {
			c.Next()
			return
		}
		if err = setConvertedRequestBody(c, jsonData); err != nil {
			c.Next()
			return
		}

		// Rewrite request path after the converted body is installed.
		c.Request.URL.Path = "/v1/video/generations"
		if image, ok := originalReq["image"]; !ok || image == "" {
			c.Set("action", constant.TaskActionTextGenerate)
		}

		// The converted body storage is already reusable by subsequent handlers.
		c.Next()
	}
}

func setConvertedRequestBody(c *gin.Context, body []byte) error {
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
	c.Set(common.KeyRequestBody, body)
	c.Request.Body = io.NopCloser(storage)
	c.Request.ContentLength = int64(len(body))
	return nil
}
