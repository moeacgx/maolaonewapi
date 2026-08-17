package oairesponses

import (
	"encoding/json"
	"fmt"
	"strings"

	"context"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	relaymedia "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/media"
	sharedclaude "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/shared/claude"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/reasoning"
)

const claudeCacheControlBreakpointLimit = 4

func shouldPreserveClaudeSuffix(info convmeta.Meta, inputModel string, opts *convmeta.Options) bool {
	model := inputModel
	if info != nil {
		if originModel := info.GetOriginModelName(); strings.TrimSpace(originModel) != "" {
			model = originModel
		}
	}
	return opts.ShouldPreserveThinkingSuffix(model)
}

func applyResponsesEffortSuffix(info convmeta.Meta, req *dto.OpenAIResponsesRequest, claudeRequest *dto.ClaudeRequest) {
	baseModel, effortLevel, ok := reasoning.TrimEffortSuffix(req.Model)
	if !ok || effortLevel == "" || !reasoning.IsClaudeThinkingModel(baseModel) || !reasoning.IsClaudeOpusReasoningModel(baseModel) {
		return
	}

	if !shouldPreserveClaudeSuffix(info, req.Model, convmeta.OptionsOf(info)) {
		claudeRequest.Model = baseModel
	}
	claudeRequest.Thinking = &dto.Thinking{Type: "adaptive"}
	claudeRequest.OutputConfig = json.RawMessage(fmt.Sprintf(`{"effort":"%s"}`, effortLevel))
	if reasoning.IsClaudeOpus47Or48(baseModel) {
		claudeRequest.Thinking.Display = "summarized"
		claudeRequest.Temperature = nil
		claudeRequest.TopP = nil
		claudeRequest.TopK = nil
	} else {
		claudeRequest.TopP = nil
		claudeRequest.Temperature = kitutil.GetPointer[float64](1.0)
	}
}

func convertOpenAIResponsesRequestToClaudeMessages(c context.Context, info convmeta.Meta, request any) (any, error) {
	responsesRequest, err := OpenAIResponsesRequestFromAny(request)
	if err != nil {
		return nil, err
	}
	return OpenAIResponsesRequestToClaudeMessages(c, info, responsesRequest)
}

func validateResponsesClaudeTools(req *dto.OpenAIResponsesRequest) error {
	if req == nil {
		return nil
	}
	if RawJSONPresent(req.Tools) {
		var tools []map[string]any
		if err := kitutil.Unmarshal(req.Tools, &tools); err != nil {
			return fmt.Errorf("invalid tools: %w", err)
		}
		for _, tool := range tools {
			toolType := strings.TrimSpace(kitutil.Interface2String(tool["type"]))
			if toolType != "function" {
				return fmt.Errorf("Claude Messages cannot safely represent Responses tool type %q", toolType)
			}
		}
	}
	return nil
}

func validateResponsesClaudeInputItems(items []map[string]any) error {
	for _, item := range items {
		switch strings.TrimSpace(kitutil.Interface2String(item["type"])) {
		case ResponsesInputTypeCustomToolCall, ResponsesInputTypeCustomToolOutput:
			return fmt.Errorf("Claude Messages cannot safely represent Responses custom tool items")
		}
	}
	return nil
}

func OpenAIResponsesRequestToClaudeMessages(c context.Context, info convmeta.Meta, req *dto.OpenAIResponsesRequest) (*dto.ClaudeRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if req.Model == "" {
		return nil, fmt.Errorf("model is required")
	}
	if err := ValidateRequestChatUnsupportedFields(req); err != nil {
		return nil, err
	}
	if err := validateResponsesClaudeTools(req); err != nil {
		return nil, err
	}

	claudeRequest := &dto.ClaudeRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
	}
	if req.MaxOutputTokens != nil && *req.MaxOutputTokens > 0 {
		claudeRequest.MaxTokens = kitutil.GetPointer(*req.MaxOutputTokens)
	}
	if claudeRequest.MaxTokens == nil || *claudeRequest.MaxTokens == 0 {
		if defaultMaxTokens, configured := convmeta.OptionsOf(info).Claude.DefaultMaxTokensFor(req.Model); configured {
			value := uint(defaultMaxTokens)
			claudeRequest.MaxTokens = &value
		}
	}

	applyResponsesEffortSuffix(info, req, claudeRequest)
	applyResponsesThinkingAdapter(info, req, claudeRequest)

	functions, err := RequestFunctionDeclarations(req.Tools)
	if err != nil {
		return nil, err
	}
	if len(functions) > 0 {
		claudeRequest.Tools = responsesFunctionDeclarationsToClaudeTools(functions)
	}

	toolChoice, err := RequestToolChoiceToChat(req.ToolChoice)
	if err != nil {
		return nil, err
	}
	if toolChoice != nil || RawJSONPresent(req.ParallelToolCalls) {
		claudeRequest.ToolChoice = sharedclaude.MapOpenAIToolChoice(toolChoice, ParallelToolCalls(req.ParallelToolCalls))
	}
	if err := applyResponsesReasoningToClaude(req, claudeRequest); err != nil {
		return nil, err
	}

	systemMessages := make([]dto.ClaudeMediaMessage, 0)
	if RawJSONPresent(req.Instructions) {
		instructions, err := JSONString(req.Instructions)
		if err != nil {
			return nil, fmt.Errorf("invalid instructions: %w", err)
		}
		if strings.TrimSpace(instructions) != "" {
			systemMessages = append(systemMessages, dto.ClaudeMediaMessage{
				Type: "text",
				Text: kitutil.GetPointer(instructions),
			})
		}
	}

	inputItems, err := InputItems(req.Input)
	if err != nil {
		return nil, err
	}
	if err := validateResponsesClaudeInputItems(inputItems); err != nil {
		return nil, err
	}

	for _, item := range inputItems {
		itemType := strings.TrimSpace(kitutil.Interface2String(item["type"]))
		switch itemType {
		case ResponsesInputTypeFunctionCall:
			claudeRequest.Messages = appendClaudeToolUse(claudeRequest.Messages, responsesFunctionCallItemToClaudeToolUse(item, "arguments"))
		case ResponsesInputTypeCustomToolCall, ResponsesInputTypeCustomToolOutput:
			return nil, fmt.Errorf("Claude Messages cannot safely represent Responses custom tool items")
		case ResponsesInputTypeFunctionCallOutput:
			claudeRequest.Messages = appendClaudeToolResult(claudeRequest.Messages, responsesFunctionOutputItemToClaudeToolResult(item))
		default:
			role := responsesClaudeRole(item)
			parts, err := responsesInputContentToClaudeMediaMessages(c, item["content"])
			if err != nil {
				return nil, err
			}
			if role == "system" {
				systemMessages = append(systemMessages, parts...)
				continue
			}
			if len(parts) == 0 {
				parts = []dto.ClaudeMediaMessage{
					{
						Type: "text",
						Text: kitutil.GetPointer("..."),
					},
				}
			}
			claudeRequest.Messages = append(claudeRequest.Messages, dto.ClaudeMessage{
				Role:    role,
				Content: parts,
			})
		}
	}

	limitClaudeCacheControlBreakpoints(systemMessages, claudeRequest.Messages)

	if len(systemMessages) > 0 {
		claudeRequest.System = systemMessages
	}
	claudeRequest.Messages = ensureClaudeMessagesStartWithUser(claudeRequest.Messages)
	// Checked last so every injection path has had its chance to satisfy the
	// required field.
	if claudeRequest.MaxTokens == nil {
		return nil, sharedclaude.ErrMissingMaxTokens
	}
	return claudeRequest, nil
}

func responsesFunctionDeclarationsToClaudeTools(functions []dto.FunctionRequest) []any {
	tools := make([]any, 0, len(functions))
	for _, function := range functions {
		tools = append(tools, &dto.Tool{
			Name:        function.Name,
			Description: function.Description,
			InputSchema: responsesFunctionParametersToClaudeInputSchema(function.Parameters),
		})
	}
	return tools
}

func responsesFunctionParametersToClaudeInputSchema(parameters any) map[string]interface{} {
	if params, ok := parameters.(map[string]any); ok {
		schema := make(map[string]interface{}, len(params))
		for key, value := range params {
			schema[key] = value
		}
		if schema["type"] == nil {
			schema["type"] = "object"
		}
		if schema["properties"] == nil {
			schema["properties"] = map[string]interface{}{}
		}
		return schema
	}
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func applyResponsesThinkingAdapter(info convmeta.Meta, req *dto.OpenAIResponsesRequest, claudeRequest *dto.ClaudeRequest) {
	opts := convmeta.OptionsOf(info)
	if !opts.Claude.ThinkingAdapterEnabled || !strings.HasSuffix(req.Model, "-thinking") {
		return
	}

	trimmedModel := strings.TrimSuffix(req.Model, "-thinking")
	if !reasoning.IsClaudeThinkingModel(trimmedModel) {
		return
	}

	if reasoning.IsClaudeOpus47Or48(trimmedModel) {
		claudeRequest.Thinking = &dto.Thinking{Type: "adaptive", Display: "summarized"}
		claudeRequest.OutputConfig = json.RawMessage(`{"effort":"high"}`)
		claudeRequest.Temperature = nil
		claudeRequest.TopP = nil
		claudeRequest.TopK = nil
	} else {
		if claudeRequest.MaxTokens == nil || *claudeRequest.MaxTokens < 1280 {
			claudeRequest.MaxTokens = kitutil.GetPointer[uint](1280)
		}
		claudeRequest.Thinking = &dto.Thinking{
			Type:         "enabled",
			BudgetTokens: kitutil.GetPointer[int](int(float64(*claudeRequest.MaxTokens) * opts.Claude.ThinkingAdapterBudgetTokensPercentage)),
		}
		claudeRequest.TopP = nil
		claudeRequest.Temperature = kitutil.GetPointer[float64](1.0)
	}
	if !shouldPreserveClaudeSuffix(info, req.Model, opts) {
		claudeRequest.Model = trimmedModel
	}
}

func applyResponsesReasoningToClaude(req *dto.OpenAIResponsesRequest, claudeRequest *dto.ClaudeRequest) error {
	effort := ReasoningEffort(req)
	switch effort {
	case "":
	case "none":
		claudeRequest.Thinking = &dto.Thinking{Type: "disabled"}
	case "minimal":
		claudeRequest.Thinking = &dto.Thinking{
			Type:         "enabled",
			BudgetTokens: kitutil.GetPointer(1024),
		}
	case "low":
		claudeRequest.Thinking = &dto.Thinking{
			Type:         "enabled",
			BudgetTokens: kitutil.GetPointer(1280),
		}
	case "medium":
		claudeRequest.Thinking = &dto.Thinking{
			Type:         "enabled",
			BudgetTokens: kitutil.GetPointer(2048),
		}
	case "high":
		claudeRequest.Thinking = &dto.Thinking{
			Type:         "enabled",
			BudgetTokens: kitutil.GetPointer(4096),
		}
	case "xhigh", "max", "ultra":
		return fmt.Errorf("Claude Messages cannot represent reasoning effort %q; supported efforts are none, minimal, low, medium, and high", effort)
	default:
		return fmt.Errorf("Claude Messages cannot represent unknown reasoning effort %q", effort)
	}
	if claudeRequest.Thinking == nil || claudeRequest.Thinking.Type != "enabled" || claudeRequest.Thinking.BudgetTokens == nil {
		return nil
	}

	budgetTokens := uint(*claudeRequest.Thinking.BudgetTokens)
	if req.MaxOutputTokens != nil && *req.MaxOutputTokens > 0 {
		finalMaxTokens := uint(0)
		if claudeRequest.MaxTokens != nil {
			finalMaxTokens = *claudeRequest.MaxTokens
		}
		if finalMaxTokens <= budgetTokens {
			return fmt.Errorf("Claude Messages reasoning budget_tokens (%d) must be less than max_output_tokens (%d)", budgetTokens, finalMaxTokens)
		}
		return nil
	}

	minimumMaxTokens := uint(1280)
	if minimumMaxTokens <= budgetTokens {
		minimumMaxTokens = budgetTokens + 1
	}
	if claudeRequest.MaxTokens == nil || *claudeRequest.MaxTokens < minimumMaxTokens {
		claudeRequest.MaxTokens = kitutil.GetPointer(minimumMaxTokens)
	}
	return nil
}

func responsesInputContentToClaudeMediaMessages(c context.Context, content any) ([]dto.ClaudeMediaMessage, error) {
	contentParts, err := ContentParts(content)
	if err != nil {
		return nil, err
	}

	parts := make([]dto.ClaudeMediaMessage, 0, len(contentParts))
	for _, contentPart := range contentParts {
		partType := strings.TrimSpace(kitutil.Interface2String(contentPart["type"]))
		cacheControl, err := responsesClaudeCacheControl(contentPart["cache_control"])
		if err != nil {
			return nil, err
		}
		switch partType {
		case "input_text", "output_text", "text":
			text := kitutil.Interface2String(contentPart["text"])
			if text != "" {
				parts = append(parts, dto.ClaudeMediaMessage{
					Type:         "text",
					Text:         kitutil.GetPointer(text),
					CacheControl: cacheControl,
				})
			}
		case "input_image", "input_file", "input_audio", "input_video":
			source := ContentPartToFileSource(contentPart)
			if source == nil {
				continue
			}
			base64Data, mimeType, err := relaymedia.ResolveBase64Data(c, source, "formatting Responses input for Claude")
			if err != nil {
				return nil, fmt.Errorf("get file data failed: %s", err.Error())
			}
			claudePart := dto.ClaudeMediaMessage{
				CacheControl: cacheControl,
				Source: &dto.ClaudeMessageSource{
					Type:      "base64",
					MediaType: mimeType,
					Data:      base64Data,
				},
			}
			if strings.HasPrefix(mimeType, "application/pdf") {
				claudePart.Type = "document"
			} else {
				claudePart.Type = "image"
			}
			parts = append(parts, claudePart)
		}
	}
	return parts, nil
}

func responsesClaudeCacheControl(value any) (json.RawMessage, error) {
	if value == nil {
		return nil, nil
	}
	raw, err := kitutil.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("invalid cache_control: %w", err)
	}
	if string(raw) == "null" {
		return nil, nil
	}
	return json.RawMessage(raw), nil
}

func responsesFunctionCallItemToClaudeToolUse(item map[string]any, inputKey string) dto.ClaudeMediaMessage {
	return dto.ClaudeMediaMessage{
		Type:  "tool_use",
		Id:    CallID(item),
		Name:  strings.TrimSpace(kitutil.Interface2String(item["name"])),
		Input: ObjectValue(item[inputKey], inputKey),
	}
}

func responsesFunctionOutputItemToClaudeToolResult(item map[string]any) dto.ClaudeMediaMessage {
	return dto.ClaudeMediaMessage{
		Type:      "tool_result",
		ToolUseId: CallID(item),
		Content:   responsesToolOutputValue(item["output"]),
	}
}

func responsesToolOutputValue(value any) any {
	if value == nil {
		return ""
	}
	return value
}

func appendClaudeToolUse(messages []dto.ClaudeMessage, toolUse dto.ClaudeMediaMessage) []dto.ClaudeMessage {
	if len(messages) > 0 && messages[len(messages)-1].Role == "assistant" {
		last := messages[len(messages)-1]
		parts := claudeMessageContentParts(last.Content)
		parts = append(parts, toolUse)
		last.Content = parts
		messages[len(messages)-1] = last
		return messages
	}
	return append(messages, dto.ClaudeMessage{
		Role:    "assistant",
		Content: []dto.ClaudeMediaMessage{toolUse},
	})
}

func appendClaudeToolResult(messages []dto.ClaudeMessage, toolResult dto.ClaudeMediaMessage) []dto.ClaudeMessage {
	if len(messages) > 0 && messages[len(messages)-1].Role == "user" {
		last := messages[len(messages)-1]
		parts := claudeMessageContentParts(last.Content)
		parts = append(parts, toolResult)
		last.Content = parts
		messages[len(messages)-1] = last
		return messages
	}
	return append(messages, dto.ClaudeMessage{
		Role:    "user",
		Content: []dto.ClaudeMediaMessage{toolResult},
	})
}

func claudeMessageContentParts(content any) []dto.ClaudeMediaMessage {
	switch typed := content.(type) {
	case []dto.ClaudeMediaMessage:
		return typed
	case string:
		if typed == "" {
			return nil
		}
		return []dto.ClaudeMediaMessage{
			{
				Type: "text",
				Text: kitutil.GetPointer(typed),
			},
		}
	default:
		parts, _ := kitutil.Any2Type[[]dto.ClaudeMediaMessage](content)
		return parts
	}
}

func responsesClaudeRole(item map[string]any) string {
	switch strings.TrimSpace(kitutil.Interface2String(item["role"])) {
	case "assistant":
		return "assistant"
	case "system", "developer":
		return "system"
	default:
		return "user"
	}
}

func ensureClaudeMessagesStartWithUser(messages []dto.ClaudeMessage) []dto.ClaudeMessage {
	if len(messages) == 0 || messages[0].Role == "user" {
		return messages
	}
	return append([]dto.ClaudeMessage{
		{
			Role: "user",
			Content: []dto.ClaudeMediaMessage{
				{
					Type: "text",
					Text: kitutil.GetPointer("..."),
				},
			},
		},
	}, messages...)
}

func limitClaudeCacheControlBreakpoints(system []dto.ClaudeMediaMessage, messages []dto.ClaudeMessage) {
	breakpoints := 0
	keepWithinLimit := func(part *dto.ClaudeMediaMessage) {
		if len(part.CacheControl) == 0 {
			return
		}
		breakpoints++
		if breakpoints > claudeCacheControlBreakpointLimit {
			part.CacheControl = nil
		}
	}

	for i := range system {
		keepWithinLimit(&system[i])
	}
	for i := range messages {
		parts, ok := messages[i].Content.([]dto.ClaudeMediaMessage)
		if !ok {
			continue
		}
		for j := range parts {
			keepWithinLimit(&parts[j])
		}
		messages[i].Content = parts
	}
}
