package model

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	PromptAuditOptionCheckSensitiveEnabled         = "CheckSensitiveEnabled"
	PromptAuditOptionCheckSensitiveOnPromptEnabled = "CheckSensitiveOnPromptEnabled"
	PromptAuditOptionSensitiveRules                = "SensitiveRules"
	PromptAuditOptionSensitiveRuleChannelIds       = "SensitiveRuleChannelIds"
)

// PromptAuditBuiltinPolicyUpdate 把无需 Guard 的审计开关与既有屏蔽词
// Option 作为一个版本化配置快照提交。SensitiveWords 属于只读旧配置，
// 迁移为规则后仍保留原值，避免保存页面时隐式删除用户数据。
type PromptAuditBuiltinPolicyUpdate struct {
	ExpectedVersion               int64
	UpstreamPolicyEnabled         bool
	UpstreamPolicyTargetType      string
	UpstreamPolicyChannelIds      string
	UpstreamPolicyGroupCodes      string
	SensitiveWordAuditEnabled     bool
	CyberPolicyAutoBanEnabled     bool
	CyberPolicyBanThreshold       int
	CyberPolicyWindowHours        int
	CheckSensitiveEnabled         bool
	CheckSensitiveOnPromptEnabled bool
	SensitiveRules                string
	SensitiveRuleChannelIds       string
	UpdatedBy                     int
	ChangeSummary                 string
}

// SavePromptAuditBuiltinPolicy 使用 prompt_audit_configs.config_version 做
// CAS，并在同一个数据库事务中保存审计开关和屏蔽词 Option。这样前端不会
// 看到“开关已更新但规则仍是旧值”的半提交状态。
func SavePromptAuditBuiltinPolicy(update PromptAuditBuiltinPolicyUpdate) error {
	if update.ExpectedVersion < 1 {
		return errors.New("prompt audit builtin policy version is invalid")
	}
	if err := validatePromptAuditCyberPolicyConfig(update.CyberPolicyBanThreshold, update.CyberPolicyWindowHours); err != nil {
		return err
	}
	if err := EnsurePromptAuditDefaults(); err != nil {
		return err
	}
	values := map[string]string{
		PromptAuditOptionCheckSensitiveEnabled:         boolOptionValue(update.CheckSensitiveEnabled),
		PromptAuditOptionCheckSensitiveOnPromptEnabled: boolOptionValue(update.CheckSensitiveOnPromptEnabled),
		PromptAuditOptionSensitiveRules:                update.SensitiveRules,
		PromptAuditOptionSensitiveRuleChannelIds:       update.SensitiveRuleChannelIds,
	}
	for key, value := range values {
		if err := validateOptionValue(key, value); err != nil {
			return err
		}
	}

	optionWriteMutex.Lock()
	defer optionWriteMutex.Unlock()
	if err := DB.Transaction(func(tx *gorm.DB) error {
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		keys = sortedUniqueOptionKeys(keys)
		if err := lockPromptAuditBuiltinOptionRows(tx, keys); err != nil {
			return err
		}
		// 通用 Option 写入也以“Option 行 -> 审计配置行”的顺序加锁。
		// 保持顺序一致，既避免跨进程死锁，也使 CAS 能检测旧页面的快照。
		now := time.Now().Unix()
		result := tx.Model(&PromptAuditConfig{}).
			Where("id = ? AND config_version = ?", PromptAuditConfigID, update.ExpectedVersion).
			Updates(map[string]interface{}{
				"config_version":                      update.ExpectedVersion + 1,
				"upstream_policy_enabled":             update.UpstreamPolicyEnabled,
				"upstream_policy_target_type":         update.UpstreamPolicyTargetType,
				"upstream_policy_channel_ids":         update.UpstreamPolicyChannelIds,
				"upstream_policy_group_codes":         update.UpstreamPolicyGroupCodes,
				"sensitive_word_audit_enabled":        update.SensitiveWordAuditEnabled,
				"cyber_policy_auto_ban_enabled":       update.CyberPolicyAutoBanEnabled,
				"cyber_policy_ban_threshold":          update.CyberPolicyBanThreshold,
				"cyber_policy_violation_window_hours": update.CyberPolicyWindowHours,
				"updated_at":                          now,
				"updated_by":                          update.UpdatedBy,
				"change_summary":                      update.ChangeSummary,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrPromptAuditConfigConflict
		}
		for _, key := range keys {
			option := Option{Key: key}
			if err := tx.FirstOrCreate(&option, Option{Key: key}).Error; err != nil {
				return err
			}
			option.Value = values[key]
			if err := tx.Save(&option).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	if err := publishPromptAuditBuiltinOptions(values); err != nil {
		return err
	}
	common.OptionMapRWMutex.Lock()
	for key, value := range values {
		common.OptionMap[key] = value
	}
	common.OptionMapRWMutex.Unlock()
	return nil
}

// publishPromptAuditBuiltinOptions 把共享 Option 一次发布为运行时快照。
// 调用前所有值必须已经校验，避免请求看到开关、规则和旧渠道范围的混合状态。
func publishPromptAuditBuiltinOptions(values map[string]string) error {
	snapshot := setting.GetSensitivePolicySnapshot()
	if value, ok := values[PromptAuditOptionCheckSensitiveEnabled]; ok {
		snapshot.CheckEnabled = value == "true"
	}
	if value, ok := values[PromptAuditOptionCheckSensitiveOnPromptEnabled]; ok {
		snapshot.CheckOnPromptEnabled = value == "true"
	}
	if value, ok := values[PromptAuditOptionSensitiveRules]; ok {
		rules, err := setting.ParseSensitiveRulesJSONString(value)
		if err != nil {
			return err
		}
		snapshot.Rules = rules
		snapshot.RulesConfigured = true
	}
	if value, ok := values[PromptAuditOptionSensitiveRuleChannelIds]; ok {
		channelIds, err := setting.ParseSensitiveRuleChannelIdsJSONString(value)
		if err != nil {
			return err
		}
		snapshot.LegacyChannelIds = channelIds
	}
	setting.ReplaceSensitivePolicySnapshot(snapshot)
	return nil
}

func isPromptAuditBuiltinOptionKey(key string) bool {
	switch key {
	case PromptAuditOptionCheckSensitiveEnabled,
		PromptAuditOptionCheckSensitiveOnPromptEnabled,
		PromptAuditOptionSensitiveRules,
		PromptAuditOptionSensitiveRuleChannelIds:
		return true
	default:
		return false
	}
}

func containsPromptAuditBuiltinOption(values map[string]string) bool {
	for key := range values {
		if isPromptAuditBuiltinOptionKey(key) {
			return true
		}
	}
	return false
}

// bumpPromptAuditConfigVersionForBuiltinOption 将旧设置入口纳入审计配置的
// CAS 语义。调用方已持有共享 Option 行锁，因此与页面保存使用相同的顺序。
func bumpPromptAuditConfigVersionForBuiltinOption(tx *gorm.DB) error {
	result := tx.Model(&PromptAuditConfig{}).
		Where("id = ?", PromptAuditConfigID).
		Updates(map[string]interface{}{
			"config_version": gorm.Expr("config_version + ?", 1),
			"updated_at":     time.Now().Unix(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("prompt audit config is missing")
	}
	return nil
}

func lockPromptAuditBuiltinOptionRows(tx *gorm.DB, keys []string) error {
	keys = sortedUniqueOptionKeys(keys)
	if len(keys) == 0 {
		return nil
	}
	var options []Option
	return lockForUpdate(tx.Model(&Option{})).
		Where(map[string]interface{}{"key": keys}).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "key"}}).
		Find(&options).Error
}

func boolOptionValue(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
