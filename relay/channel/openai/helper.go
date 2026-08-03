package openai

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
)

// 辅助函数
func HandleStreamFormat(c *gin.Context, info *relaycommon.RelayInfo, data string, forceFormat bool, thinkToContent bool) error {
	info.SendResponseCount++

	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		return sendStreamData(c, info, data, forceFormat, thinkToContent)
	case types.RelayFormatClaude:
		return handleClaudeFormat(c, data, info)
	case types.RelayFormatGemini:
		return handleGeminiFormat(c, data, info)
	}
	return nil
}

func handleClaudeFormat(c *gin.Context, data string, info *relaycommon.RelayInfo) error {
	var streamResponse dto.ChatCompletionsStreamResponse
	if err := common.Unmarshal(common.StringToByteSlice(data), &streamResponse); err != nil {
		return err
	}

	if streamResponse.Usage != nil {
		info.ClaudeConvertInfo.Usage = streamResponse.Usage
	}
	claudeResponses := service.StreamResponseOpenAI2Claude(&streamResponse, info)
	for _, resp := range claudeResponses {
		helper.ClaudeData(c, *resp)
	}
	return nil
}

func handleGeminiFormat(c *gin.Context, data string, info *relaycommon.RelayInfo) error {
	var streamResponse dto.ChatCompletionsStreamResponse
	if err := common.Unmarshal(common.StringToByteSlice(data), &streamResponse); err != nil {
		logger.LogError(c, "failed to unmarshal stream response: "+err.Error())
		return err
	}

	geminiResponse := service.StreamResponseOpenAI2Gemini(&streamResponse, info)

	// 如果返回 nil，表示没有实际内容，跳过发送
	if geminiResponse == nil {
		return nil
	}

	geminiResponseStr, err := common.Marshal(geminiResponse)
	if err != nil {
		logger.LogError(c, "failed to marshal gemini response: "+err.Error())
		return err
	}

	return helper.StringData(c, string(geminiResponseStr))
}

func ProcessStreamResponse(streamResponse dto.ChatCompletionsStreamResponse, responseTextBuilder *strings.Builder, toolCount *int) error {
	for _, choice := range streamResponse.Choices {
		responseTextBuilder.WriteString(choice.Delta.GetContentString())
		responseTextBuilder.WriteString(choice.Delta.GetReasoningContent())
		if choice.Delta.ToolCalls != nil {
			if len(choice.Delta.ToolCalls) > *toolCount {
				*toolCount = len(choice.Delta.ToolCalls)
			}
			for _, tool := range choice.Delta.ToolCalls {
				responseTextBuilder.WriteString(tool.Function.Name)
				responseTextBuilder.WriteString(tool.Function.Arguments)
			}
		}
	}
	return nil
}

func HasMeaningfulStreamOutput(streamResponse dto.ChatCompletionsStreamResponse) bool {
	for _, choice := range streamResponse.Choices {
		if choice.Delta.GetContentString() != "" {
			return true
		}
		if choice.Delta.GetReasoningContent() != "" {
			return true
		}
		if len(choice.Delta.ToolCalls) > 0 {
			return true
		}
	}
	return false
}

func processTokenData(relayMode int, data string, responseTextBuilder *strings.Builder, toolCount *int) error {
	switch relayMode {
	case relayconstant.RelayModeChatCompletions:
		var streamResponse dto.ChatCompletionsStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			return err
		}
		return ProcessStreamResponse(streamResponse, responseTextBuilder, toolCount)
	case relayconstant.RelayModeCompletions:
		var streamResponse dto.CompletionsStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			return err
		}
		processCompletionsStreamResponse(streamResponse, responseTextBuilder)
	}
	return nil
}

func processCompletionsStreamResponse(streamResponse dto.CompletionsStreamResponse, responseTextBuilder *strings.Builder) {
	for _, choice := range streamResponse.Choices {
		responseTextBuilder.WriteString(choice.Text)
	}
}

func handleLastResponse(lastStreamData string, responseId *string, createAt *int64,
	systemFingerprint *string, model *string, usage **dto.Usage,
	containStreamUsage *bool) error {

	var lastStreamResponse dto.ChatCompletionsStreamResponse
	if err := common.Unmarshal(common.StringToByteSlice(lastStreamData), &lastStreamResponse); err != nil {
		return err
	}

	*responseId = lastStreamResponse.Id
	*createAt = lastStreamResponse.Created
	*systemFingerprint = lastStreamResponse.GetSystemFingerprint()
	*model = lastStreamResponse.Model

	if normalizeAndValidateOpenAIUsage(lastStreamResponse.Usage) {
		*containStreamUsage = true
		*usage = lastStreamResponse.Usage
	}

	return nil
}

func shouldForwardOpenAIStreamData(data string, info *relaycommon.RelayInfo) bool {
	if info == nil || info.ShouldIncludeUsage {
		return true
	}

	var streamResponse dto.ChatCompletionsStreamResponse
	if err := common.Unmarshal(common.StringToByteSlice(data), &streamResponse); err != nil {
		return true
	}
	if !normalizeAndValidateOpenAIUsage(streamResponse.Usage) {
		return true
	}

	return lo.SomeBy(streamResponse.Choices, func(choice dto.ChatCompletionsStreamResponseChoice) bool {
		return choice.Delta.GetContentString() != "" ||
			choice.Delta.GetReasoningContent() != "" ||
			len(choice.Delta.ToolCalls) > 0
	})
}

func parseOpenAIStreamData(data string) (*dto.ChatCompletionsStreamResponse, bool, error) {
	var streamResponse dto.ChatCompletionsStreamResponse
	if err := common.Unmarshal(common.StringToByteSlice(data), &streamResponse); err != nil {
		return nil, false, err
	}
	if len(streamResponse.Choices) == 0 {
		return &streamResponse, normalizeAndValidateOpenAIUsage(streamResponse.Usage), nil
	}
	for _, choice := range streamResponse.Choices {
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			return &streamResponse, true, nil
		}
	}
	return &streamResponse, false, nil
}

func HandleFinalResponse(c *gin.Context, info *relaycommon.RelayInfo, lastStreamData string,
	responseId string, createAt int64, model string, systemFingerprint string,
	usage *dto.Usage, containStreamUsage bool) {

	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		if info.ShouldIncludeUsage && !containStreamUsage {
			response := helper.GenerateFinalUsageResponse(responseId, createAt, model, *usage)
			response.SetSystemFingerprint(systemFingerprint)
			helper.ObjectData(c, response)
		}
		helper.Done(c)

	case types.RelayFormatClaude:
		streamResponse, terminal, err := parseOpenAIStreamData(lastStreamData)
		if err != nil {
			common.SysLog("error unmarshalling stream response: " + err.Error())
			return
		}
		if !terminal {
			if info.ClaudeConvertInfo == nil || info.ClaudeConvertInfo.Done {
				return
			}
			streamResponse = helper.GenerateStopResponse(responseId, createAt, model, "stop")
		}

		info.ClaudeConvertInfo.Usage = usage

		claudeResponses := service.StreamResponseOpenAI2Claude(streamResponse, info)
		for _, resp := range claudeResponses {
			_ = helper.ClaudeData(c, *resp)
		}
		info.ClaudeConvertInfo.Done = true

	case types.RelayFormatGemini:
		return
	}
}

type responsesStreamDataItem struct {
	response dto.ResponsesStreamResponse
	data     string
}

func sendResponsesStreamData(c *gin.Context, streamResponse dto.ResponsesStreamResponse, data string) error {
	return sendResponsesStreamDataBatch(c, []responsesStreamDataItem{{
		response: streamResponse, data: data,
	}})
}

func sendResponsesStreamDataBatch(c *gin.Context, items []responsesStreamDataItem) error {
	chunks := make([]helper.ResponseChunkDataItem, 0, len(items))
	for _, item := range items {
		if item.data == "" {
			continue
		}
		chunks = append(chunks, helper.ResponseChunkDataItem{
			Response: item.response,
			Data:     item.data,
		})
	}
	return helper.ResponseChunkDataBatch(c, chunks)
}

func sendCommittedStreamAPIError(c *gin.Context, info *relaycommon.RelayInfo, relayErr *types.NewAPIError) error {
	if c == nil || c.Writer == nil || !c.Writer.Written() || relayErr == nil {
		return nil
	}

	if info != nil && info.RelayFormat == types.RelayFormatClaude {
		if err := helper.ClaudeData(c, dto.ClaudeResponse{
			Type:  "error",
			Error: relayErr.ToClaudeError(),
		}); err != nil {
			return err
		}
		return helper.FlushSensitiveStreamData(c)
	}

	if err := helper.ObjectData(c, struct {
		Error types.OpenAIError `json:"error"`
	}{
		Error: relayErr.ToOpenAIError(),
	}); err != nil {
		return err
	}
	return helper.FlushSensitiveStreamData(c)
}

func sendCommittedResponsesStreamAPIError(c *gin.Context, relayErr *types.NewAPIError) error {
	if c == nil || c.Writer == nil || !c.Writer.Written() || relayErr == nil {
		return nil
	}
	clientError := relayErr.ToOpenAIError()
	event := dto.ResponsesStreamResponse{
		Type:    "error",
		Code:    clientError.Code,
		Message: clientError.Message,
		Param:   clientError.Param,
	}
	data, err := common.Marshal(event)
	if err != nil {
		return err
	}
	if err := sendResponsesStreamData(c, event, string(data)); err != nil {
		return err
	}
	return helper.FlushSensitiveStreamData(c)
}
