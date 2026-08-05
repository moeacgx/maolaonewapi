package setting

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

var CheckSensitiveEnabled = true
var CheckSensitiveOnPromptEnabled = true

//var CheckSensitiveOnCompletionEnabled = true

// StopOnSensitiveEnabled 如果检测到敏感词，是否立刻停止生成，否则替换敏感词
var StopOnSensitiveEnabled = true

// StreamCacheQueueLength 流模式缓存队列长度，0表示无缓存
var StreamCacheQueueLength = 0

// SensitiveWords 敏感词
// var SensitiveWords []string
var SensitiveWords = []string{
	"test_sensitive",
}

const (
	SensitiveRuleActionMask  = "mask"
	SensitiveRuleActionBlock = "block"

	SensitiveRuleScopeRequest  = "request"
	SensitiveRuleScopeResponse = "response"
	SensitiveRuleScopeBoth     = "both"

	SensitiveRuleTargetChannels    = "channels"
	SensitiveRuleTargetChannelTags = "channel_tags"
	SensitiveRuleTargetGroups      = "groups"
	SensitiveRuleTargetRoutes      = "routes"
	SensitiveRuleTargetAll         = "all"

	DefaultSensitiveMaskReplacement = "[REDACTED]"
)

type SensitiveRule struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Enabled     bool     `json:"enabled"`
	Action      string   `json:"action"`
	Scope       string   `json:"scope,omitempty"`
	Replacement string   `json:"replacement,omitempty"`
	Keywords    []string `json:"keywords"`
	GroupRefs   []string `json:"group_refs,omitempty"`
	TargetType  string   `json:"target_type,omitempty"`
	ChannelIds  []int    `json:"channel_ids,omitempty"`
	ChannelTags []string `json:"channel_tags,omitempty"`
	GroupCodes  []string `json:"group_codes,omitempty"`
}

type SensitiveRuleConfig struct {
	Rules []SensitiveRule `json:"rules"`
}

var SensitiveRules []SensitiveRule
var SensitiveRulesConfigured bool
var SensitiveRuleChannelIds []int

var sensitivePolicyMutex sync.RWMutex

// SensitivePolicySnapshot 是屏蔽词热路径使用的不可变配置快照。
// 开关、规则、旧渠道范围和旧关键词必须在同一次读取中取得，避免热更新时混用新旧配置。
type SensitivePolicySnapshot struct {
	CheckEnabled         bool
	CheckOnPromptEnabled bool
	Words                []string
	Rules                []SensitiveRule
	RulesConfigured      bool
	LegacyChannelIds     []int
}

func cloneSensitiveRules(rules []SensitiveRule) []SensitiveRule {
	if len(rules) == 0 {
		return nil
	}
	cloned := make([]SensitiveRule, len(rules))
	for index, rule := range rules {
		cloned[index] = rule
		cloned[index].Keywords = append([]string(nil), rule.Keywords...)
		cloned[index].GroupRefs = append([]string(nil), rule.GroupRefs...)
		cloned[index].ChannelIds = append([]int(nil), rule.ChannelIds...)
		cloned[index].ChannelTags = append([]string(nil), rule.ChannelTags...)
		cloned[index].GroupCodes = append([]string(nil), rule.GroupCodes...)
	}
	return cloned
}

func GetSensitivePolicySnapshot() SensitivePolicySnapshot {
	sensitivePolicyMutex.RLock()
	defer sensitivePolicyMutex.RUnlock()
	return SensitivePolicySnapshot{
		CheckEnabled:         CheckSensitiveEnabled,
		CheckOnPromptEnabled: CheckSensitiveOnPromptEnabled,
		Words:                append([]string(nil), SensitiveWords...),
		Rules:                cloneSensitiveRules(SensitiveRules),
		RulesConfigured:      SensitiveRulesConfigured,
		LegacyChannelIds:     append([]int(nil), SensitiveRuleChannelIds...),
	}
}

// ReplaceSensitivePolicySnapshot 一次发布完整屏蔽词策略，供版本化管理页和批量 Option 更新使用。
func ReplaceSensitivePolicySnapshot(snapshot SensitivePolicySnapshot) {
	sensitivePolicyMutex.Lock()
	defer sensitivePolicyMutex.Unlock()
	CheckSensitiveEnabled = snapshot.CheckEnabled
	CheckSensitiveOnPromptEnabled = snapshot.CheckOnPromptEnabled
	SensitiveWords = append([]string(nil), snapshot.Words...)
	SensitiveRules = cloneSensitiveRules(snapshot.Rules)
	SensitiveRulesConfigured = snapshot.RulesConfigured
	SensitiveRuleChannelIds = NormalizeSensitiveRuleChannelIds(snapshot.LegacyChannelIds)
}

func SetCheckSensitiveEnabled(enabled bool) {
	sensitivePolicyMutex.Lock()
	CheckSensitiveEnabled = enabled
	sensitivePolicyMutex.Unlock()
}

func SetCheckSensitiveOnPromptEnabled(enabled bool) {
	sensitivePolicyMutex.Lock()
	CheckSensitiveOnPromptEnabled = enabled
	sensitivePolicyMutex.Unlock()
}

func (snapshot SensitivePolicySnapshot) ShouldCheckPromptSensitive() bool {
	return snapshot.CheckEnabled && snapshot.CheckOnPromptEnabled
}

func SensitiveWordsToString() string {
	return strings.Join(GetSensitivePolicySnapshot().Words, "\n")
}

func SensitiveWordsFromString(s string) {
	words := make([]string, 0)
	sw := strings.Split(s, "\n")
	for _, w := range sw {
		w = strings.TrimSpace(w)
		if w != "" {
			words = append(words, w)
		}
	}
	sensitivePolicyMutex.Lock()
	SensitiveWords = words
	sensitivePolicyMutex.Unlock()
}

func ShouldCheckPromptSensitive() bool {
	return GetSensitivePolicySnapshot().ShouldCheckPromptSensitive()
}

func SensitiveRulesToJSONString() string {
	bytes, err := common.Marshal(SensitiveRuleConfig{Rules: NormalizeSensitiveRules(GetSensitivePolicySnapshot().Rules)})
	if err != nil {
		return `{"rules":[]}`
	}
	return string(bytes)
}

func ParseSensitiveRulesJSONString(s string) ([]SensitiveRule, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var config SensitiveRuleConfig
	if err := common.UnmarshalJsonStr(s, &config); err != nil {
		return nil, err
	}
	if err := validateSensitiveRuleTargets(config.Rules); err != nil {
		return nil, err
	}
	return NormalizeSensitiveRules(config.Rules), nil
}

func CheckSensitiveRulesJSONString(s string) error {
	_, err := ParseSensitiveRulesJSONString(s)
	return err
}

func UpdateSensitiveRulesByJSONString(s string) error {
	rules, err := ParseSensitiveRulesJSONString(s)
	if err != nil {
		return err
	}
	sensitivePolicyMutex.Lock()
	defer sensitivePolicyMutex.Unlock()
	SensitiveRules = rules
	SensitiveRulesConfigured = true
	return nil
}

func SensitiveRuleChannelIdsToJSONString() string {
	bytes, err := common.Marshal(NormalizeSensitiveRuleChannelIds(GetSensitivePolicySnapshot().LegacyChannelIds))
	if err != nil {
		return "[]"
	}
	return string(bytes)
}

func ParseSensitiveRuleChannelIdsJSONString(s string) ([]int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var channelIds []int
	if err := common.UnmarshalJsonStr(s, &channelIds); err != nil {
		return nil, err
	}
	return NormalizeSensitiveRuleChannelIds(channelIds), nil
}

func CheckSensitiveRuleChannelIdsJSONString(s string) error {
	_, err := ParseSensitiveRuleChannelIdsJSONString(s)
	return err
}

func UpdateSensitiveRuleChannelIdsByJSONString(s string) error {
	channelIds, err := ParseSensitiveRuleChannelIdsJSONString(s)
	if err != nil {
		return err
	}
	sensitivePolicyMutex.Lock()
	defer sensitivePolicyMutex.Unlock()
	SensitiveRuleChannelIds = channelIds
	return nil
}

func NormalizeSensitiveRuleChannelIds(channelIds []int) []int {
	return normalizePositiveSensitiveRuleIds(channelIds)
}

func NormalizeSensitiveRuleChannelTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(tags))
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
	}
	sort.Strings(result)
	return result
}

func NormalizeSensitiveRuleGroupCodes(codes []string) []string {
	if len(codes) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(codes))
	result := make([]string, 0, len(codes))
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" || strings.EqualFold(code, "auto") {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, code)
	}
	sort.Strings(result)
	return result
}

func normalizePositiveSensitiveRuleIds(ids []int) []int {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(ids))
	result := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sort.Ints(result)
	return result
}

type SensitiveRuleTargets struct {
	ChannelIds  []int
	ChannelTags []string
	GroupCodes  []string
	All         bool
}

// ResolveSensitiveRuleTargets 返回规则的实际路由范围。历史规则没有
// target_type 时继续继承全局渠道 Option，保证升级前后的过滤范围一致。
func (snapshot SensitivePolicySnapshot) ResolveSensitiveRuleTargets(rule SensitiveRule) SensitiveRuleTargets {
	switch rule.TargetType {
	case SensitiveRuleTargetChannels:
		return SensitiveRuleTargets{ChannelIds: NormalizeSensitiveRuleChannelIds(rule.ChannelIds)}
	case SensitiveRuleTargetChannelTags:
		return SensitiveRuleTargets{ChannelTags: NormalizeSensitiveRuleChannelTags(rule.ChannelTags)}
	case SensitiveRuleTargetGroups:
		return SensitiveRuleTargets{GroupCodes: NormalizeSensitiveRuleGroupCodes(rule.GroupCodes)}
	case SensitiveRuleTargetRoutes:
		return SensitiveRuleTargets{
			ChannelIds: NormalizeSensitiveRuleChannelIds(rule.ChannelIds),
			GroupCodes: NormalizeSensitiveRuleGroupCodes(rule.GroupCodes),
		}
	case SensitiveRuleTargetAll:
		return SensitiveRuleTargets{All: true}
	default:
		return SensitiveRuleTargets{ChannelIds: NormalizeSensitiveRuleChannelIds(snapshot.LegacyChannelIds)}
	}
}

func ResolveSensitiveRuleTargets(rule SensitiveRule) SensitiveRuleTargets {
	return GetSensitivePolicySnapshot().ResolveSensitiveRuleTargets(rule)
}

func SensitiveRuleHasRoutingTargets(rule SensitiveRule) bool {
	targets := ResolveSensitiveRuleTargets(rule)
	return targets.All || len(targets.ChannelIds) > 0 || len(targets.ChannelTags) > 0 || len(targets.GroupCodes) > 0
}

func validateSensitiveRuleTargets(rules []SensitiveRule) error {
	for index, rule := range rules {
		targetType := strings.ToLower(strings.TrimSpace(rule.TargetType))
		hasContent := len(normalizeSensitiveKeywords(rule.Keywords)) > 0 || len(normalizeSensitiveGroupRefs(rule.GroupRefs)) > 0
		switch targetType {
		case "":
			if len(rule.ChannelIds) > 0 || len(rule.ChannelTags) > 0 || len(rule.GroupCodes) > 0 {
				return fmt.Errorf("规则 %d 缺少 target_type", index+1)
			}
		case SensitiveRuleTargetChannels:
			if len(rule.ChannelTags) > 0 || len(rule.GroupCodes) > 0 {
				return fmt.Errorf("规则 %d 的渠道范围不能同时包含分组", index+1)
			}
			if rule.Enabled && hasContent && len(NormalizeSensitiveRuleChannelIds(rule.ChannelIds)) == 0 {
				return fmt.Errorf("规则 %d 必须至少选择一个渠道", index+1)
			}
		case SensitiveRuleTargetChannelTags:
			if len(rule.ChannelIds) > 0 || len(rule.GroupCodes) > 0 {
				return fmt.Errorf("规则 %d 的渠道标签分组范围不能同时包含渠道或业务分组", index+1)
			}
			if rule.Enabled && hasContent && len(NormalizeSensitiveRuleChannelTags(rule.ChannelTags)) == 0 {
				return fmt.Errorf("规则 %d 必须至少选择一个渠道分组", index+1)
			}
		case SensitiveRuleTargetGroups:
			if len(rule.ChannelIds) > 0 || len(rule.ChannelTags) > 0 {
				return fmt.Errorf("规则 %d 的分组范围不能同时包含渠道或渠道标签", index+1)
			}
			if rule.Enabled && hasContent && len(NormalizeSensitiveRuleGroupCodes(rule.GroupCodes)) == 0 {
				return fmt.Errorf("规则 %d 必须至少选择一个分组", index+1)
			}
		case SensitiveRuleTargetRoutes:
			if len(rule.ChannelTags) > 0 {
				return fmt.Errorf("规则 %d 的组合范围不能包含渠道标签", index+1)
			}
			if rule.Enabled && hasContent &&
				len(NormalizeSensitiveRuleChannelIds(rule.ChannelIds)) == 0 &&
				len(NormalizeSensitiveRuleGroupCodes(rule.GroupCodes)) == 0 {
				return fmt.Errorf("规则 %d 必须至少选择一个渠道或业务分组", index+1)
			}
		case SensitiveRuleTargetAll:
			if len(rule.ChannelIds) > 0 || len(rule.ChannelTags) > 0 || len(rule.GroupCodes) > 0 {
				return fmt.Errorf("规则 %d 的全部渠道范围不能同时包含其他目标", index+1)
			}
		default:
			return fmt.Errorf("规则 %d 的 target_type 无效", index+1)
		}
	}
	return nil
}

func ShouldApplySensitiveRulesToChannel(channelId int) bool {
	if channelId <= 0 {
		return false
	}
	for _, configuredId := range NormalizeSensitiveRuleChannelIds(GetSensitivePolicySnapshot().LegacyChannelIds) {
		if configuredId == channelId {
			return true
		}
	}
	return false
}

func NormalizeSensitiveRules(rules []SensitiveRule) []SensitiveRule {
	normalized := make([]SensitiveRule, 0, len(rules))
	for _, rule := range rules {
		rule.ID = strings.TrimSpace(rule.ID)
		rule.Name = strings.TrimSpace(rule.Name)
		rule.Action = strings.TrimSpace(strings.ToLower(rule.Action))
		rule.Scope = strings.TrimSpace(strings.ToLower(rule.Scope))
		rule.TargetType = strings.TrimSpace(strings.ToLower(rule.TargetType))
		rule.Replacement = strings.TrimSpace(rule.Replacement)
		if rule.Action != SensitiveRuleActionMask && rule.Action != SensitiveRuleActionBlock {
			rule.Action = SensitiveRuleActionBlock
		}
		if rule.Scope != SensitiveRuleScopeRequest && rule.Scope != SensitiveRuleScopeResponse && rule.Scope != SensitiveRuleScopeBoth {
			rule.Scope = SensitiveRuleScopeRequest
		}
		if rule.Action == SensitiveRuleActionMask && rule.Replacement == "" {
			rule.Replacement = DefaultSensitiveMaskReplacement
		}
		rule.Keywords = normalizeSensitiveKeywords(rule.Keywords)
		rule.GroupRefs = normalizeSensitiveGroupRefs(rule.GroupRefs)
		switch rule.TargetType {
		case SensitiveRuleTargetChannels:
			rule.ChannelIds = NormalizeSensitiveRuleChannelIds(rule.ChannelIds)
			rule.ChannelTags = nil
			rule.GroupCodes = nil
		case SensitiveRuleTargetChannelTags:
			rule.ChannelIds = nil
			rule.ChannelTags = NormalizeSensitiveRuleChannelTags(rule.ChannelTags)
			rule.GroupCodes = nil
		case SensitiveRuleTargetGroups:
			rule.ChannelIds = nil
			rule.ChannelTags = nil
			rule.GroupCodes = NormalizeSensitiveRuleGroupCodes(rule.GroupCodes)
		case SensitiveRuleTargetRoutes:
			rule.ChannelIds = NormalizeSensitiveRuleChannelIds(rule.ChannelIds)
			rule.ChannelTags = nil
			rule.GroupCodes = NormalizeSensitiveRuleGroupCodes(rule.GroupCodes)
		case SensitiveRuleTargetAll:
			rule.ChannelIds = nil
			rule.ChannelTags = nil
			rule.GroupCodes = nil
		default:
			rule.TargetType = ""
			rule.ChannelIds = nil
			rule.ChannelTags = nil
			rule.GroupCodes = nil
		}
		if len(rule.Keywords) == 0 && len(rule.GroupRefs) == 0 {
			continue
		}
		fallbackName := ""
		if len(rule.Keywords) > 0 {
			fallbackName = rule.Keywords[0]
		} else {
			fallbackName = rule.GroupRefs[0]
		}
		if rule.ID == "" {
			rule.ID = strings.ToLower(fallbackName)
		}
		if rule.Name == "" {
			rule.Name = fallbackName
		}
		normalized = append(normalized, rule)
	}
	return normalized
}

func (snapshot SensitivePolicySnapshot) GetEffectiveSensitiveRules() []SensitiveRule {
	rules := NormalizeSensitiveRules(snapshot.Rules)
	if len(rules) > 0 {
		return rules
	}
	if snapshot.RulesConfigured {
		return nil
	}
	keywords := normalizeSensitiveKeywords(snapshot.Words)
	if len(keywords) == 0 {
		return nil
	}
	return []SensitiveRule{
		{
			ID:       "legacy-sensitive-words",
			Name:     "Legacy sensitive words",
			Enabled:  true,
			Action:   SensitiveRuleActionBlock,
			Scope:    SensitiveRuleScopeRequest,
			Keywords: keywords,
		},
	}
}

func GetEffectiveSensitiveRules() []SensitiveRule {
	return GetSensitivePolicySnapshot().GetEffectiveSensitiveRules()
}

func (snapshot SensitivePolicySnapshot) GetEffectiveSensitiveRulesByScope(scope string) []SensitiveRule {
	scope = strings.TrimSpace(strings.ToLower(scope))
	if scope != SensitiveRuleScopeRequest && scope != SensitiveRuleScopeResponse {
		return nil
	}
	rules := snapshot.GetEffectiveSensitiveRules()
	if len(rules) == 0 {
		return nil
	}
	result := make([]SensitiveRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Scope == scope || rule.Scope == SensitiveRuleScopeBoth {
			result = append(result, rule)
		}
	}
	return result
}

func GetEffectiveSensitiveRulesByScope(scope string) []SensitiveRule {
	return GetSensitivePolicySnapshot().GetEffectiveSensitiveRulesByScope(scope)
}

func normalizeSensitiveKeywords(keywords []string) []string {
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

func normalizeSensitiveGroupRefs(groupRefs []string) []string {
	result := make([]string, 0, len(groupRefs))
	seen := make(map[string]struct{}, len(groupRefs))
	for _, groupRef := range groupRefs {
		groupRef = strings.TrimSpace(groupRef)
		if groupRef == "" {
			continue
		}
		key := strings.ToLower(groupRef)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, groupRef)
	}
	return result
}

//func ShouldCheckCompletionSensitive() bool {
//	return CheckSensitiveEnabled && CheckSensitiveOnCompletionEnabled
//}
