package service

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// DisplayGroupName 返回面向客户端的分组显示名称。
// 内部路由、计费和日志仍使用稳定 code；这里只影响错误消息等展示文本。
func DisplayGroupName(c *gin.Context, group string) string {
	group = strings.TrimSpace(group)
	if group == "" {
		return group
	}
	if strings.Contains(group, ",") {
		return DisplayGroupList(c, group)
	}
	if name := displayGroupNameFromContext(c, group); name != "" {
		return name
	}
	if model.DB != nil {
		if groupNames, err := model.GetGroupDisplayNameMap(); err == nil {
			if name := strings.TrimSpace(groupNames[group]); name != "" {
				return name
			}
		}
	}
	return group
}

// DisplayGroupList 将逗号分隔的多分组 code 转换为显示名称列表。
func DisplayGroupList(c *gin.Context, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	groups, err := ParseTokenGroupList(raw)
	if err != nil || len(groups) == 0 {
		parts := strings.Split(raw, ",")
		groups = groups[:0]
		for _, part := range parts {
			group := strings.TrimSpace(part)
			if group != "" {
				groups = append(groups, group)
			}
		}
		if len(groups) == 0 {
			return raw
		}
	}
	display := make([]string, 0, len(groups))
	for _, group := range groups {
		display = append(display, DisplayGroupName(c, group))
	}
	return strings.Join(display, ",")
}

// DisplaySelectedGroupName 返回一次渠道选择对外展示的分组名称。
// tokenGroup 保留令牌原始选择；selectedGroup 是本次实际命中的业务分组。
func DisplaySelectedGroupName(c *gin.Context, tokenGroup, selectedGroup string) string {
	tokenGroup = strings.TrimSpace(tokenGroup)
	selectedGroup = strings.TrimSpace(selectedGroup)
	if strings.EqualFold(tokenGroup, model.TokenGroupModeAuto) {
		if selectedGroup == "" || strings.EqualFold(selectedGroup, model.TokenGroupModeAuto) {
			return model.TokenGroupModeAuto
		}
		return "auto(" + DisplayGroupName(c, selectedGroup) + ")"
	}
	return DisplayGroupList(c, tokenGroup)
}

func displayGroupNameFromContext(c *gin.Context, group string) string {
	if c == nil {
		return ""
	}
	details, ok := common.GetContextKeyType[[]model.GroupReference](c, constant.ContextKeyTokenGroupDetails)
	if !ok {
		return ""
	}
	for _, detail := range details {
		if strings.EqualFold(strings.TrimSpace(detail.Code), group) {
			if name := strings.TrimSpace(detail.Name); name != "" {
				return name
			}
		}
	}
	return ""
}
