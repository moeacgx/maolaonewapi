package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetConversationArchiveConfig(c *gin.Context) {
	cfg, err := service.GetConversationArchiveConfig(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "对话归档配置加载失败"})
		return
	}
	common.ApiSuccess(c, cfg)
}

func UpdateConversationArchiveConfig(c *gin.Context) {
	var req service.ConversationArchiveConfigUpdate
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "对话归档配置参数无效"})
		return
	}
	cfg, err := service.SaveConversationArchiveConfig(c.Request.Context(), req, c.GetInt("id"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrConversationArchiveConfigConflict) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	common.ApiSuccess(c, cfg)
}

func ListConversationArchives(c *gin.Context) {
	groupCodes := splitArchiveValues(append(c.QueryArray("group_code"), c.QueryArray("group_codes")...))
	userValues := splitArchiveValues(append(c.QueryArray("user_id"), c.QueryArray("user_ids")...))
	userIds := make([]int, 0, len(userValues))
	for _, value := range userValues {
		id, err := strconv.Atoi(value)
		if err != nil || id <= 0 || id > 1_000_000_000 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "用户 ID 无效"})
			return
		}
		userIds = append(userIds, id)
	}
	if len(groupCodes) > service.ConversationArchiveMaxGroups || len(userIds) > service.ConversationArchiveMaxUsers {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "归档筛选项数量超限"})
		return
	}
	filter := service.ConversationArchiveFilter{GroupCodes: groupCodes, UserIds: userIds}
	if len(groupCodes) == 1 {
		filter.GroupCode = groupCodes[0]
	}
	if len(userIds) == 1 {
		filter.UserId = &userIds[0]
	}
	filter.StartAt, _ = strconv.ParseInt(c.Query("start_at"), 10, 64)
	filter.EndAt, _ = strconv.ParseInt(c.Query("end_at"), 10, 64)
	filter.Page, _ = strconv.Atoi(c.Query("page"))
	filter.PageSize, _ = strconv.Atoi(c.Query("page_size"))
	rows, total, err := service.ListConversationArchives(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "对话归档加载失败"})
		return
	}
	common.ApiSuccess(c, gin.H{"items": rows, "total": total, "page": filter.Page, "page_size": filter.PageSize})
}

func splitArchiveValues(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '，' || r == ' ' || r == '\t' || r == '\n' }) {
			part = strings.TrimSpace(part)
			if part != "" && !seen[part] {
				seen[part] = true
				result = append(result, part)
			}
		}
	}
	return result
}

func GetConversationArchiveGroups(c *gin.Context) {
	groups, err := model.GetAllGroups(false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "分组列表加载失败"})
		return
	}
	type option struct {
		Id   int    `json:"id"`
		Code string `json:"code"`
		Name string `json:"name"`
	}
	items := make([]option, 0, len(groups))
	for _, group := range groups {
		if group != nil && group.Id > 0 && strings.TrimSpace(group.Code) != "" {
			items = append(items, option{Id: group.Id, Code: group.Code, Name: group.Name})
		}
	}
	common.ApiSuccess(c, items)
}

func GetConversationArchive(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	row, err := service.GetConversationArchive(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "对话归档不存在"})
		return
	}
	common.ApiSuccess(c, row)
}

func ClearConversationArchives(c *gin.Context) {
	var request struct {
		Confirm bool `json:"confirm"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || !request.Confirm {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "必须明确确认后才能清空对话归档"})
		return
	}
	deleted, err := service.ClearConversationArchives(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "清空对话归档失败"})
		return
	}
	common.ApiSuccess(c, gin.H{"deleted": deleted})
}
