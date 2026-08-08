package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const (
	PromptAuditSourceGuard          = "prompt_guard"
	PromptAuditSourceSensitiveWord  = "sensitive_word"
	PromptAuditSourceUpstreamPolicy = "upstream_policy"

	securityAuditRequestSnapshotContextKey = "security_audit_request_snapshot"
	securityAuditEventDedupeContextKey     = "security_audit_event_dedupe"
	securityAuditTokenGroupModeNone        = "none"
	upstreamCyberPolicyCode                = "cyber_policy"
)

type securityAuditEventDedupe struct {
	mu   sync.Mutex
	keys map[string]struct{}
}

// SetSecurityAuditRequestSnapshot 保存当前请求的原始文本快照，供后续屏蔽词和
// 上游安全策略事件复用。快照只在请求生命周期内存在，不会把明文写入日志或缓存。
func SetSecurityAuditRequestSnapshot(c *gin.Context, snapshot PromptAuditSnapshot) {
	if c == nil || strings.TrimSpace(snapshot.PromptHash) == "" {
		return
	}
	copy := snapshot
	copy.RequestArchive = cloneRequestArchiveRequest(snapshot.RequestArchive)
	c.Set(securityAuditRequestSnapshotContextKey, &copy)
}

func getSecurityAuditRequestSnapshot(c *gin.Context) *PromptAuditSnapshot {
	if c == nil {
		return nil
	}
	value, exists := c.Get(securityAuditRequestSnapshotContextKey)
	if !exists {
		return nil
	}
	snapshot, _ := value.(*PromptAuditSnapshot)
	if snapshot == nil {
		return nil
	}
	copy := *snapshot
	copy.RequestArchive = cloneRequestArchiveRequest(snapshot.RequestArchive)
	return &copy
}

// CaptureSecurityAuditRequestSnapshot 从尚未改写的请求正文生成内存快照。
// 提取失败不会改变转发语义；Guard 门禁仍由调用方按其模式单独处理错误。
func CaptureSecurityAuditRequestSnapshot(c *gin.Context, relayFormat types.RelayFormat, body []byte) (*PromptAuditSnapshot, error) {
	if c == nil || len(body) == 0 {
		return nil, ErrPromptAuditNoText
	}
	protocol, provider := securityAuditProtocolForRelayFormat(relayFormat)
	req := securityAuditRequestMetadata(c, protocol, provider, "request")
	req.Body = body
	snapshot, err := ExtractPromptAuditSnapshot(req)
	if err != nil {
		return nil, err
	}
	SetSecurityAuditRequestSnapshot(c, snapshot)
	return &snapshot, nil
}

// CaptureSecurityAuditRealtimeSnapshot 保存最近一个包含文本的客户端 Realtime
// 控制帧。上游随后返回 cyber_policy 时，事件可以关联到实际触发的文本。
func CaptureSecurityAuditRealtimeSnapshot(c *gin.Context, req PromptAuditRequest) (*PromptAuditSnapshot, error) {
	snapshot, err := ExtractPromptAuditSnapshot(req)
	if err != nil {
		return nil, err
	}
	SetSecurityAuditRequestSnapshot(c, snapshot)
	return &snapshot, nil
}

// RecordSensitiveWordAuditEvent 记录屏蔽词规则命中。snapshot 为空时使用当前
// 请求的原始快照；响应过滤可传入只包含命中响应文本的专用快照。
func RecordSensitiveWordAuditEvent(c *gin.Context, stage string, matches []SensitiveFilterMatch, snapshot *PromptAuditSnapshot) {
	if c == nil || len(matches) == 0 || model.DB == nil {
		return
	}
	cfg, err := GetPromptAuditConfig(context.Background())
	if cfg == nil || !cfg.SensitiveWordAuditEnabled {
		return
	}
	if err != nil {
		// 可用旧快照仍允许记录；配置完全不可用时 cfg 已为 nil。
		logger.LogWarn(c, "安全审计配置使用缓存快照记录屏蔽词事件")
	}
	stage = normalizeSecurityAuditStage(stage, "request")
	matchIDs := securityAuditMatchIdentifiers(matches)
	dedupeKey := PromptAuditSourceSensitiveWord + ":" + stage + ":" + strings.Join(matchIDs, ",")
	if shouldDedupeSecurityAuditStage(stage) && !claimSecurityAuditEvent(c, dedupeKey) {
		return
	}
	if snapshot == nil {
		snapshot = getSecurityAuditRequestSnapshot(c)
	}
	if snapshot == nil {
		snapshot = captureSecurityAuditEventSnapshot(c, stage)
	}

	action := "Mask"
	decision, riskLevel, riskScore := "flag", "medium", 0.5
	for _, match := range matches {
		if strings.EqualFold(match.Action, "block") {
			action = "Block"
			decision, riskLevel, riskScore = "critical", "critical", 1
			break
		}
	}
	event := buildBuiltinSecurityAuditEvent(c, cfg, snapshot, PromptAuditSourceSensitiveWord, stage)
	event.Decision = decision
	event.RiskLevel = riskLevel
	event.RiskScore = riskScore
	event.Action = action
	event.Safety = "Unsafe"
	event.Categories = marshalSecurityAuditStrings([]string{"sensitive_word"})
	event.MatchedScanners = marshalSecurityAuditStrings(matchIDs)
	if storedKeywords, storeErr := StorePromptAuditMatchedKeywords(securityAuditMatchedKeywords(matches)); storeErr != nil {
		logger.LogError(c, "保存安全审计命中词失败: "+storeErr.Error())
	} else {
		event.MatchedKeywordsCiphertext = storedKeywords
	}
	promptAuditStats.total.Add(1)
	if action == "Block" {
		promptAuditStats.blocked.Add(1)
	} else {
		promptAuditStats.flagged.Add(1)
	}
	persistBuiltinSecurityAuditEvent(c, event)
}

// RecordUpstreamPolicyPayload 只匹配结构化 JSON 的固定路径，不扫描错误文案。
func RecordUpstreamPolicyPayload(c *gin.Context, payload []byte, stage string) bool {
	if !IsUpstreamCyberPolicyPayload(payload) {
		return false
	}
	MarkContentPolicyRejected(c)
	recordUpstreamPolicyEvent(c, stage)
	return true
}

// RecordUpstreamPolicyError 只接受由结构化上游错误转换出的 cyber_policy code。
func RecordUpstreamPolicyError(c *gin.Context, relayErr *types.NewAPIError, stage string) bool {
	if !IsUpstreamCyberPolicyError(relayErr) {
		return false
	}
	MarkContentPolicyRejected(c)
	recordUpstreamPolicyEvent(c, stage)
	return true
}

// RecordUpstreamPolicyCode 用于任务类适配器已经结构化解析出的错误码。
func RecordUpstreamPolicyCode(c *gin.Context, code string, stage string) bool {
	if !strings.EqualFold(strings.TrimSpace(code), upstreamCyberPolicyCode) {
		return false
	}
	MarkContentPolicyRejected(c)
	recordUpstreamPolicyEvent(c, stage)
	return true
}

func recordUpstreamPolicyEvent(c *gin.Context, stage string) {
	if c == nil {
		return
	}
	cfg, err := GetPromptAuditConfig(context.Background())
	if model.DB == nil {
		return
	}
	if cfg == nil || !cfg.UpstreamPolicyEnabled {
		return
	}
	if err != nil {
		logger.LogWarn(c, "安全审计配置使用缓存快照记录上游策略事件")
	}
	if !upstreamPolicyScopeIncludesSelectedChannel(c, cfg) {
		return
	}
	stage = normalizeSecurityAuditStage(stage, "response")
	// HTTP/SSE 的同一次上游拒绝可能先由流式响应识别，随后又在控制器的
	// 结构化错误转换中识别。非 Realtime 统一占用请求级键，避免跨阶段重复计数。
	if shouldDedupeSecurityAuditStage(stage) && !claimSecurityAuditEvent(c, PromptAuditSourceUpstreamPolicy) {
		return
	}
	snapshot := getSecurityAuditRequestSnapshot(c)
	if snapshot == nil {
		snapshot = captureSecurityAuditEventSnapshot(c, stage)
	}
	event := buildBuiltinSecurityAuditEvent(c, cfg, snapshot, PromptAuditSourceUpstreamPolicy, stage)
	event.Decision = "critical"
	event.RiskLevel = "critical"
	event.RiskScore = 1
	event.Action = "Block"
	event.Safety = "Unsafe"
	event.Categories = marshalSecurityAuditStrings([]string{upstreamCyberPolicyCode})
	event.MatchedScanners = marshalSecurityAuditStrings([]string{PromptAuditSourceUpstreamPolicy})
	event.ErrorCode = upstreamCyberPolicyCode
	event.ErrorMessage = "上游安全策略拒绝请求"
	promptAuditStats.total.Add(1)
	promptAuditStats.blocked.Add(1)
	if persistBuiltinSecurityAuditEvent(c, event) {
		applyCyberPolicyAutoBan(c, cfg, event)
	}
}

func upstreamPolicyScopeIncludesSelectedChannel(c *gin.Context, cfg *PromptAuditConfig) bool {
	if cfg == nil {
		return false
	}
	switch cfg.UpstreamPolicyTargetType {
	case "", PromptAuditUpstreamPolicyTargetAll:
		return true
	case PromptAuditUpstreamPolicyTargetChannels:
		channelId, _ := selectedSecurityAuditChannel(c)
		index := sort.SearchInts(cfg.UpstreamPolicyChannelIds, channelId)
		return channelId > 0 && index < len(cfg.UpstreamPolicyChannelIds) && cfg.UpstreamPolicyChannelIds[index] == channelId
	case PromptAuditUpstreamPolicyTargetGroups:
		groupCode := selectedSecurityAuditGroupCode(c)
		if groupCode == "" {
			return false
		}
		included, err := promptAuditGroupCodesInclude(cfg.UpstreamPolicyGroupCodes, groupCode)
		if err != nil {
			logger.LogWarn(c, "解析安全审计官方风控分组范围失败: "+err.Error())
			return false
		}
		return included
	}
	return false
}

func promptAuditGroupCodesInclude(groupCodes []string, groupCode string) (bool, error) {
	groupCode = strings.TrimSpace(groupCode)
	if groupCode == "" || len(groupCodes) == 0 {
		return false, nil
	}
	index := sort.SearchStrings(groupCodes, groupCode)
	if index < len(groupCodes) && groupCodes[index] == groupCode {
		return true, nil
	}
	expanded, err := model.ExpandPromptAuditGroupIdentifiers(groupCodes)
	if err != nil {
		return false, err
	}
	index = sort.SearchStrings(expanded, groupCode)
	return index < len(expanded) && expanded[index] == groupCode, nil
}

func selectedSecurityAuditGroupCode(c *gin.Context) string {
	if c == nil {
		return ""
	}
	groupCode := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeySelectedChannelGroup))
	if groupCode == "" {
		return ""
	}
	if group, err := model.GetGroupByCodeOrAlias(groupCode); err == nil && group != nil {
		return strings.TrimSpace(group.Code)
	}
	return groupCode
}

func selectedSecurityAuditChannel(c *gin.Context) (int, *model.Channel) {
	if c == nil {
		return 0, nil
	}
	if channel, ok := common.GetContextKeyType[*model.Channel](c, constant.ContextKeySelectedChannel); ok && channel != nil {
		return channel.Id, channel
	}
	return common.GetContextKeyInt(c, constant.ContextKeyChannelId), nil
}

func securityAuditChannelMetadata(c *gin.Context, stage string) (int, string, []model.PromptAuditEventChannelGroup) {
	groups := make([]model.PromptAuditEventChannelGroup, 0)
	if c == nil {
		return 0, "", groups
	}

	var channel *model.Channel
	if selected, ok := common.GetContextKeyType[*model.Channel](c, constant.ContextKeySelectedChannel); ok && selected != nil {
		channel = selected
	}
	channelId := 0
	if channel != nil {
		channelId = channel.Id
	} else if securityAuditUsesSelectedChannelGroup(stage) {
		channelId = common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	} else {
		// 请求阶段只有固定渠道令牌能在分配前提供确定渠道；普通用户分组、
		// 候选渠道或历史 channel_id 都不能冒充本次实际渠道。
		channelId = sensitiveFixedChannelId(c)
	}
	if channelId <= 0 {
		return 0, "", groups
	}

	channelName := ""
	var groupDetails []model.GroupReference
	if channel != nil && channel.Id == channelId {
		channelName = strings.TrimSpace(channel.Name)
		groupDetails = channel.GroupDetails
	}
	if model.DB != nil && (channel == nil || channel.Id != channelId || groupDetails == nil || channelName == "") {
		if loaded, err := model.GetChannelById(channelId, false); err == nil && loaded != nil {
			if channel == nil || channel.Id != channelId {
				channel = loaded
			}
			if groupDetails == nil {
				groupDetails = loaded.GroupDetails
			}
			if channelName == "" {
				channelName = strings.TrimSpace(loaded.Name)
			}
		}
	}

	if channel != nil && channel.Id == channelId {
		seen := make(map[int]struct{}, len(groupDetails))
		for _, detail := range groupDetails {
			if detail.Id <= 0 {
				continue
			}
			if _, exists := seen[detail.Id]; exists {
				continue
			}
			seen[detail.Id] = struct{}{}
			groups = append(groups, model.PromptAuditEventChannelGroup{
				Id: detail.Id, Code: strings.TrimSpace(detail.Code), Name: strings.TrimSpace(detail.Name),
			})
		}
	}
	if channelName == "" && common.GetContextKeyInt(c, constant.ContextKeyChannelId) == channelId {
		channelName = strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyChannelName))
	}
	return channelId, channelName, groups
}

func securityAuditTokenGroupCodes(value string) []string {
	parts := strings.Split(value, ",")
	codes := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		code := strings.TrimSpace(part)
		key := code
		if code == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		codes = append(codes, code)
	}
	return codes
}

func securityAuditTokenGroupMetadata(c *gin.Context) (string, []model.PromptAuditEventTokenGroup) {
	groups := make([]model.PromptAuditEventTokenGroup, 0)
	if c == nil {
		return "", groups
	}
	// Canvas、Playground 等站内入口使用 Id=0 的临时令牌承载路由分组。
	// 这些请求没有真实令牌，不能把临时路由分组冒充为令牌绑定分组。
	if common.GetContextKeyInt(c, constant.ContextKeyTokenId) <= 0 {
		return securityAuditTokenGroupModeNone, groups
	}

	mode := strings.ToLower(strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyTokenGroupMode)))
	legacy := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyTokenGroup))
	ids, _ := common.GetContextKeyType[[]int](c, constant.ContextKeyTokenGroupIds)
	ids = append([]int(nil), ids...)
	if mode == "" {
		switch {
		case strings.EqualFold(legacy, model.TokenGroupModeAuto):
			mode = model.TokenGroupModeAuto
		case len(ids) > 0 || legacy != "":
			mode = model.TokenGroupModeExplicit
		default:
			mode = model.TokenGroupModeInherit
		}
	}
	if mode == model.TokenGroupModeAuto {
		return mode, groups
	}
	if mode == model.TokenGroupModeExplicit {
		details, _ := common.GetContextKeyType[[]model.GroupReference](c, constant.ContextKeyTokenGroupDetails)
		if len(details) > 0 {
			seen := make(map[int]struct{}, len(details))
			for _, detail := range details {
				if detail.Id <= 0 {
					continue
				}
				if _, exists := seen[detail.Id]; exists {
					continue
				}
				seen[detail.Id] = struct{}{}
				groups = append(groups, model.PromptAuditEventTokenGroup{
					Id: detail.Id, Code: strings.TrimSpace(detail.Code), Name: strings.TrimSpace(detail.Name),
				})
			}
			return mode, groups
		}
	}

	codes := securityAuditTokenGroupCodes(legacy)
	if mode == model.TokenGroupModeInherit {
		ids = []int{common.GetContextKeyInt(c, constant.ContextKeyUserGroupId)}
		codes = securityAuditTokenGroupCodes(common.GetContextKeyString(c, constant.ContextKeyUserGroup))
	}
	if mode != model.TokenGroupModeExplicit && mode != model.TokenGroupModeInherit {
		return mode, groups
	}

	loaded := make(map[int]*model.Group)
	// 正常鉴权上下文已经携带结构化令牌分组详情。数据库查询只为旧上下文
	// 的显式令牌兜底，避免安全审计中间件给每个继承令牌请求增加查询。
	if mode == model.TokenGroupModeExplicit && model.DB != nil && len(ids) > 0 {
		if resolved, err := model.GetGroupsByIds(ids); err == nil {
			loaded = resolved
		}
	}
	seenIds := make(map[int]struct{}, len(ids))
	seenCodes := make(map[string]struct{}, len(codes))
	for index, id := range ids {
		if id <= 0 {
			continue
		}
		if _, exists := seenIds[id]; exists {
			continue
		}
		seenIds[id] = struct{}{}
		if group := loaded[id]; group != nil {
			groups = append(groups, model.PromptAuditEventTokenGroup{Id: group.Id, Code: strings.TrimSpace(group.Code), Name: strings.TrimSpace(group.Name)})
			seenCodes[strings.TrimSpace(group.Code)] = struct{}{}
			continue
		}
		code := ""
		if index < len(codes) {
			code = codes[index]
			seenCodes[code] = struct{}{}
		}
		groups = append(groups, model.PromptAuditEventTokenGroup{Id: id, Code: code})
	}
	for _, code := range codes {
		key := code
		if _, exists := seenCodes[key]; exists {
			continue
		}
		seenCodes[key] = struct{}{}
		if mode == model.TokenGroupModeExplicit && model.DB != nil {
			if group, err := model.GetGroupByCodeOrAlias(code); err == nil && group != nil {
				if _, exists := seenIds[group.Id]; exists {
					continue
				}
				seenIds[group.Id] = struct{}{}
				groups = append(groups, model.PromptAuditEventTokenGroup{Id: group.Id, Code: strings.TrimSpace(group.Code), Name: strings.TrimSpace(group.Name)})
				continue
			}
		}
		groups = append(groups, model.PromptAuditEventTokenGroup{Code: code})
	}
	return mode, groups
}

func normalizePromptAuditGroupCode(groupCode string) string {
	normalized, err := model.NormalizeGroupCode(groupCode)
	if err != nil {
		return ""
	}
	return normalized
}

// hydratePromptAuditEventGroupCode 只在事件确定要持久化时解析旧别名或旧快照。
// 普通请求快照不得通过显示名称猜测编码，也不会因此增加热路径数据库查询。
func hydratePromptAuditEventGroupCode(event *model.PromptAuditEvent) {
	if event == nil {
		return
	}
	explicit := normalizePromptAuditGroupCode(event.GroupCode)
	if explicit != "" {
		event.GroupCode = explicit
		if model.DB == nil {
			return
		}
		if group, err := model.GetGroupByCodeOrAlias(explicit); err == nil && group != nil {
			event.GroupId = group.Id
			event.GroupCode = strings.TrimSpace(group.Code)
			return
		}
	}
	if event.GroupId > 0 && model.DB != nil {
		if groups, err := model.GetGroupsByIds([]int{event.GroupId}); err == nil {
			if group := groups[event.GroupId]; group != nil {
				event.GroupCode = strings.TrimSpace(group.Code)
				return
			}
		}
	}
	if explicit != "" {
		return
	}
	event.GroupCode = ""

	const preallocationPrefix = "pre_allocation:"
	candidate := strings.TrimSpace(event.GroupName)
	if model.DB == nil || !strings.HasPrefix(strings.ToLower(candidate), preallocationPrefix) {
		return
	}
	candidate = strings.TrimSpace(candidate[len(preallocationPrefix):])
	group, err := model.GetGroupByCodeOrAlias(candidate)
	if err != nil || group == nil {
		return
	}
	event.GroupId = group.Id
	event.GroupCode = strings.TrimSpace(group.Code)
}

// PopulatePromptAuditRequestRoutingMetadata 把当前实际渠道、业务分组和令牌分组固化到
// Guard 请求快照。请求发生在选渠前时渠道保持零值，选渠后的 Realtime 帧使用当前渠道。
func PopulatePromptAuditRequestRoutingMetadata(c *gin.Context, req *PromptAuditRequest) {
	if req == nil {
		return
	}
	req.GroupCode = normalizePromptAuditGroupCode(req.GroupCode)
	req.ChannelId, req.ChannelName, req.ChannelGroups = securityAuditChannelMetadata(c, req.Stage)
	req.TokenGroupMode, req.TokenGroups = securityAuditTokenGroupMetadata(c)
}

// IsUpstreamCyberPolicyPayload 精确支持普通 OpenAI 错误和 Responses
// response.failed 两种结构，不递归扫描任意 code 字段。
func IsUpstreamCyberPolicyPayload(payload []byte) bool {
	trimmed := strings.TrimSpace(string(payload))
	if trimmed == "" || trimmed == "[DONE]" {
		return false
	}
	var root map[string]interface{}
	if err := common.Unmarshal([]byte(trimmed), &root); err != nil {
		return false
	}
	if securityAuditObjectErrorCode(root["error"]) == upstreamCyberPolicyCode {
		return true
	}
	response, ok := root["response"].(map[string]interface{})
	if !ok {
		return false
	}
	return securityAuditObjectErrorCode(response["error"]) == upstreamCyberPolicyCode
}

func IsUpstreamCyberPolicyError(relayErr *types.NewAPIError) bool {
	if relayErr == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(string(relayErr.GetErrorCode())), upstreamCyberPolicyCode) {
		return true
	}
	switch typed := relayErr.RelayError.(type) {
	case types.OpenAIError:
		return strings.EqualFold(strings.TrimSpace(fmt.Sprint(typed.Code)), upstreamCyberPolicyCode)
	case *types.OpenAIError:
		return typed != nil && strings.EqualFold(strings.TrimSpace(fmt.Sprint(typed.Code)), upstreamCyberPolicyCode)
	default:
		return false
	}
}

func securityAuditObjectErrorCode(value interface{}) string {
	object, ok := value.(map[string]interface{})
	if !ok {
		return ""
	}
	code, ok := object["code"].(string)
	if !ok {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(code))
}

func buildBuiltinSecurityAuditEvent(c *gin.Context, cfg *PromptAuditConfig, snapshot *PromptAuditSnapshot, source, stage string) *model.PromptAuditEvent {
	now := time.Now().Unix()
	retentionDays := 30
	configVersion := int64(0)
	if cfg != nil {
		if cfg.RetentionDays > 0 {
			retentionDays = cfg.RetentionDays
		}
		configVersion = cfg.ConfigVersion
	}
	stage = normalizeSecurityAuditStage(stage, "request")
	groupId, groupCode, groupName := securityAuditGroupMetadata(c, stage)
	channelId, channelName, channelGroups := securityAuditChannelMetadata(c, stage)
	tokenGroupMode, tokenGroups := securityAuditTokenGroupMetadata(c)
	event := &model.PromptAuditEvent{
		RequestId:         c.GetString(common.RequestIdKey),
		UserId:            common.GetContextKeyInt(c, constant.ContextKeyUserId),
		Username:          common.GetContextKeyString(c, constant.ContextKeyUserName),
		UserEmail:         common.GetContextKeyString(c, constant.ContextKeyUserEmail),
		TokenId:           common.GetContextKeyInt(c, constant.ContextKeyTokenId),
		TokenName:         c.GetString("token_name"),
		GroupId:           groupId,
		GroupCode:         groupCode,
		GroupName:         groupName,
		ChannelId:         channelId,
		ChannelName:       channelName,
		ChannelGroups:     channelGroups,
		TokenGroupMode:    tokenGroupMode,
		TokenGroups:       tokenGroups,
		Provider:          securityAuditProviderFromContext(c),
		Endpoint:          securityAuditEndpointFromContext(c),
		Protocol:          securityAuditProtocolFromContext(c),
		Model:             common.GetContextKeyString(c, constant.ContextKeyOriginalModel),
		PromptCipherKind:  model.PromptAuditCipherKindPrompt,
		PromptAvailable:   false,
		Source:            source,
		Stage:             stage,
		Categories:        "[]",
		MatchedScanners:   "[]",
		UnknownCategories: "[]",
		ConfigVersion:     configVersion,
		CreatedAt:         now,
		ExpiresAt:         now + int64(retentionDays)*24*60*60,
	}
	if snapshot != nil {
		if segments, err := StorePromptAuditContextSegments(snapshot.ContextSegments); err == nil {
			event.ContextSegments = segments
		}
		event.RequestId = defaultSecurityAuditString(snapshot.RequestId, event.RequestId)
		event.UserId = defaultSecurityAuditInt(snapshot.UserId, event.UserId)
		event.Username = defaultSecurityAuditString(snapshot.Username, event.Username)
		event.UserEmail = defaultSecurityAuditString(snapshot.UserEmail, event.UserEmail)
		event.TokenId = defaultSecurityAuditInt(snapshot.TokenId, event.TokenId)
		event.TokenName = defaultSecurityAuditString(snapshot.TokenName, event.TokenName)
		if event.TokenGroupMode == "" {
			event.TokenGroupMode = snapshot.TokenGroupMode
		}
		if len(event.TokenGroups) == 0 && len(snapshot.TokenGroups) > 0 {
			event.TokenGroups = append([]model.PromptAuditEventTokenGroup(nil), snapshot.TokenGroups...)
		}
		if !securityAuditUsesSelectedChannelGroup(stage) {
			event.GroupId = defaultSecurityAuditInt(snapshot.GroupId, event.GroupId)
			event.GroupCode = defaultSecurityAuditString(snapshot.GroupCode, event.GroupCode)
			event.GroupName = defaultSecurityAuditString(snapshot.GroupName, event.GroupName)
		}
		event.GroupCode = normalizePromptAuditGroupCode(event.GroupCode)
		// 响应阶段的当前上下文代表最终实际渠道，不能被请求快照中的首次
		// 渠道覆盖。只有当前上下文没有渠道时才使用快照兜底。
		if event.ChannelId <= 0 && snapshot.ChannelId > 0 {
			event.ChannelId = snapshot.ChannelId
			event.ChannelName = snapshot.ChannelName
		} else if event.ChannelName == "" && event.ChannelId == snapshot.ChannelId {
			event.ChannelName = snapshot.ChannelName
		}
		event.Provider = defaultSecurityAuditString(snapshot.Provider, event.Provider)
		event.Endpoint = defaultSecurityAuditString(snapshot.Endpoint, event.Endpoint)
		event.Protocol = defaultSecurityAuditString(snapshot.Protocol, event.Protocol)
		event.Model = defaultSecurityAuditString(snapshot.Model, event.Model)
		event.PromptHash = snapshot.PromptHash
		event.PromptLength = snapshot.PromptLength
		event.PromptTruncated = snapshot.PromptTruncated
		event.MessageCount = snapshot.MessageCount
		if strings.TrimSpace(snapshot.FullPrompt) != "" {
			if stored, cipherKind, err := StorePromptAuditSecret(snapshot.FullPrompt); err == nil && stored != "" {
				event.PromptCiphertext = model.PromptAuditLargeText(stored)
				event.PromptCipherKind = cipherKind
				event.RedactedPreview = snapshot.RedactedPreview
				event.PromptAvailable = true
			}
		}
	}
	return event
}

func persistBuiltinSecurityAuditEvent(c *gin.Context, event *model.PromptAuditEvent) bool {
	if event == nil {
		return false
	}
	hydratePromptAuditEventGroupCode(event)
	if err := model.CreatePromptAuditEvent(event); err != nil {
		promptAuditStats.recordFailed.Add(1)
		logger.LogError(c, "写入安全审计事件失败: "+err.Error())
		return false
	}
	QueuePendingRequestArchiveForAuditEvent(c, event)
	return true
}

func applyCyberPolicyAutoBan(c *gin.Context, cfg *PromptAuditConfig, event *model.PromptAuditEvent) {
	if cfg == nil || event == nil || !cfg.CyberPolicyAutoBanEnabled || event.UserId <= 0 ||
		event.Source != PromptAuditSourceUpstreamPolicy || event.ErrorCode != upstreamCyberPolicyCode {
		return
	}
	if err := validateCyberPolicyAutoBanConfig(cfg.CyberPolicyBanThreshold, cfg.CyberPolicyWindowHours); err != nil {
		logger.LogError(c, "cyber_policy 自动封禁配置无效: "+err.Error())
		return
	}
	scope, err := BuildPromptAuditCyberPolicyScope(cfg)
	if err != nil {
		logger.LogError(c, "cyber_policy 自动封禁作用范围无效: "+err.Error())
		return
	}
	if len(scope.ExemptGroupCodes) > 0 {
		groupCode := strings.TrimSpace(event.GroupCode)
		// 当白名单开启时，当前事件必须能明确证明来自非白名单分组；
		// 否则只留存审计记录，不触发惩罚性处置。
		if groupCode == "" {
			return
		}
		exempt, matchErr := promptAuditGroupCodesInclude(scope.ExemptGroupCodes, groupCode)
		if matchErr != nil {
			logger.LogError(c, "解析 cyber_policy 自动封禁分组白名单失败: "+matchErr.Error())
			return
		}
		if exempt {
			return
		}
	}
	until := time.Now().Unix()
	since := until - int64(cfg.CyberPolicyWindowHours)*int64(time.Hour/time.Second)
	count, disabled, err := model.DisableCommonUserOnCyberPolicyThreshold(
		event.UserId, since, until, cfg.CyberPolicyBanThreshold, scope,
	)
	if err != nil {
		logger.LogError(c, "cyber_policy 自动封禁执行失败: "+err.Error())
		return
	}
	if !disabled {
		return
	}
	if err := model.InvalidateUserCache(event.UserId); err != nil {
		common.SysLog(fmt.Sprintf("cyber_policy 自动封禁后清理用户 %d 缓存失败: %s", event.UserId, err.Error()))
	}
	if err := model.InvalidateUserTokensCache(event.UserId); err != nil {
		common.SysLog(fmt.Sprintf("cyber_policy 自动封禁后清理用户 %d 令牌缓存失败: %s", event.UserId, err.Error()))
	}
	model.RecordLogWithAdminInfo(event.UserId, model.LogTypeManage,
		"安全审计检测到 cyber_policy 达到阈值，已自动禁用用户", map[string]interface{}{
			"admin_id":                      0,
			"admin_username":                "security_audit",
			"action":                        "cyber_policy_auto_ban",
			"event_id":                      event.Id,
			"violation_count":               count,
			"ban_threshold":                 cfg.CyberPolicyBanThreshold,
			"violation_window_hours":        cfg.CyberPolicyWindowHours,
			"exempt_group_count":            len(scope.ExemptGroupCodes),
			"security_audit_config_version": cfg.ConfigVersion,
		})
}

func claimSecurityAuditEvent(c *gin.Context, key string) bool {
	if c == nil || strings.TrimSpace(key) == "" {
		return false
	}
	value, exists := c.Get(securityAuditEventDedupeContextKey)
	state, _ := value.(*securityAuditEventDedupe)
	if !exists || state == nil {
		state = &securityAuditEventDedupe{keys: make(map[string]struct{})}
		c.Set(securityAuditEventDedupeContextKey, state)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if _, duplicate := state.keys[key]; duplicate {
		return false
	}
	state.keys[key] = struct{}{}
	return true
}

// HTTP/SSE 的同一内容可能经过多层错误转换或重复流式片段，因此继续按请求
// 上下文去重。Realtime 的同一个 Gin 上下文覆盖整条 WebSocket 连接，逐帧命中
// 必须分别留痕，不能让首帧占用连接级去重键后吞掉后续事件。
func shouldDedupeSecurityAuditStage(stage string) bool {
	return !strings.HasPrefix(strings.ToLower(strings.TrimSpace(stage)), "realtime_")
}

func securityAuditMatchIdentifiers(matches []SensitiveFilterMatch) []string {
	seen := make(map[string]struct{}, len(matches))
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		identifier := strings.TrimSpace(match.RuleID)
		if identifier == "" {
			identifier = strings.TrimSpace(match.RuleName)
		}
		if identifier == "" {
			identifier = "unnamed_rule"
		}
		identifier = "rule:" + trimSecurityAuditRunes(identifier, 128)
		if _, duplicate := seen[identifier]; duplicate {
			continue
		}
		seen[identifier] = struct{}{}
		result = append(result, identifier)
	}
	sort.Strings(result)
	return result
}

func securityAuditMatchedKeywords(matches []SensitiveFilterMatch) []string {
	seen := make(map[string]struct{}, len(matches))
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		keyword := strings.TrimSpace(match.Keyword)
		if keyword == "" {
			continue
		}
		if _, duplicate := seen[keyword]; duplicate {
			continue
		}
		seen[keyword] = struct{}{}
		result = append(result, keyword)
	}
	return result
}

func marshalSecurityAuditStrings(values []string) string {
	encoded, err := common.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func securityAuditRequestMetadata(c *gin.Context, protocol, provider, stage string) PromptAuditRequest {
	req := PromptAuditRequest{Protocol: protocol, Provider: provider, Stage: stage}
	if c == nil {
		return req
	}
	req.RequestId = c.GetString(common.RequestIdKey)
	req.UserId = common.GetContextKeyInt(c, constant.ContextKeyUserId)
	req.Username = common.GetContextKeyString(c, constant.ContextKeyUserName)
	req.UserEmail = common.GetContextKeyString(c, constant.ContextKeyUserEmail)
	req.TokenId = common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	req.TokenName = c.GetString("token_name")
	req.GroupId, req.GroupCode, req.GroupName = securityAuditGroupMetadata(c, stage)
	PopulatePromptAuditRequestRoutingMetadata(c, &req)
	req.Model = common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
	if c.Request != nil && c.Request.URL != nil {
		req.Endpoint = c.Request.URL.Path
		if req.Model == "" {
			req.Model = c.Query("model")
		}
	}
	return req
}

// captureSecurityAuditEventSnapshot 是事件写入前的兜底快照。部分历史路由只挂载
// 敏感词过滤或上游错误处理，没有经过 PromptAudit 中间件；这时仍从可复用请求体
// 读取原始文本，补齐正文和模型，避免事件退化成“未保存正文”和模型横杠。
func captureSecurityAuditEventSnapshot(c *gin.Context, stage string) *PromptAuditSnapshot {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return nil
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil
	}
	body, err := storage.Bytes()
	if err != nil || len(strings.TrimSpace(string(body))) == 0 {
		return nil
	}
	req := securityAuditRequestMetadata(c, securityAuditProtocolFromContext(c), securityAuditProviderFromContext(c), stage)
	req.Body = body
	var document map[string]interface{}
	if err := common.Unmarshal(body, &document); err == nil {
		if modelName, ok := document["model"].(string); ok {
			req.Model = strings.TrimSpace(modelName)
		}
		if req.Model == "" {
			if modelName, ok := document["model_name"].(string); ok {
				req.Model = strings.TrimSpace(modelName)
			}
		}
	}
	snapshot, err := ExtractPromptAuditSnapshot(req)
	if err != nil {
		return nil
	}
	SetSecurityAuditRequestSnapshot(c, snapshot)
	return &snapshot
}

func securityAuditUsesSelectedChannelGroup(stage string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(stage)), "response")
}

func securityAuditGroupMetadata(c *gin.Context, stage string) (int, string, string) {
	if c == nil {
		return 0, "", ""
	}
	userGroupId := common.GetContextKeyInt(c, constant.ContextKeyUserGroupId)
	userGroup := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyUserGroup))
	candidate := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
	fallbackId := userGroupId
	selectedGroup := false
	if candidate == "" {
		candidate = userGroup
	}

	if securityAuditUsesSelectedChannelGroup(stage) {
		selected := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeySelectedChannelGroup))
		if selected != "" {
			candidate = selected
			fallbackId = 0
			selectedGroup = true
		}
	}
	if model.DB != nil && candidate != "" {
		if group, err := model.GetGroupByCodeOrAlias(candidate); err == nil && group != nil {
			return group.Id, strings.TrimSpace(group.Code), group.Name
		}
	}
	if selectedGroup {
		return 0, normalizePromptAuditGroupCode(candidate), candidate
	}
	if fallbackId > 0 && strings.EqualFold(candidate, userGroup) {
		return fallbackId, "", candidate
	}
	return 0, "", candidate
}
func buildSecurityAuditResponseSnapshot(c *gin.Context, payload interface{}, stage string) *PromptAuditSnapshot {
	texts := collectResponseTextFields(payload)
	if len(texts) == 0 {
		return nil
	}
	protocol := securityAuditProtocolFromContext(c)
	provider := securityAuditProviderFromContext(c)
	req := securityAuditRequestMetadata(c, protocol, provider, stage)
	snapshot, err := BuildPromptAuditTextSnapshot(req, strings.Join(texts, "\n\n"))
	if err != nil {
		return nil
	}
	return &snapshot
}

func securityAuditProtocolForRelayFormat(relayFormat types.RelayFormat) (string, string) {
	switch relayFormat {
	case types.RelayFormatClaude:
		return "anthropic_messages", "anthropic"
	case types.RelayFormatGemini:
		return "gemini", "gemini"
	case types.RelayFormatOpenAIResponses, types.RelayFormatOpenAIResponsesCompaction:
		return "openai_responses", "openai"
	case types.RelayFormatOpenAIRealtime:
		return "openai_realtime", "openai"
	case types.RelayFormatEmbedding:
		return "embedding", "openai"
	case types.RelayFormatRerank:
		return "rerank", "openai"
	case types.RelayFormatOpenAIImage:
		return "openai_images", "openai"
	case types.RelayFormatOpenAIAudio:
		return "audio", "openai"
	case types.RelayFormatTask:
		return "task", "task"
	default:
		return "openai_chat_completions", "openai"
	}
}

func securityAuditProviderFromContext(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if channelName := common.GetContextKeyString(c, constant.ContextKeyChannelName); channelName != "" {
		return trimSecurityAuditRunes(channelName, 64)
	}
	return ""
}

func securityAuditEndpointFromContext(c *gin.Context) string {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return ""
	}
	return trimSecurityAuditRunes(c.Request.URL.Path, 255)
}

func securityAuditProtocolFromContext(c *gin.Context) string {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return ""
	}
	path := strings.ToLower(c.Request.URL.Path)
	switch {
	case strings.Contains(path, "/realtime"):
		return "openai_realtime"
	case strings.Contains(path, "/responses"):
		return "openai_responses"
	case strings.Contains(path, "/messages"):
		return "anthropic_messages"
	case strings.Contains(path, "/v1beta/models/") || strings.Contains(path, "/v1/models/"):
		return "gemini"
	default:
		return "openai_chat_completions"
	}
}

func normalizeSecurityAuditStage(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		value = fallback
	}
	return trimSecurityAuditRunes(value, 32)
}

func defaultSecurityAuditString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func defaultSecurityAuditInt(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}

func trimSecurityAuditRunes(value string, limit int) string {
	runes := []rune(value)
	if limit <= 0 || len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
