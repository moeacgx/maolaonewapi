package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestEnqueueChannelNotificationUsesNotificationCenterEvents(t *testing.T) {
	setupNotificationServiceTestDB(t)

	bot := &model.NotificationBot{Name: "channel bot", Token: "test-token", Enabled: true}
	require.NoError(t, model.CreateNotificationBot(bot))
	for _, eventType := range []string{
		model.NotificationEventTypeChannelDisabled,
		model.NotificationEventTypeChannelEnabled,
	} {
		task := &model.NotificationTask{
			Name:      eventType,
			EventType: eventType,
			BotId:     bot.Id,
			Template:  "{{channel_name}} {{channel_id}} {{reason}}",
			Enabled:   true,
		}
		require.NoError(t, model.CreateNotificationTask(task))
		target := &model.NotificationTarget{TaskId: task.Id, ChatId: "-10001", Enabled: true}
		require.NoError(t, model.CreateNotificationTarget(target))
	}

	enqueueChannelNotification(
		model.NotificationEventTypeChannelDisabled,
		42,
		"渠道 A",
		"status_code=403, upstream rejected",
		"upstream rejected",
		"bad_response_status_code",
		403,
	)
	enqueueChannelNotification(model.NotificationEventTypeChannelEnabled, 42, "渠道 A", "", "", "", 0)

	var events []model.NotificationEvent
	require.NoError(t, model.DB.Order("id asc").Find(&events).Error)
	require.Len(t, events, 2)
	var disabledPayload map[string]any
	require.NoError(t, common.UnmarshalJsonStr(events[0].Payload, &disabledPayload))
	require.Equal(t, float64(403), disabledPayload["status_code"])
	require.Equal(t, "bad_response_status_code", disabledPayload["error_code"])
	require.Equal(t, "upstream rejected", disabledPayload["error_message"])
	require.Equal(t, "status_code=403, upstream rejected", disabledPayload["reason"])

	var deliveryCount int64
	require.NoError(t, model.DB.Model(&model.NotificationDelivery{}).Count(&deliveryCount).Error)
	require.EqualValues(t, 2, deliveryCount)
}
