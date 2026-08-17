package zhipu

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type closeNotifyRecorder struct {
	*httptest.ResponseRecorder
	closeNotify <-chan bool
}

func (r *closeNotifyRecorder) CloseNotify() <-chan bool {
	return r.closeNotify
}

func TestZhipuHandlerPreservesV3EnvelopeAndBillingUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{
		"code": 200,
		"msg": "success",
		"success": true,
		"data": {
			"task_id": "task-non-stream",
			"request_id": "request-non-stream",
			"task_status": "SUCCESS",
			"choices": [{"role": "assistant", "content": "hello from zhipu"}],
			"usage": {"prompt_tokens": 7, "completion_tokens": 4, "total_tokens": 11}
		}
	}`)

	var envelope ZhipuResponse
	require.NoError(t, json.Unmarshal(body, &envelope))
	require.Equal(t, "task-non-stream", envelope.Data.TaskId)
	require.Equal(t, "request-non-stream", envelope.Data.RequestId)
	require.Equal(t, "SUCCESS", envelope.Data.TaskStatus)
	require.Len(t, envelope.Data.Choices, 1)
	require.Equal(t, "hello from zhipu", envelope.Data.Choices[0].Content)
	require.Equal(t, 7, envelope.Data.Usage.PromptTokens)
	require.Equal(t, 4, envelope.Data.Usage.CompletionTokens)
	require.Equal(t, 11, envelope.Data.Usage.TotalTokens)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	usage, apiErr := zhipuHandler(c, nil, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, 7, usage.PromptTokens)
	require.Equal(t, 4, usage.CompletionTokens)
	require.Equal(t, 11, usage.TotalTokens)
	require.Positive(t, usage.TotalTokens)

	var response dto.OpenAITextResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "task-non-stream", response.Id)
	require.Len(t, response.Choices, 1)
	require.Equal(t, "hello from zhipu", response.Choices[0].Message.StringContent())
	require.Equal(t, "stop", response.Choices[0].FinishReason)
	require.Equal(t, 7, response.Usage.PromptTokens)
	require.Equal(t, 4, response.Usage.CompletionTokens)
	require.Equal(t, 11, response.Usage.TotalTokens)
}

func TestZhipuStreamHandlerPreservesV3MetaAndBillingUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	meta := `{"request_id":"request-stream","task_id":"task-stream","task_status":"SUCCESS","usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`
	var envelope ZhipuStreamMetaResponse
	require.NoError(t, json.Unmarshal([]byte(meta), &envelope))
	require.Equal(t, "request-stream", envelope.RequestId)
	require.Equal(t, "task-stream", envelope.TaskId)
	require.Equal(t, "SUCCESS", envelope.TaskStatus)
	require.Equal(t, 5, envelope.Usage.PromptTokens)
	require.Equal(t, 3, envelope.Usage.CompletionTokens)
	require.Equal(t, 8, envelope.Usage.TotalTokens)

	streamBody := "data:hello from zhipu stream\nmeta:" + meta + "\n"
	recorder := &closeNotifyRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		closeNotify:      make(chan bool),
	}
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	usage, apiErr := zhipuStreamHandler(c, nil, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(streamBody)),
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, 5, usage.PromptTokens)
	require.Equal(t, 3, usage.CompletionTokens)
	require.Equal(t, 8, usage.TotalTokens)
	require.Positive(t, usage.TotalTokens)

	var chunks []dto.ChatCompletionsStreamResponse
	for _, event := range strings.Split(recorder.Body.String(), "\n\n") {
		data := strings.TrimPrefix(strings.TrimSpace(event), "data: ")
		if data == "" || data == "[DONE]" {
			continue
		}
		var chunk dto.ChatCompletionsStreamResponse
		require.NoError(t, json.Unmarshal([]byte(data), &chunk))
		chunks = append(chunks, chunk)
	}

	require.Len(t, chunks, 2)
	require.Len(t, chunks[0].Choices, 1)
	require.Equal(t, "hello from zhipu stream", chunks[0].Choices[0].Delta.GetContentString())
	require.Equal(t, "request-stream", chunks[1].Id)
	require.Len(t, chunks[1].Choices, 1)
	require.NotNil(t, chunks[1].Choices[0].FinishReason)
	require.Equal(t, "stop", *chunks[1].Choices[0].FinishReason)
}
