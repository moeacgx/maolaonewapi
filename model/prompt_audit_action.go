package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
)

const (
	promptAuditUpstreamPolicySource = "upstream_policy"
	promptAuditCyberPolicyCode      = "cyber_policy"
	promptAuditMaxBanThreshold      = 1000000
	promptAuditMaxWindowHours       = 87600
)

func validatePromptAuditCyberPolicyConfig(threshold, windowHours int) error {
	if threshold < 1 || threshold > promptAuditMaxBanThreshold ||
		windowHours < 1 || windowHours > promptAuditMaxWindowHours {
		return errors.New("cyber policy auto-ban config is invalid")
	}
	return nil
}

// DisableCommonUserOnCyberPolicyThreshold 只统计精确的上游策略事件，
// 并通过角色、状态条件更新保证并发命中时最多一个执行者完成禁用。
func DisableCommonUserOnCyberPolicyThreshold(userId int, since, until int64, threshold int) (int64, bool, error) {
	if userId <= 0 || since <= 0 || until < since || threshold < 1 {
		return 0, false, errors.New("cyber policy auto-ban parameters are invalid")
	}
	var count int64
	if err := DB.Model(&PromptAuditEvent{}).
		Where("user_id = ? AND source = ? AND error_code = ? AND created_at >= ? AND created_at <= ?",
			userId, promptAuditUpstreamPolicySource, promptAuditCyberPolicyCode, since, until).
		Count(&count).Error; err != nil {
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
