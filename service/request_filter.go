package service

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

var ErrSensitiveResponseBlocked = errors.New("sensitive words detected")

const (
	SensitiveFilterHTTPStatus          = http.StatusForbidden
	SensitiveFilterSSEHTTPStatus       = http.StatusOK
	SensitiveFilterRealtimeCloseCode   = 4403
	SensitiveFilterRealtimeCloseReason = "content_audit_blocked"
)

func sensitiveFilterClientMessage() string {
	return "内容审计命中风险规则，请调整输入后重试"
}

func SensitiveFilterHTTPMessage() string {
	return sensitiveFilterClientMessage()
}

func SensitiveFilterSSEMessage() string {
	return sensitiveFilterClientMessage()
}

func SensitiveFilterRealtimeMessage(c *gin.Context) string {
	message := sensitiveFilterClientMessage()
	if c == nil {
		return message
	}
	requestID := c.GetString(common.RequestIdKey)
	if requestID == "" {
		return message
	}
	return common.MessageWithRequestId(message, requestID)
}

func MarkContentPolicyRejected(c *gin.Context) {
	if c != nil {
		common.SetContextKey(c, constant.ContextKeyContentPolicyRejected, true)
	}
}

func IsContentPolicyRejected(c *gin.Context) bool {
	if c == nil {
		return false
	}
	return common.GetContextKeyBool(c, constant.ContextKeyContentPolicyRejected)
}

func shouldRecordRelaySuccess(c *gin.Context) bool {
	return !IsContentPolicyRejected(c)
}

type SensitiveFilterMatch struct {
	RuleID   string `json:"rule_id"`
	RuleName string `json:"rule_name"`
	Action   string `json:"action"`
	Keyword  string `json:"keyword"`
}

type SensitiveFilterResult struct {
	Blocked bool
	Mutated bool
	Matches []SensitiveFilterMatch
}

type SensitiveStreamDataFilterResult struct {
	Blocked bool
	Mutated bool
	Held    bool
	Matches []SensitiveFilterMatch
	Items   []SensitiveStreamDataItem
}

type SensitiveStreamDataItem struct {
	Data      string
	EventLine string
}

type compiledSensitiveRule struct {
	setting.SensitiveRule
	order    int
	keywords []compiledSensitiveKeyword
}

type compiledSensitiveKeyword struct {
	origin      string
	runes       []rune
	prefixTable []int
}

type textRangeMatch struct {
	start int
	end   int
	rule  compiledSensitiveRule
	word  compiledSensitiveKeyword
}

type sensitiveTextFilter struct {
	blockRules []compiledSensitiveRule
	maskRules  []compiledSensitiveRule
}

const sensitiveStreamDelayBufferContextKey = "sensitive_response_stream_delay_buffer"
const sensitiveRequestPrecheckedContextKey = "sensitive_request_prechecked_before_distribution"
const sensitivePolicySnapshotContextKey constant.ContextKey = "sensitive_policy_snapshot"
const sensitiveWordPrefillGroupType = "sensitive_word"
const sensitiveWordPrefillGroupCacheTTL = 30 * time.Second

var sensitiveWordPrefillGroupCache = struct {
	sync.RWMutex
	loadedAt time.Time
	groups   []*model.PrefillGroup
}{}

func sensitivePolicyForContext(c *gin.Context) setting.SensitivePolicySnapshot {
	if c != nil {
		if snapshot, ok := common.GetContextKeyType[setting.SensitivePolicySnapshot](c, sensitivePolicySnapshotContextKey); ok {
			return snapshot
		}
	}
	snapshot := setting.GetSensitivePolicySnapshot()
	if c != nil {
		c.Set(string(sensitivePolicySnapshotContextKey), snapshot)
	}
	return snapshot
}

func InvalidateSensitiveWordPrefillGroupCache() {
	sensitiveWordPrefillGroupCache.Lock()
	defer sensitiveWordPrefillGroupCache.Unlock()
	sensitiveWordPrefillGroupCache.loadedAt = time.Time{}
	sensitiveWordPrefillGroupCache.groups = nil
}

type sensitiveStreamDelayBuffer struct {
	filter      *sensitiveTextFilter
	queue       []sensitiveStreamChunk
	historyTail string
}

type sensitiveStreamChunk struct {
	data       string
	eventLine  string
	payload    any
	text       string
	textRunes  int
	batchStart bool
}

func ApplySensitiveFilterToRequestBody(c *gin.Context, relayFormat types.RelayFormat) (*SensitiveFilterResult, error) {
	if c != nil && c.GetBool(sensitiveRequestPrecheckedContextKey) {
		return &SensitiveFilterResult{}, nil
	}
	return applySensitiveFilterToRequestBody(c, relayFormat, false)
}

// ApplySensitiveFilterToRequestBodyBeforeDistribution 在渠道分配前执行能够确定作用范围的
// 请求敏感词规则。动态选渠下的指定渠道和渠道标签规则会延后到实际渠道选定后精确匹配，
// 避免把“候选渠道可能命中”错误扩大为整条 auto 路由都命中。
func ApplySensitiveFilterToRequestBodyBeforeDistribution(c *gin.Context, relayFormat types.RelayFormat, routeInfo ...string) (*SensitiveFilterResult, error) {
	return applySensitiveFilterToRequestBody(c, relayFormat, true, routeInfo...)
}

func applySensitiveFilterToRequestBody(c *gin.Context, relayFormat types.RelayFormat, beforeDistribution bool, routeInfo ...string) (*SensitiveFilterResult, error) {
	result := &SensitiveFilterResult{}
	if c == nil || c.Request == nil {
		return result, nil
	}
	// 即使 Guard 与屏蔽词均未启用，也保留请求生命周期内的文本快照，
	// 供上游 cyber_policy 事件关联。读取失败不改变原请求转发语义。
	captureSecurityAuditRequestBody(c, relayFormat)
	policy := sensitivePolicyForContext(c)
	if !policy.ShouldCheckPromptSensitive() {
		return result, nil
	}
	prechecked := beforeDistribution
	rules := policy.GetEffectiveSensitiveRulesByScope(setting.SensitiveRuleScopeRequest)
	if beforeDistribution {
		modelName, requestedGroup := sensitiveRouteInfo(routeInfo)
		route := resolveSensitiveRouteBeforeDistribution(c, modelName, requestedGroup)
		if sensitivePrecheckAllowed(rules, route, policy) {
			rules = selectSensitiveRulesForRoute(rules, route, policy)
		} else {
			// 渠道、渠道标签或业务分组目标尚未能绑定到最终渠道时，
			// 整批规则延后到渠道选定后的控制器阶段，避免 all 规则
			// 与指定范围规则混合时产生错误的提前阻断。
			rules = nil
		}
	} else {
		rules = selectSensitiveRulesForSelectedRoute(c, rules, policy)
	}
	filter := newSensitiveTextFilter(rules)
	if filter.empty() {
		// 预分配阶段可能尚未确定实际渠道。此时不能把请求标记为已检查，
		// 否则渠道分配完成后将跳过指定渠道/渠道标签规则的精确匹配。
		return result, nil
	}
	if !isJSONContentType(c.Request.Header.Get("Content-Type")) {
		return result, nil
	}

	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, err
	}
	body, err := storage.Bytes()
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		if prechecked {
			c.Set(sensitiveRequestPrecheckedContextKey, true)
		}
		return result, nil
	}

	var payload any
	if err := common.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	blockScan := &requestTextProcessor{filter: filter, mode: setting.SensitiveRuleActionBlock}
	processRelayTextFields(payload, relayFormat, blockScan)
	if len(blockScan.matches) > 0 {
		result.Blocked = true
		result.Matches = blockScan.matches
		MarkContentPolicyRejected(c)
		RecordSensitiveWordAuditEvent(c, "request", result.Matches, nil)
		if prechecked {
			c.Set(sensitiveRequestPrecheckedContextKey, true)
		}
		return result, nil
	}

	maskScan := &requestTextProcessor{filter: filter, mode: setting.SensitiveRuleActionMask}
	processRelayTextFields(payload, relayFormat, maskScan)
	if !maskScan.mutated {
		if prechecked {
			c.Set(sensitiveRequestPrecheckedContextKey, true)
		}
		return result, nil
	}

	rewritten, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	newStorage, err := common.CreateBodyStorage(rewritten)
	if err != nil {
		return nil, err
	}
	_ = storage.Close()
	c.Set(common.KeyBodyStorage, newStorage)
	if _, err := newStorage.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	c.Request.Body = io.NopCloser(newStorage)
	c.Request.ContentLength = int64(len(rewritten))

	result.Mutated = true
	result.Matches = maskScan.matches
	RecordSensitiveWordAuditEvent(c, "request", result.Matches, nil)
	if prechecked {
		c.Set(sensitiveRequestPrecheckedContextKey, true)
	}
	return result, nil
}

func shouldRunSensitiveFilterBeforeDistribution(c *gin.Context, policy setting.SensitivePolicySnapshot) bool {
	if c == nil || !policy.CheckEnabled {
		return false
	}
	rules := policy.GetEffectiveSensitiveRulesByScope(setting.SensitiveRuleScopeRequest)
	route := resolveSensitiveRouteBeforeDistribution(c, "", "")
	return sensitivePrecheckAllowed(rules, route, policy)
}

func sensitivePrecheckAllowed(rules []setting.SensitiveRule, route sensitiveRuleRouteScope, policy setting.SensitivePolicySnapshot) bool {
	if !route.before {
		return true
	}
	hasEnabledRule := false
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		hasEnabledRule = true
		if route.channelId <= 0 {
			targets := policy.ResolveSensitiveRuleTargets(rule)
			if !targets.All && (rule.TargetType != "" || len(targets.ChannelIds) == 0) {
				// 没有最终渠道时只允许全部渠道规则预检；其余规则
				// 必须延后，避免 auto 候选集扩大实际作用范围。
				// 旧版未设置 target_type 的规则沿用旧的全局渠道配置，
				// 继续保留其兼容的预检入口。
				return false
			}
		}
	}
	return hasEnabledRule
}

func ShouldCheckSensitiveBeforeDistribution(c *gin.Context) bool {
	policy := sensitivePolicyForContext(c)
	return policy.ShouldCheckPromptSensitive() && shouldRunSensitiveFilterBeforeDistribution(c, policy)
}

func ApplySensitiveFilterToResponseBody(c *gin.Context, contentType string, body []byte) (*SensitiveFilterResult, []byte, error) {
	result := &SensitiveFilterResult{}
	policy := sensitivePolicyForContext(c)
	if c == nil || !policy.CheckEnabled {
		return result, body, nil
	}
	filter := newSensitiveTextFilter(selectSensitiveRulesForSelectedRoute(
		c, policy.GetEffectiveSensitiveRulesByScope(setting.SensitiveRuleScopeResponse), policy,
	))
	if filter.empty() {
		return result, body, nil
	}
	if !isJSONContentType(contentType) {
		return result, body, nil
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return result, body, nil
	}

	var payload any
	if err := common.Unmarshal(body, &payload); err != nil {
		return nil, nil, err
	}
	auditSnapshot := buildSecurityAuditResponseSnapshot(c, payload, "response")

	blockScan := &responseTextProcessor{filter: filter, mode: setting.SensitiveRuleActionBlock}
	processResponseTextFields(payload, blockScan)
	if len(blockScan.matches) > 0 {
		result.Blocked = true
		result.Matches = blockScan.matches
		MarkContentPolicyRejected(c)
		RecordSensitiveWordAuditEvent(c, "response", result.Matches, auditSnapshot)
		return result, body, nil
	}

	maskScan := &responseTextProcessor{filter: filter, mode: setting.SensitiveRuleActionMask}
	processResponseTextFields(payload, maskScan)
	if !maskScan.mutated {
		return result, body, nil
	}

	rewritten, err := common.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	result.Mutated = true
	result.Matches = maskScan.matches
	RecordSensitiveWordAuditEvent(c, "response", result.Matches, auditSnapshot)
	return result, rewritten, nil
}

func ApplySensitiveFilterToStreamData(c *gin.Context, data string) (*SensitiveFilterResult, string, error) {
	result := &SensitiveFilterResult{}
	trimmed := strings.TrimSpace(data)
	policy := sensitivePolicyForContext(c)
	if trimmed == "" || trimmed == "[DONE]" || (!strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[")) || c == nil || !policy.CheckEnabled {
		return result, data, nil
	}
	filter := newSensitiveTextFilter(selectSensitiveRulesForSelectedRoute(
		c, policy.GetEffectiveSensitiveRulesByScope(setting.SensitiveRuleScopeResponse), policy,
	))
	if filter.empty() {
		return result, data, nil
	}

	var payload any
	if err := common.UnmarshalJsonStr(trimmed, &payload); err != nil {
		return nil, "", err
	}
	auditSnapshot := buildSecurityAuditResponseSnapshot(c, payload, "response_stream")

	streamText := strings.Join(collectResponseTextFields(payload), "")
	if matches := filter.blockMatchesWithEnd(streamText, false); len(matches) > 0 {
		result.Blocked = true
		result.Matches = matches
		MarkContentPolicyRejected(c)
		RecordSensitiveWordAuditEvent(c, "response_stream", result.Matches, auditSnapshot)
		return result, data, nil
	}

	maskScan := &responseTextProcessor{filter: filter, mode: setting.SensitiveRuleActionMask, stream: true}
	processResponseTextFields(payload, maskScan)
	if !maskScan.mutated {
		return result, data, nil
	}

	rewritten, err := common.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	result.Mutated = true
	result.Matches = maskScan.matches
	RecordSensitiveWordAuditEvent(c, "response_stream", result.Matches, auditSnapshot)
	return result, string(rewritten), nil
}

func captureSecurityAuditRequestBody(c *gin.Context, relayFormat types.RelayFormat) {
	if c == nil || c.Request == nil || !isJSONContentType(c.Request.Header.Get("Content-Type")) {
		return
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return
	}
	body, err := storage.Bytes()
	if err != nil || len(bytes.TrimSpace(body)) == 0 {
		return
	}
	_, _ = CaptureSecurityAuditRequestSnapshot(c, relayFormat, body)
}

// ApplySensitiveFilterToRealtimeRequestFrame 在写入上游前处理 Realtime 文本控制帧。
// 音频二进制帧由调用方直接透传，不进入本函数。
func ApplySensitiveFilterToRealtimeRequestFrame(c *gin.Context, body []byte) (*SensitiveFilterResult, []byte, error) {
	return applySensitiveFilterToRealtimeRequestFrame(c, body, false)
}

// ApplySensitiveFilterToRealtimeRequestFrameBeforeDistribution 用于首个控制帧
// 的渠道分配前门禁，并与 HTTP 一样把不确定的渠道范围延后到实际选渠后精确匹配。
func ApplySensitiveFilterToRealtimeRequestFrameBeforeDistribution(c *gin.Context, body []byte, routeInfo ...string) (*SensitiveFilterResult, []byte, error) {
	return applySensitiveFilterToRealtimeRequestFrame(c, body, true, routeInfo...)
}

func applySensitiveFilterToRealtimeRequestFrame(c *gin.Context, body []byte, beforeDistribution bool, routeInfo ...string) (*SensitiveFilterResult, []byte, error) {
	result := &SensitiveFilterResult{}
	if c == nil || len(bytes.TrimSpace(body)) == 0 {
		return result, body, nil
	}
	req := securityAuditRequestMetadata(c, "openai_realtime", "openai", "realtime_request")
	req.Body = body
	_, _ = CaptureSecurityAuditRealtimeSnapshot(c, req)
	policy := sensitivePolicyForContext(c)
	if !policy.ShouldCheckPromptSensitive() {
		return result, body, nil
	}
	rules := policy.GetEffectiveSensitiveRulesByScope(setting.SensitiveRuleScopeRequest)
	if beforeDistribution {
		modelName, requestedGroup := sensitiveRouteInfo(routeInfo)
		route := resolveSensitiveRouteBeforeDistribution(c, modelName, requestedGroup)
		if sensitivePrecheckAllowed(rules, route, policy) {
			rules = selectSensitiveRulesForRoute(rules, route, policy)
		} else {
			rules = nil
		}
	} else {
		rules = selectSensitiveRulesForSelectedRoute(c, rules, policy)
	}
	filter := newSensitiveTextFilter(rules)
	if filter.empty() {
		return result, body, nil
	}
	var payload any
	if err := common.Unmarshal(body, &payload); err != nil {
		return nil, nil, err
	}

	blockScan := &requestTextProcessor{filter: filter, mode: setting.SensitiveRuleActionBlock}
	processRealtimeTextFields(payload, blockScan)
	if len(blockScan.matches) > 0 {
		result.Blocked = true
		result.Matches = blockScan.matches
		MarkContentPolicyRejected(c)
		RecordSensitiveWordAuditEvent(c, "realtime_request", result.Matches, nil)
		return result, body, nil
	}

	maskScan := &requestTextProcessor{filter: filter, mode: setting.SensitiveRuleActionMask}
	processRealtimeTextFields(payload, maskScan)
	if !maskScan.mutated {
		return result, body, nil
	}
	rewritten, err := common.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	result.Mutated = true
	result.Matches = maskScan.matches
	RecordSensitiveWordAuditEvent(c, "realtime_request", result.Matches, nil)
	return result, rewritten, nil
}

// ApplySensitiveFilterToRealtimeResponseFrame 在写给客户端前执行现有返回范围规则。
func ApplySensitiveFilterToRealtimeResponseFrame(c *gin.Context, body []byte) (*SensitiveFilterResult, []byte, error) {
	result := &SensitiveFilterResult{}
	policy := sensitivePolicyForContext(c)
	if c == nil || len(bytes.TrimSpace(body)) == 0 || !policy.CheckEnabled {
		return result, body, nil
	}
	filter := newSensitiveTextFilter(selectSensitiveRulesForSelectedRoute(
		c, policy.GetEffectiveSensitiveRulesByScope(setting.SensitiveRuleScopeResponse), policy,
	))
	if filter.empty() {
		return result, body, nil
	}
	var payload any
	if err := common.Unmarshal(body, &payload); err != nil {
		return nil, nil, err
	}
	auditSnapshot := buildSecurityAuditResponseSnapshot(c, payload, "realtime_response")

	blockScan := &responseTextProcessor{filter: filter, mode: setting.SensitiveRuleActionBlock}
	processRealtimeTextFields(payload, blockScan)
	if len(blockScan.matches) > 0 {
		result.Blocked = true
		result.Matches = blockScan.matches
		MarkContentPolicyRejected(c)
		RecordSensitiveWordAuditEvent(c, "realtime_response", result.Matches, auditSnapshot)
		return result, body, nil
	}

	maskScan := &responseTextProcessor{filter: filter, mode: setting.SensitiveRuleActionMask}
	processRealtimeTextFields(payload, maskScan)
	if !maskScan.mutated {
		return result, body, nil
	}
	rewritten, err := common.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	result.Mutated = true
	result.Matches = maskScan.matches
	RecordSensitiveWordAuditEvent(c, "realtime_response", result.Matches, auditSnapshot)
	return result, rewritten, nil
}

type realtimeSensitiveTextProcessor interface {
	process(text string) (string, bool)
}

func processRealtimeTextFields(value any, processor realtimeSensitiveTextProcessor) {
	processRealtimeTextValue(value, "", processor)
}

func processRealtimeTextValue(value any, key string, processor realtimeSensitiveTextProcessor) {
	if processor == nil {
		return
	}
	switch typed := value.(type) {
	case map[string]any:
		for childKey, childValue := range typed {
			if text, ok := childValue.(string); ok {
				if !isRealtimeSensitiveTextField(childKey, text) {
					continue
				}
				updated, changed := processor.process(text)
				if changed {
					typed[childKey] = updated
				}
				continue
			}
			processRealtimeTextValue(childValue, childKey, processor)
		}
	case []any:
		for index, item := range typed {
			if text, ok := item.(string); ok {
				if !isRealtimeSensitiveTextField(key, text) {
					continue
				}
				updated, changed := processor.process(text)
				if changed {
					typed[index] = updated
				}
				continue
			}
			processRealtimeTextValue(item, key, processor)
		}
	}
}

func isRealtimeSensitiveTextField(key string, value string) bool {
	normalized := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(strings.TrimSpace(key)))
	switch normalized {
	case "instructions", "instruction", "text", "inputtext", "outputtext", "transcript",
		"delta", "prompt", "content", "arguments", "description", "input", "output", "refusal",
		"context", "title", "question", "message":
		return !looksLikeEncodedOrURL(value)
	default:
		return false
	}
}

func ApplySensitiveFilterToStreamDataForSend(c *gin.Context, data string, eventLine ...string) (*SensitiveStreamDataFilterResult, error) {
	line := ""
	if len(eventLine) > 0 {
		line = eventLine[0]
	}
	return ApplySensitiveFilterToStreamDataBatchForSend(c, []SensitiveStreamDataItem{{
		Data: data, EventLine: line,
	}})
}

// ApplySensitiveFilterToStreamDataBatchForSend 原子处理同一上游事件转换出的
// 多个 SSE 片段。首段正文仍需延迟判定时，前导生命周期事件也会一起保留，
// 避免只提交前导事件后失去跨渠道重试能力。
func ApplySensitiveFilterToStreamDataBatchForSend(c *gin.Context, items []SensitiveStreamDataItem) (*SensitiveStreamDataFilterResult, error) {
	if len(items) == 0 {
		return &SensitiveStreamDataFilterResult{}, nil
	}
	if len(items) == 1 && strings.TrimSpace(items[0].Data) == "[DONE]" {
		return &SensitiveStreamDataFilterResult{Items: items}, nil
	}
	buffer := getOrCreateSensitiveStreamDelayBuffer(c)
	if buffer == nil {
		return &SensitiveStreamDataFilterResult{Items: items}, nil
	}
	return buffer.pushBatch(c, items)
}

func FlushSensitiveStreamDataForSend(c *gin.Context) (*SensitiveStreamDataFilterResult, error) {
	buffer := getSensitiveStreamDelayBuffer(c)
	if buffer == nil {
		return &SensitiveStreamDataFilterResult{}, nil
	}
	return buffer.flush(c)
}

// ResetSensitiveStreamDataForRetry 丢弃当前上游尝试尚未下发的响应缓冲。
// 跨渠道重试时不能把上一渠道的 SSE 片段拼接到下一渠道响应中。
func ResetSensitiveStreamDataForRetry(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(sensitiveStreamDelayBufferContextKey, nil)
	c.Set("sensitive_response_stream_blocked", false)
}

func NewSensitiveFilterAPIError(c *gin.Context) *types.NewAPIError {
	apiErr := types.NewError(errors.New(SensitiveFilterHTTPMessage()), types.ErrorCodeSensitiveWordsDetected,
		types.ErrOptionWithStatusCode(SensitiveFilterHTTPStatus), types.ErrOptionWithSkipRetry())
	if c != nil {
		if requestId := c.GetString(common.RequestIdKey); requestId != "" {
			apiErr.SetMessage(common.MessageWithRequestId(apiErr.Error(), requestId))
		}
	}
	return apiErr
}

// SensitiveFilterClientOpenAIError 隐藏内部分类，只向客户端返回可读正文。
func SensitiveFilterClientOpenAIError(apiErr *types.NewAPIError) types.OpenAIError {
	if apiErr == nil {
		return types.OpenAIError{}
	}
	clientErr := apiErr.ToOpenAIError()
	clientErr.Code = nil
	clientErr.Metadata = nil
	return clientErr
}

func SensitiveFilterOpenAIErrorBody(c *gin.Context) []byte {
	body, err := common.Marshal(map[string]any{
		"error": SensitiveFilterClientOpenAIError(NewSensitiveFilterAPIError(c)),
	})
	if err != nil {
		return []byte(`{"error":{"message":"内容审计命中风险规则，请调整输入后重试","type":"new_api_error","param":"","code":null}}`)
	}
	return body
}

func SensitiveFilterSSEOpenAIErrorBody(c *gin.Context) []byte {
	apiErr := types.NewError(errors.New(SensitiveFilterSSEMessage()), types.ErrorCodeSensitiveWordsDetected,
		types.ErrOptionWithStatusCode(SensitiveFilterSSEHTTPStatus), types.ErrOptionWithSkipRetry())
	if c != nil {
		if requestId := c.GetString(common.RequestIdKey); requestId != "" {
			apiErr.SetMessage(common.MessageWithRequestId(apiErr.Error(), requestId))
		}
	}
	body, err := common.Marshal(map[string]any{
		"error": SensitiveFilterClientOpenAIError(apiErr),
	})
	if err != nil {
		return []byte(`{"error":{"message":"内容审计命中风险规则，请调整输入后重试","type":"new_api_error","param":"","code":null}}`)
	}
	return body
}

func shouldRunSensitiveFilter(c *gin.Context) bool {
	policy := sensitivePolicyForContext(c)
	if c == nil || !policy.CheckEnabled {
		return false
	}
	return len(selectSensitiveRulesForSelectedRoute(c, policy.GetEffectiveSensitiveRules(), policy)) > 0
}

type sensitiveRuleRouteScope struct {
	channelId               int
	channelTag              string
	channelTagKnown         bool
	channelGroupCodes       []string
	channelGroupsKnown      bool
	channelEligible         bool
	channelEligibilityKnown bool
	candidateGroupCodes     []string
	modelName               string
	before                  bool
	unknownCandidateGroups  bool
}

func sensitiveRouteInfo(values []string) (string, string) {
	modelName, requestedGroup := "", ""
	if len(values) > 0 {
		modelName = strings.TrimSpace(values[0])
	}
	if len(values) > 1 {
		requestedGroup = strings.TrimSpace(values[1])
	}
	return modelName, requestedGroup
}

func selectSensitiveRulesBeforeDistribution(c *gin.Context, rules []setting.SensitiveRule, policy setting.SensitivePolicySnapshot, modelName, requestedGroup string) []setting.SensitiveRule {
	return selectSensitiveRulesForRoute(rules, resolveSensitiveRouteBeforeDistribution(c, modelName, requestedGroup), policy)
}

func selectSensitiveRulesForSelectedRoute(c *gin.Context, rules []setting.SensitiveRule, policy setting.SensitivePolicySnapshot) []setting.SensitiveRule {
	return selectSensitiveRulesForRoute(rules, resolveSensitiveSelectedRoute(c), policy)
}

func selectSensitiveRulesForRoute(rules []setting.SensitiveRule, route sensitiveRuleRouteScope, policy setting.SensitivePolicySnapshot) []setting.SensitiveRule {
	selected := make([]setting.SensitiveRule, 0, len(rules))
	if route.before && route.channelId > 0 && route.channelEligibilityKnown && !route.channelEligible {
		return selected
	}
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		targets := policy.ResolveSensitiveRuleTargets(rule)
		// 历史规则只有全局 SensitiveRuleChannelIds。动态选渠阶段继续沿用
		// 原有的安全优先语义，避免升级后因候选缓存暂时缺失而跳过过滤。
		if rule.TargetType == "" && route.before && route.channelId <= 0 && len(targets.ChannelIds) > 0 {
			selected = append(selected, rule)
			continue
		}
		if sensitiveChannelTargetsMatchRoute(targets.ChannelIds, route) ||
			sensitiveTagTargetsMatchRoute(targets.ChannelTags, route) ||
			sensitiveGroupTargetsMatchRoute(targets.GroupCodes, route) ||
			targets.All {
			selected = append(selected, rule)
		}
	}
	return selected
}

func sensitiveChannelTargetsMatchRoute(channelIds []int, route sensitiveRuleRouteScope) bool {
	if len(channelIds) == 0 {
		return false
	}
	if route.channelId > 0 {
		return containsSensitiveRouteId(channelIds, route.channelId)
	}
	if !route.before {
		return false
	}
	// 指定渠道必须等实际渠道选定后再匹配。动态候选集只能说明“可能会选中”，
	// 不能把未勾选的渠道规则提前扩大到整个 auto 请求。
	return false
}

func sensitiveTagTargetsMatchRoute(tags []string, route sensitiveRuleRouteScope) bool {
	if len(tags) == 0 {
		return false
	}
	if route.channelId > 0 {
		if !route.channelTagKnown {
			return true
		}
		for _, tag := range tags {
			if strings.TrimSpace(tag) == route.channelTag {
				return true
			}
		}
		return false
	}
	if !route.before {
		return false
	}
	// 渠道标签同样依赖最终选中的渠道，不能仅凭 auto 的候选范围预先命中。
	return false
}

func sensitiveGroupTargetsMatchRoute(groups []string, route sensitiveRuleRouteScope) bool {
	if len(groups) == 0 {
		return false
	}
	if route.channelId > 0 {
		if !route.channelGroupsKnown {
			return true
		}
		return sensitiveStringTargetsIntersect(groups, route.channelGroupCodes)
	}
	if !route.before {
		return false
	}
	if route.unknownCandidateGroups || len(route.candidateGroupCodes) == 0 {
		return false
	}
	return sensitiveStringTargetsIntersect(groups, route.candidateGroupCodes)
}

func sensitiveStringTargetsIntersect(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(left))
	for _, value := range left {
		set[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
	}
	for _, value := range right {
		if _, ok := set[strings.ToLower(strings.TrimSpace(value))]; ok {
			return true
		}
	}
	return false
}

func resolveSensitiveRouteBeforeDistribution(c *gin.Context, modelName, requestedGroup string) sensitiveRuleRouteScope {
	route := sensitiveRuleRouteScope{
		channelId: sensitiveFixedChannelId(c),
		modelName: strings.TrimSpace(modelName),
		before:    true,
	}
	if route.channelId > 0 {
		if channel, ok := common.GetContextKeyType[*model.Channel](c, constant.ContextKeySelectedChannel); ok &&
			channel != nil && channel.Id == route.channelId {
			route.channelEligibilityKnown = true
			route.channelEligible = channel.Status == common.ChannelStatusEnabled
			route.channelTagKnown = true
			if channel.Tag != nil {
				route.channelTag = strings.TrimSpace(*channel.Tag)
			}
			route.channelGroupCodes = sensitiveChannelGroupCodes(channel)
			route.channelGroupsKnown = channel.GroupDetails != nil
			if !route.channelGroupsKnown {
				if codes, groupErr := model.GetChannelGroupCodes(route.channelId); groupErr == nil {
					route.channelGroupCodes = codes
					route.channelGroupsKnown = true
				}
			}
			return route
		}
		status, tag, exists, err := model.GetChannelStatusAndTag(route.channelId)
		if err == nil {
			route.channelEligibilityKnown = true
			route.channelEligible = exists && status == common.ChannelStatusEnabled
			route.channelTagKnown = exists
			route.channelTag = tag
			if codes, groupErr := model.GetChannelGroupCodes(route.channelId); groupErr == nil {
				route.channelGroupCodes = codes
				route.channelGroupsKnown = true
			}
		}
		return route
	}
	requestedGroup = strings.TrimSpace(requestedGroup)
	if requestedGroup != "" {
		if strings.EqualFold(requestedGroup, model.TokenGroupModeAuto) {
			route.unknownCandidateGroups = true
			return route
		}
		group, err := model.GetGroupByCodeOrAlias(requestedGroup)
		if err != nil || group == nil {
			route.unknownCandidateGroups = true
			return route
		}
		route.candidateGroupCodes = []string{group.Code}
		return route
	}

	usingGroup := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
	tokenGroup := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyTokenGroup))
	groupMode := strings.ToLower(strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyTokenGroupMode)))
	if usingGroup == "" {
		usingGroup = strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyUserGroup))
	}
	if tokenGroup == "" {
		tokenGroup = usingGroup
	}
	if strings.EqualFold(usingGroup, model.TokenGroupModeAuto) ||
		strings.EqualFold(tokenGroup, model.TokenGroupModeAuto) || groupMode == model.TokenGroupModeAuto {
		route.unknownCandidateGroups = true
		return route
	}

	route.candidateGroupCodes = sensitiveRouteGroupCodes(tokenGroup)
	if len(route.candidateGroupCodes) == 0 {
		route.candidateGroupCodes = sensitiveRouteGroupCodes(usingGroup)
	}
	route.unknownCandidateGroups = len(route.candidateGroupCodes) == 0
	return route
}

func resolveSensitiveSelectedRoute(c *gin.Context) sensitiveRuleRouteScope {
	route := sensitiveRuleRouteScope{}
	if c == nil {
		return route
	}
	route.channelId = common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	if route.channelId > 0 {
		route.channelTag, route.channelTagKnown = resolveSensitiveChannelTag(c, route.channelId)
		if channel, ok := common.GetContextKeyType[*model.Channel](c, constant.ContextKeySelectedChannel); ok &&
			channel != nil && channel.Id == route.channelId {
			route.channelGroupCodes = sensitiveChannelGroupCodes(channel)
			route.channelGroupsKnown = channel.GroupDetails != nil
		}
		if !route.channelGroupsKnown {
			if codes, err := model.GetChannelGroupCodes(route.channelId); err == nil {
				route.channelGroupCodes = codes
				route.channelGroupsKnown = true
			}
		}
	}
	return route
}

func sensitiveChannelGroupCodes(channel *model.Channel) []string {
	if channel == nil {
		return nil
	}
	codes := make([]string, 0, len(channel.GroupDetails))
	for _, detail := range channel.GroupDetails {
		if strings.TrimSpace(detail.Code) != "" {
			codes = append(codes, detail.Code)
		}
	}
	return codes
}

func resolveSensitiveChannelTag(c *gin.Context, channelId int) (string, bool) {
	if c != nil {
		if channel, ok := common.GetContextKeyType[*model.Channel](c, constant.ContextKeySelectedChannel); ok &&
			channel != nil && channel.Id == channelId {
			if channel.Tag == nil {
				return "", true
			}
			return strings.TrimSpace(*channel.Tag), true
		}
	}
	tag, err := model.GetChannelTag(channelId)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(tag), true
}

func sensitiveRouteGroupCodes(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || strings.EqualFold(part, model.TokenGroupModeAuto) {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		result = append(result, part)
	}
	return result
}

func sensitiveFixedChannelId(c *gin.Context) int {
	if c == nil {
		return 0
	}
	value, ok := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId)
	if !ok {
		return 0
	}
	text, ok := value.(string)
	if !ok {
		return 0
	}
	id, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

func containsSensitiveRouteId(ids []int, target int) bool {
	if target <= 0 {
		return false
	}
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func FormatSensitiveFilterMatches(matches []SensitiveFilterMatch) string {
	if len(matches) == 0 {
		return ""
	}
	parts := make([]string, 0, len(matches))
	for _, match := range matches {
		name := strings.TrimSpace(match.RuleName)
		if name == "" {
			name = match.RuleID
		}
		parts = append(parts, fmt.Sprintf("%s:%s", match.Action, name))
	}
	return strings.Join(parts, ", ")
}

func isJSONContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = contentType
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	return mediaType == "" || mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func newSensitiveTextFilter(rules []setting.SensitiveRule) *sensitiveTextFilter {
	filter := &sensitiveTextFilter{}
	for idx, rule := range expandSensitiveRuleGroupRefs(setting.NormalizeSensitiveRules(rules)) {
		if !rule.Enabled {
			continue
		}
		compiled := compiledSensitiveRule{
			SensitiveRule: rule,
			order:         idx,
			keywords:      make([]compiledSensitiveKeyword, 0, len(rule.Keywords)),
		}
		for _, keyword := range rule.Keywords {
			lower := strings.ToLower(keyword)
			lowerRunes := []rune(lower)
			compiled.keywords = append(compiled.keywords, compiledSensitiveKeyword{
				origin:      keyword,
				runes:       lowerRunes,
				prefixTable: buildSensitiveKeywordPrefixTable(lowerRunes),
			})
		}
		if len(compiled.keywords) == 0 {
			continue
		}
		switch rule.Action {
		case setting.SensitiveRuleActionMask:
			filter.maskRules = append(filter.maskRules, compiled)
		default:
			filter.blockRules = append(filter.blockRules, compiled)
		}
	}
	return filter
}

func expandSensitiveRuleGroupRefs(rules []setting.SensitiveRule) []setting.SensitiveRule {
	if len(rules) == 0 || !hasSensitiveRuleGroupRefs(rules) {
		return rules
	}
	groupKeywords := loadSensitiveWordPrefillGroupKeywords()
	if len(groupKeywords) == 0 {
		return rules
	}
	expanded := make([]setting.SensitiveRule, 0, len(rules))
	for _, rule := range rules {
		if len(rule.GroupRefs) == 0 {
			expanded = append(expanded, rule)
			continue
		}
		keywords := append([]string{}, rule.Keywords...)
		for _, groupRef := range rule.GroupRefs {
			keywords = append(keywords, groupKeywords[strings.ToLower(strings.TrimSpace(groupRef))]...)
		}
		rule.Keywords = normalizeSensitiveFilterKeywords(keywords)
		expanded = append(expanded, rule)
	}
	return expanded
}

func hasSensitiveRuleGroupRefs(rules []setting.SensitiveRule) bool {
	for _, rule := range rules {
		if len(rule.GroupRefs) > 0 {
			return true
		}
	}
	return false
}

func loadSensitiveWordPrefillGroupKeywords() map[string][]string {
	groups, err := getSensitiveWordPrefillGroups()
	if err != nil || len(groups) == 0 {
		return nil
	}
	result := make(map[string][]string, len(groups)*2)
	for _, group := range groups {
		if group == nil {
			continue
		}
		keywords := parseSensitiveWordPrefillGroupItems(group.Items)
		if len(keywords) == 0 {
			continue
		}
		result[strings.ToLower(strings.TrimSpace(group.Name))] = keywords
		if group.Id > 0 {
			result[fmt.Sprintf("%d", group.Id)] = keywords
		}
	}
	return result
}

func getSensitiveWordPrefillGroups() ([]*model.PrefillGroup, error) {
	now := time.Now()
	sensitiveWordPrefillGroupCache.RLock()
	if now.Sub(sensitiveWordPrefillGroupCache.loadedAt) < sensitiveWordPrefillGroupCacheTTL {
		groups := sensitiveWordPrefillGroupCache.groups
		sensitiveWordPrefillGroupCache.RUnlock()
		return groups, nil
	}
	sensitiveWordPrefillGroupCache.RUnlock()

	sensitiveWordPrefillGroupCache.Lock()
	defer sensitiveWordPrefillGroupCache.Unlock()
	if now.Sub(sensitiveWordPrefillGroupCache.loadedAt) < sensitiveWordPrefillGroupCacheTTL {
		return sensitiveWordPrefillGroupCache.groups, nil
	}
	groups, err := model.GetAllPrefillGroups(sensitiveWordPrefillGroupType)
	if err != nil {
		return nil, err
	}
	sensitiveWordPrefillGroupCache.groups = groups
	sensitiveWordPrefillGroupCache.loadedAt = now
	return groups, nil
}

func parseSensitiveWordPrefillGroupItems(items model.JSONValue) []string {
	if len(items) == 0 {
		return nil
	}
	var parsed []string
	if err := common.Unmarshal(items, &parsed); err != nil {
		return nil
	}
	return normalizeSensitiveFilterKeywords(parsed)
}

func normalizeSensitiveFilterKeywords(keywords []string) []string {
	result := make([]string, 0, len(keywords))
	seen := make(map[string]struct{}, len(keywords))
	for _, keyword := range keywords {
		keyword = strings.TrimSpace(keyword)
		if keyword == "" {
			continue
		}
		key := strings.ToLower(keyword)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, keyword)
	}
	return result
}

func (f *sensitiveTextFilter) empty() bool {
	return f == nil || (len(f.blockRules) == 0 && len(f.maskRules) == 0)
}

func (f *sensitiveTextFilter) blockMatches(text string) []SensitiveFilterMatch {
	return f.blockMatchesWithEnd(text, true)
}

func (f *sensitiveTextFilter) blockMatchesWithEnd(text string, endKnown bool) []SensitiveFilterMatch {
	if f == nil || text == "" {
		return nil
	}
	lowerRunes := []rune(strings.ToLower(text))
	var matches []SensitiveFilterMatch
	for _, rule := range f.blockRules {
		for _, keyword := range rule.keywords {
			if indexSensitiveKeywordWithEnd(lowerRunes, keyword.runes, 0, endKnown) >= 0 {
				matches = append(matches, rule.toMatch(keyword))
				break
			}
		}
	}
	return matches
}

func (f *sensitiveTextFilter) maskText(text string) (string, []SensitiveFilterMatch, bool) {
	return f.maskTextWithEnd(text, true)
}

func (f *sensitiveTextFilter) maskTextWithEnd(text string, endKnown bool) (string, []SensitiveFilterMatch, bool) {
	if f == nil || text == "" {
		return text, nil, false
	}
	ranges := f.selectedMaskRanges(text, endKnown)
	if len(ranges) == 0 {
		return text, nil, false
	}

	source := []rune(text)
	var builder strings.Builder
	matches := make([]SensitiveFilterMatch, 0, len(ranges))
	cursor := 0
	for _, item := range ranges {
		builder.WriteString(string(source[cursor:item.start]))
		builder.WriteString(item.rule.Replacement)
		cursor = item.end
		matches = append(matches, item.rule.toMatch(item.word))
	}
	builder.WriteString(string(source[cursor:]))
	return builder.String(), matches, true
}

func (f *sensitiveTextFilter) maskRangesWithEnd(text string, endKnown bool) []textRangeMatch {
	lowerRunes := []rune(strings.ToLower(text))
	var ranges []textRangeMatch
	for _, rule := range f.maskRules {
		for _, keyword := range rule.keywords {
			start := 0
			for start <= len(lowerRunes)-len(keyword.runes) {
				absolute := indexSensitiveKeywordWithEnd(lowerRunes, keyword.runes, start, endKnown)
				if absolute < 0 {
					break
				}
				ranges = append(ranges, textRangeMatch{
					start: absolute,
					end:   absolute + len(keyword.runes),
					rule:  rule,
					word:  keyword,
				})
				start = absolute + len(keyword.runes)
			}
		}
	}
	return ranges
}

func (f *sensitiveTextFilter) selectedMaskRanges(text string, endKnown bool) []textRangeMatch {
	ranges := f.maskRangesWithEnd(text, endKnown)
	sort.SliceStable(ranges, func(i, j int) bool {
		if ranges[i].start != ranges[j].start {
			return ranges[i].start < ranges[j].start
		}
		if ranges[i].end != ranges[j].end {
			return ranges[i].end > ranges[j].end
		}
		return ranges[i].rule.order < ranges[j].rule.order
	})

	selected := make([]textRangeMatch, 0, len(ranges))
	lastEnd := -1
	for _, item := range ranges {
		if item.start < lastEnd {
			continue
		}
		selected = append(selected, item)
		lastEnd = item.end
	}
	return selected
}

func (f *sensitiveTextFilter) pendingSuffixRunes(text string) int {
	if f == nil || text == "" {
		return 0
	}
	lowerRunes := []rune(strings.ToLower(text))
	longest := 0
	for _, rules := range [][]compiledSensitiveRule{f.blockRules, f.maskRules} {
		for _, rule := range rules {
			for _, keyword := range rule.keywords {
				if length := pendingSensitiveKeywordSuffix(lowerRunes, keyword); length > longest {
					longest = length
				}
			}
		}
	}
	return longest
}

func buildSensitiveKeywordPrefixTable(pattern []rune) []int {
	table := make([]int, len(pattern))
	for index, matched := 1, 0; index < len(pattern); index++ {
		for matched > 0 && pattern[index] != pattern[matched] {
			matched = table[matched-1]
		}
		if pattern[index] == pattern[matched] {
			matched++
		}
		table[index] = matched
	}
	return table
}

func pendingSensitiveKeywordSuffix(text []rune, keyword compiledSensitiveKeyword) int {
	if len(text) == 0 || len(keyword.runes) == 0 {
		return 0
	}
	start := max(0, len(text)-len(keyword.runes))
	matched := 0
	for _, value := range text[start:] {
		for matched > 0 && (matched == len(keyword.runes) || value != keyword.runes[matched]) {
			matched = keyword.prefixTable[matched-1]
		}
		if value == keyword.runes[matched] {
			matched++
		}
	}
	for matched > 0 {
		matchStart := len(text) - matched
		leftBoundaryOK := !isASCIISensitiveWordRune(keyword.runes[0]) ||
			matchStart == 0 || !isASCIISensitiveWordRune(text[matchStart-1])
		fullNonASCIIEnd := matched == len(keyword.runes) &&
			!isASCIISensitiveWordRune(keyword.runes[len(keyword.runes)-1])
		if leftBoundaryOK && !fullNonASCIIEnd {
			return matched
		}
		matched = keyword.prefixTable[matched-1]
	}
	return 0
}

// indexSensitiveKeyword 执行大小写折叠后的字面匹配。关键词首尾是 ASCII
// 字母、数字或下划线时，额外要求相邻字符不是同类字符，避免 "Master Key"
// 跨入 "Webmaster Keyword" 的两个单词内部；中文等非 ASCII 关键词仍保持
// 原有的连续子串语义。
func indexSensitiveKeywordWithEnd(text []rune, pattern []rune, start int, endKnown bool) int {
	if len(pattern) == 0 || start < 0 || start > len(text)-len(pattern) {
		return -1
	}
	for start <= len(text)-len(pattern) {
		idx := indexRunes(text[start:], pattern)
		if idx < 0 {
			return -1
		}
		absolute := start + idx
		end := absolute + len(pattern)
		startBoundaryOK := !isASCIISensitiveWordRune(pattern[0]) ||
			absolute == 0 || !isASCIISensitiveWordRune(text[absolute-1])
		endBoundaryOK := !isASCIISensitiveWordRune(pattern[len(pattern)-1]) ||
			end < len(text) && !isASCIISensitiveWordRune(text[end]) ||
			end == len(text) && endKnown
		if startBoundaryOK && endBoundaryOK {
			return absolute
		}
		start = absolute + 1
	}
	return -1
}

func isASCIISensitiveWordRune(value rune) bool {
	return value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' || value == '_'
}

func indexRunes(text []rune, pattern []rune) int {
	if len(pattern) == 0 || len(text) < len(pattern) {
		return -1
	}
	for i := 0; i <= len(text)-len(pattern); i++ {
		matched := true
		for j := range pattern {
			if text[i+j] != pattern[j] {
				matched = false
				break
			}
		}
		if matched {
			return i
		}
	}
	return -1
}

func (r compiledSensitiveRule) toMatch(keyword compiledSensitiveKeyword) SensitiveFilterMatch {
	return SensitiveFilterMatch{
		RuleID:   r.ID,
		RuleName: r.Name,
		Action:   r.Action,
		Keyword:  keyword.origin,
	}
}

type requestTextProcessor struct {
	filter  *sensitiveTextFilter
	mode    string
	mutated bool
	matches []SensitiveFilterMatch
}

type responseTextProcessor struct {
	filter  *sensitiveTextFilter
	mode    string
	stream  bool
	mutated bool
	matches []SensitiveFilterMatch
}

func (p *responseTextProcessor) process(text string) (string, bool) {
	switch p.mode {
	case setting.SensitiveRuleActionBlock:
		matches := p.filter.blockMatches(text)
		p.matches = append(p.matches, matches...)
		return text, false
	case setting.SensitiveRuleActionMask:
		updated, matches, changed := p.filter.maskTextWithEnd(text, !p.stream)
		if changed {
			p.mutated = true
			p.matches = append(p.matches, matches...)
		}
		return updated, changed
	default:
		return text, false
	}
}

func (p *requestTextProcessor) process(text string) (string, bool) {
	switch p.mode {
	case setting.SensitiveRuleActionBlock:
		matches := p.filter.blockMatches(text)
		p.matches = append(p.matches, matches...)
		return text, false
	case setting.SensitiveRuleActionMask:
		updated, matches, changed := p.filter.maskText(text)
		if changed {
			p.mutated = true
			p.matches = append(p.matches, matches...)
		}
		return updated, changed
	default:
		return text, false
	}
}

func processResponseTextFields(value any, processor *responseTextProcessor) {
	processResponseValue(value, "", processor)
}

func collectResponseTextFields(value any) []string {
	var texts []string
	collectResponseTextValue(value, "", &texts)
	return texts
}

func collectResponseTextValue(value any, key string, texts *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		for _, childKey := range sortedStringKeys(typed) {
			childValue := typed[childKey]
			if shouldSkipResponseField(childKey, childValue) {
				continue
			}
			if text, ok := childValue.(string); ok {
				*texts = append(*texts, text)
				continue
			}
			collectResponseTextValue(childValue, childKey, texts)
		}
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				if shouldSkipResponseField(key, text) {
					continue
				}
				*texts = append(*texts, text)
				continue
			}
			collectResponseTextValue(item, key, texts)
		}
	}
}

func processResponseValue(value any, key string, processor *responseTextProcessor) {
	if processor == nil {
		return
	}
	switch typed := value.(type) {
	case map[string]any:
		for _, childKey := range sortedStringKeys(typed) {
			childValue := typed[childKey]
			if shouldSkipResponseField(childKey, childValue) {
				continue
			}
			if text, ok := childValue.(string); ok {
				updated, changed := processor.process(text)
				if changed {
					typed[childKey] = updated
				}
				continue
			}
			processResponseValue(childValue, childKey, processor)
		}
	case []any:
		for idx, item := range typed {
			if text, ok := item.(string); ok {
				if shouldSkipResponseField(key, text) {
					continue
				}
				updated, changed := processor.process(text)
				if changed {
					typed[idx] = updated
				}
				continue
			}
			processResponseValue(item, key, processor)
		}
	}
}

func sortedStringKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func shouldSkipResponseField(key string, value any) bool {
	normalizedKey := strings.ToLower(strings.TrimSpace(key))
	if normalizedKey == "" {
		return false
	}
	switch normalizedKey {
	case "id", "object", "model", "created", "created_at", "updated_at", "system_fingerprint",
		"metadata", "usage", "prompt_tokens", "completion_tokens", "total_tokens", "input_tokens",
		"output_tokens", "cached_tokens", "finish_reason", "index", "role", "type", "status",
		"name", "function", "tool_calls", "tools", "tool_choice", "item_id", "response_id",
		"previous_response_id", "call_id", "tool_call_id":
		return true
	}
	if strings.Contains(normalizedKey, "url") ||
		strings.Contains(normalizedKey, "base64") ||
		strings.Contains(normalizedKey, "b64") ||
		strings.Contains(normalizedKey, "image") ||
		strings.Contains(normalizedKey, "audio") ||
		strings.Contains(normalizedKey, "file") ||
		strings.Contains(normalizedKey, "mime") ||
		strings.Contains(normalizedKey, "schema") {
		return true
	}
	if str, ok := value.(string); ok && looksLikeEncodedOrURL(str) {
		return true
	}
	return false
}

func looksLikeEncodedOrURL(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	if strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "data:") {
		return true
	}
	if len(text) < 128 {
		return false
	}
	base64Chars := 0
	for _, r := range text {
		switch {
		case r >= 'a' && r <= 'z':
			base64Chars++
		case r >= 'A' && r <= 'Z':
			base64Chars++
		case r >= '0' && r <= '9':
			base64Chars++
		case r == '+' || r == '/' || r == '=' || r == '-' || r == '_':
			base64Chars++
		}
	}
	return float64(base64Chars)/float64(len([]rune(text))) > 0.95
}

func lastRunes(text string, limit int) string {
	if limit <= 0 || text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[len(runes)-limit:])
}

func getOrCreateSensitiveStreamDelayBuffer(c *gin.Context) *sensitiveStreamDelayBuffer {
	if c == nil {
		return nil
	}
	if buffer := getSensitiveStreamDelayBuffer(c); buffer != nil {
		return buffer
	}
	if !shouldRunSensitiveFilter(c) {
		return nil
	}
	policy := sensitivePolicyForContext(c)
	filter := newSensitiveTextFilter(selectSensitiveRulesForSelectedRoute(
		c, policy.GetEffectiveSensitiveRulesByScope(setting.SensitiveRuleScopeResponse), policy,
	))
	if filter.empty() {
		return nil
	}
	buffer := &sensitiveStreamDelayBuffer{filter: filter}
	c.Set(sensitiveStreamDelayBufferContextKey, buffer)
	return buffer
}

func getSensitiveStreamDelayBuffer(c *gin.Context) *sensitiveStreamDelayBuffer {
	if c == nil {
		return nil
	}
	value, ok := c.Get(sensitiveStreamDelayBufferContextKey)
	if !ok {
		return nil
	}
	buffer, _ := value.(*sensitiveStreamDelayBuffer)
	return buffer
}

func (b *sensitiveStreamDelayBuffer) pushBatch(c *gin.Context, items []SensitiveStreamDataItem) (*SensitiveStreamDataFilterResult, error) {
	chunks := make([]sensitiveStreamChunk, 0, len(items))
	for _, item := range items {
		chunk, err := newSensitiveStreamChunk(item.Data, item.EventLine)
		if err != nil {
			return nil, err
		}
		chunk.batchStart = len(chunks) == 0
		chunks = append(chunks, chunk)
	}
	b.queue = append(b.queue, chunks...)
	return b.evaluate(c, false)
}

func (b *sensitiveStreamDelayBuffer) flush(c *gin.Context) (*SensitiveStreamDataFilterResult, error) {
	if b == nil || len(b.queue) == 0 {
		return &SensitiveStreamDataFilterResult{}, nil
	}
	return b.evaluate(c, true)
}

func (b *sensitiveStreamDelayBuffer) evaluate(c *gin.Context, endKnown bool) (*SensitiveStreamDataFilterResult, error) {
	result := &SensitiveStreamDataFilterResult{}
	if b == nil || b.filter == nil || len(b.queue) == 0 {
		return result, nil
	}

	candidate := b.historyTail + sensitiveStreamQueueText(b.queue)
	auditSnapshot := b.auditSnapshot(c)
	if matches := b.filter.blockMatchesWithEnd(candidate, endKnown); len(matches) > 0 {
		result.Blocked = true
		result.Matches = matches
		b.queue = nil
		MarkContentPolicyRejected(c)
		RecordSensitiveWordAuditEvent(c, "response_stream", matches, auditSnapshot)
		return result, nil
	}

	historyRunes := len([]rune(b.historyTail))
	maskRanges := b.filter.selectedMaskRanges(candidate, endKnown)
	holdIndex := len(b.queue)
	if !endKnown {
		pendingRunes := b.filter.pendingSuffixRunes(candidate)
		if pendingRunes > 0 {
			holdStart := len([]rune(candidate)) - pendingRunes
			holdIndex = sensitiveStreamQueueIndexAtOffset(b.queue, historyRunes, holdStart)
		}
	}

	readyEnd := sensitiveStreamQueueOffset(b.queue, historyRunes, holdIndex)
	for _, item := range maskRanges {
		if item.start < readyEnd && item.end > readyEnd {
			holdIndex = sensitiveStreamQueueIndexAtOffset(b.queue, historyRunes, item.start)
			readyEnd = sensitiveStreamQueueOffset(b.queue, historyRunes, holdIndex)
		}
	}
	if holdIndex < len(b.queue) {
		holdIndex = sensitiveStreamAtomicPrefixIndex(b.queue, holdIndex)
		readyEnd = sensitiveStreamQueueOffset(b.queue, historyRunes, holdIndex)
	}
	readyRanges := sensitiveMaskRangesWithin(maskRanges, historyRunes, readyEnd)
	items, mutated, err := rewriteSensitiveStreamChunks(b.queue[:holdIndex], historyRunes, readyRanges)
	if err != nil {
		return nil, err
	}
	result.Items = items
	result.Mutated = mutated
	result.Held = holdIndex < len(b.queue)
	result.Matches = sensitiveMatchesForRanges(readyRanges)
	if mutated {
		RecordSensitiveWordAuditEvent(c, "response_stream", result.Matches, auditSnapshot)
	}

	emittedText := sensitiveStreamQueueText(b.queue[:holdIndex])
	b.historyTail = lastRunes(b.historyTail+emittedText, 1)
	b.queue = b.queue[holdIndex:]
	return result, nil
}

func newSensitiveStreamChunk(data, eventLine string) (sensitiveStreamChunk, error) {
	chunk := sensitiveStreamChunk{data: data, eventLine: eventLine}
	trimmed := strings.TrimSpace(data)
	if trimmed == "" || trimmed == "[DONE]" || (!strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[")) {
		return chunk, nil
	}
	if err := common.UnmarshalJsonStr(trimmed, &chunk.payload); err != nil {
		return sensitiveStreamChunk{}, err
	}
	chunk.text = strings.Join(collectResponseTextFields(chunk.payload), "")
	chunk.textRunes = len([]rune(chunk.text))
	return chunk, nil
}

func sensitiveStreamQueueText(queue []sensitiveStreamChunk) string {
	var builder strings.Builder
	for _, chunk := range queue {
		builder.WriteString(chunk.text)
	}
	return builder.String()
}

func sensitiveStreamQueueOffset(queue []sensitiveStreamChunk, start, count int) int {
	offset := start
	if count > len(queue) {
		count = len(queue)
	}
	for _, chunk := range queue[:count] {
		offset += chunk.textRunes
	}
	return offset
}

func sensitiveStreamQueueIndexAtOffset(queue []sensitiveStreamChunk, start, target int) int {
	offset := start
	for index, chunk := range queue {
		if offset+chunk.textRunes > target {
			return index
		}
		offset += chunk.textRunes
	}
	return len(queue)
}

func sensitiveStreamAtomicPrefixIndex(queue []sensitiveStreamChunk, index int) int {
	if index >= len(queue) {
		return len(queue)
	}
	for index > 0 && !queue[index].batchStart && queue[index-1].textRunes == 0 {
		index--
	}
	return index
}

func sensitiveMaskRangesWithin(ranges []textRangeMatch, start, end int) []textRangeMatch {
	selected := make([]textRangeMatch, 0, len(ranges))
	for _, item := range ranges {
		if item.start >= start && item.end <= end {
			selected = append(selected, item)
		}
	}
	return selected
}

func sensitiveMatchesForRanges(ranges []textRangeMatch) []SensitiveFilterMatch {
	matches := make([]SensitiveFilterMatch, 0, len(ranges))
	for _, item := range ranges {
		matches = append(matches, item.rule.toMatch(item.word))
	}
	return matches
}

func rewriteSensitiveStreamChunks(chunks []sensitiveStreamChunk, start int, ranges []textRangeMatch) ([]SensitiveStreamDataItem, bool, error) {
	items := make([]SensitiveStreamDataItem, 0, len(chunks))
	offset := start
	mutated := false
	for index := range chunks {
		chunk := &chunks[index]
		changed := false
		if chunk.payload != nil && len(ranges) > 0 {
			changed = rewriteResponseTextRanges(chunk.payload, "", &offset, ranges)
		} else {
			offset += chunk.textRunes
		}
		data := chunk.data
		if changed {
			rewritten, err := common.Marshal(chunk.payload)
			if err != nil {
				return nil, false, err
			}
			data = string(rewritten)
			mutated = true
		}
		items = append(items, SensitiveStreamDataItem{Data: data, EventLine: chunk.eventLine})
	}
	return items, mutated, nil
}

func rewriteResponseTextRanges(value any, key string, offset *int, ranges []textRangeMatch) bool {
	changed := false
	switch typed := value.(type) {
	case map[string]any:
		for _, childKey := range sortedStringKeys(typed) {
			childValue := typed[childKey]
			if shouldSkipResponseField(childKey, childValue) {
				continue
			}
			if text, ok := childValue.(string); ok {
				updated, itemChanged := rewriteSensitiveTextSegment(text, *offset, ranges)
				*offset += len([]rune(text))
				if itemChanged {
					typed[childKey] = updated
					changed = true
				}
				continue
			}
			if rewriteResponseTextRanges(childValue, childKey, offset, ranges) {
				changed = true
			}
		}
	case []any:
		for index, item := range typed {
			if text, ok := item.(string); ok {
				if shouldSkipResponseField(key, text) {
					continue
				}
				updated, itemChanged := rewriteSensitiveTextSegment(text, *offset, ranges)
				*offset += len([]rune(text))
				if itemChanged {
					typed[index] = updated
					changed = true
				}
				continue
			}
			if rewriteResponseTextRanges(item, key, offset, ranges) {
				changed = true
			}
		}
	}
	return changed
}

func rewriteSensitiveTextSegment(text string, start int, ranges []textRangeMatch) (string, bool) {
	source := []rune(text)
	end := start + len(source)
	cursor := 0
	changed := false
	var builder strings.Builder
	for _, item := range ranges {
		if item.end <= start || item.start >= end {
			continue
		}
		localStart := max(item.start-start, 0)
		localEnd := min(item.end-start, len(source))
		if localStart > cursor {
			builder.WriteString(string(source[cursor:localStart]))
		}
		if item.start >= start && item.start < end {
			builder.WriteString(item.rule.Replacement)
		}
		if localEnd > cursor {
			cursor = localEnd
		}
		changed = true
	}
	if !changed {
		return text, false
	}
	builder.WriteString(string(source[cursor:]))
	return builder.String(), true
}

func (b *sensitiveStreamDelayBuffer) auditSnapshot(c *gin.Context) *PromptAuditSnapshot {
	text := sensitiveStreamQueueText(b.queue)
	if text == "" {
		return nil
	}
	req := securityAuditRequestMetadata(
		c,
		securityAuditProtocolFromContext(c),
		securityAuditProviderFromContext(c),
		"response_stream",
	)
	snapshot, err := BuildPromptAuditTextSnapshot(req, text)
	if err != nil {
		return nil
	}
	return &snapshot
}

func processRelayTextFields(payload any, relayFormat types.RelayFormat, processor *requestTextProcessor) {
	obj, ok := payload.(map[string]any)
	if !ok || processor == nil {
		return
	}
	switch relayFormat {
	case types.RelayFormatClaude:
		processClaudePayload(obj, processor)
	case types.RelayFormatGemini:
		processGeminiPayload(obj, processor)
	case types.RelayFormatOpenAIResponses, types.RelayFormatOpenAIResponsesCompaction:
		processResponsesPayload(obj, processor)
	case types.RelayFormatOpenAIImage:
		processStringKey(obj, "prompt", processor)
	case types.RelayFormatOpenAIAudio:
		processStringKey(obj, "input", processor)
		processStringKey(obj, "instructions", processor)
	case types.RelayFormatEmbedding:
		processStringOrStringArrayKey(obj, "input", processor)
	case types.RelayFormatRerank:
		processStringKey(obj, "query", processor)
		processRerankDocuments(obj["documents"], processor)
	default:
		processOpenAIPayload(obj, processor)
	}
}

func processOpenAIPayload(obj map[string]any, processor *requestTextProcessor) {
	processStringOrStringArrayKey(obj, "prompt", processor)
	processStringOrStringArrayKey(obj, "input", processor)
	processStringOrStringArrayKey(obj, "prefix", processor)
	processStringOrStringArrayKey(obj, "suffix", processor)
	processStringKey(obj, "instruction", processor)
	processOpenAIMessages(obj["messages"], "text", processor)
}

func processResponsesPayload(obj map[string]any, processor *requestTextProcessor) {
	processStringKey(obj, "instructions", processor)
	processStringKey(obj, "prompt", processor)
	processStringOrStringArrayKey(obj, "input", processor)
	processResponsesInput(obj["input"], processor)
}

func processClaudePayload(obj map[string]any, processor *requestTextProcessor) {
	processClaudeContent(obj, "system", processor)
	processOpenAIMessages(obj["messages"], "text", processor)
}

func processGeminiPayload(obj map[string]any, processor *requestTextProcessor) {
	processGeminiContent(obj["systemInstruction"], processor)
	processGeminiContents(obj["contents"], processor)
	processGeminiRequests(obj["requests"], processor)
}

func processOpenAIMessages(value any, textType string, processor *requestTextProcessor) {
	messages, ok := value.([]any)
	if !ok {
		return
	}
	for _, item := range messages {
		message, ok := item.(map[string]any)
		if !ok {
			continue
		}
		processTypedContent(message, "content", textType, processor)
	}
}

func processClaudeContent(obj map[string]any, key string, processor *requestTextProcessor) {
	processTypedContent(obj, key, "text", processor)
}

func processResponsesInput(value any, processor *requestTextProcessor) {
	switch input := value.(type) {
	case string:
		updated, changed := processor.process(input)
		if changed {
			// Caller holds the map for direct key updates; this branch is handled
			// by processStringKey where possible.
			_ = updated
		}
	case []any:
		for _, item := range input {
			inputItem, ok := item.(map[string]any)
			if !ok {
				continue
			}
			processTypedContent(inputItem, "content", "input_text", processor)
		}
	}
}

func processTypedContent(obj map[string]any, key string, textType string, processor *requestTextProcessor) {
	switch content := obj[key].(type) {
	case string:
		updated, changed := processor.process(content)
		if changed {
			obj[key] = updated
		}
	case []any:
		for _, partAny := range content {
			part, ok := partAny.(map[string]any)
			if !ok {
				continue
			}
			if partType, _ := part["type"].(string); partType == textType {
				processStringKey(part, "text", processor)
			}
		}
	}
}

func processGeminiRequests(value any, processor *requestTextProcessor) {
	requests, ok := value.([]any)
	if !ok {
		return
	}
	for _, item := range requests {
		if request, ok := item.(map[string]any); ok {
			processGeminiPayload(request, processor)
		}
	}
}

func processGeminiContents(value any, processor *requestTextProcessor) {
	contents, ok := value.([]any)
	if !ok {
		return
	}
	for _, item := range contents {
		processGeminiContent(item, processor)
	}
}

func processGeminiContent(value any, processor *requestTextProcessor) {
	content, ok := value.(map[string]any)
	if !ok {
		return
	}
	parts, ok := content["parts"].([]any)
	if !ok {
		return
	}
	for _, partAny := range parts {
		part, ok := partAny.(map[string]any)
		if !ok {
			continue
		}
		processStringKey(part, "text", processor)
	}
}

func processRerankDocuments(value any, processor *requestTextProcessor) {
	documents, ok := value.([]any)
	if !ok {
		return
	}
	for index, item := range documents {
		switch document := item.(type) {
		case string:
			updated, changed := processor.process(document)
			if changed {
				documents[index] = updated
			}
		case map[string]any:
			processStringKey(document, "text", processor)
		}
	}
}

func processStringOrStringArrayKey(obj map[string]any, key string, processor *requestTextProcessor) {
	switch value := obj[key].(type) {
	case string:
		updated, changed := processor.process(value)
		if changed {
			obj[key] = updated
		}
	case []any:
		for idx, item := range value {
			if str, ok := item.(string); ok {
				updated, changed := processor.process(str)
				if changed {
					value[idx] = updated
				}
			}
		}
	}
}

func processStringKey(obj map[string]any, key string, processor *requestTextProcessor) {
	value, ok := obj[key].(string)
	if !ok {
		return
	}
	updated, changed := processor.process(value)
	if changed {
		obj[key] = updated
	}
}
