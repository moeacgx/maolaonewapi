package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

var (
	ErrPromptAuditNoText = errors.New("提示词审计请求不包含可审计文本")
)

// promptAuditPrioritySeparator 只存在于加密任务负载中，用于保证最新用户输入优先分片。
const promptAuditPrioritySeparator = "\x00NEW_API_PROMPT_AUDIT_PRIORITY_END\x00"

// PromptAuditRequest 是协议无关的提示词审计输入。Body 必须是请求正文或 Realtime 文本帧的 JSON 快照。
type PromptAuditRequest struct {
	RequestId      string
	UserId         int
	Username       string
	UserEmail      string
	TokenId        int
	TokenName      string
	GroupId        int
	GroupName      string
	ChannelId      int
	ChannelName    string
	ChannelGroups  []model.PromptAuditEventChannelGroup
	Provider       string
	Endpoint       string
	Protocol       string
	Model          string
	Body           []byte
	Stage          string
	RequestArchive *RequestArchiveRequest
}

type promptAuditSegment struct {
	text string
	user bool
	role string
}

// ExtractPromptAuditSnapshot 按协议提取客户端可控文本，并生成不含正文的索引元数据。
func ExtractPromptAuditSnapshot(req PromptAuditRequest) (PromptAuditSnapshot, error) {
	var document interface{}
	if err := common.Unmarshal(req.Body, &document); err != nil {
		return PromptAuditSnapshot{}, errors.New("提示词审计请求 JSON 无效")
	}
	segments := normalizePromptAuditSegments(extractPromptAuditProtocolSegments(req.Protocol, document))
	if len(segments) == 0 {
		return PromptAuditSnapshot{}, ErrPromptAuditNoText
	}
	scanText, metadataText, contextSegments := buildPromptAuditPrioritizedText(segments)
	snapshot := buildPromptAuditSnapshot(req, scanText, metadataText, len(segments))
	snapshot.ContextSegments = contextSegments
	snapshot.RequestArchive = cloneRequestArchiveRequest(req.RequestArchive)
	return snapshot, nil
}

// BuildPromptAuditTextSnapshot 用于已经完成协议解析的文本入口，例如 Realtime 单帧审计。
func BuildPromptAuditTextSnapshot(req PromptAuditRequest, text string) (PromptAuditSnapshot, error) {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\x00", "�"))
	if text == "" {
		return PromptAuditSnapshot{}, ErrPromptAuditNoText
	}
	snapshot := buildPromptAuditSnapshot(req, text, text, 1)
	snapshot.ContextSegments = []PromptAuditContextSegment{{Role: "user", Kind: "client", Start: 0, End: len([]rune(text)), Text: text}}
	snapshot.RequestArchive = cloneRequestArchiveRequest(req.RequestArchive)
	return snapshot, nil
}

func buildPromptAuditSnapshot(req PromptAuditRequest, scanText, metadataText string, messageCount int) PromptAuditSnapshot {
	digest := sha256.Sum256([]byte(metadataText))
	fullPrompt, truncated := capPromptAuditFullText(metadataText, PromptAuditMaxFullPromptRunes)
	boundedScanText, _ := capPromptAuditScanText(scanText, PromptAuditMaxFullPromptRunes)
	stage := strings.TrimSpace(req.Stage)
	if stage == "" {
		stage = "http"
	}
	return PromptAuditSnapshot{
		RequestId: req.RequestId, UserId: req.UserId, Username: req.Username,
		UserEmail: req.UserEmail, TokenId: req.TokenId, TokenName: req.TokenName,
		GroupId: req.GroupId, GroupName: req.GroupName,
		ChannelId: req.ChannelId, ChannelName: req.ChannelName,
		ChannelGroups: append([]model.PromptAuditEventChannelGroup(nil), req.ChannelGroups...), Provider: req.Provider,
		Endpoint: req.Endpoint, Protocol: req.Protocol, Model: req.Model,
		PromptHash: hex.EncodeToString(digest[:]), RedactedPreview: BuildPromptAuditPreview(metadataText),
		PromptLength: utf8.RuneCountInString(metadataText), PromptTruncated: truncated,
		MessageCount: messageCount, Stage: stage, FullPrompt: fullPrompt, ScanText: boundedScanText,
	}
}

func extractPromptAuditProtocolSegments(protocol string, document interface{}) []promptAuditSegment {
	root, _ := document.(map[string]interface{})
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "openai_chat_completions", "openai_chat", "chat_completions", "openai_completions", "completions":
		segments := extractPromptAuditMessages(root["messages"], promptAuditClientRoles...)
		segments = append(segments, promptAuditUserSegments(promptAuditScalarTexts(root["prompt"]))...)
		segments = append(segments, promptAuditSystemSegments(extractPromptAuditToolDefinitionTexts(root["tools"]))...)
		segments = append(segments, promptAuditSystemSegments(extractPromptAuditToolDefinitionTexts(root["functions"]))...)
		return segments
	case "anthropic_messages", "claude_messages", "messages", "claude":
		segments := append(extractPromptAuditSystem(root["system"]), extractPromptAuditMessages(root["messages"], promptAuditClientRoles...)...)
		return append(segments, promptAuditSystemSegments(extractPromptAuditToolDefinitionTexts(root["tools"]))...)
	case "gemini", "gemini_generate_content", "gemini_generate_content_stream":
		return extractPromptAuditGeminiRoot(root)
	case "openai_responses", "responses", "responses_websocket":
		return extractPromptAuditResponsesRoot(root, strings.ToLower(strings.TrimSpace(protocol)) == "responses_websocket")
	case "openai_realtime", "realtime":
		return extractPromptAuditRealtime(root)
	case "openai_images", "grok_media", "media", "images", "embedding", "embeddings", "rerank", "audio", "video", "task":
		return promptAuditUserSegments(extractPromptAuditMediaPrompts(root))
	default:
		if messages := extractPromptAuditMessages(root["messages"], promptAuditClientRoles...); len(messages) > 0 {
			return messages
		}
		if responses := extractPromptAuditResponsesRoot(root, false); len(responses) > 0 {
			return responses
		}
		if gemini := extractPromptAuditGeminiRoot(root); len(gemini) > 0 {
			return gemini
		}
		return promptAuditUserSegments(extractPromptAuditMediaPrompts(root))
	}
}

var promptAuditClientRoles = []string{"user", "system", "developer", "assistant", "tool"}

func extractPromptAuditMessages(value interface{}, wantedRoles ...string) []promptAuditSegment {
	items, ok := value.([]interface{})
	if !ok {
		return nil
	}
	wanted := make(map[string]struct{}, len(wantedRoles))
	for _, role := range wantedRoles {
		wanted[strings.ToLower(strings.TrimSpace(role))] = struct{}{}
	}
	result := make([]promptAuditSegment, 0, len(items))
	for _, item := range items {
		message, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		role := strings.ToLower(promptAuditString(message["role"]))
		if _, match := wanted[role]; !match {
			// 整个 HTTP 请求体均由客户端提交。上游新增角色或客户端省略
			// role 时仍必须审核其中的文本，避免本地角色白名单成为绕过点。
			role = "user"
		}
		for _, text := range promptAuditContentTexts(message["content"]) {
			result = append(result, promptAuditSegment{text: text, user: role == "user", role: role})
		}
		// 部分 OpenAI 兼容接口会把上一轮推理正文放在
		// reasoning_content / reasoning 中并原样传给上游。它们与普通
		// assistant 内容一样属于客户端可控的模型上下文，不能绕过 Guard。
		for _, key := range []string{"reasoning_content", "reasoning"} {
			for _, text := range promptAuditScalarTexts(message[key]) {
				result = append(result, promptAuditSegment{text: text, user: role == "user", role: role})
			}
		}
		for _, text := range extractPromptAuditMessageToolTexts(message) {
			result = append(result, promptAuditSegment{text: text, user: role == "user" || role == "tool", role: role})
		}
	}
	return result
}

func extractPromptAuditMessageToolTexts(message map[string]interface{}) []string {
	if message == nil {
		return nil
	}
	result := make([]string, 0, 2)
	if functionCall, ok := message["function_call"].(map[string]interface{}); ok {
		result = append(result, promptAuditStructuredTexts(functionCall["arguments"])...)
	}
	if toolCalls, ok := message["tool_calls"].([]interface{}); ok {
		for _, item := range toolCalls {
			toolCall, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if functionCall, ok := toolCall["function"].(map[string]interface{}); ok {
				result = append(result, promptAuditStructuredTexts(functionCall["arguments"])...)
			}
			result = append(result, promptAuditStructuredTexts(toolCall["arguments"])...)
		}
	}
	return result
}

func extractPromptAuditSystem(value interface{}) []promptAuditSegment {
	switch typed := value.(type) {
	case string:
		if text := strings.TrimSpace(typed); text != "" {
			return []promptAuditSegment{{text: text, role: "system"}}
		}
	case []interface{}, map[string]interface{}:
		return promptAuditSystemSegments(promptAuditContentTexts(typed))
	}
	return nil
}

func extractPromptAuditResponsesRoot(root map[string]interface{}, websocket bool) []promptAuditSegment {
	if root == nil {
		return nil
	}
	target := root
	if websocket {
		frameType := promptAuditString(root["type"])
		if frameType != "response.create" {
			return nil
		}
		if input, exists := root["input"]; !exists || input == nil {
			if response, ok := root["response"].(map[string]interface{}); ok {
				target = response
			}
		}
	}
	result := append(extractPromptAuditSystem(target["instructions"]), extractPromptAuditResponses(target["input"])...)
	result = append(result, promptAuditUserSegments(extractPromptAuditPromptVariables(target["prompt"]))...)
	result = append(result, promptAuditSystemSegments(extractPromptAuditToolDefinitionTexts(target["tools"]))...)
	return result
}

func extractPromptAuditResponses(value interface{}) []promptAuditSegment {
	switch typed := value.(type) {
	case string:
		return []promptAuditSegment{{text: typed, user: true, role: "user"}}
	case []interface{}:
		result := make([]promptAuditSegment, 0, len(typed))
		for _, item := range typed {
			switch entry := item.(type) {
			case string:
				result = append(result, promptAuditSegment{text: entry, user: true, role: "user"})
			case map[string]interface{}:
				role := promptAuditClientRoleOrUser(entry["role"])
				if content, exists := entry["content"]; exists {
					result = append(result, promptAuditSegmentsForRole(promptAuditContentTexts(content), role)...)
				} else if text := promptAuditString(entry["text"]); text != "" {
					result = append(result, promptAuditSegment{text: text, user: role == "user", role: role})
				}
				result = append(result, promptAuditSegmentsForRole(extractPromptAuditMessageToolTexts(entry), "user")...)
				result = append(result, promptAuditSegmentsForRole(extractPromptAuditResponseFunctionTexts(entry), "user")...)
			}
		}
		return result
	case map[string]interface{}:
		role := promptAuditClientRoleOrUser(typed["role"])
		result := promptAuditSegmentsForRole(promptAuditContentTexts(typed["content"]), role)
		result = append(result, promptAuditSegmentsForRole(extractPromptAuditMessageToolTexts(typed), "user")...)
		return append(result, promptAuditSegmentsForRole(extractPromptAuditResponseFunctionTexts(typed), "user")...)
	}
	return nil
}

func extractPromptAuditResponseFunctionTexts(entry map[string]interface{}) []string {
	if entry == nil {
		return nil
	}
	typeName := strings.ToLower(promptAuditString(entry["type"]))
	result := make([]string, 0, 2)
	switch typeName {
	case "function_call", "tool_call", "custom_tool_call":
		result = append(result, promptAuditStructuredTexts(entry["arguments"])...)
		result = append(result, promptAuditStructuredTexts(entry["input"])...)
	case "function_call_output", "tool_result", "computer_call_output", "custom_tool_call_output":
		result = append(result, promptAuditStructuredTexts(entry["output"])...)
		result = append(result, promptAuditStructuredTexts(entry["content"])...)
	}
	return result
}

func extractPromptAuditRealtime(root map[string]interface{}) []promptAuditSegment {
	if root == nil {
		return nil
	}
	switch promptAuditString(root["type"]) {
	case "session.update", "transcription_session.update":
		session, _ := root["session"].(map[string]interface{})
		result := extractPromptAuditRealtimeSession(session)
		if len(result) == 0 {
			return extractPromptAuditUnknownRealtimeEvent(root)
		}
		return result
	case "conversation.item.create":
		item, _ := root["item"].(map[string]interface{})
		// 整个 item 都来自客户端。即使上游新增了角色名称，也不能因为本地
		// 角色白名单尚未更新而跳过其中的文本。
		result := promptAuditUserSegments(promptAuditContentTexts(item["content"]))
		result = append(result, promptAuditUserSegments(extractPromptAuditMessageToolTexts(item))...)
		result = append(result, promptAuditUserSegments(extractPromptAuditResponseFunctionTexts(item))...)
		if len(result) == 0 {
			return extractPromptAuditUnknownRealtimeEvent(root)
		}
		return result
	case "response.create":
		// Realtime API 使用嵌套 response；Responses WebSocket 兼容客户端会把
		// response.create 字段直接放在根对象。两种形式都属于客户端输入。
		response, ok := root["response"].(map[string]interface{})
		if !ok || response == nil {
			response = root
		}
		result := append(extractPromptAuditSystem(response["instructions"]), extractPromptAuditResponses(response["input"])...)
		result = append(result, promptAuditUserSegments(extractPromptAuditPromptVariables(response["prompt"]))...)
		result = append(result, extractPromptAuditRealtimeToolDefinitions(response["tools"])...)
		if len(result) == 0 {
			return extractPromptAuditUnknownRealtimeEvent(root)
		}
		return result
	default:
		// Realtime 协议会持续增加控制事件。未知事件不能因为本地
		// 类型白名单尚未更新而跳过客户端可控文本；仅读取具有明确
		// 文本语义的字段，避免把事件类型、ID 或媒体载荷当作提示词。
		return extractPromptAuditUnknownRealtimeEvent(root)
	}
}

func extractPromptAuditUnknownRealtimeEvent(root map[string]interface{}) []promptAuditSegment {
	if root == nil {
		return nil
	}
	result := make([]promptAuditSegment, 0, 4)
	for _, key := range []string{"text", "prompt", "instructions", "content", "input", "output", "arguments", "message", "query", "data"} {
		value, exists := root[key]
		if !exists {
			continue
		}
		switch key {
		case "content", "input", "output", "arguments", "data":
			// 未知事件的 data 常用于承载音频/图片分片。保留短小的
			// 普通文本，同时丢弃 data URI、URL 和长 base64，避免把
			// 二进制载荷误送到 Guard；嵌套 content 仍按协议文本规则解析。
			result = append(result, promptAuditUserSegments(
				filterPromptAuditRealtimeUnknownMedia(promptAuditContentTexts(value)),
			)...)
		default:
			result = append(result, promptAuditUserSegments(promptAuditScalarTexts(value))...)
		}
	}
	for _, key := range []string{"item", "response", "session"} {
		if nested, ok := root[key].(map[string]interface{}); ok {
			result = append(result, extractPromptAuditUnknownRealtimeEvent(nested)...)
		}
	}
	return result
}

func filterPromptAuditRealtimeUnknownMedia(values []string) []string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" || looksLikePromptAuditMediaPayload(value) {
			continue
		}
		filtered = append(filtered, value)
	}
	return filtered
}

func extractPromptAuditRealtimeSession(session map[string]interface{}) []promptAuditSegment {
	if session == nil {
		return nil
	}
	result := extractPromptAuditSystem(session["instructions"])
	result = append(result, extractPromptAuditRealtimeToolDefinitions(session["tools"])...)
	result = append(result, promptAuditUserSegments(extractPromptAuditRealtimeTranscriptionPrompts(session))...)
	return append(result, promptAuditUserSegments(extractPromptAuditPromptVariables(session["prompt"]))...)
}

func extractPromptAuditRealtimeToolDefinitions(value interface{}) []promptAuditSegment {
	return promptAuditSystemSegments(extractPromptAuditToolDefinitionTexts(value))
}

func extractPromptAuditRealtimeTranscriptionPrompts(session map[string]interface{}) []string {
	if session == nil {
		return nil
	}
	result := make([]string, 0, 2)
	if transcription, ok := session["input_audio_transcription"].(map[string]interface{}); ok {
		result = append(result, promptAuditScalarTexts(transcription["prompt"])...)
	}
	if audio, ok := session["audio"].(map[string]interface{}); ok {
		if input, ok := audio["input"].(map[string]interface{}); ok {
			if transcription, ok := input["transcription"].(map[string]interface{}); ok {
				result = append(result, promptAuditScalarTexts(transcription["prompt"])...)
			}
		}
	}
	return result
}

func isPromptAuditClientRole(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "user", "system", "developer", "assistant", "tool", "model":
		return true
	default:
		return false
	}
}

// promptAuditClientRoleOrUser 仅用于客户端提交的协议对象。未知、缺失或类型非法的
// role 都按 user 处理，确保上游扩展角色时不会静默跳过客户端可控文本。
func promptAuditClientRoleOrUser(value interface{}) string {
	role := strings.ToLower(promptAuditString(value))
	if role == "" || !isPromptAuditClientRole(role) {
		return "user"
	}
	return role
}

func extractPromptAuditGeminiRoot(root map[string]interface{}) []promptAuditSegment {
	if root == nil {
		return nil
	}
	result := extractPromptAuditGeminiSystem(root["systemInstruction"])
	result = append(result, extractPromptAuditGeminiSystem(root["system_instruction"])...)
	result = append(result, promptAuditUserSegments(promptAuditScalarTexts(root["contents"]))...)
	result = append(result, extractPromptAuditGemini(root["contents"])...)
	result = append(result, promptAuditUserSegments(promptAuditScalarTexts(root["content"]))...)
	result = append(result, extractPromptAuditGemini(root["content"])...)
	result = append(result, promptAuditUserSegments(promptAuditScalarTexts(root["input"]))...)
	result = append(result, extractPromptAuditGeminiInstances(root["instances"])...)
	result = append(result, promptAuditSystemSegments(extractPromptAuditToolDefinitionTexts(root["tools"]))...)
	if requests, ok := root["requests"].([]interface{}); ok {
		for _, item := range requests {
			request, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			result = append(result, extractPromptAuditGeminiSystem(request["systemInstruction"])...)
			result = append(result, extractPromptAuditGeminiSystem(request["system_instruction"])...)
			result = append(result, promptAuditUserSegments(promptAuditScalarTexts(request["contents"]))...)
			result = append(result, extractPromptAuditGemini(request["contents"])...)
			result = append(result, promptAuditUserSegments(promptAuditScalarTexts(request["content"]))...)
			result = append(result, extractPromptAuditGemini(request["content"])...)
			result = append(result, promptAuditUserSegments(promptAuditScalarTexts(request["input"]))...)
			result = append(result, extractPromptAuditGeminiInstances(request["instances"])...)
			result = append(result, promptAuditSystemSegments(extractPromptAuditToolDefinitionTexts(request["tools"]))...)
		}
	}
	return result
}

func extractPromptAuditGemini(value interface{}) []promptAuditSegment {
	var contents []interface{}
	switch typed := value.(type) {
	case []interface{}:
		contents = typed
	case map[string]interface{}:
		contents = []interface{}{typed}
	default:
		return nil
	}
	result := make([]promptAuditSegment, 0, len(contents))
	for _, item := range contents {
		content, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		role := promptAuditClientRoleOrUser(content["role"])
		parts, _ := content["parts"].([]interface{})
		for _, part := range parts {
			if object, ok := part.(map[string]interface{}); ok {
				if text := promptAuditString(object["text"]); text != "" {
					result = append(result, promptAuditSegment{text: text, user: role == "user", role: role})
				}
				if functionCall, ok := object["functionCall"].(map[string]interface{}); ok {
					result = append(result, promptAuditSegmentsForRole(promptAuditStructuredTexts(functionCall["args"]), "user")...)
				}
				if functionResponse, ok := object["functionResponse"].(map[string]interface{}); ok {
					result = append(result, promptAuditSegmentsForRole(promptAuditStructuredTexts(functionResponse["response"]), "user")...)
				}
				// Gemini 会把可执行代码及其执行结果作为下一轮模型上下文。
				// 只提取明确的代码/输出文本，不读取 inlineData 等媒体负载。
				if executableCode, ok := object["executableCode"].(map[string]interface{}); ok {
					result = append(result, promptAuditSegmentsForRole(promptAuditScalarTexts(executableCode["code"]), "user")...)
				}
				if codeResult, ok := object["codeExecutionResult"].(map[string]interface{}); ok {
					result = append(result, promptAuditSegmentsForRole(promptAuditScalarTexts(codeResult["output"]), "user")...)
				}
			}
		}
	}
	return result
}

func extractPromptAuditGeminiSystem(value interface{}) []promptAuditSegment {
	switch typed := value.(type) {
	case string:
		if text := strings.TrimSpace(typed); text != "" {
			return []promptAuditSegment{{text: text, role: "system"}}
		}
	case map[string]interface{}:
		if parts, ok := typed["parts"].([]interface{}); ok {
			result := make([]promptAuditSegment, 0, len(parts))
			for _, part := range parts {
				if object, ok := part.(map[string]interface{}); ok {
					if text := promptAuditString(object["text"]); text != "" {
						result = append(result, promptAuditSegment{text: text, role: "system"})
					}
				}
			}
			return result
		}
		return promptAuditSystemSegments(promptAuditContentTexts(typed))
	case []interface{}:
		segments := extractPromptAuditGemini(typed)
		for index := range segments {
			segments[index].user = false
		}
		return segments
	}
	return nil
}

func extractPromptAuditGeminiInstances(value interface{}) []promptAuditSegment {
	instances, ok := value.([]interface{})
	if !ok {
		return nil
	}
	result := make([]promptAuditSegment, 0, len(instances))
	for _, item := range instances {
		if instance, ok := item.(map[string]interface{}); ok {
			result = append(result, promptAuditUserSegments(promptAuditScalarTexts(instance["prompt"]))...)
			result = append(result, promptAuditUserSegments(promptAuditScalarTexts(instance["input"]))...)
			result = append(result, promptAuditUserSegments(promptAuditScalarTexts(instance["text"]))...)
			// Vertex Embeddings 常用 instances[].content，既可能是纯字符串，
			// 也可能复用 Gemini 的 {parts:[{text: ...}]} 内容结构。
			result = append(result, promptAuditUserSegments(promptAuditScalarTexts(instance["content"]))...)
			contentSegments := extractPromptAuditGemini(instance["content"])
			if len(contentSegments) == 0 {
				contentSegments = promptAuditSegmentsForRole(promptAuditContentTexts(instance["content"]), "user")
			}
			for index := range contentSegments {
				contentSegments[index].user = true
			}
			result = append(result, contentSegments...)
		}
	}
	return result
}

func extractPromptAuditMediaPrompts(root map[string]interface{}) []string {
	if root == nil {
		return nil
	}
	result := make([]string, 0, 4)
	seen := map[string]struct{}{}
	var walk func(interface{}, string)
	walk = func(value interface{}, key string) {
		switch typed := value.(type) {
		case map[string]interface{}:
			keys := make([]string, 0, len(typed))
			for childKey := range typed {
				keys = append(keys, childKey)
			}
			sort.Strings(keys)
			for _, childKey := range keys {
				walk(typed[childKey], childKey)
			}
		case []interface{}:
			for _, item := range typed {
				walk(item, key)
			}
		case []string:
			// 表单解析器会把重复字段保留为 []string，而 JSON
			// 解码器对应的是 []interface{}。两种形态都必须覆盖，
			// 否则 multipart/urlencoded 的多值 prompt 会绕过审计。
			for _, item := range typed {
				walk(item, key)
			}
		case string:
			if !isPromptAuditMediaKey(key) || looksLikePromptAuditMediaPayload(typed) {
				return
			}
			text := strings.TrimSpace(typed)
			if text == "" {
				return
			}
			if _, duplicate := seen[text]; duplicate {
				return
			}
			seen[text] = struct{}{}
			result = append(result, text)
		}
	}
	walk(root, "")
	return result
}

func isPromptAuditMediaKey(key string) bool {
	normalized := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(strings.TrimSpace(key)))
	switch normalized {
	case "prompt", "inputprompt", "textprompt", "description", "query", "lyrics", "negativeprompt",
		"positiveprompt", "gptdescriptionprompt", "prompten", "finalprompt", "finalzhprompt",
		"origprompt", "actualprompt", "imageprompt", "input", "texts", "text", "documents", "document",
		"caption", "alt", "alttext", "title", "style", "instruction", "instructions", "question",
		"message", "content", "inputtext", "outputtext", "sourceprompt", "targetprompt", "reftext",
		"logotextcontent", "tags":
		return true
	default:
		return false
	}
}

func looksLikePromptAuditMediaPayload(value string) bool {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "data:image/") || strings.HasPrefix(lower, "data:audio/") ||
		strings.HasPrefix(lower, "data:video/") || strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return true
	}
	if len(trimmed) < 256 {
		return false
	}
	for _, r := range trimmed {
		alphaNumeric := r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if !alphaNumeric && r != '+' && r != '/' && r != '=' {
			return false
		}
	}
	return true
}

func promptAuditContentTexts(value interface{}) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []interface{}:
		result := make([]string, 0, len(typed))
		for _, part := range typed {
			switch item := part.(type) {
			case string:
				result = append(result, item)
			case map[string]interface{}:
				typeName := strings.ToLower(promptAuditString(item["type"]))
				if typeName == "tool_result" {
					result = append(result, promptAuditContentTexts(item["content"])...)
					continue
				}
				if typeName == "function_call_output" || typeName == "custom_tool_call_output" {
					result = append(result, promptAuditStructuredTexts(item["output"])...)
					continue
				}
				if typeName == "tool_use" || typeName == "function_call" || typeName == "tool_call" {
					result = append(result, promptAuditStructuredTexts(item["input"])...)
					result = append(result, promptAuditStructuredTexts(item["arguments"])...)
					continue
				}
				if typeName == "input_audio" {
					if transcript := promptAuditString(item["transcript"]); transcript != "" {
						result = append(result, transcript)
					}
					continue
				}
				if typeName == "thinking" {
					if thinking := promptAuditString(item["thinking"]); thinking != "" {
						result = append(result, thinking)
					}
					continue
				}
				if typeName != "" && typeName != "text" && typeName != "input_text" && typeName != "output_text" {
					// 协议新增内容类型不能成为文本绕过点。只提取具有
					// 明确文本语义的字段；媒体、文件和 URL 仍由结构化
					// 提取器过滤，避免把二进制载荷送入 Guard。
					if text := promptAuditString(item["text"]); text != "" && !looksLikePromptAuditMediaPayload(text) {
						result = append(result, text)
					}
					if content, exists := item["content"]; exists {
						result = append(result, promptAuditContentTexts(content)...)
					}
					for _, key := range []string{"input", "output", "arguments", "value", "transcript", "thinking", "refusal", "context", "title"} {
						if value, exists := item[key]; exists {
							result = append(result, promptAuditStructuredTexts(value)...)
						}
					}
					continue
				}
				if text := promptAuditString(item["text"]); text != "" {
					result = append(result, text)
				}
			}
		}
		return result
	case []string:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if strings.TrimSpace(item) != "" {
				result = append(result, item)
			}
		}
		return result
	case map[string]interface{}:
		if text := promptAuditString(typed["text"]); text != "" {
			return []string{text}
		}
		if content, exists := typed["content"]; exists {
			return promptAuditContentTexts(content)
		}
		result := make([]string, 0, 2)
		for _, key := range []string{"input", "output", "arguments", "value", "prompt", "description", "transcript", "thinking", "refusal", "context", "title"} {
			if value, exists := typed[key]; exists {
				result = append(result, promptAuditStructuredTexts(value)...)
			}
		}
		return result
	}
	return nil
}

// promptAuditStructuredTexts 提取工具参数和工具输出中的客户端可控字符串。
// URL、媒体/base64 载荷和文件字段继续排除，避免把二进制内容送入文本 Guard。
func promptAuditStructuredTexts(value interface{}) []string {
	result := make([]string, 0, 4)
	var walk func(interface{}, string)
	walk = func(current interface{}, key string) {
		switch typed := current.(type) {
		case string:
			text := strings.TrimSpace(typed)
			if text == "" || shouldSkipPromptAuditStructuredKey(key) || looksLikePromptAuditMediaPayload(text) {
				return
			}
			result = append(result, text)
		case []interface{}:
			for _, item := range typed {
				walk(item, key)
			}
		case []string:
			for _, item := range typed {
				walk(item, key)
			}
		case map[string]interface{}:
			keys := make([]string, 0, len(typed))
			for childKey := range typed {
				keys = append(keys, childKey)
			}
			sort.Strings(keys)
			for _, childKey := range keys {
				walk(typed[childKey], childKey)
			}
		}
	}
	walk(value, "")
	return result
}

// extractPromptAuditToolDefinitionTexts 提取各协议工具定义中会进入模型上下文的
// 描述和参数模式；工具名称、类型及媒体字段不作为自然语言提示词处理。
func extractPromptAuditToolDefinitionTexts(value interface{}) []string {
	result := make([]string, 0, 4)
	var walk func(interface{})
	walk = func(current interface{}) {
		switch typed := current.(type) {
		case []interface{}:
			for _, item := range typed {
				walk(item)
			}
		case map[string]interface{}:
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				normalized := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(strings.TrimSpace(key)))
				switch normalized {
				case "description", "parameters", "parametersjsonschema", "inputschema":
					result = append(result, promptAuditStructuredTexts(typed[key])...)
				case "function", "functions", "functiondeclarations", "tools":
					walk(typed[key])
				}
			}
		}
	}
	walk(value)
	return result
}

func extractPromptAuditPromptVariables(value interface{}) []string {
	// Responses 标准协议使用 {variables: {...}}，但部分兼容客户端会
	// 直接把 prompt 作为字符串，或使用 text/content 扩展字段。不能因为
	// 只实现标准形状就让这些客户端的文本跳过 Guard；同时不把 prompt id
	// 和 version 等控制元数据当作自然语言送入审计。
	switch prompt := value.(type) {
	case string:
		return promptAuditScalarTexts(prompt)
	case []interface{}:
		return promptAuditStructuredTexts(prompt)
	case map[string]interface{}:
		if variables, exists := prompt["variables"]; exists {
			return promptAuditStructuredTexts(variables)
		}
		result := make([]string, 0, 2)
		for _, key := range []string{"text", "content", "input", "output", "instructions"} {
			if nested, exists := prompt[key]; exists {
				result = append(result, promptAuditStructuredTexts(nested)...)
			}
		}
		return result
	default:
		return nil
	}
}

func shouldSkipPromptAuditStructuredKey(key string) bool {
	normalized := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(strings.TrimSpace(key)))
	if strings.Contains(normalized, "url") || strings.Contains(normalized, "base64") ||
		strings.Contains(normalized, "image") || strings.Contains(normalized, "audio") ||
		strings.Contains(normalized, "video") || strings.Contains(normalized, "file") ||
		strings.Contains(normalized, "mime") {
		return true
	}
	return false
}

func promptAuditScalarTexts(value interface{}) []string {
	switch typed := value.(type) {
	case string:
		if text := strings.TrimSpace(typed); text != "" {
			return []string{text}
		}
	case []interface{}:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				result = append(result, text)
			}
		}
		return result
	case []string:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if strings.TrimSpace(item) != "" {
				result = append(result, item)
			}
		}
		return result
	}
	return nil
}

func normalizePromptAuditSegments(values []promptAuditSegment) []promptAuditSegment {
	normalized := make([]promptAuditSegment, 0, len(values))
	for _, value := range values {
		// PostgreSQL TEXT 不接受 NUL；替换为单个合法字符既保留字符计数，也防止客户端伪造内部优先级分隔符。
		value.text = strings.TrimSpace(strings.ReplaceAll(value.text, "\x00", "�"))
		if value.text != "" {
			normalized = append(normalized, value)
		}
	}
	if len(normalized) == 0 {
		return nil
	}
	// 保留协议中的原始消息顺序。最新用户消息的优先级只应用于
	// Guard 扫描文本，不能改变审核员在线查看的完整会话顺序。
	return normalized
}

func buildPromptAuditPrioritizedText(segments []promptAuditSegment) (string, string, []PromptAuditContextSegment) {
	texts := make([]string, 0, len(segments))
	context := make([]PromptAuditContextSegment, 0, len(segments))
	for _, segment := range segments {
		texts = append(texts, segment.text)
		role := segment.role
		if role == "" {
			if segment.user {
				role = "user"
			} else {
				role = "assistant"
			}
		}
		kind := "llm"
		if segment.user || role == "system" || role == "developer" || role == "tool" {
			kind = "client"
		}
		context = append(context, PromptAuditContextSegment{Role: role, Kind: kind, Text: segment.text})
	}
	metadataText := strings.Join(texts, "\n\n")
	priorityIndex := len(segments) - 1
	for index := len(segments) - 1; index >= 0; index-- {
		if segments[index].user {
			priorityIndex = index
			break
		}
	}
	scanText := metadataText
	if len(texts) > 1 {
		priorityTexts := make([]string, 0, len(texts))
		priorityTexts = append(priorityTexts, texts[priorityIndex])
		for index, text := range texts {
			if index != priorityIndex {
				priorityTexts = append(priorityTexts, text)
			}
		}
		scanText = priorityTexts[0] + promptAuditPrioritySeparator + strings.Join(priorityTexts[1:], "\n\n")
	}
	offset := 0
	for index := range context {
		context[index].Start = offset
		context[index].End = offset + len([]rune(context[index].Text))
		offset = context[index].End
		if index < len(context)-1 {
			offset += 2
		}
	}
	return scanText, metadataText, context
}

func promptAuditSegmentsForRole(texts []string, role string) []promptAuditSegment {
	result := make([]promptAuditSegment, 0, len(texts))
	for _, text := range texts {
		result = append(result, promptAuditSegment{text: text, user: role == "" || role == "user", role: role})
	}
	return result
}

func promptAuditUserSegments(texts []string) []promptAuditSegment {
	return promptAuditSegmentsForRole(texts, "user")
}

func promptAuditSystemSegments(texts []string) []promptAuditSegment {
	return promptAuditSegmentsForRole(texts, "system")
}

func promptAuditString(value interface{}) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

// BuildPromptAuditPreview 保留正文开头的一小段，供 Root 审核员在列表页快速判断。
// 完整上下文由详情接口返回；这里不再做内容脱敏，避免审计列表失去判断价值。
func BuildPromptAuditPreview(value string) string {
	return strings.TrimSpace(trimPromptAuditRunes(value, PromptAuditPreviewRunes, true))
}

func capPromptAuditFullText(value string, maxRunes int) (string, bool) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\x00", ""))
	if maxRunes <= 0 {
		return "", value != ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value, false
	}
	return string(runes[:maxRunes]), true
}

func capPromptAuditScanText(value string, maxRunes int) (string, bool) {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 {
		return "", value != ""
	}
	segments := strings.SplitN(value, promptAuditPrioritySeparator, 2)
	if len(segments) == 1 {
		runes := []rune(value)
		if len(runes) <= maxRunes {
			return value, false
		}
		return string(runes[:maxRunes]), true
	}
	priorityRunes := []rune(segments[0])
	remainderRunes := []rune(segments[1])
	if len(priorityRunes)+len(remainderRunes) <= maxRunes {
		return value, false
	}
	if len(priorityRunes) >= maxRunes {
		return string(priorityRunes[:maxRunes]), true
	}
	remaining := maxRunes - len(priorityRunes)
	return segments[0] + promptAuditPrioritySeparator + string(remainderRunes[:remaining]), true
}

func trimPromptAuditRunes(value string, limit int, ellipsis bool) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if ellipsis && limit > 0 {
		return string(runes[:limit]) + "…"
	}
	return string(runes[:limit])
}
