package model

import (
	"errors"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	promptAuditUpstreamPolicySource = "upstream_policy"
	promptAuditCyberPolicyCode      = "cyber_policy"
	promptAuditMaxBanThreshold      = 1000000
	promptAuditMaxWindowHours       = 87600
)

// PromptAuditCyberPolicyScope 描述官方风控事件累计和自动禁用使用的当前作用范围。
// 事件已经固化实际渠道与路由分组，因此统计可以在三种数据库上使用普通 GORM 条件完成。
type PromptAuditCyberPolicyScope struct {
	TargetType       string
	ChannelIDs       []int
	GroupCodes       []string
	ExemptGroupCodes []string
}

func validatePromptAuditCyberPolicyConfig(threshold, windowHours int) error {
	if threshold < 1 || threshold > promptAuditMaxBanThreshold ||
		windowHours < 1 || windowHours > promptAuditMaxWindowHours {
		return errors.New("cyber policy auto-ban config is invalid")
	}
	return nil
}

// DisableCommonUserOnCyberPolicyThreshold 只统计精确的上游策略事件，
// 并通过角色、状态条件更新保证并发命中时最多一个执行者完成禁用。
func DisableCommonUserOnCyberPolicyThreshold(userId int, since, until int64, threshold int, scope PromptAuditCyberPolicyScope) (int64, bool, error) {
	if userId <= 0 || since <= 0 || until < since || threshold < 1 {
		return 0, false, errors.New("cyber policy auto-ban parameters are invalid")
	}
	var count int64
	query := DB.Model(&PromptAuditEvent{}).
		Where("user_id = ? AND source = ? AND error_code = ? AND created_at >= ? AND created_at <= ?",
			userId, promptAuditUpstreamPolicySource, promptAuditCyberPolicyCode, since, until)
	query, err := applyPromptAuditCyberPolicyScope(query, scope)
	if err != nil {
		return 0, false, err
	}
	if err := query.Count(&count).Error; err != nil {
		return 0, false, err
	}
	if count < int64(threshold) {
		return count, false, nil
	}
	result := DB.Model(&User{}).
		Where("id = ? AND role = ? AND status = ?",
			userId, common.RoleCommonUser, common.UserStatusEnabled).
		Update("status", common.UserStatusDisabled)
	if result.Error != nil {
		return count, false, result.Error
	}
	return count, result.RowsAffected == 1, nil
}

// CountCyberPolicyEventsByUsers 返回指定用户在时间窗口内的官方风控累计次数。
// 统计条件必须与自动封禁保持一致，避免列表展示的次数和实际封禁判断不一致。
func CountCyberPolicyEventsByUsers(userIds []int, since, until int64, scope PromptAuditCyberPolicyScope) (map[int]int64, error) {
	counts := make(map[int]int64, len(userIds))
	if len(userIds) == 0 {
		return counts, nil
	}
	if since <= 0 || until < since {
		return nil, errors.New("cyber policy count time window is invalid")
	}
	uniqueIds := make([]int, 0, len(userIds))
	seen := make(map[int]struct{}, len(userIds))
	for _, userId := range userIds {
		if userId <= 0 {
			continue
		}
		if _, exists := seen[userId]; exists {
			continue
		}
		seen[userId] = struct{}{}
		uniqueIds = append(uniqueIds, userId)
	}
	if len(uniqueIds) == 0 {
		return counts, nil
	}
	type countRow struct {
		UserId int   `gorm:"column:user_id"`
		Count  int64 `gorm:"column:count"`
	}
	rows := make([]countRow, 0, len(uniqueIds))
	query := DB.Model(&PromptAuditEvent{}).
		Select("user_id, COUNT(*) AS count").
		Where("user_id IN ? AND source = ? AND error_code = ? AND created_at >= ? AND created_at <= ?",
			uniqueIds, promptAuditUpstreamPolicySource, promptAuditCyberPolicyCode, since, until)
	query, err := applyPromptAuditCyberPolicyScope(query, scope)
	if err != nil {
		return nil, err
	}
	if err := query.Group("user_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.UserId] = row.Count
	}
	return counts, nil
}

func applyPromptAuditCyberPolicyScope(query *gorm.DB, scope PromptAuditCyberPolicyScope) (*gorm.DB, error) {
	if query == nil {
		return query, nil
	}
	switch scope.TargetType {
	case "channels":
		if len(scope.ChannelIDs) == 0 {
			return query.Where("1 = 0"), nil
		}
		query = query.Where("channel_id IN ?", scope.ChannelIDs)
	case "groups":
		if len(scope.GroupCodes) == 0 {
			return query.Where("1 = 0"), nil
		}
		groupCodes, err := ExpandPromptAuditGroupIdentifiers(scope.GroupCodes)
		if err != nil {
			return nil, err
		}
		if len(groupCodes) == 0 {
			return query.Where("1 = 0"), nil
		}
		query = query.Where("group_code IN ?", groupCodes)
	}
	if len(scope.ExemptGroupCodes) > 0 {
		exemptGroupCodes, err := ExpandPromptAuditGroupIdentifiers(scope.ExemptGroupCodes)
		if err != nil {
			return nil, err
		}
		// 白名单开启后，升级前没有分组快照的旧事件不能证明来自非白名单分组，
		// 因此不参与惩罚性累计；当前 code 与历史别名按同一分组身份排除。
		query = query.Where("group_code <> ?", "")
		if len(exemptGroupCodes) > 0 {
			query = query.Where("group_code NOT IN ?", exemptGroupCodes)
		}
	}
	return query, nil
}

// ExpandPromptAuditGroupIdentifiers 同时返回当前 code 和仍有效的历史别名。
// 审计事件保留发生时的不可变分组快照，因此显式 code 迁移后只能在查询边界
// 合并身份，不能批量改写历史事件。输入本身始终保留，以兼容迁移期间的短期缓存。
func ExpandPromptAuditGroupIdentifiers(codes []string) ([]string, error) {
	identifiers := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code != "" {
			identifiers[code] = struct{}{}
		}
	}
	if len(identifiers) == 0 {
		return []string{}, nil
	}
	if DB == nil || !DB.Migrator().HasTable(&Group{}) {
		return sortedPromptAuditGroupIdentifiers(identifiers), nil
	}

	inputs := sortedPromptAuditGroupIdentifiers(identifiers)
	for _, code := range inputs {
		group, err := GetGroupByCodeOrAliasWithDB(DB, code)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return nil, err
		}
		legacyIdentifiers, _, err := groupLegacyIdentifiers(DB, group)
		if err != nil {
			return nil, err
		}
		for _, identifier := range legacyIdentifiers {
			identifier = strings.TrimSpace(identifier)
			if identifier != "" {
				identifiers[identifier] = struct{}{}
			}
		}
	}
	return sortedPromptAuditGroupIdentifiers(identifiers), nil
}

func sortedPromptAuditGroupIdentifiers(identifiers map[string]struct{}) []string {
	result := make([]string, 0, len(identifiers))
	for identifier := range identifiers {
		result = append(result, identifier)
	}
	sort.Strings(result)
	return result
}
