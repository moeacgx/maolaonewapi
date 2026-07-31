package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
)

// SecurityAuditBuiltinPolicy 是无需 Guard 节点即可运行的安全策略管理视图。
// JSON 字符串字段保持与既有屏蔽词编辑器一致，便于无损迁移现有配置。
type SecurityAuditBuiltinPolicy struct {
	ConfigVersion                      int64    `json:"config_version"`
	UpstreamPolicyEnabled              bool     `json:"upstream_policy_enabled"`
	UpstreamPolicyTargetType           string   `json:"upstream_policy_target_type"`
	UpstreamPolicyChannelIds           []int    `json:"upstream_policy_channel_ids"`
	UpstreamPolicyGroupCodes           []string `json:"upstream_policy_group_codes"`
	SensitiveWordAuditEnabled          bool     `json:"sensitive_word_audit_enabled"`
	CyberPolicyAutoBanEnabled          bool     `json:"cyber_policy_auto_ban_enabled"`
	CyberPolicyAutoBanExemptGroupCodes []string `json:"cyber_policy_auto_ban_exempt_group_codes"`
	CyberPolicyBanThreshold            int      `json:"cyber_policy_ban_threshold"`
	CyberPolicyWindowHours             int      `json:"cyber_policy_violation_window_hours"`
	CheckSensitiveEnabled              bool     `json:"check_sensitive_enabled"`
	CheckSensitiveOnPromptEnabled      bool     `json:"check_sensitive_on_prompt_enabled"`
	SensitiveWords                     string   `json:"sensitive_words"`
	SensitiveRules                     string   `json:"sensitive_rules"`
	SensitiveRuleChannelIds            string   `json:"sensitive_rule_channel_ids"`
	UsesLegacySensitiveWords           bool     `json:"uses_legacy_sensitive_words"`
	UpdatedAt                          int64    `json:"updated_at"`
	UpdatedBy                          int      `json:"updated_by"`
}

type SecurityAuditBuiltinPolicyUpdateRequest struct {
	ExpectedConfigVersion              int64     `json:"expected_version"`
	UpstreamPolicyEnabled              *bool     `json:"upstream_policy_enabled"`
	UpstreamPolicyTargetType           *string   `json:"upstream_policy_target_type"`
	UpstreamPolicyChannelIds           *[]int    `json:"upstream_policy_channel_ids"`
	UpstreamPolicyGroupCodes           *[]string `json:"upstream_policy_group_codes"`
	SensitiveWordAuditEnabled          *bool     `json:"sensitive_word_audit_enabled"`
	CyberPolicyAutoBanEnabled          *bool     `json:"cyber_policy_auto_ban_enabled"`
	CyberPolicyAutoBanExemptGroupCodes *[]string `json:"cyber_policy_auto_ban_exempt_group_codes"`
	CyberPolicyBanThreshold            *int      `json:"cyber_policy_ban_threshold"`
	CyberPolicyWindowHours             *int      `json:"cyber_policy_violation_window_hours"`
	CheckSensitiveEnabled              *bool     `json:"check_sensitive_enabled"`
	CheckSensitiveOnPromptEnabled      *bool     `json:"check_sensitive_on_prompt_enabled"`
	SensitiveRules                     *string   `json:"sensitive_rules"`
	SensitiveRuleChannelIds            *string   `json:"sensitive_rule_channel_ids"`
}

func GetSecurityAuditBuiltinPolicy() (*SecurityAuditBuiltinPolicy, error) {
	row, _, err := model.LoadPromptAuditConfig()
	if err != nil {
		return nil, err
	}
	sensitivePolicy := setting.GetSensitivePolicySnapshot()
	rulesJSON, _, err := canonicalSecurityAuditSensitiveRules(nil, sensitivePolicy)
	if err != nil {
		return nil, err
	}
	channelIdsJSON, _, err := canonicalSecurityAuditChannelIds(nil, sensitivePolicy)
	if err != nil {
		return nil, err
	}
	upstreamPolicyTargetType, upstreamPolicyChannelIds, upstreamPolicyGroupCodes, err := promptAuditUpstreamPolicyScopeFromModel(row)
	if err != nil {
		return nil, err
	}
	cyberPolicyAutoBanExemptGroupCodes, err := promptAuditAutoBanExemptGroupCodesFromModel(row)
	if err != nil {
		return nil, err
	}
	return &SecurityAuditBuiltinPolicy{
		ConfigVersion:                      row.ConfigVersion,
		UpstreamPolicyEnabled:              row.UpstreamPolicyEnabled,
		UpstreamPolicyTargetType:           upstreamPolicyTargetType,
		UpstreamPolicyChannelIds:           upstreamPolicyChannelIds,
		UpstreamPolicyGroupCodes:           upstreamPolicyGroupCodes,
		SensitiveWordAuditEnabled:          row.SensitiveWordAuditEnabled,
		CyberPolicyAutoBanEnabled:          row.CyberPolicyAutoBanEnabled,
		CyberPolicyAutoBanExemptGroupCodes: cyberPolicyAutoBanExemptGroupCodes,
		CyberPolicyBanThreshold:            row.CyberPolicyBanThreshold,
		CyberPolicyWindowHours:             row.CyberPolicyWindowHours,
		CheckSensitiveEnabled:              sensitivePolicy.CheckEnabled,
		CheckSensitiveOnPromptEnabled:      sensitivePolicy.CheckOnPromptEnabled,
		SensitiveWords:                     strings.Join(sensitivePolicy.Words, "\n"),
		SensitiveRules:                     rulesJSON,
		SensitiveRuleChannelIds:            channelIdsJSON,
		UsesLegacySensitiveWords:           !sensitivePolicy.RulesConfigured && len(sensitivePolicy.Words) > 0,
		UpdatedAt:                          row.UpdatedAt,
		UpdatedBy:                          row.UpdatedBy,
	}, nil
}

func SaveSecurityAuditBuiltinPolicy(req SecurityAuditBuiltinPolicyUpdateRequest, actorId int) (*SecurityAuditBuiltinPolicy, error) {
	if req.ExpectedConfigVersion < 1 {
		return nil, errors.New("expected_version 必须大于 0")
	}
	row, _, err := model.LoadPromptAuditConfig()
	if err != nil {
		return nil, err
	}
	if row.ConfigVersion != req.ExpectedConfigVersion {
		return nil, model.ErrPromptAuditConfigConflict
	}

	upstreamPolicyEnabled := row.UpstreamPolicyEnabled
	if req.UpstreamPolicyEnabled != nil {
		upstreamPolicyEnabled = *req.UpstreamPolicyEnabled
	}
	upstreamPolicyTargetType, upstreamPolicyChannelIds, upstreamPolicyGroupCodes, err := promptAuditUpstreamPolicyScopeFromModel(row)
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
	sensitiveWordAuditEnabled := row.SensitiveWordAuditEnabled
	if req.SensitiveWordAuditEnabled != nil {
		sensitiveWordAuditEnabled = *req.SensitiveWordAuditEnabled
	}
	cyberPolicyAutoBanEnabled := row.CyberPolicyAutoBanEnabled
	if req.CyberPolicyAutoBanEnabled != nil {
		cyberPolicyAutoBanEnabled = *req.CyberPolicyAutoBanEnabled
	}
	cyberPolicyAutoBanExemptGroupCodes, err := promptAuditAutoBanExemptGroupCodesFromModel(row)
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
	cyberPolicyBanThreshold := row.CyberPolicyBanThreshold
	if req.CyberPolicyBanThreshold != nil {
		cyberPolicyBanThreshold = *req.CyberPolicyBanThreshold
	}
	cyberPolicyWindowHours := row.CyberPolicyWindowHours
	if req.CyberPolicyWindowHours != nil {
		cyberPolicyWindowHours = *req.CyberPolicyWindowHours
	}
	if err := validateCyberPolicyAutoBanConfig(cyberPolicyBanThreshold, cyberPolicyWindowHours); err != nil {
		return nil, err
	}
	if cyberPolicyAutoBanEnabled && !upstreamPolicyEnabled {
		return nil, errors.New("启用 cyber_policy 自动禁用前必须先启用上游安全策略事件记录")
	}
	sensitivePolicy := setting.GetSensitivePolicySnapshot()
	checkSensitiveEnabled := sensitivePolicy.CheckEnabled
	if req.CheckSensitiveEnabled != nil {
		checkSensitiveEnabled = *req.CheckSensitiveEnabled
	}
	checkSensitiveOnPromptEnabled := sensitivePolicy.CheckOnPromptEnabled
	if req.CheckSensitiveOnPromptEnabled != nil {
		checkSensitiveOnPromptEnabled = *req.CheckSensitiveOnPromptEnabled
	}

	rulesJSON, rules, err := canonicalSecurityAuditSensitiveRules(req.SensitiveRules, sensitivePolicy)
	if err != nil {
		return nil, fmt.Errorf("屏蔽词规则格式无效: %w", err)
	}
	channelIdsJSON, channelIds, err := canonicalSecurityAuditChannelIds(req.SensitiveRuleChannelIds, sensitivePolicy)
	if err != nil {
		return nil, errors.New("屏蔽词渠道范围格式无效")
	}
	if err := validateSecurityAuditSensitiveRuleTargets(rules); err != nil {
		return nil, err
	}
	// 历史无 target_type 的规则继承同一次请求提交的全局渠道范围，
	// 变更摘要不能继续使用保存前的旧快照。
	targetPolicy := sensitivePolicy
	targetPolicy.LegacyChannelIds = append([]int(nil), channelIds...)
	targetChannelCount, targetTagCount, targetGroupCount, targetAllCount := securityAuditSensitiveTargetCounts(rules, targetPolicy)
	summaryJSON, err := common.Marshal(map[string]interface{}{
		"upstream_policy_enabled":                  upstreamPolicyEnabled,
		"upstream_policy_target_type":              upstreamPolicyTargetType,
		"upstream_policy_channel_count":            len(upstreamPolicyChannelIds),
		"upstream_policy_group_count":              len(upstreamPolicyGroupCodes),
		"sensitive_word_audit_enabled":             sensitiveWordAuditEnabled,
		"cyber_policy_auto_ban_enabled":            cyberPolicyAutoBanEnabled,
		"cyber_policy_auto_ban_exempt_group_count": len(cyberPolicyAutoBanExemptGroupCodes),
		"cyber_policy_ban_threshold":               cyberPolicyBanThreshold,
		"cyber_policy_violation_window_hours":      cyberPolicyWindowHours,
		"check_sensitive_enabled":                  checkSensitiveEnabled,
		"check_sensitive_on_prompt_enabled":        checkSensitiveOnPromptEnabled,
		"sensitive_rule_count":                     len(rules),
		"sensitive_rule_channel_count":             len(channelIds),
		"sensitive_rule_target_channel_count":      targetChannelCount,
		"sensitive_rule_target_tag_count":          targetTagCount,
		"sensitive_rule_target_group_count":        targetGroupCount,
		"sensitive_rule_target_all_count":          targetAllCount,
	})
	if err != nil {
		return nil, err
	}
	if err := model.SavePromptAuditBuiltinPolicy(model.PromptAuditBuiltinPolicyUpdate{
		ExpectedVersion:                    req.ExpectedConfigVersion,
		UpstreamPolicyEnabled:              upstreamPolicyEnabled,
		UpstreamPolicyTargetType:           upstreamPolicyTargetType,
		UpstreamPolicyChannelIds:           string(upstreamPolicyChannelIdsJSON),
		UpstreamPolicyGroupCodes:           string(upstreamPolicyGroupCodesJSON),
		SensitiveWordAuditEnabled:          sensitiveWordAuditEnabled,
		CyberPolicyAutoBanEnabled:          cyberPolicyAutoBanEnabled,
		CyberPolicyAutoBanExemptGroupCodes: string(cyberPolicyAutoBanExemptGroupCodesJSON),
		CyberPolicyBanThreshold:            cyberPolicyBanThreshold,
		CyberPolicyWindowHours:             cyberPolicyWindowHours,
		CheckSensitiveEnabled:              checkSensitiveEnabled,
		CheckSensitiveOnPromptEnabled:      checkSensitiveOnPromptEnabled,
		SensitiveRules:                     rulesJSON,
		SensitiveRuleChannelIds:            channelIdsJSON,
		UpdatedBy:                          actorId,
		ChangeSummary:                      string(summaryJSON),
	}); err != nil {
		return nil, err
	}
	InvalidatePromptAuditConfig()
	return GetSecurityAuditBuiltinPolicy()
}

func validateSecurityAuditSensitiveRuleTargets(rules []setting.SensitiveRule) error {
	for index, rule := range rules {
		if !rule.Enabled || rule.TargetType == "" {
			continue
		}
		if !setting.SensitiveRuleHasRoutingTargets(rule) {
			return fmt.Errorf("第 %d 条已启用的屏蔽词规则必须至少选择一个渠道或渠道分组", index+1)
		}
	}
	return nil
}

func securityAuditSensitiveTargetCounts(rules []setting.SensitiveRule, snapshot setting.SensitivePolicySnapshot) (int, int, int, int) {
	channels := make(map[int]struct{})
	tags := make(map[string]struct{})
	groups := make(map[string]struct{})
	all := 0
	for _, rule := range rules {
		targets := snapshot.ResolveSensitiveRuleTargets(rule)
		for _, channelId := range targets.ChannelIds {
			channels[channelId] = struct{}{}
		}
		for _, tag := range targets.ChannelTags {
			tags[tag] = struct{}{}
		}
		for _, group := range targets.GroupCodes {
			groups[group] = struct{}{}
		}
		if targets.All {
			all++
		}
	}
	return len(channels), len(tags), len(groups), all
}

func canonicalSecurityAuditSensitiveRules(raw *string, snapshot setting.SensitivePolicySnapshot) (string, []setting.SensitiveRule, error) {
	rules := snapshot.GetEffectiveSensitiveRules()
	if raw != nil {
		var err error
		rules, err = setting.ParseSensitiveRulesJSONString(*raw)
		if err != nil {
			return "", nil, err
		}
	}
	rules = setting.NormalizeSensitiveRules(rules)
	if rules == nil {
		rules = make([]setting.SensitiveRule, 0)
	}
	encoded, err := common.Marshal(setting.SensitiveRuleConfig{Rules: rules})
	if err != nil {
		return "", nil, err
	}
	return string(encoded), rules, nil
}

func canonicalSecurityAuditChannelIds(raw *string, snapshot setting.SensitivePolicySnapshot) (string, []int, error) {
	channelIds := setting.NormalizeSensitiveRuleChannelIds(snapshot.LegacyChannelIds)
	if raw != nil {
		var err error
		channelIds, err = setting.ParseSensitiveRuleChannelIdsJSONString(*raw)
		if err != nil {
			return "", nil, err
		}
	}
	if channelIds == nil {
		channelIds = make([]int, 0)
	}
	encoded, err := common.Marshal(channelIds)
	if err != nil {
		return "", nil, err
	}
	return string(encoded), channelIds, nil
}
