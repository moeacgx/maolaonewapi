package service

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

func enqueueChannelNotification(eventType string, channelId int, channelName, reason, errorMessage, errorCode string, statusCode int) {
	payload := map[string]any{
		"channel_id":    channelId,
		"channel_name":  channelName,
		"status_code":   statusCode,
		"error_message": errorMessage,
		"error_code":    errorCode,
	}
	if strings.TrimSpace(reason) != "" {
		payload["reason"] = reason
	}
	eventKey := fmt.Sprintf("channel:%d:%s:%s", channelId, eventType, common.GetUUID())
	if err := model.EnqueueNotificationEvent(eventType, eventKey, payload); err != nil {
		common.SysLog(fmt.Sprintf("failed to enqueue channel notification for channel %d: %s", channelId, err.Error()))
	}
}

// DisableChannel disables a failed channel and queues a Telegram notification event.
func DisableChannel(channelError types.ChannelError, reason string) {
	disableChannel(channelError, reason, reason, "", 0)
}

func DisableChannelWithError(channelError types.ChannelError, apiErr *types.NewAPIError) {
	if apiErr == nil {
		DisableChannel(channelError, "")
		return
	}
	disableChannel(channelError, apiErr.ErrorWithStatusCode(), apiErr.Error(), string(apiErr.GetErrorCode()), apiErr.StatusCode)
}

func disableChannel(channelError types.ChannelError, reason, errorMessage, errorCode string, statusCode int) {
	common.SysLog(fmt.Sprintf("通道「%s」（#%d）发生错误，准备禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, common.LocalLogPreview(reason)))

	if !channelError.AutoBan {
		common.SysLog(fmt.Sprintf("通道「%s」（#%d）未启用自动禁用功能，跳过禁用操作", channelError.ChannelName, channelError.ChannelId))
		return
	}

	success := model.UpdateChannelStatus(channelError.ChannelId, channelError.UsingKey, common.ChannelStatusAutoDisabled, reason)
	if success {
		enqueueChannelNotification(model.NotificationEventTypeChannelDisabled, channelError.ChannelId, channelError.ChannelName, reason, errorMessage, errorCode, statusCode)
	}
}

func EnableChannel(channelId int, usingKey string, channelName string) {
	success := model.UpdateChannelStatus(channelId, usingKey, common.ChannelStatusEnabled, "")
	if success {
		enqueueChannelNotification(model.NotificationEventTypeChannelEnabled, channelId, channelName, "", "", "", 0)
	}
}

func ShouldDisableChannel(err *types.NewAPIError) bool {
	if !common.AutomaticDisableChannelEnabled {
		return false
	}
	if err == nil {
		return false
	}
	if types.IsChannelError(err) {
		return true
	}
	if types.IsSkipRetryError(err) {
		return false
	}
	if operation_setting.ShouldDisableByStatusCode(err.StatusCode) {
		return true
	}

	lowerMessage := strings.ToLower(err.Error())
	search, _ := AcSearch(lowerMessage, operation_setting.AutomaticDisableKeywords, true)
	return search
}

func ShouldEnableChannel(newAPIError *types.NewAPIError, status int) bool {
	if !common.AutomaticEnableChannelEnabled {
		return false
	}
	if newAPIError != nil {
		return false
	}
	if status != common.ChannelStatusAutoDisabled {
		return false
	}
	return true
}
