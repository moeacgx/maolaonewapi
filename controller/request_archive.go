package controller

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetRequestArchiveConfig(c *gin.Context) {
	config, err := service.GetRequestArchiveConfig(c.Request.Context())
	if err != nil {
		writeRequestArchiveAdminError(c, http.StatusInternalServerError, "request_archive_config_load_failed", "完整请求归档配置加载失败")
		return
	}
	common.ApiSuccess(c, config)
}

func UpdateRequestArchiveConfig(c *gin.Context) {
	var request service.RequestArchiveUpdateRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		writeRequestArchiveAdminError(c, http.StatusBadRequest, "request_archive_invalid_request", "完整请求归档配置参数无效")
		return
	}
	config, err := service.SaveRequestArchiveConfig(c.Request.Context(), request, c.GetInt("id"))
	if err != nil {
		switch {
		case errors.Is(err, model.ErrRequestArchiveConfigConflict):
			writeRequestArchiveAdminError(c, http.StatusConflict, "request_archive_config_conflict", "完整请求归档配置已被其他管理员更新，请刷新后重试")
		case errors.Is(err, model.ErrRequestArchiveTargetInUse):
			writeRequestArchiveAdminError(c, http.StatusConflict, "request_archive_target_in_use", "存储目标仍有关联的归档任务或对象，请新增目标并切换后重试")
		case errors.Is(err, service.ErrRequestArchivePersistence), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			writeRequestArchiveAdminError(c, http.StatusInternalServerError, "request_archive_config_save_failed", "完整请求归档配置保存失败")
		default:
			writeRequestArchiveAdminError(c, http.StatusBadRequest, "request_archive_config_invalid", err.Error())
		}
		return
	}
	recordRequestArchiveAdminLog(c, "更新了完整请求归档配置", gin.H{
		"config_version": config.ConfigVersion,
		"enabled":        config.Enabled,
		"archive_scope":  config.ArchiveScope,
		"target_count":   len(config.Targets),
	})
	common.ApiSuccess(c, config)
}

func ProbeRequestArchiveTarget(c *gin.Context) {
	var target service.RequestArchiveUpdateTarget
	if err := common.DecodeJson(c.Request.Body, &target); err != nil {
		writeRequestArchiveAdminError(c, http.StatusBadRequest, "request_archive_invalid_request", "请求归档存储目标探测参数无效")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	result, err := service.ProbeRequestArchiveTarget(ctx, target)
	if err != nil {
		if errors.Is(err, service.ErrRequestArchivePersistence) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			writeRequestArchiveAdminError(c, http.StatusInternalServerError, "request_archive_target_probe_failed", "请求归档存储目标探测失败")
		} else {
			writeRequestArchiveAdminError(c, http.StatusBadRequest, "request_archive_target_invalid", err.Error())
		}
		return
	}
	recordRequestArchiveAdminLog(c, "探测了完整请求归档存储目标", gin.H{
		"target_id": target.Id, "status": result.Status, "error_code": result.ErrorCode,
	})
	common.ApiSuccess(c, result)
}

func GetRequestArchiveRuntime(c *gin.Context) {
	runtime, err := service.GetRequestArchiveRuntimeSnapshot(c.Request.Context())
	if err != nil {
		writeRequestArchiveAdminError(c, http.StatusInternalServerError, "request_archive_runtime_failed", "完整请求归档运行状态加载失败")
		return
	}
	common.ApiSuccess(c, runtime)
}

func writeRequestArchiveAdminError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"success": false, "message": message, "code": code})
}

func recordRequestArchiveAdminLog(c *gin.Context, content string, details map[string]interface{}) {
	actorID := c.GetInt("id")
	adminInfo := map[string]interface{}{
		"admin_id": actorID, "admin_username": c.GetString("username"),
	}
	for key, value := range details {
		adminInfo[key] = value
	}
	model.RecordLogWithAdminInfo(actorID, model.LogTypeManage, content, adminInfo)
}
