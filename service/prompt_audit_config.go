package service

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	PromptAuditUpstreamPolicyTargetAll      = "all"
	PromptAuditUpstreamPolicyTargetChannels = "channels"
	PromptAuditUpstreamPolicyTargetGroups   = "groups"
	PromptAuditPolicySourceCyber            = "cyber_policy"
	PromptAuditPolicySourceBiologicalRisk   = "biological_risk"
)

var promptAuditPolicyActionSourceSet = map[string]struct{}{
	PromptAuditPolicySourceCyber:          {},
	PromptAuditPolicySourceBiologicalRisk: {},
}

func canonicalPromptAuditPolicyActionSources(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		source := strings.ToLower(strings.TrimSpace(value))
		if _, ok := promptAuditPolicyActionSourceSet[source]; !ok {
			continue
		}
		seen[source] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for source := range seen {
		result = append(result, source)
	}
	sort.Strings(result)
	return result
}

func promptAuditPolicyActionSourcesFromModel(row *model.PromptAuditConfig) ([]string, error) {
	if row == nil {
		return []string{PromptAuditPolicySourceCyber}, nil
	}
	values := []string{}
	if strings.TrimSpace(row.PolicyActionSources) != "" {
		if err := common.UnmarshalJsonStr(row.PolicyActionSources, &values); err != nil {
			return nil, errors.New("安全审计策略处置来源配置无效")
		}
		for _, value := range values {
			if value = strings.ToLower(strings.TrimSpace(value)); value == "" {
				continue
			}
			if _, ok := promptAuditPolicyActionSourceSet[value]; !ok {
				return nil, fmt.Errorf("安全审计策略处置来源配置无效：%s", value)
			}
		}
	}
	values = canonicalPromptAuditPolicyActionSources(values)
	if len(values) == 0 {
		// 旧版本没有来源列时保持原 cyber_policy 语义；显式保存的 []
		// 表示管理员关闭了所有处置来源，不能被旧默认值覆盖。
		if strings.TrimSpace(row.PolicyActionSources) == "" {
			values = []string{PromptAuditPolicySourceCyber}
		}
	}
	return values, nil
}

func promptAuditPolicyActionSourcesInclude(values []string, source string) bool {
	source = strings.ToLower(strings.TrimSpace(source))
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), source) {
			return true
		}
	}
	return false
}

func promptAuditPolicyActionEnabled(cfg *PromptAuditConfig, source string) bool {
	if cfg == nil {
		return false
	}
	values := cfg.PolicyActionSources
	if values == nil {
		// 兼容旧版本构造的配置快照：未提供来源字段等同于仅启用 cyber_policy。
		return source == PromptAuditPolicySourceCyber
	}
	return promptAuditPolicyActionSourcesInclude(values, source)
}

func normalizePromptAuditPolicyActionSources(values []string) ([]string, error) {
	result := canonicalPromptAuditPolicyActionSources(values)
	if len(result) != len(values) {
		// 允许空数组表示不对任何上游策略执行处置；但显式未知值必须拒绝。
		for _, value := range values {
			if strings.TrimSpace(value) == "" {
				continue
			}
			if _, ok := promptAuditPolicyActionSourceSet[strings.ToLower(strings.TrimSpace(value))]; !ok {
				return nil, fmt.Errorf("安全审计策略处置来源无效：%s", value)
			}
		}
	}
	return result, nil
}

// BuildPromptAuditCyberPolicyScope 将当前配置转换为事件累计使用的数据库范围。
// 官方风控指定分组需要解析当前启用分组；自动封禁白名单则直接使用
// 保存时已校验的稳定编码，分组后续停用不得静默放大惩罚范围。
func BuildPromptAuditCyberPolicyScope(cfg *PromptAuditConfig) (model.PromptAuditCyberPolicyScope, error) {
	scope := model.PromptAuditCyberPolicyScope{TargetType: PromptAuditUpstreamPolicyTargetAll}
	if cfg == nil {
		return scope, errors.New("安全审计配置不存在")
	}
	targetType, err := normalizePromptAuditUpstreamPolicyTargetType(cfg.UpstreamPolicyTargetType)
	if err != nil {
		return scope, err
	}
	scope.TargetType = targetType
	scope.ChannelIDs = canonicalPromptAuditChannelIds(cfg.UpstreamPolicyChannelIds)
	scope.ExemptGroupCodes, err = canonicalPromptAuditAutoBanExemptGroupCodes(cfg.CyberPolicyAutoBanExemptGroupCodes)
	if err != nil {
		return scope, err
	}
	if targetType == PromptAuditUpstreamPolicyTargetGroups {
		resolvedCodes, resolveErr := resolvePromptAuditGroupCodes(cfg.UpstreamPolicyGroupCodes)
		if resolveErr != nil {
			return scope, resolveErr
		}
		scope.GroupCodes = resolvedCodes
		if len(scope.GroupCodes) == 0 {
			return scope, errors.New("官方风控指定分组模式至少需要选择一个有效业务分组")
		}
	}
	return scope, nil
}

func resolvePromptAuditGroupCodes(codes []string) ([]string, error) {
	return resolvePromptAuditGroupCodesForUsage(codes, "官方风控作用范围")
}

func resolvePromptAuditAutoBanExemptGroupCodes(codes []string) ([]string, error) {
	canonical, err := canonicalPromptAuditAutoBanExemptGroupCodes(codes)
	if err != nil {
		return nil, err
	}
	return resolvePromptAuditGroupCodesForUsage(canonical, "cyber_policy 自动封禁分组白名单")
}

func resolvePromptAuditAutoBanExemptGroupCodesForUpdate(current, requested []string) ([]string, error) {
	currentCodes, err := canonicalPromptAuditAutoBanExemptGroupCodes(current)
	if err != nil {
		return nil, err
	}
	requestedCodes, err := canonicalPromptAuditAutoBanExemptGroupCodes(requested)
	if err != nil {
		return nil, err
	}
	currentSet := make(map[string]struct{}, len(currentCodes))
	for _, code := range currentCodes {
		currentSet[code] = struct{}{}
	}
	resultSet := make(map[string]struct{}, len(requestedCodes))
	newCodes := make([]string, 0, len(requestedCodes))
	for _, code := range requestedCodes {
		if _, preserved := currentSet[code]; preserved {
			// 已保存引用后来停用或被外部删除时继续保留豁免；只有新增引用
			// 才要求当前分组存在且启用，避免无关策略保存被失效引用阻断。
			resultSet[code] = struct{}{}
			continue
		}
		newCodes = append(newCodes, code)
	}
	resolvedNewCodes, err := resolvePromptAuditAutoBanExemptGroupCodes(newCodes)
	if err != nil {
		return nil, err
	}
	for _, code := range resolvedNewCodes {
		resultSet[code] = struct{}{}
	}
	result := make([]string, 0, len(resultSet))
	for code := range resultSet {
		result = append(result, code)
	}
	sort.Strings(result)
	return result, nil
}

func resolvePromptAuditGroupCodesForUsage(codes []string, usage string) ([]string, error) {
	resolved := make([]string, 0, len(codes))
	seen := make(map[string]struct{}, len(codes))
	for _, code := range canonicalPromptAuditGroupCodes(codes) {
		group, err := model.GetGroupByCodeOrAlias(code)
		if err != nil || group == nil || group.Status != model.GroupStatusActive {
			return nil, fmt.Errorf("%s 引用的分组不存在或已停用：%s", usage, code)
		}
		canonical := strings.TrimSpace(group.Code)
		if canonical == "" {
			return nil, fmt.Errorf("%s 引用的分组编码无效：%s", usage, code)
		}
		if _, exists := seen[canonical]; exists {
			continue
		}
		seen[canonical] = struct{}{}
		resolved = append(resolved, canonical)
	}
	sort.Strings(resolved)
	return resolved, nil
}

func GetPublicPromptAuditConfig() (*PromptAuditConfig, error) {
	row, endpoints, err := model.LoadPromptAuditConfig()
	if err != nil {
		return nil, err
	}
	cfg, err := promptAuditConfigFromModels(row, endpoints, false)
	if cfg == nil {
		return nil, err
	}
	// 公共管理视图不需要解密节点令牌。密钥缺失或已轮换时仍应允许
	// Root 打开页面并看到 unreadable 状态，以便关闭审计或清除旧令牌；
	// 请求热路径仍会保留该解密错误并在 blocking 模式下 fail-closed。
	return PublicPromptAuditConfig(cfg), nil
}

func SavePromptAuditConfig(req PromptAuditUpdateRequest, actorId int) (*PromptAuditConfig, error) {
	if strings.TrimSpace(req.Mode) != "" {
		switch strings.ToLower(strings.TrimSpace(req.Mode)) {
		case PromptAuditModeOff:
			req.Enabled, req.BlockingEnabled = false, false
		case PromptAuditModeAsync:
			req.Enabled, req.BlockingEnabled = true, false
		case PromptAuditModeBlocking:
			req.Enabled, req.BlockingEnabled = true, true
		default:
			return nil, errors.New("安全审计 mode 仅支持 off、async_audit 或 blocking")
		}
	}
	if err := validatePromptAuditUpdate(req); err != nil {
		return nil, err
	}
	currentRow, currentEndpoints, err := model.LoadPromptAuditConfig()
	if err != nil {
		return nil, err
	}
	if currentRow.ConfigVersion != req.ExpectedConfigVersion {
		return nil, model.ErrPromptAuditConfigConflict
	}
	upstreamPolicyEnabled := currentRow.UpstreamPolicyEnabled
	if req.UpstreamPolicyEnabled != nil {
		upstreamPolicyEnabled = *req.UpstreamPolicyEnabled
	}
	sensitiveWordAuditEnabled := currentRow.SensitiveWordAuditEnabled
	if req.SensitiveWordAuditEnabled != nil {
		sensitiveWordAuditEnabled = *req.SensitiveWordAuditEnabled
	}
	cyberPolicyAutoBanEnabled := currentRow.CyberPolicyAutoBanEnabled
	if req.CyberPolicyAutoBanEnabled != nil {
		cyberPolicyAutoBanEnabled = *req.CyberPolicyAutoBanEnabled
	}
	cyberPolicyAutoBanExemptGroupCodes, err := promptAuditAutoBanExemptGroupCodesFromModel(currentRow)
	if err != nil {
		return nil, err
	}
	if req.CyberPolicyAutoBanExemptGroupCodes != nil {
		cyberPolicyAutoBanExemptGroupCodes, err = resolvePromptAuditAutoBanExemptGroupCodesForUpdate(
			cyberPolicyAutoBanExemptGroupCodes,
			*req.CyberPolicyAutoBanExemptGroupCodes,
		)
		if err != nil {
			return nil, err
		}
	}
	cyberPolicyAutoBanExemptGroupCodesJSON, err := common.Marshal(cyberPolicyAutoBanExemptGroupCodes)
	if err != nil {
		return nil, err
	}
	cyberPolicyBanThreshold := currentRow.CyberPolicyBanThreshold
	if req.CyberPolicyBanThreshold != nil {
		cyberPolicyBanThreshold = *req.CyberPolicyBanThreshold
	}
	cyberPolicyWindowHours := currentRow.CyberPolicyWindowHours
	if req.CyberPolicyWindowHours != nil {
		cyberPolicyWindowHours = *req.CyberPolicyWindowHours
	}
	cyberSessionBlockEnabled := currentRow.CyberSessionBlockEnabled
	if req.CyberSessionBlockEnabled != nil {
		cyberSessionBlockEnabled = *req.CyberSessionBlockEnabled
	}
	cyberSessionBlockTTLSeconds := normalizeCyberSessionBlockTTLSeconds(currentRow.CyberSessionBlockTTLSeconds)
	if req.CyberSessionBlockTTLSeconds != nil {
		cyberSessionBlockTTLSeconds = *req.CyberSessionBlockTTLSeconds
	}
	if err := validateCyberSessionBlockConfig(cyberSessionBlockTTLSeconds); err != nil {
		return nil, err
	}
	policyActionSources, err := promptAuditPolicyActionSourcesFromModel(currentRow)
	if err != nil {
		return nil, err
	}
	if req.PolicyActionSources != nil {
		policyActionSources, err = normalizePromptAuditPolicyActionSources(*req.PolicyActionSources)
		if err != nil {
			return nil, err
		}
	}
	policyActionSourcesJSON, err := common.Marshal(policyActionSources)
	if err != nil {
		return nil, err
	}
	if cyberSessionBlockEnabled && !upstreamPolicyEnabled {
		return nil, errors.New("启用 cyber_policy 会话屏蔽前必须先启用上游安全策略事件记录")
	}
	if err := validateCyberPolicyAutoBanConfig(cyberPolicyBanThreshold, cyberPolicyWindowHours); err != nil {
		return nil, err
	}
	if cyberPolicyAutoBanEnabled && !upstreamPolicyEnabled {
		return nil, errors.New("启用 cyber_policy 自动禁用前必须先启用上游安全策略事件记录")
	}
	upstreamPolicyTargetType, upstreamPolicyChannelIds, upstreamPolicyGroupCodes, err := promptAuditUpstreamPolicyScopeFromModel(currentRow)
	if err != nil {
		return nil, err
	}
	if req.UpstreamPolicyTargetType != nil {
		upstreamPolicyTargetType, err = normalizePromptAuditUpstreamPolicyTargetType(*req.UpstreamPolicyTargetType)
		if err != nil {
			return nil, err
		}
	}
	if req.UpstreamPolicyChannelIds != nil {
		upstreamPolicyChannelIds = canonicalPromptAuditChannelIds(*req.UpstreamPolicyChannelIds)
	}
	if req.UpstreamPolicyGroupCodes != nil {
		upstreamPolicyGroupCodes = canonicalPromptAuditGroupCodes(*req.UpstreamPolicyGroupCodes)
	}
	if upstreamPolicyTargetType == PromptAuditUpstreamPolicyTargetGroups {
		upstreamPolicyGroupCodes, err = resolvePromptAuditGroupCodes(upstreamPolicyGroupCodes)
		if err != nil {
			return nil, err
		}
	}
	if err := validatePromptAuditUpstreamPolicyScope(upstreamPolicyTargetType, upstreamPolicyChannelIds, upstreamPolicyGroupCodes); err != nil {
		return nil, err
	}
	upstreamPolicyChannelIdsJSON, err := common.Marshal(upstreamPolicyChannelIds)
	if err != nil {
		return nil, err
	}
	upstreamPolicyGroupCodesJSON, err := common.Marshal(upstreamPolicyGroupCodes)
	if err != nil {
		return nil, err
	}
	currentById := make(map[string]model.PromptAuditEndpoint, len(currentEndpoints))
	for _, endpoint := range currentEndpoints {
		currentById[endpoint.Id] = endpoint
	}

	scanners := canonicalPromptAuditScanners(req.Scanners)
	groups := canonicalPromptAuditGroupIds(req.GroupIds)
	scannerJson, err := common.Marshal(scanners)
	if err != nil {
		return nil, err
	}
	groupJson, err := common.Marshal(groups)
	if err != nil {
		return nil, err
	}
	summaryJson, err := common.Marshal(map[string]interface{}{
		"enabled": req.Enabled, "blocking_enabled": req.BlockingEnabled,
		"store_pass_events":                        req.StorePassEvents,
		"upstream_policy_enabled":                  upstreamPolicyEnabled,
		"upstream_policy_target_type":              upstreamPolicyTargetType,
		"upstream_policy_channel_count":            len(upstreamPolicyChannelIds),
		"upstream_policy_group_count":              len(upstreamPolicyGroupCodes),
		"sensitive_word_audit_enabled":             sensitiveWordAuditEnabled,
		"cyber_policy_auto_ban_enabled":            cyberPolicyAutoBanEnabled,
		"policy_action_sources":                    policyActionSources,
		"cyber_policy_auto_ban_exempt_group_count": len(cyberPolicyAutoBanExemptGroupCodes),
		"cyber_policy_ban_threshold":               cyberPolicyBanThreshold,
		"cyber_policy_violation_window_hours":      cyberPolicyWindowHours,
		"cyber_session_block_enabled":              cyberSessionBlockEnabled,
		"cyber_session_block_ttl_seconds":          cyberSessionBlockTTLSeconds,
		"endpoint_count":                           len(req.Endpoints),
		"scanner_count":                            len(scanners), "all_groups": req.AllGroups, "group_count": len(groups),
	})
	if err != nil {
		return nil, err
	}
	row := &model.PromptAuditConfig{
		Id: model.PromptAuditConfigID, Enabled: req.Enabled, BlockingEnabled: req.BlockingEnabled,
		StorePassEvents: req.StorePassEvents, UpstreamPolicyEnabled: upstreamPolicyEnabled,
		UpstreamPolicyTargetType:           upstreamPolicyTargetType,
		UpstreamPolicyChannelIds:           string(upstreamPolicyChannelIdsJSON),
		UpstreamPolicyGroupCodes:           string(upstreamPolicyGroupCodesJSON),
		SensitiveWordAuditEnabled:          sensitiveWordAuditEnabled,
		CyberSessionBlockEnabled:           cyberSessionBlockEnabled,
		CyberSessionBlockTTLSeconds:        cyberSessionBlockTTLSeconds,
		CyberPolicyAutoBanEnabled:          cyberPolicyAutoBanEnabled,
		PolicyActionSources:                string(policyActionSourcesJSON),
		CyberPolicyAutoBanExemptGroupCodes: string(cyberPolicyAutoBanExemptGroupCodesJSON),
		CyberPolicyBanThreshold:            cyberPolicyBanThreshold, CyberPolicyWindowHours: cyberPolicyWindowHours,
		Strategy: "priority", WorkerCount: req.WorkerCount,
		QueueCapacity: req.QueueCapacity, RetentionDays: req.RetentionDays,
		Scanners: string(scannerJson), AllGroups: req.AllGroups, GroupIds: string(groupJson),
		UpdatedBy: actorId, ChangeSummary: string(summaryJson),
	}
	now := time.Now().Unix()
	endpointRows := make([]model.PromptAuditEndpoint, 0, len(req.Endpoints))
	for priority, input := range req.Endpoints {
		input.Id = strings.TrimSpace(input.Id)
		input.Name = strings.TrimSpace(input.Name)
		input.Model = strings.TrimSpace(input.Model)
		input.TokenAction = strings.ToLower(strings.TrimSpace(input.TokenAction))
		normalizedUrl, normalizeErr := NormalizePromptAuditBaseURL(input.BaseUrl)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		existing := currentById[input.Id]
		tokenCiphertext := existing.TokenCiphertext
		switch input.TokenAction {
		case PromptAuditTokenKeep:
			// 已保存的令牌只允许绑定到原节点地址。否则持有 Root 会话但
			// 不知道令牌明文的一方，可以把地址改到受控服务并借 keep
			// 操作外带旧令牌。地址变化时必须显式替换或清除令牌。
			if tokenCiphertext != "" {
				existingURL, existingErr := NormalizePromptAuditBaseURL(existing.BaseUrl)
				if existingErr != nil || existingURL != normalizedUrl {
					return nil, errors.New("Guard 节点地址变化时必须显式替换或清除令牌")
				}
			}
		case PromptAuditTokenClear:
			tokenCiphertext = ""
		case PromptAuditTokenReplace:
			if !PromptAuditCryptoReady() {
				return nil, errors.New("保存 Guard 节点令牌前必须显式配置 CRYPTO_SECRET")
			}
			tokenCiphertext, err = EncryptPromptAuditSecret(strings.TrimSpace(input.Token))
			if err != nil {
				return nil, err
			}
		}
		createdAt := existing.CreatedAt
		if createdAt == 0 {
			createdAt = now
		}
		endpointRows = append(endpointRows, model.PromptAuditEndpoint{
			Id: input.Id, Name: input.Name, Protocol: "openai_compatible",
			BaseUrl: normalizedUrl, Model: defaultPromptAuditString(input.Model, PromptAuditDefaultModel),
			TokenCiphertext: tokenCiphertext, TimeoutMs: input.TimeoutMs, InputLimit: input.InputLimit,
			Enabled: input.Enabled, Priority: priority, CreatedAt: createdAt, UpdatedAt: now,
		})
	}
	if req.Enabled {
		for _, endpoint := range endpointRows {
			if !endpoint.Enabled || endpoint.TokenCiphertext == "" {
				continue
			}
			if _, err := DecryptPromptAuditSecret(endpoint.TokenCiphertext); err != nil {
				return nil, errors.New("启用提示词安全审计前必须确认现有 Guard 节点令牌可解密")
			}
		}
	}
	if err := model.SavePromptAuditConfig(req.ExpectedConfigVersion, row, endpointRows); err != nil {
		return nil, err
	}
	InvalidatePromptAuditConfig()
	return GetPublicPromptAuditConfig()
}

func validatePromptAuditUpdate(req PromptAuditUpdateRequest) error {
	if req.ExpectedConfigVersion < 1 {
		return errors.New("expected_version 必须大于 0")
	}
	if req.BlockingEnabled && !req.Enabled {
		return errors.New("开启同步阻断前必须先启用提示词安全审计")
	}
	if req.Strategy != "" && req.Strategy != "priority" {
		return errors.New("安全审计节点策略仅支持 priority")
	}
	if req.WorkerCount < 1 || req.WorkerCount > 32 {
		return errors.New("Worker 数量必须在 1 到 32 之间")
	}
	if req.QueueCapacity < 1 || req.QueueCapacity > 100000 {
		return errors.New("队列容量必须在 1 到 100000 之间")
	}
	if req.RetentionDays < 1 || req.RetentionDays > 365 {
		return errors.New("事件保留天数必须在 1 到 365 之间")
	}
	if req.CyberPolicyBanThreshold != nil && (*req.CyberPolicyBanThreshold < 1 || *req.CyberPolicyBanThreshold > 1000000) {
		return errors.New("cyber_policy 自动封禁阈值必须在 1 到 1000000 之间")
	}
	if req.CyberPolicyWindowHours != nil && (*req.CyberPolicyWindowHours < 1 || *req.CyberPolicyWindowHours > 87600) {
		return errors.New("cyber_policy 违规窗口必须在 1 到 87600 小时之间")
	}
	if req.CyberSessionBlockTTLSeconds != nil && (*req.CyberSessionBlockTTLSeconds < 1 || *req.CyberSessionBlockTTLSeconds > CyberSessionBlockMaxTTLSeconds) {
		return fmt.Errorf("cyber_policy 会话屏蔽 TTL 必须在 1 到 %d 秒之间", CyberSessionBlockMaxTTLSeconds)
	}
	if len(canonicalPromptAuditScanners(req.Scanners)) == 0 {
		return errors.New("至少需要启用一个风险分类")
	}
	if !req.AllGroups {
		groups := canonicalPromptAuditGroupIds(req.GroupIds)
		if len(groups) == 0 {
			return errors.New("指定分组模式至少需要选择一个分组")
		}
		found, err := model.GetGroupsByIds(groups)
		if err != nil {
			return err
		}
		if len(found) != len(groups) {
			return errors.New("安全审计分组包含不存在的 ID")
		}
	}
	seen := make(map[string]struct{}, len(req.Endpoints))
	enabled := 0
	for _, endpoint := range req.Endpoints {
		endpoint.Id = strings.TrimSpace(endpoint.Id)
		endpoint.Name = strings.TrimSpace(endpoint.Name)
		endpoint.Model = strings.TrimSpace(endpoint.Model)
		endpoint.TokenAction = strings.ToLower(strings.TrimSpace(endpoint.TokenAction))
		if endpoint.Id == "" || len(endpoint.Id) > 64 || endpoint.Name == "" || utf8.RuneCountInString(endpoint.Name) > 128 {
			return errors.New("Guard 节点 ID 和名称不能为空")
		}
		if utf8.RuneCountInString(endpoint.Model) > 255 {
			return errors.New("Guard 节点模型名称不能超过 255 个字符")
		}
		if len(endpoint.Token) > 8192 {
			return errors.New("Guard 节点令牌长度不能超过 8192 个字符")
		}
		token := strings.TrimSpace(endpoint.Token)
		switch endpoint.TokenAction {
		case PromptAuditTokenKeep, PromptAuditTokenClear:
			if token != "" {
				return errors.New("Guard 节点令牌仅允许在 replace 操作中提交")
			}
		case PromptAuditTokenReplace:
			if token == "" {
				return errors.New("replace 操作必须提供 Guard 节点令牌")
			}
		default:
			return errors.New("Guard 节点 token_action 仅支持 keep、replace 或 clear")
		}
		if _, exists := seen[endpoint.Id]; exists {
			return errors.New("Guard 节点 ID 不能重复")
		}
		seen[endpoint.Id] = struct{}{}
		if endpoint.Protocol != "" && endpoint.Protocol != "openai_compatible" {
			return errors.New("Guard 节点仅支持 openai_compatible 协议")
		}
		if _, err := NormalizePromptAuditBaseURL(endpoint.BaseUrl); err != nil {
			return err
		}
		if endpoint.TimeoutMs < 100 || endpoint.TimeoutMs > 30000 {
			return errors.New("Guard 节点超时必须在 100 到 30000 毫秒之间")
		}
		if endpoint.InputLimit < 128 || endpoint.InputLimit > 100000 {
			return errors.New("Guard 节点输入上限必须在 128 到 100000 字符之间")
		}
		if endpoint.Enabled {
			enabled++
		}
	}
	if req.Enabled {
		if !PromptAuditCryptoReady() {
			return errors.New("启用提示词安全审计前必须显式配置 CRYPTO_SECRET")
		}
		if enabled == 0 {
			return errors.New("启用提示词安全审计前至少需要一个启用的 Guard 节点")
		}
	}
	return nil
}

func validateCyberPolicyAutoBanConfig(threshold, windowHours int) error {
	if threshold < 1 || threshold > 1000000 {
		return errors.New("cyber_policy 自动封禁阈值必须在 1 到 1000000 之间")
	}
	if windowHours < 1 || windowHours > 87600 {
		return errors.New("cyber_policy 违规窗口必须在 1 到 87600 小时之间")
	}
	return nil
}

func normalizePromptAuditUpstreamPolicyTargetType(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return PromptAuditUpstreamPolicyTargetAll, nil
	}
	switch value {
	case PromptAuditUpstreamPolicyTargetAll, PromptAuditUpstreamPolicyTargetChannels, PromptAuditUpstreamPolicyTargetGroups:
		return value, nil
	default:
		return "", errors.New("官方风控作用范围仅支持 all、channels 或 groups")
	}
}

func canonicalPromptAuditChannelIds(values []int) []int {
	seen := make(map[int]struct{}, len(values))
	result := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Ints(result)
	return result
}

func canonicalPromptAuditGroupCodes(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		normalized, err := model.NormalizeGroupCode(value)
		if err != nil {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	sort.Strings(result)
	return result
}

func canonicalPromptAuditAutoBanExemptGroupCodes(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		normalized, err := model.NormalizeGroupCode(value)
		if err != nil {
			return nil, fmt.Errorf("cyber_policy 自动封禁分组白名单编码无效：%w", err)
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	sort.Strings(result)
	return result, nil
}

func validatePromptAuditUpstreamPolicyScope(targetType string, channelIds []int, groupCodes []string) error {
	switch targetType {
	case PromptAuditUpstreamPolicyTargetAll:
		return nil
	case PromptAuditUpstreamPolicyTargetChannels:
		if len(channelIds) == 0 {
			return errors.New("官方风控指定渠道模式至少需要选择一个渠道")
		}
	case PromptAuditUpstreamPolicyTargetGroups:
		if len(groupCodes) == 0 {
			return errors.New("官方风控指定分组模式至少需要选择一个业务分组")
		}
	default:
		return errors.New("官方风控作用范围仅支持 all、channels 或 groups")
	}
	return nil
}

func promptAuditUpstreamPolicyScopeFromModel(row *model.PromptAuditConfig) (string, []int, []string, error) {
	if row == nil {
		return "", nil, nil, errors.New("安全审计配置不存在")
	}
	targetType, err := normalizePromptAuditUpstreamPolicyTargetType(row.UpstreamPolicyTargetType)
	if err != nil {
		return "", nil, nil, err
	}
	channelIds := make([]int, 0)
	if strings.TrimSpace(row.UpstreamPolicyChannelIds) != "" {
		if err := common.UnmarshalJsonStr(row.UpstreamPolicyChannelIds, &channelIds); err != nil {
			return "", nil, nil, errors.New("官方风控渠道范围配置无效")
		}
	}
	groupCodes := make([]string, 0)
	if strings.TrimSpace(row.UpstreamPolicyGroupCodes) != "" {
		if err := common.UnmarshalJsonStr(row.UpstreamPolicyGroupCodes, &groupCodes); err != nil {
			return "", nil, nil, errors.New("官方风控分组范围配置无效")
		}
	}
	return targetType, canonicalPromptAuditChannelIds(channelIds), canonicalPromptAuditGroupCodes(groupCodes), nil
}

func promptAuditAutoBanExemptGroupCodesFromModel(row *model.PromptAuditConfig) ([]string, error) {
	if row == nil {
		return nil, errors.New("安全审计配置不存在")
	}
	groupCodes := make([]string, 0)
	if strings.TrimSpace(row.CyberPolicyAutoBanExemptGroupCodes) != "" {
		if err := common.UnmarshalJsonStr(row.CyberPolicyAutoBanExemptGroupCodes, &groupCodes); err != nil {
			return nil, errors.New("cyber_policy 自动封禁分组白名单配置无效")
		}
	}
	return canonicalPromptAuditAutoBanExemptGroupCodes(groupCodes)
}

func canonicalPromptAuditScanners(values []string) []string {
	allowed := make(map[string]struct{}, len(PromptAuditScannerIDs))
	for _, scanner := range PromptAuditScannerIDs {
		allowed[scanner] = struct{}{}
	}
	selected := make(map[string]struct{}, len(values))
	for _, value := range values {
		normalized := NormalizePromptAuditCategory(value)
		if _, ok := allowed[normalized]; ok {
			selected[normalized] = struct{}{}
		}
	}
	result := make([]string, 0, len(selected))
	for _, scanner := range PromptAuditScannerIDs {
		if _, ok := selected[scanner]; ok {
			result = append(result, scanner)
		}
	}
	return result
}

func canonicalPromptAuditGroupIds(values []int) []int {
	seen := make(map[int]struct{}, len(values))
	result := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Ints(result)
	return result
}

func defaultPromptAuditString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func NormalizePromptAuditBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("Guard 节点地址无效")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("Guard 节点仅支持 HTTP(S)")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("Guard 节点地址不能包含凭据、查询参数或片段")
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return "", errors.New("Guard 节点地址缺少主机名")
	}
	if isForbiddenPromptAuditHostname(host) {
		return "", errors.New("Guard 节点不能指向云元数据或 link-local 地址")
	}
	// url.Parse 已将 Path 解码为语义路径；把 EscapedPath 直接写回
	// parsed.Path 会让 url.URL.String 再次转义百分号，导致例如
	// `/guard%20node` 被错误规范化为 `/guard%2520node`。使用解码后的
	// Path，并清空 RawPath，让 String 只执行一次规范转义。
	path := strings.TrimRight(parsed.Path, "/")
	if strings.EqualFold(path, "/v1") {
		path = ""
	}
	parsed.Path = path
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func PromptAuditChatCompletionsURL(baseUrl string) (string, error) {
	normalized, err := NormalizePromptAuditBaseURL(baseUrl)
	if err != nil {
		return "", err
	}
	return normalized + "/v1/chat/completions", nil
}

// PromptAuditModelsURL 返回节点能力探测使用的 OpenAI 兼容模型列表地址。
// 与聊天地址共用同一套规范化和 SSRF 校验，避免探测路径绕过节点安全边界。
func PromptAuditModelsURL(baseUrl string) (string, error) {
	normalized, err := NormalizePromptAuditBaseURL(baseUrl)
	if err != nil {
		return "", err
	}
	return normalized + "/v1/models", nil
}

func isForbiddenPromptAuditHostname(host string) bool {
	lower := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	switch lower {
	case "metadata", "metadata.google.internal", "metadata.azure.internal", "instance-data", "instance-data.ec2.internal":
		return true
	}
	ip := net.ParseIP(lower)
	return ip != nil && isForbiddenPromptAuditIP(ip)
}

func isForbiddenPromptAuditIP(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		value := v4.String()
		return v4[0] == 169 && v4[1] == 254 || value == "100.100.100.200" || value == "168.63.129.16"
	}
	// AWS IMDS 的 IPv6 地址。
	if strings.EqualFold(ip.String(), "fd00:ec2::254") {
		return true
	}
	return false
}

func promptAuditConfigConflictError(err error) error {
	if errors.Is(err, model.ErrPromptAuditConfigConflict) {
		return fmt.Errorf("%s: 配置已被其他管理员更新", PromptAuditConfigConflictCode)
	}
	return err
}
