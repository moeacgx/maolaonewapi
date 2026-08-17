package claude

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"runtime"
	"strings"

	"github.com/QuantumNous/new-api/common"
	rootconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	if err := applyClaudeCodeRequestFingerprint(c, info, request); err != nil {
		return nil, err
	}
	NormalizeClaudeSamplingParameters(request)
	return request, nil
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	requestURL := fmt.Sprintf("%s/v1/messages", info.ChannelBaseUrl)
	if !shouldAppendClaudeBetaQuery(info) {
		return requestURL, nil
	}

	parsedURL, err := url.Parse(requestURL)
	if err != nil {
		return "", err
	}
	query := parsedURL.Query()
	query.Set("beta", "true")
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String(), nil
}

func shouldAppendClaudeBetaQuery(info *relaycommon.RelayInfo) bool {
	if info == nil {
		return false
	}
	if info.IsClaudeBetaQuery {
		return true
	}
	if info.ChannelOtherSettings.ClaudeBetaQuery {
		return true
	}
	return false
}

func CommonClaudeHeadersOperation(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) {
	// common headers operation
	anthropicBeta := c.Request.Header.Get("anthropic-beta")
	if anthropicBeta != "" {
		req.Set("anthropic-beta", anthropicBeta)
	}
	model_setting.GetClaudeSettings().WriteHeaders(info.OriginModelName, req)
}

const claudeCodeUserAgentPatternText = `(?i)^claude-cli/\d+\.\d+\.\d+`

var claudeCodeUserAgentPattern = regexp.MustCompile(claudeCodeUserAgentPatternText)

var claudeCodeCompatibilityHeaders = map[string]struct{}{
	"user-agent":        {},
	"x-app":             {},
	"anthropic-version": {},
	"anthropic-beta":    {},
	"anthropic-dangerous-direct-browser-access": {},
	"x-client-request-id":                       {},
	"x-claude-code-session-id":                  {},
	"x-stainless-lang":                          {},
	"x-stainless-package-version":               {},
	"x-stainless-os":                            {},
	"x-stainless-arch":                          {},
	"x-stainless-runtime":                       {},
	"x-stainless-runtime-version":               {},
	"x-stainless-retry-count":                   {},
	"x-stainless-timeout":                       {},
}

func isRealClaudeCodeClient(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	userAgent := c.Request.Header.Get("User-Agent")
	if claudeCodeUserAgentPattern.MatchString(userAgent) {
		return true
	}
	xApp := c.Request.Header.Get("X-App")
	if strings.EqualFold(xApp, "claude-code") {
		return true
	}
	if strings.EqualFold(xApp, "cli") &&
		c.Request.Header.Get("X-Stainless-Package-Version") != "" &&
		strings.EqualFold(c.Request.Header.Get("X-Stainless-Lang"), "js") {
		return true
	}
	return c.Request.Header.Get("X-Claude-Code-Session-Id") != "" &&
		strings.TrimSpace(userAgent) == "" && strings.TrimSpace(xApp) == ""
}

func shouldUseClaudeCodeOriginalPassThrough(c *gin.Context, info *relaycommon.RelayInfo) bool {
	if info == nil || info.ChannelMeta == nil || info.ApiType != rootconstant.APITypeAnthropic ||
		info.RelayFormat != types.RelayFormatClaude || !isRealClaudeCodeClient(c) {
		return false
	}
	return model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled
}

func shouldUseClaudeCodeBodyFingerprint(info *relaycommon.RelayInfo) bool {
	return info != nil && info.ChannelOtherSettings.ClaudeCodeFingerprintEnabled
}

func shouldUseClaudeCodeFingerprint(info *relaycommon.RelayInfo) bool {
	return shouldUseClaudeCodeBodyFingerprint(info)
}

func shouldApplyClaudeCodeSyntheticFingerprint(c *gin.Context, info *relaycommon.RelayInfo) bool {
	return shouldUseClaudeCodeBodyFingerprint(info) && info != nil &&
		info.ApiType == rootconstant.APITypeAnthropic && info.RelayFormat == types.RelayFormatClaude &&
		!isRealClaudeCodeClient(c)
}

func shouldApplyClaudeCodeTransportFingerprint(c *gin.Context, info *relaycommon.RelayInfo) bool {
	return info != nil && info.ChannelOtherSettings.ClaudeCodeTransportFingerprintEnabled &&
		info.ApiType == rootconstant.APITypeAnthropic && info.RelayFormat == types.RelayFormatClaude &&
		!isRealClaudeCodeClient(c)
}
func applyIncomingClaudeCodeHeaders(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) {
	if !shouldUseClaudeCodeOriginalPassThrough(c, info) || req == nil {
		return
	}
	for name, values := range c.Request.Header {
		if _, ok := claudeCodeCompatibilityHeaders[strings.ToLower(name)]; !ok {
			continue
		}
		for _, value := range values {
			if strings.TrimSpace(value) != "" {
				req.Set(name, value)
				break
			}
		}
	}
}

func applyClaudeCodeHeaderFingerprint(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) {
	if req == nil || !shouldApplyClaudeCodeTransportFingerprint(c, info) {
		return
	}
	entrypoint := getClaudeCodeEntrypoint(info)
	req.Set("User-Agent", fmt.Sprintf("claude-cli/%s (external, %s)", getClaudeCodeVersion(info), entrypoint))
	req.Set("X-Stainless-Lang", "js")
	req.Set("X-Stainless-Package-Version", "0.94.0")
	req.Set("X-Stainless-OS", mapStainlessOS(runtime.GOOS))
	req.Set("X-Stainless-Arch", mapStainlessArch(runtime.GOARCH))
	req.Set("X-Stainless-Runtime", "node")
	req.Set("X-Stainless-Runtime-Version", "v24.13.0")
	req.Set("X-Stainless-Retry-Count", "0")
	req.Set("X-Stainless-Timeout", "600")
	req.Set("X-App", entrypoint)
	req.Set("anthropic-version", "2023-06-01")
	req.Set("anthropic-beta", "interleaved-thinking-2025-05-14,fine-grained-tool-streaming-2025-05-14,claude-code-20250219,oauth-2025-04-20,context-management-2025-06-27,extended-cache-ttl-2025-04-11,prompt-caching-scope-2026-01-05")
	req.Set("anthropic-dangerous-direct-browser-access", "true")
	req.Set("x-client-request-id", uuid.NewString())
}

func getClaudeCodeVersion(info *relaycommon.RelayInfo) string {
	if info != nil && strings.TrimSpace(info.ChannelOtherSettings.ClaudeCodeVersion) != "" {
		return strings.TrimSpace(info.ChannelOtherSettings.ClaudeCodeVersion)
	}
	return "2.8.2"
}

func getClaudeCodeEntrypoint(info *relaycommon.RelayInfo) string {
	entrypoint := "cli"
	if info != nil && strings.TrimSpace(info.ChannelOtherSettings.ClaudeCodeEntrypoint) != "" {
		entrypoint = strings.TrimSpace(info.ChannelOtherSettings.ClaudeCodeEntrypoint)
	}
	for _, r := range entrypoint {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return "cli"
	}
	return entrypoint
}

func mapStainlessOS(goos string) string {
	switch goos {
	case "linux":
		return "Linux"
	case "darwin":
		return "MacOS"
	case "windows":
		return "Windows"
	default:
		return "Linux"
	}
}

func mapStainlessArch(goarch string) string {
	switch goarch {
	case "amd64":
		return "x64"
	case "arm64":
		return "arm64"
	case "386":
		return "x86"
	default:
		return "x64"
	}
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("x-api-key", info.ApiKey)
	if shouldUseClaudeCodeOriginalPassThrough(c, info) {
		CommonClaudeHeadersOperation(c, req, info)
		applyIncomingClaudeCodeHeaders(c, req, info)
		if req.Get("anthropic-version") == "" {
			req.Set("anthropic-version", "2023-06-01")
		}
		return nil
	}
	anthropicVersion := c.Request.Header.Get("anthropic-version")
	if anthropicVersion == "" {
		anthropicVersion = "2023-06-01"
	}
	req.Set("anthropic-version", anthropicVersion)
	CommonClaudeHeadersOperation(c, req, info)
	applyClaudeCodeHeaderFingerprint(c, req, info)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	result, err := relayconvert.ConvertRequest(c, info, types.RelayFormatClaude, request)
	if err != nil {
		return nil, err
	}
	claudeRequest, ok := result.Value.(*dto.ClaudeRequest)
	if !ok {
		return result.Value, nil
	}
	NormalizeClaudeSamplingParameters(claudeRequest)
	if err := applyClaudeCodeRequestFingerprint(c, info, claudeRequest); err != nil {
		return nil, err
	}
	return claudeRequest, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	result, err := relayconvert.ConvertRequest(c, info, types.RelayFormatClaude, &request)
	if err != nil {
		return nil, err
	}
	claudeRequest, ok := result.Value.(*dto.ClaudeRequest)
	if !ok {
		return nil, fmt.Errorf("expected Claude request, got %T", result.Value)
	}
	NormalizeClaudeSamplingParameters(claudeRequest)
	if err := applyClaudeCodeRequestFingerprint(c, info, claudeRequest); err != nil {
		return nil, err
	}
	if info != nil {
		if info.GetReasoningEffort() == "" && request.Reasoning != nil {
			info.SetReasoningEffort(request.Reasoning.Effort)
		}
		info.FinalRequestRelayFormat = types.RelayFormatClaude
	}
	return claudeRequest, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	info.FinalRequestRelayFormat = types.RelayFormatClaude
	if info.RelayMode == relayconstant.RelayModeResponses {
		if info.IsStream {
			return ClaudeMessagesToResponsesStreamHandler(c, info, resp)
		}
		return ClaudeMessagesToResponsesHandler(c, info, resp)
	}
	if info.IsStream {
		return ClaudeStreamHandler(c, resp, info)
	} else {
		return ClaudeHandler(c, resp, info)
	}
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

const (
	claudeCodeSystemText      = "You are Claude Code, Anthropic's official CLI for Claude."
	claudeCodeBillingPrefix   = "x-anthropic-billing-header:"
	claudeCodeFingerprintSalt = "59cf53e54c78"
	claudeCodeUserID          = "user_" + "0000000000000000000000000000000000000000000000000000000000000000" + "_account__session_" + "00000000-0000-0000-0000-000000000000"
)

var claudeCodeLegacySub2APIUserIDPattern = regexp.MustCompile(`^user_[a-fA-F0-9]{64}_account__session_[\w-]+$`)

func applyClaudeCodeRequestFingerprint(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) error {
	if request == nil || !shouldApplyClaudeCodeSyntheticFingerprint(c, info) {
		return nil
	}
	ensureClaudeCodeSystem(request, info)
	return ensureClaudeCodeMetadata(request)
}

func ensureClaudeCodeMetadata(request *dto.ClaudeRequest) error {
	metadata := make(map[string]interface{})
	if len(request.Metadata) > 0 {
		if err := common.Unmarshal(request.Metadata, &metadata); err != nil {
			return err
		}
		if metadata == nil {
			metadata = make(map[string]interface{})
		}
	}
	userID := strings.TrimSpace(common.Interface2String(metadata["user_id"]))
	if userID == "" || !claudeCodeLegacySub2APIUserIDPattern.MatchString(userID) {
		metadata["user_id"] = claudeCodeUserID
	}
	metadataBytes, err := common.Marshal(metadata)
	if err != nil {
		return err
	}
	request.Metadata = metadataBytes
	return nil
}

func ensureClaudeCodeSystem(request *dto.ClaudeRequest, info *relaycommon.RelayInfo) {
	original := request.ParseSystem()
	if request.IsStringSystem() {
		if text := strings.TrimSpace(request.GetStringSystem()); text != "" {
			original = []dto.ClaudeMediaMessage{newClaudeTextBlock(request.GetStringSystem())}
		}
	}
	kept := make([]dto.ClaudeMediaMessage, 0, len(original)+2)
	for _, block := range original {
		text := block.GetText()
		if strings.HasPrefix(text, claudeCodeBillingPrefix) {
			continue
		}
		if text == claudeCodeSystemText && strings.TrimSpace(string(block.CacheControl)) == `{"type":"ephemeral"}` {
			continue
		}
		kept = append(kept, block)
	}
	billing := newClaudeTextBlock(buildBillingBlockText(request, info))
	marker := newClaudeTextBlock(claudeCodeSystemText)
	marker.CacheControl = json.RawMessage(`{"type":"ephemeral"}`)
	request.System = append([]dto.ClaudeMediaMessage{marker, billing}, kept...)
	capClaudeCacheControlBreakpoints(request)
}

const claudeMaxCacheControlBreakpoints = 4

func hasClaudeCacheControl(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null"
}

func hasClaudeDynamicCacheControl(value any) bool {
	if value == nil {
		return false
	}
	if raw, ok := value.(json.RawMessage); ok {
		return hasClaudeCacheControl(raw)
	}
	return true
}

func retainClaudeCacheControl(raw json.RawMessage, count *int, limit int) json.RawMessage {
	if !hasClaudeCacheControl(raw) {
		return raw
	}
	if *count >= limit {
		return nil
	}
	(*count)++
	return raw
}

func capClaudeDirectCacheControl(value any, count *int, limit int) {
	switch item := value.(type) {
	case map[string]any:
		raw, ok := item["cache_control"]
		if !ok || !hasClaudeDynamicCacheControl(raw) {
			return
		}
		if *count >= limit {
			delete(item, "cache_control")
		} else {
			(*count)++
		}
	case map[string]json.RawMessage:
		raw, ok := item["cache_control"]
		if !ok || !hasClaudeCacheControl(raw) {
			return
		}
		if *count >= limit {
			delete(item, "cache_control")
		} else {
			(*count)++
		}
	}
}

func capClaudeToolsCacheControl(tools any, count *int, limit int) {
	switch values := tools.(type) {
	case []any:
		for _, value := range values {
			capClaudeDirectCacheControl(value, count, limit)
		}
	case []map[string]any:
		for _, value := range values {
			capClaudeDirectCacheControl(value, count, limit)
		}
	case []map[string]json.RawMessage:
		for _, value := range values {
			capClaudeDirectCacheControl(value, count, limit)
		}
	default:
		capClaudeDirectCacheControl(tools, count, limit)
	}
}

func capClaudeMessageContentCacheControl(content any, count *int, limit int) {
	switch values := content.(type) {
	case []any:
		for _, value := range values {
			capClaudeDirectCacheControl(value, count, limit)
		}
	case []map[string]any:
		for _, value := range values {
			capClaudeDirectCacheControl(value, count, limit)
		}
	case []map[string]json.RawMessage:
		for _, value := range values {
			capClaudeDirectCacheControl(value, count, limit)
		}
	case []dto.ClaudeMediaMessage:
		for index := range values {
			values[index].CacheControl = retainClaudeCacheControl(values[index].CacheControl, count, limit)
		}
	case []*dto.ClaudeMediaMessage:
		for _, value := range values {
			if value != nil {
				value.CacheControl = retainClaudeCacheControl(value.CacheControl, count, limit)
			}
		}
	case map[string]any, map[string]json.RawMessage:
		capClaudeDirectCacheControl(content, count, limit)
	}
}

func capClaudeCacheControlBreakpoints(request *dto.ClaudeRequest) {
	if request == nil {
		return
	}

	count := 0
	// Anthropic evaluates tool cache controls before system and message content.
	// Reserve one slot for the stable Claude Code marker at the front of system.
	capClaudeToolsCacheControl(request.Tools, &count, claudeMaxCacheControlBreakpoints-1)

	system := request.ParseSystem()
	for index := range system {
		block := &system[index]
		if index == 0 && block.GetText() == claudeCodeSystemText && hasClaudeCacheControl(block.CacheControl) {
			count++
			continue
		}
		block.CacheControl = retainClaudeCacheControl(block.CacheControl, &count, claudeMaxCacheControlBreakpoints)
	}
	request.System = system

	for index := range request.Messages {
		capClaudeMessageContentCacheControl(request.Messages[index].Content, &count, claudeMaxCacheControlBreakpoints)
	}
}

func newClaudeTextBlock(text string) dto.ClaudeMediaMessage {
	block := dto.ClaudeMediaMessage{Type: dto.ContentTypeText}
	block.SetText(text)
	return block
}

func buildBillingBlockText(request *dto.ClaudeRequest, info *relaycommon.RelayInfo) string {
	version := getClaudeCodeVersion(info)
	chars := make([]byte, 0, 3)
	firstText := ""
	for _, message := range request.Messages {
		if message.Role == "user" {
			firstText = message.GetStringContent()
			break
		}
	}
	for _, index := range []int{4, 7, 20} {
		if index < len(firstText) {
			chars = append(chars, firstText[index])
		} else {
			chars = append(chars, '0')
		}
	}
	sum := sha256.Sum256([]byte(claudeCodeFingerprintSalt + string(chars) + version))
	fingerprint := hex.EncodeToString(sum[:])[:3]
	return fmt.Sprintf("%s cc_version=%s.%s; cc_entrypoint=%s;", claudeCodeBillingPrefix, version, fingerprint, getClaudeCodeEntrypoint(info))
}

func capClaudeRawCacheControlObject(object map[string]json.RawMessage, count *int, limit int, forceRetain bool) bool {
	cacheControl, ok := object["cache_control"]
	if !ok || !hasClaudeCacheControl(cacheControl) {
		return false
	}
	if forceRetain || *count < limit {
		(*count)++
		return false
	}
	delete(object, "cache_control")
	return true
}

func capClaudeRawCacheControlArray(value json.RawMessage, count *int, limit int, markerFirst bool) (json.RawMessage, bool) {
	var items []json.RawMessage
	if err := common.Unmarshal(value, &items); err != nil {
		return value, false
	}
	changed := false
	for index := range items {
		var object map[string]json.RawMessage
		if err := common.Unmarshal(items[index], &object); err != nil {
			continue
		}
		forceRetain := false
		if markerFirst && index == 0 {
			var text string
			if rawText, ok := object["text"]; ok && common.Unmarshal(rawText, &text) == nil {
				forceRetain = text == claudeCodeSystemText
			}
		}
		if !capClaudeRawCacheControlObject(object, count, limit, forceRetain) {
			continue
		}
		updated, err := common.Marshal(object)
		if err != nil {
			continue
		}
		items[index] = updated
		changed = true
	}
	if !changed {
		return value, false
	}
	updated, err := common.Marshal(items)
	if err != nil {
		return value, false
	}
	return updated, true
}

func capClaudeRawMessagesCacheControl(value json.RawMessage, count *int, limit int) (json.RawMessage, bool) {
	var messages []json.RawMessage
	if err := common.Unmarshal(value, &messages); err != nil {
		return value, false
	}
	changed := false
	for index := range messages {
		var message map[string]json.RawMessage
		if err := common.Unmarshal(messages[index], &message); err != nil {
			continue
		}
		content, ok := message["content"]
		if !ok {
			continue
		}
		updatedContent, contentChanged := capClaudeRawCacheControlArray(content, count, limit, false)
		if !contentChanged {
			continue
		}
		message["content"] = updatedContent
		updatedMessage, err := common.Marshal(message)
		if err != nil {
			continue
		}
		messages[index] = updatedMessage
		changed = true
	}
	if !changed {
		return value, false
	}
	updated, err := common.Marshal(messages)
	if err != nil {
		return value, false
	}
	return updated, true
}

func capClaudeRawBodyCacheControl(raw map[string]json.RawMessage) {
	count := 0
	if tools, ok := raw["tools"]; ok {
		if updated, changed := capClaudeRawCacheControlArray(tools, &count, claudeMaxCacheControlBreakpoints-1, false); changed {
			raw["tools"] = updated
		}
	}
	if system, ok := raw["system"]; ok {
		if updated, changed := capClaudeRawCacheControlArray(system, &count, claudeMaxCacheControlBreakpoints, true); changed {
			raw["system"] = updated
		}
	}
	if messages, ok := raw["messages"]; ok {
		if updated, changed := capClaudeRawMessagesCacheControl(messages, &count, claudeMaxCacheControlBreakpoints); changed {
			raw["messages"] = updated
		}
	}
}

func ApplyClaudeCodeFinalBodyFingerprint(c *gin.Context, info *relaycommon.RelayInfo, body []byte) ([]byte, error) {
	if info == nil || info.ChannelMeta == nil || info.ApiType != rootconstant.APITypeAnthropic ||
		info.GetFinalRequestRelayFormat() != types.RelayFormatClaude || !shouldUseClaudeCodeBodyFingerprint(info) ||
		isRealClaudeCodeClient(c) {
		return body, nil
	}
	return applyClaudeCodeBodyFingerprint(info, body)
}

func ApplyClaudeCodePassthroughBodyFingerprint(c *gin.Context, info *relaycommon.RelayInfo, body []byte) ([]byte, error) {
	if info == nil || info.ChannelMeta == nil || info.ApiType != rootconstant.APITypeAnthropic ||
		info.GetFinalRequestRelayFormat() != types.RelayFormatClaude || !shouldUseClaudeCodeBodyFingerprint(info) ||
		(!model_setting.GetGlobalSettings().PassThroughRequestEnabled && !info.ChannelSetting.PassThroughBodyEnabled) ||
		isRealClaudeCodeClient(c) {
		return body, nil
	}
	return applyClaudeCodeBodyFingerprint(info, body)
}

func applyClaudeCodeBodyFingerprint(info *relaycommon.RelayInfo, body []byte) ([]byte, error) {
	var raw map[string]json.RawMessage
	if err := common.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	var request dto.ClaudeRequest
	if err := common.Unmarshal(body, &request); err != nil {
		return nil, err
	}
	ensureClaudeCodeSystem(&request, info)
	if err := ensureClaudeCodeMetadata(&request); err != nil {
		return nil, err
	}
	system, err := common.Marshal(request.System)
	if err != nil {
		return nil, err
	}
	raw["system"] = system
	raw["metadata"] = request.Metadata
	capClaudeRawBodyCacheControl(raw)
	return common.Marshal(raw)
}
