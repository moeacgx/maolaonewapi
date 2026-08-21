package model

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/utils/tests"
)

func setupNotificationTestDB(t *testing.T) {
	t.Helper()
	originalDB := DB
	originalDBType := common.MainDatabaseType()

	dsn := fmt.Sprintf("file:notification-%s?mode=memory&cache=shared&_pragma=busy_timeout(5000)", strings.ReplaceAll(t.Name(), "/", "-"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)

	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	require.NoError(t, DB.AutoMigrate(
		&NotificationBot{},
		&NotificationTask{},
		&NotificationTarget{},
		&NotificationEventReceipt{},
		&NotificationEvent{},
		&NotificationDelivery{},
	))

	t.Cleanup(func() {
		_ = sqlDB.Close()
		DB = originalDB
		common.SetMainDatabaseType(originalDBType)
	})
}

func createNotificationFixture(t *testing.T) (*NotificationBot, *NotificationTask, *NotificationTarget) {
	t.Helper()
	bot := &NotificationBot{Name: "invoice bot", Token: "secret-token", Enabled: true}
	require.NoError(t, CreateNotificationBot(bot))
	task := &NotificationTask{Name: "invoice pending", EventType: NotificationEventTypeInvoicePending, BotId: bot.Id, Enabled: true}
	require.NoError(t, CreateNotificationTask(task))
	target := &NotificationTarget{TaskId: task.Id, ChatId: "-10001", MentionUserId: "42", MentionName: "管理员", Enabled: true}
	require.NoError(t, CreateNotificationTarget(target))
	return bot, task, target
}

func TestNotificationBotTokenIsNeverMarshaled(t *testing.T) {
	bot := NotificationBot{Id: 1, Name: "bot", Type: NotificationEndpointTypeTelegram, Token: "must-not-leak", Enabled: true}
	data, err := common.Marshal(bot)
	require.NoError(t, err)
	require.NotContains(t, string(data), "must-not-leak")
	require.NotContains(t, string(data), "token")
}

func TestNotificationBotTokenEncryptedRotationAndLegacyMigration(t *testing.T) {
	setupNotificationTestDB(t)
	originalSecret := common.CryptoSecret
	common.CryptoSecret = "notification-test-secret"
	t.Cleanup(func() { common.CryptoSecret = originalSecret })

	bot := &NotificationBot{Name: "encrypted bot", Token: "first-secret-token", Enabled: true}
	require.NoError(t, CreateNotificationBot(bot))
	require.True(t, strings.HasPrefix(bot.Token, notificationTokenCipherPrefix))
	require.NotContains(t, bot.Token, "first-secret-token")
	plaintext, err := NotificationBotToken(bot.Id)
	require.NoError(t, err)
	require.Equal(t, "first-secret-token", plaintext)

	rotated := "rotated-secret-token"
	require.NoError(t, UpdateNotificationBot(bot, &rotated))
	var stored NotificationBot
	require.NoError(t, DB.First(&stored, bot.Id).Error)
	require.True(t, strings.HasPrefix(stored.Token, notificationTokenCipherPrefix))
	require.NotContains(t, stored.Token, rotated)
	plaintext, err = NotificationBotToken(bot.Id)
	require.NoError(t, err)
	require.Equal(t, rotated, plaintext)

	legacy := NotificationBot{Name: "legacy bot", Type: NotificationEndpointTypeTelegram, Token: "legacy-plaintext-token", Enabled: true}
	require.NoError(t, DB.Create(&legacy).Error)
	plaintext, err = NotificationBotToken(legacy.Id)
	require.NoError(t, err)
	require.Equal(t, "legacy-plaintext-token", plaintext)
	require.NoError(t, DB.First(&legacy, legacy.Id).Error)
	require.True(t, strings.HasPrefix(legacy.Token, notificationTokenCipherPrefix))
	require.NotContains(t, legacy.Token, "legacy-plaintext-token")
}

func TestNotificationBotTokenWrongKeyFailsClosedWithoutSecretExposure(t *testing.T) {
	setupNotificationTestDB(t)
	originalSecret := common.CryptoSecret
	common.CryptoSecret = "original-notification-key"
	t.Cleanup(func() { common.CryptoSecret = originalSecret })
	bot := &NotificationBot{Name: "bot", Token: "never-expose-this-token", Enabled: true}
	require.NoError(t, CreateNotificationBot(bot))
	common.CryptoSecret = "different-notification-key"
	_, err := NotificationBotToken(bot.Id)
	require.ErrorIs(t, err, ErrNotificationTokenUnavailable)
	require.NotContains(t, err.Error(), "never-expose-this-token")
	require.NotContains(t, err.Error(), bot.Token)
}

func TestNotificationBotTestResultStoresOnlyStableStatus(t *testing.T) {
	setupNotificationTestDB(t)
	bot := &NotificationBot{Name: "bot", Token: "secret", Enabled: true}
	require.NoError(t, CreateNotificationBot(bot))
	require.NoError(t, RecordNotificationBotTestResult(bot.Id, false))
	var stored NotificationBot
	require.NoError(t, DB.First(&stored, bot.Id).Error)
	require.Positive(t, stored.LastTestAt)
	require.Equal(t, "telegram test failed", stored.LastTestError)
	data, err := common.Marshal(NotificationBotView{NotificationBot: stored, TokenConfigured: true})
	require.NoError(t, err)
	require.NotContains(t, string(data), "secret")
	require.NotContains(t, string(data), `"token"`)

	require.NoError(t, RecordNotificationBotTestResult(bot.Id, true))
	require.NoError(t, DB.First(&stored, bot.Id).Error)
	require.Empty(t, stored.LastTestError)
}

func TestNotificationBotViewsAreAscendingAndSecretFree(t *testing.T) {
	setupNotificationTestDB(t)
	first := &NotificationBot{Name: "first", Token: "first-token", Enabled: true}
	second := &NotificationBot{Name: "second", Token: "second-token", Enabled: true}
	require.NoError(t, CreateNotificationBot(first))
	require.NoError(t, CreateNotificationBot(second))
	views, err := NotificationBotViews()
	require.NoError(t, err)
	require.Len(t, views, 2)
	require.Equal(t, []int{first.Id, second.Id}, []int{views[0].Id, views[1].Id})
	data, err := common.Marshal(views)
	require.NoError(t, err)
	require.NotContains(t, string(data), "first-token")
	require.NotContains(t, string(data), "second-token")
	require.Contains(t, string(data), `"token_configured":true`)
}

func TestNotificationTaskOnlyReceivesNewEvents(t *testing.T) {
	setupNotificationTestDB(t)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:old", map[string]any{"invoice_id": 1})
	}))
	_, task, _ := createNotificationFixture(t)

	var deliveries int64
	require.NoError(t, DB.Model(&NotificationDelivery{}).Count(&deliveries).Error)
	require.Zero(t, deliveries)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:new", map[string]any{"invoice_id": 2})
	}))
	require.NoError(t, DB.Model(&NotificationDelivery{}).Count(&deliveries).Error)
	require.EqualValues(t, 1, deliveries)

	require.NoError(t, SetNotificationTaskEnabled(task.Id, false))
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:disabled", map[string]any{"invoice_id": 3})
	}))
	require.NoError(t, SetNotificationTaskEnabled(task.Id, true))
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:reenabled", map[string]any{"invoice_id": 4})
	}))
	require.NoError(t, DB.Model(&NotificationDelivery{}).Count(&deliveries).Error)
	require.EqualValues(t, 2, deliveries)
}

func TestUpdateNotificationTaskCancelsDeliveriesFromOldEventType(t *testing.T) {
	setupNotificationTestDB(t)
	_, task, _ := createNotificationFixture(t)
	for i := 1; i <= 3; i++ {
		require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
			return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, fmt.Sprintf("invoice:event-type-%d", i), map[string]any{"invoice_id": i})
		}))
	}

	var deliveries []NotificationDelivery
	require.NoError(t, DB.Order("id asc").Find(&deliveries).Error)
	require.Len(t, deliveries, 3)
	require.NoError(t, DB.Model(&NotificationDelivery{}).Where("id = ?", deliveries[1].Id).Update("status", NotificationDeliveryRetrying).Error)
	require.NoError(t, DB.Model(&NotificationDelivery{}).Where("id = ?", deliveries[2].Id).Update("status", NotificationDeliveryClaimed).Error)

	task.EventType = "extension.test.created"
	require.NoError(t, UpdateNotificationTask(task))

	var canceled []NotificationDelivery
	require.NoError(t, DB.Order("id asc").Find(&canceled).Error)
	require.Len(t, canceled, 3)
	for _, delivery := range canceled {
		require.Equal(t, NotificationDeliveryCanceled, delivery.Status)
		require.Equal(t, "notification task event type changed", delivery.LastError)
	}

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:old-type-after-update", map[string]any{"invoice_id": 4})
	}))
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, "extension.test.created", "extension:test:new-type", map[string]any{"id": 1})
	}))
	var deliveryCount int64
	require.NoError(t, DB.Model(&NotificationDelivery{}).Count(&deliveryCount).Error)
	require.EqualValues(t, 4, deliveryCount, "旧事件类型不应再投递，新事件类型应从新的基线开始")
}

func TestNotificationTaskLastTriggeredAtUsesLatestDelivery(t *testing.T) {
	setupNotificationTestDB(t)
	_, task, _ := createNotificationFixture(t)
	triggeredAt, err := NotificationTaskLastTriggeredAt()
	require.NoError(t, err)
	require.NotContains(t, triggeredAt, task.Id)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:last-triggered", map[string]any{"invoice_id": 5})
	}))
	triggeredAt, err = NotificationTaskLastTriggeredAt()
	require.NoError(t, err)
	require.Positive(t, triggeredAt[task.Id])
}

func TestCreateNotificationTaskRejectsMissingBotInsideSequence(t *testing.T) {
	setupNotificationTestDB(t)
	task := &NotificationTask{Name: "orphan", EventType: "event", BotId: 999, Enabled: true}
	require.ErrorIs(t, CreateNotificationTask(task), gorm.ErrRecordNotFound)
}

func TestEnqueueNotificationEventIsIdempotent(t *testing.T) {
	setupNotificationTestDB(t)
	createNotificationFixture(t)
	for range 3 {
		require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
			return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:dedupe", map[string]any{"invoice_id": 9})
		}))
	}
	var eventCount, deliveryCount int64
	require.NoError(t, DB.Model(&NotificationEvent{}).Count(&eventCount).Error)
	require.NoError(t, DB.Model(&NotificationDelivery{}).Count(&deliveryCount).Error)
	require.EqualValues(t, 1, eventCount)
	require.EqualValues(t, 1, deliveryCount)
}

func TestDisabledBotDoesNotRetainNotificationEvent(t *testing.T) {
	setupNotificationTestDB(t)
	bot, _, _ := createNotificationFixture(t)
	require.NoError(t, DB.Model(&NotificationBot{}).Where("id = ?", bot.Id).Update("enabled", false).Error)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:disabled-bot", map[string]any{"invoice_id": 10})
	}))

	var eventCount, deliveryCount int64
	require.NoError(t, DB.Model(&NotificationEvent{}).Count(&eventCount).Error)
	require.NoError(t, DB.Model(&NotificationDelivery{}).Count(&deliveryCount).Error)
	require.Zero(t, eventCount)
	require.Zero(t, deliveryCount)
}

func TestDisableNotificationBotCancelsUnsentDeliveries(t *testing.T) {
	setupNotificationTestDB(t)
	bot, _, _ := createNotificationFixture(t)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:disable-bot", map[string]any{"invoice_id": 10})
	}))
	bot.Enabled = false
	require.NoError(t, UpdateNotificationBot(bot, nil))
	var stored NotificationBot
	require.NoError(t, DB.First(&stored, bot.Id).Error)
	require.False(t, stored.Enabled)
	var delivery NotificationDelivery
	require.NoError(t, DB.First(&delivery).Error)
	require.Equal(t, NotificationDeliveryCanceled, delivery.Status)
	require.Equal(t, "notification bot disabled", delivery.LastError)
}

func TestStaleSendingDeliveryBecomesDeadWithoutAutomaticRetry(t *testing.T) {
	setupNotificationTestDB(t)
	createNotificationFixture(t)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:unknown-result", map[string]any{"invoice_id": 11})
	}))
	now := time.Now().Unix()
	work, err := ClaimNotificationDeliveries(10, now, 120)
	require.NoError(t, err)
	require.Len(t, work, 1)
	deliveryID := work[0].Delivery.Id
	_, err = MarkNotificationDeliverySending(deliveryID, now)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&NotificationDelivery{}).Where("id = ?", deliveryID).Update("updated_at", now-180).Error)

	work, err = ClaimNotificationDeliveries(10, now, 120)
	require.NoError(t, err)
	require.Empty(t, work)
	var delivery NotificationDelivery
	require.NoError(t, DB.First(&delivery, deliveryID).Error)
	require.Equal(t, NotificationDeliveryDead, delivery.Status)
	require.Contains(t, delivery.LastError, "duplicate notification")
}

func TestNotificationHistoryKeepsFiveTerminalEventsAndPending(t *testing.T) {
	setupNotificationTestDB(t)
	_, task, _ := createNotificationFixture(t)
	for i := 1; i <= 7; i++ {
		key := fmt.Sprintf("invoice:%d", i)
		require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
			return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, key, map[string]any{"invoice_id": i})
		}))
	}
	work, err := ClaimNotificationDeliveries(20, 0, 120)
	require.NoError(t, err)
	require.Len(t, work, 7)
	for _, item := range work {
		_, err := MarkNotificationDeliverySending(item.Delivery.Id, 0)
		require.NoError(t, err)
		require.NoError(t, MarkNotificationDeliverySuccess(item.Delivery.Id, 0))
	}
	var terminalCount int64
	require.NoError(t, DB.Model(&NotificationDelivery{}).Where("task_id = ?", task.Id).Count(&terminalCount).Error)
	require.EqualValues(t, 5, terminalCount)
	var receiptCount int64
	require.NoError(t, DB.Model(&NotificationEventReceipt{}).Where("dedupe_key <> ?", notificationSequenceLockKey).Count(&receiptCount).Error)
	require.EqualValues(t, 7, receiptCount)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:1", map[string]any{"invoice_id": 1})
	}))
	work, err = ClaimNotificationDeliveries(20, 0, 120)
	require.NoError(t, err)
	require.Empty(t, work, "历史 payload 被裁剪后仍必须通过哈希凭据保持幂等")

	for i := 8; i <= 9; i++ {
		key := fmt.Sprintf("invoice:%d", i)
		require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
			return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, key, map[string]any{"invoice_id": i})
		}))
	}
	work, err = ClaimNotificationDeliveries(20, 0, 120)
	require.NoError(t, err)
	require.Len(t, work, 2)
	_, err = MarkNotificationDeliverySending(work[0].Delivery.Id, 0)
	require.NoError(t, err)
	_, err = MarkNotificationDeliverySending(work[1].Delivery.Id, 0)
	require.NoError(t, err)
	require.NoError(t, MarkNotificationDeliveryRetry(work[0].Delivery.Id, 9999999999, "temporary"))
	require.NoError(t, MarkNotificationDeliverySuccess(work[1].Delivery.Id, 0))

	var retryCount int64
	require.NoError(t, DB.Model(&NotificationDelivery{}).Where("task_id = ? AND status = ?", task.Id, NotificationDeliveryRetrying).Count(&retryCount).Error)
	require.EqualValues(t, 1, retryCount)
	require.NoError(t, DB.Model(&NotificationDelivery{}).Where("task_id = ? AND status IN ?", task.Id, []string{NotificationDeliverySuccess, NotificationDeliveryDead, NotificationDeliveryCanceled}).Count(&terminalCount).Error)
	require.EqualValues(t, 5, terminalCount)
}
func TestEnqueueNotificationEventReturnsStatus(t *testing.T) {
	setupNotificationTestDB(t)
	createNotificationFixture(t)

	var first NotificationEnqueueResult
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		first, err = EnqueueNotificationEventTxWithResult(tx, NotificationEventTypeInvoicePending, "invoice:status", map[string]any{"invoice_id": 1})
		return err
	}))
	require.Equal(t, NotificationEnqueueQueued, first.Status)
	require.Equal(t, 1, first.DeliveryCount)

	var duplicate NotificationEnqueueResult
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		duplicate, err = EnqueueNotificationEventTxWithResult(tx, NotificationEventTypeInvoicePending, "invoice:status", map[string]any{"invoice_id": 1})
		return err
	}))
	require.Equal(t, NotificationEnqueueDuplicate, duplicate.Status)
	require.Zero(t, duplicate.DeliveryCount)

	var withoutSubscriber NotificationEnqueueResult
	require.NoError(t, DB.Model(&NotificationTask{}).Where("event_type = ?", NotificationEventTypeInvoicePending).Update("enabled", false).Error)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		withoutSubscriber, err = EnqueueNotificationEventTxWithResult(tx, NotificationEventTypeInvoicePending, "invoice:no-subscriber", map[string]any{"invoice_id": 2})
		return err
	}))
	require.Equal(t, NotificationEnqueueAcceptedWithoutSubscriber, withoutSubscriber.Status)
	require.Zero(t, withoutSubscriber.DeliveryCount)
}

func TestNotificationReceiptClaimUsesStoredToken(t *testing.T) {
	setupNotificationTestDB(t)
	now := time.Now().Unix()
	existingKey := notificationEventDedupeKey("event", "existing")
	require.NoError(t, DB.Create(&NotificationEventReceipt{
		DedupeKey: existingKey, ClaimToken: "first-writer", CreatedAt: now,
	}).Error)

	var claimed bool
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		claimed, err = claimNotificationReceiptTx(tx, existingKey, now)
		return err
	}))
	require.False(t, claimed, "重复判断必须以回读的 claim token 为准，不能依赖 RowsAffected")

	newKey := notificationEventDedupeKey("event", "new")
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		claimed, err = claimNotificationReceiptTx(tx, newKey, now)
		return err
	}))
	require.True(t, claimed)
}

func TestExpiredNotificationReceiptsAreCleanedWithoutSubscriber(t *testing.T) {
	setupNotificationTestDB(t)
	now := time.Now().Unix()
	eventType := "extension.test.created"
	eventKey := "without-subscriber"
	currentExpiredKey := notificationEventDedupeKey(eventType, eventKey)
	otherExpiredKey := notificationEventDedupeKey("event", "expired")
	freshKey := notificationEventDedupeKey("event", "fresh")
	require.NoError(t, DB.Create(&NotificationEventReceipt{
		DedupeKey: currentExpiredKey, ClaimToken: "expired-current", CreatedAt: now - NotificationEventReceiptTTLSeconds - 1,
	}).Error)
	require.NoError(t, DB.Create(&NotificationEventReceipt{
		DedupeKey: otherExpiredKey, ClaimToken: "expired-other", CreatedAt: now - NotificationEventReceiptTTLSeconds - 1,
	}).Error)
	require.NoError(t, DB.Create(&NotificationEventReceipt{
		DedupeKey: freshKey, ClaimToken: "fresh", CreatedAt: now,
	}).Error)

	var result NotificationEnqueueResult
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = EnqueueNotificationEventTxWithResult(tx, eventType, eventKey, map[string]any{"id": "1"})
		return err
	}))
	require.Equal(t, NotificationEnqueueAcceptedWithoutSubscriber, result.Status)

	var currentCount, expiredCount, freshCount, lockCount int64
	require.NoError(t, DB.Model(&NotificationEventReceipt{}).Where("dedupe_key = ?", currentExpiredKey).Count(&currentCount).Error)
	require.NoError(t, DB.Model(&NotificationEventReceipt{}).Where("dedupe_key = ?", otherExpiredKey).Count(&expiredCount).Error)
	require.NoError(t, DB.Model(&NotificationEventReceipt{}).Where("dedupe_key = ?", freshKey).Count(&freshCount).Error)
	require.NoError(t, DB.Model(&NotificationEventReceipt{}).Where("dedupe_key = ?", notificationSequenceLockKey).Count(&lockCount).Error)
	require.EqualValues(t, 1, currentCount, "超过 TTL 后相同事件键应作为新事件重新写入收据")
	require.Zero(t, expiredCount)
	require.EqualValues(t, 1, freshCount)
	require.EqualValues(t, 1, lockCount)
}
func TestNotificationReceiptWindowExpiresTerminalHistoryButNotActiveWork(t *testing.T) {
	t.Run("terminal history permits republish after ninety days", func(t *testing.T) {
		setupNotificationTestDB(t)
		createNotificationFixture(t)
		eventKey := "invoice:receipt-window"
		enqueue := func(invoiceID int) NotificationEnqueueResult {
			var result NotificationEnqueueResult
			require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
				var err error
				result, err = EnqueueNotificationEventTxWithResult(tx, NotificationEventTypeInvoicePending, eventKey, map[string]any{"invoice_id": invoiceID})
				return err
			}))
			return result
		}

		require.Equal(t, NotificationEnqueueQueued, enqueue(1).Status)
		work, err := ClaimNotificationDeliveries(1, time.Now().Unix(), 120)
		require.NoError(t, err)
		require.Len(t, work, 1)
		_, err = MarkNotificationDeliverySending(work[0].Delivery.Id, time.Now().Unix())
		require.NoError(t, err)
		require.NoError(t, MarkNotificationDeliverySuccess(work[0].Delivery.Id, time.Now().Unix()))

		require.Equal(t, NotificationEnqueueDuplicate, enqueue(2).Status, "a terminal event remains idempotent inside the receipt window")
		dedupeKey := notificationEventDedupeKey(NotificationEventTypeInvoicePending, eventKey)
		require.NoError(t, DB.Model(&NotificationEventReceipt{}).Where("dedupe_key = ?", dedupeKey).
			Update("created_at", time.Now().Unix()-NotificationEventReceiptTTLSeconds-1).Error)

		republished := enqueue(3)
		require.Equal(t, NotificationEnqueueQueued, republished.Status)
		require.Equal(t, 1, republished.DeliveryCount)
		var events, deliveries int64
		require.NoError(t, DB.Model(&NotificationEvent{}).Where("event_type = ? AND event_key = ?", NotificationEventTypeInvoicePending, eventKey).Count(&events).Error)
		require.NoError(t, DB.Model(&NotificationDelivery{}).Count(&deliveries).Error)
		require.EqualValues(t, 1, events, "expired terminal history must be replaced rather than preserving a unique-key tombstone")
		require.EqualValues(t, 1, deliveries)
		var event NotificationEvent
		require.NoError(t, DB.Where("event_type = ? AND event_key = ?", NotificationEventTypeInvoicePending, eventKey).Take(&event).Error)
		require.Contains(t, event.Payload, `"invoice_id":3`)
	})

	t.Run("active work keeps ownership after receipt expiry", func(t *testing.T) {
		setupNotificationTestDB(t)
		createNotificationFixture(t)
		eventKey := "invoice:active-receipt-window"
		var first NotificationEnqueueResult
		require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
			var err error
			first, err = EnqueueNotificationEventTxWithResult(tx, NotificationEventTypeInvoicePending, eventKey, map[string]any{"invoice_id": 1})
			return err
		}))
		require.Equal(t, NotificationEnqueueQueued, first.Status)
		dedupeKey := notificationEventDedupeKey(NotificationEventTypeInvoicePending, eventKey)
		require.NoError(t, DB.Model(&NotificationEventReceipt{}).Where("dedupe_key = ?", dedupeKey).
			Update("created_at", time.Now().Unix()-NotificationEventReceiptTTLSeconds-1).Error)

		var duplicate NotificationEnqueueResult
		require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
			var err error
			duplicate, err = EnqueueNotificationEventTxWithResult(tx, NotificationEventTypeInvoicePending, eventKey, map[string]any{"invoice_id": 2})
			return err
		}))
		require.Equal(t, NotificationEnqueueDuplicate, duplicate.Status)
		var events, deliveries, receipts int64
		require.NoError(t, DB.Model(&NotificationEvent{}).Where("event_type = ? AND event_key = ?", NotificationEventTypeInvoicePending, eventKey).Count(&events).Error)
		require.NoError(t, DB.Model(&NotificationDelivery{}).Count(&deliveries).Error)
		require.NoError(t, DB.Model(&NotificationEventReceipt{}).Where("dedupe_key = ?", dedupeKey).Count(&receipts).Error)
		require.EqualValues(t, 1, events)
		require.EqualValues(t, 1, deliveries)
		require.EqualValues(t, 1, receipts, "active work must refresh its finite receipt ownership")
	})
}

func TestClaimedDeliveryIsSafelyReclaimedBeforeSending(t *testing.T) {
	setupNotificationTestDB(t)
	createNotificationFixture(t)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:reclaim", map[string]any{"invoice_id": 12})
	}))
	now := time.Now().Unix()
	work, err := ClaimNotificationDeliveries(10, now, 120)
	require.NoError(t, err)
	require.Len(t, work, 1)
	deliveryID := work[0].Delivery.Id
	require.Equal(t, NotificationDeliveryClaimed, work[0].Delivery.Status)
	require.Zero(t, work[0].Delivery.AttemptCount)
	require.NoError(t, DB.Model(&NotificationDelivery{}).Where("id = ?", deliveryID).Update("updated_at", now-180).Error)

	work, err = ClaimNotificationDeliveries(10, now, 120)
	require.NoError(t, err)
	require.Len(t, work, 1)
	require.Equal(t, deliveryID, work[0].Delivery.Id)
	require.Equal(t, NotificationDeliveryClaimed, work[0].Delivery.Status)
	require.Zero(t, work[0].Delivery.AttemptCount)
}

func TestClaimMissingRelationDoesNotLeaveSendingDelivery(t *testing.T) {
	setupNotificationTestDB(t)
	now := time.Now().Unix()
	delivery := NotificationDelivery{
		EventId: 999, TaskId: 999, TargetId: 999, Status: NotificationDeliveryPending,
		NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, DB.Create(&delivery).Error)

	work, err := ClaimNotificationDeliveries(10, now, 120)
	require.Error(t, err)
	require.Empty(t, work)
	require.NoError(t, DB.First(&delivery, delivery.Id).Error)
	require.Equal(t, NotificationDeliveryDead, delivery.Status)
	require.NotEqual(t, NotificationDeliverySending, delivery.Status)
	require.NotEqual(t, NotificationDeliveryClaimed, delivery.Status)
}

func TestNotificationDeliveryRequiresSendingTransition(t *testing.T) {
	setupNotificationTestDB(t)
	createNotificationFixture(t)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:transition", map[string]any{"invoice_id": 13})
	}))
	work, err := ClaimNotificationDeliveries(10, 0, 120)
	require.NoError(t, err)
	require.Len(t, work, 1)
	deliveryID := work[0].Delivery.Id
	require.ErrorIs(t, MarkNotificationDeliverySuccess(deliveryID, 0), ErrNotificationDeliveryState)
	attempt, err := MarkNotificationDeliverySending(deliveryID, 0)
	require.NoError(t, err)
	require.Equal(t, 1, attempt)
	_, err = MarkNotificationDeliverySending(deliveryID, 0)
	require.ErrorIs(t, err, ErrNotificationDeliveryState)
	require.NoError(t, MarkNotificationDeliverySuccess(deliveryID, 0))
	require.ErrorIs(t, MarkNotificationDeliverySuccess(deliveryID, 0), ErrNotificationDeliveryState)
}

func TestTruncateNotificationErrorPreservesUTF8(t *testing.T) {
	value := strings.Repeat("a", 2047) + "通知"
	truncated := truncateNotificationError(value)
	require.LessOrEqual(t, len(truncated), 2048)
	require.True(t, utf8.ValidString(truncated))
}

func TestPruneNotificationHistoryOnlyTouchesRequestedTask(t *testing.T) {
	setupNotificationTestDB(t)
	bot := &NotificationBot{Name: "prune bot", Token: "token", Enabled: true}
	require.NoError(t, CreateNotificationBot(bot))
	taskOne := &NotificationTask{Name: "one", EventType: "event", BotId: bot.Id, Enabled: true}
	taskTwo := &NotificationTask{Name: "two", EventType: "event", BotId: bot.Id, Enabled: true}
	require.NoError(t, CreateNotificationTask(taskOne))
	require.NoError(t, CreateNotificationTask(taskTwo))
	now := time.Now().Unix()
	for index := 0; index < 7; index++ {
		event := NotificationEvent{EventType: "event", EventKey: fmt.Sprintf("event:%d", index), Payload: `{}`, CreatedAt: now + int64(index)}
		require.NoError(t, DB.Create(&event).Error)
		require.NoError(t, DB.Create(&NotificationDelivery{
			EventId: event.Id, TaskId: taskOne.Id, TargetId: index + 1, Status: NotificationDeliverySuccess,
			CreatedAt: now, UpdatedAt: now,
		}).Error)
		require.NoError(t, DB.Create(&NotificationDelivery{
			EventId: event.Id, TaskId: taskTwo.Id, TargetId: index + 100, Status: NotificationDeliverySuccess,
			CreatedAt: now, UpdatedAt: now,
		}).Error)
	}

	require.NoError(t, PruneNotificationHistoryForTask(taskOne.Id))
	var taskOneCount, taskTwoCount int64
	require.NoError(t, DB.Model(&NotificationDelivery{}).Where("task_id = ?", taskOne.Id).Count(&taskOneCount).Error)
	require.NoError(t, DB.Model(&NotificationDelivery{}).Where("task_id = ?", taskTwo.Id).Count(&taskTwoCount).Error)
	require.EqualValues(t, 5, taskOneCount)
	require.EqualValues(t, 7, taskTwoCount)
}

func TestNotificationSequenceLockUsesSharedDialectHelper(t *testing.T) {
	dummyDB, err := gorm.Open(tests.DummyDialector{}, &gorm.Config{DryRun: true})
	require.NoError(t, err)
	buildSQL := func() string {
		var receipt NotificationEventReceipt
		return notificationSequenceLockQuery(dummyDB).Take(&receipt).Statement.SQL.String()
	}
	originalType := common.MainDatabaseType()
	t.Cleanup(func() { common.SetMainDatabaseType(originalType) })

	common.SetMainDatabaseType(common.DatabaseTypeMySQL)
	require.Contains(t, buildSQL(), "FOR UPDATE")
	common.SetMainDatabaseType(common.DatabaseTypePostgreSQL)
	require.Contains(t, buildSQL(), "FOR UPDATE")
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	require.NotContains(t, buildSQL(), "FOR UPDATE")
}

func TestNotificationSequenceLockSerializesSQLiteTransactions(t *testing.T) {
	setupNotificationTestDB(t)
	firstLocked := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- DB.Transaction(func(tx *gorm.DB) error {
			if err := LockNotificationSequenceTx(tx); err != nil {
				return err
			}
			close(firstLocked)
			<-releaseFirst
			return nil
		})
	}()
	<-firstLocked

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- DB.Transaction(func(tx *gorm.DB) error {
			return LockNotificationSequenceTx(tx)
		})
	}()
	select {
	case err := <-secondDone:
		t.Fatalf("第二个事务不应越过序列锁: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseFirst)
	require.NoError(t, <-firstDone)
	require.NoError(t, <-secondDone)

	var lockRows int64
	require.NoError(t, DB.Model(&NotificationEventReceipt{}).Where("dedupe_key = ?", notificationSequenceLockKey).Count(&lockRows).Error)
	require.EqualValues(t, 1, lockRows)
}

func TestNotificationEventDedupeSpansTasksAndConcurrentPublishers(t *testing.T) {
	setupNotificationTestDB(t)
	bot := &NotificationBot{Name: "shared bot", Token: "secret", Enabled: true}
	require.NoError(t, CreateNotificationBot(bot))
	for index := range 2 {
		task := &NotificationTask{Name: fmt.Sprintf("task-%d", index), EventType: "extension.orders.created", BotId: bot.Id, Enabled: true}
		require.NoError(t, CreateNotificationTask(task))
		require.NoError(t, CreateNotificationTarget(&NotificationTarget{TaskId: task.Id, ChatId: fmt.Sprintf("chat-%d", index), Enabled: true}))
	}

	const publishers = 6
	start := make(chan struct{})
	errs := make(chan error, publishers)
	var wait sync.WaitGroup
	for range publishers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			errs <- DB.Transaction(func(tx *gorm.DB) error {
				_, err := EnqueueNotificationEventTxWithResult(tx, "extension.orders.created", "order:42", map[string]any{"id": "42"})
				return err
			})
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	var events, deliveries int64
	require.NoError(t, DB.Model(&NotificationEvent{}).Count(&events).Error)
	require.NoError(t, DB.Model(&NotificationDelivery{}).Count(&deliveries).Error)
	require.EqualValues(t, 1, events)
	require.EqualValues(t, 2, deliveries, "one unique delivery must exist for each subscribed task target")
}

func TestNotificationDeliveryConcurrentClaimIsSingleOwner(t *testing.T) {
	setupNotificationTestDB(t)
	createNotificationFixture(t)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, NotificationEventTypeInvoicePending, "invoice:concurrent-claim", map[string]any{"invoice_id": 42})
	}))

	const claimers = 8
	start := make(chan struct{})
	claimed := make(chan []NotificationDeliveryWork, claimers)
	errs := make(chan error, claimers)
	var wait sync.WaitGroup
	now := time.Now().Unix()
	for range claimers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			work, err := ClaimNotificationDeliveries(1, now, 120)
			claimed <- work
			errs <- err
		}()
	}
	close(start)
	wait.Wait()
	close(claimed)
	close(errs)
	owners := 0
	for work := range claimed {
		owners += len(work)
	}
	for err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, 1, owners)
}

func TestListNotificationHistoryReturnsNewestFive(t *testing.T) {
	setupNotificationTestDB(t)
	_, task, target := createNotificationFixture(t)
	now := time.Now().Unix()
	for index := 1; index <= 7; index++ {
		event := NotificationEvent{EventType: task.EventType, EventKey: fmt.Sprintf("history:%d", index), Payload: `{}`, CreatedAt: now + int64(index)}
		require.NoError(t, DB.Create(&event).Error)
		require.NoError(t, DB.Create(&NotificationDelivery{EventId: event.Id, TaskId: task.Id, TargetId: target.Id, Status: NotificationDeliverySuccess, CreatedAt: now + int64(index), UpdatedAt: now + int64(index)}).Error)
	}
	items, err := ListNotificationHistory(task.Id, 99)
	require.NoError(t, err)
	require.Len(t, items, 5)
	for index := 1; index < len(items); index++ {
		require.Greater(t, items[index-1].Id, items[index].Id)
	}
}

func TestNotificationTaskFilterConfigMatchesStatusAndKeywords(t *testing.T) {
	config := NotificationTaskFilterConfig{
		StatusCodes:   "403,500-599",
		ErrorKeywords: []string{"insufficient account balance", "quota"},
	}
	payload := map[string]any{
		"status_code":   403,
		"error_message": "Provider: Insufficient Account Balance",
	}
	assert.True(t, config.Matches(payload))
	assert.False(t, config.Matches(map[string]any{
		"status_code":   408,
		"error_message": "Provider: Insufficient Account Balance",
	}))
	assert.False(t, config.Matches(map[string]any{
		"status_code":   403,
		"error_message": "temporary upstream failure",
	}))
	assert.True(t, (NotificationTaskFilterConfig{}).Matches(payload))
}

func TestNotificationTaskFilterConfigFiltersDeliveriesAtEnqueue(t *testing.T) {
	setupNotificationTestDB(t)
	bot := &NotificationBot{Name: "filter bot", Token: "secret", Enabled: true}
	require.NoError(t, CreateNotificationBot(bot))
	tasks := []*NotificationTask{
		{Name: "status match", EventType: NotificationEventTypeChannelDisabled, BotId: bot.Id, Enabled: true, FilterConfig: `{"status_codes":"403"}`},
		{Name: "keyword match", EventType: NotificationEventTypeChannelDisabled, BotId: bot.Id, Enabled: true, FilterConfig: `{"error_keywords":["balance"]}`},
		{Name: "no match", EventType: NotificationEventTypeChannelDisabled, BotId: bot.Id, Enabled: true, FilterConfig: `{"status_codes":"500-599"}`},
	}
	for index, task := range tasks {
		require.NoError(t, CreateNotificationTask(task))
		require.NoError(t, CreateNotificationTarget(&NotificationTarget{TaskId: task.Id, ChatId: fmt.Sprintf("chat-%d", index), Enabled: true}))
	}
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return EnqueueNotificationEventTx(tx, NotificationEventTypeChannelDisabled, "channel:filter:1", map[string]any{
			"channel_id":    1,
			"status_code":   403,
			"error_message": "upstream account balance is insufficient",
		})
	}))
	var deliveries []NotificationDelivery
	require.NoError(t, DB.Order("id asc").Find(&deliveries).Error)
	require.Len(t, deliveries, 2)
	assert.Equal(t, tasks[0].Id, deliveries[0].TaskId)
	assert.Equal(t, tasks[1].Id, deliveries[1].TaskId)
}
