package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type notificationRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn notificationRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func setupNotificationServiceTestDB(t *testing.T) {
	t.Helper()
	originalDB := model.DB
	originalDBType := common.MainDatabaseType()
	dsn := fmt.Sprintf("file:notification-service-%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	require.NoError(t, db.AutoMigrate(
		&model.NotificationBot{},
		&model.NotificationTask{},
		&model.NotificationTarget{},
		&model.NotificationEventReceipt{},
		&model.NotificationEvent{},
		&model.NotificationDelivery{},
	))
	t.Cleanup(func() {
		_ = sqlDB.Close()
		model.DB = originalDB
		common.SetMainDatabaseType(originalDBType)
	})
}

func createNotificationServiceWork(t *testing.T, eventKey string) model.NotificationDeliveryWork {
	t.Helper()
	bot := &model.NotificationBot{Name: "service bot", Token: "test-token", Enabled: true}
	require.NoError(t, model.CreateNotificationBot(bot))
	task := &model.NotificationTask{
		Name: "service task", EventType: model.NotificationEventTypeInvoicePending,
		BotId: bot.Id, Template: "{{invoice_id}}", Enabled: true,
	}
	require.NoError(t, model.CreateNotificationTask(task))
	target := &model.NotificationTarget{TaskId: task.Id, ChatId: "-10001", Enabled: true}
	require.NoError(t, model.CreateNotificationTarget(target))
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		return model.EnqueueNotificationEventTx(tx, model.NotificationEventTypeInvoicePending, eventKey, map[string]any{"invoice_id": "1"})
	}))
	work, err := model.ClaimNotificationDeliveries(1, time.Now().Unix(), 120)
	require.NoError(t, err)
	require.Len(t, work, 1)
	return work[0]
}

func useTelegramTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	originalBase := notificationTelegramAPIBase
	originalClient := httpClient
	fetchSetting := system_setting.GetFetchSetting()
	originalFetchSetting := *fetchSetting
	notificationTelegramAPIBase = server.URL
	httpClient = server.Client()
	fetchSetting.EnableSSRFProtection = false
	t.Cleanup(func() {
		server.Close()
		notificationTelegramAPIBase = originalBase
		httpClient = originalClient
		*fetchSetting = originalFetchSetting
	})
	return server
}

func TestRenderNotificationTemplateEscapesPayloadAndBuildsMention(t *testing.T) {
	target := &model.NotificationTarget{MentionUserId: "42", MentionName: `<管理员>`}
	content, err := RenderNotificationTemplate(`{{mention}} 标题：{{title}} 金额：{{total_amount}}`, map[string]any{
		"title":        `<script>alert(1)</script>`,
		"total_amount": 88.5,
	}, target)
	require.NoError(t, err)
	require.Contains(t, content, `href="tg://user?id=42"`)
	require.Contains(t, content, "&lt;管理员&gt;")
	require.Contains(t, content, "&lt;script&gt;alert(1)&lt;/script&gt;")
	require.NotContains(t, content, "<script>")
}

func TestRenderNotificationTemplateKeepsInvoiceAmountAvailable(t *testing.T) {
	content, err := RenderNotificationTemplate(model.NotificationTaskDefaultTemplate, map[string]any{
		"invoice_id":   "invoice-1",
		"total_amount": "123.45",
	}, nil)
	require.NoError(t, err)
	require.Contains(t, content, "123.45")
}

func TestRenderNotificationTemplateRejectsUnknownVariable(t *testing.T) {
	_, err := RenderNotificationTemplate(`{{missing}}`, nil, nil)
	require.ErrorContains(t, err, "unknown notification template variable")
}

func TestRenderNotificationTemplateRejectsNegativeMentionUserID(t *testing.T) {
	_, err := RenderNotificationTemplate(`{{mention}}`, nil, &model.NotificationTarget{MentionUserId: "-42"})
	require.ErrorContains(t, err, "positive number")
}
func TestSendTelegramNotificationUsesExactHTMLRequestSchema(t *testing.T) {
	var body string
	useTelegramTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		body = string(data)
		require.Equal(t, "/bottest-token/sendMessage", r.URL.Path)
		var request map[string]any
		require.NoError(t, common.Unmarshal(data, &request))
		require.Equal(t, map[string]any{
			"chat_id": "-10001", "text": "<b>hello</b>", "parse_mode": "HTML", "disable_web_page_preview": true,
		}, request)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))

	require.NoError(t, SendTelegramNotification("test-token", "-10001", "<b>hello</b>"))
	require.Contains(t, body, `"parse_mode":"HTML"`)
	require.Contains(t, body, `"chat_id":"-10001"`)
}

func TestSendTelegramNotificationHandlesRetryAfter(t *testing.T) {
	useTelegramTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":7}}`))
	}))

	err := SendTelegramNotification("test-token", "-10001", "hello")
	require.Error(t, err)
	deliveryErr, ok := err.(*telegramDeliveryError)
	require.True(t, ok)
	require.True(t, deliveryErr.retryable)
	require.Equal(t, 7, deliveryErr.retryAfter)
	require.NotContains(t, err.Error(), "test-token")
}

func TestSendTelegramNotificationRejectsOversizedMessage(t *testing.T) {
	err := SendTelegramNotification("test-token", "-10001", strings.Repeat("a", 4097))
	require.ErrorContains(t, err, "exceeds 4096")
}

func TestTelegramNetworkErrorDoesNotLeakToken(t *testing.T) {
	server := useTelegramTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()
	err := SendTelegramNotification("highly-secret-token", "-10001", "hello")
	require.Error(t, err)
	require.False(t, strings.Contains(err.Error(), "highly-secret-token"))
	deliveryErr, ok := err.(*telegramDeliveryError)
	require.True(t, ok)
	require.False(t, deliveryErr.retryable)
}

func TestTelegramResponseErrorAndSizeAreRedacted(t *testing.T) {
	useTelegramTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"echoed highly-secret-token and database detail"}`))
	}))
	err := SendTelegramNotification("highly-secret-token", "-10001", "hello")
	require.EqualError(t, err, "telegram rejected message")
	require.NotContains(t, err.Error(), "highly-secret-token")
	require.NotContains(t, err.Error(), "database detail")

	useTelegramTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", notificationTelegramResponseMax+1)))
	}))
	err = SendTelegramNotification("highly-secret-token", "-10001", "hello")
	require.EqualError(t, err, "telegram response exceeds size limit")
}

func TestTelegramSSRFAndRedirectRejectionsAreStable(t *testing.T) {
	fetchSetting := system_setting.GetFetchSetting()
	originalSetting := *fetchSetting
	originalBase := notificationTelegramAPIBase
	originalProtectedClient := ssrfProtectedHTTPClient
	t.Cleanup(func() {
		*fetchSetting = originalSetting
		notificationTelegramAPIBase = originalBase
		ssrfProtectedHTTPClient = originalProtectedClient
	})
	fetchSetting.EnableSSRFProtection = true
	fetchSetting.AllowPrivateIp = false
	fetchSetting.DomainFilterMode = false
	fetchSetting.DomainList = nil
	fetchSetting.IpFilterMode = false
	fetchSetting.IpList = nil
	fetchSetting.AllowedPorts = []string{"80", "443"}
	fetchSetting.ApplyIPFilterForDomain = false

	notificationTelegramAPIBase = "http://127.0.0.1"
	err := SendTelegramNotification("redirect-secret", "-10001", "hello")
	require.EqualError(t, err, "telegram endpoint rejected by SSRF policy")
	require.NotContains(t, err.Error(), "redirect-secret")

	notificationTelegramAPIBase = "https://api.telegram.org"
	ssrfProtectedHTTPClient = &http.Client{
		Transport: notificationRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": []string{"http://127.0.0.1/private"}},
				Body:       io.NopCloser(strings.NewReader("redirect")),
				Request:    req,
			}, nil
		}),
		CheckRedirect: checkProtectedFetchRedirect,
	}
	err = SendTelegramNotification("redirect-secret", "-10001", "hello")
	require.EqualError(t, err, "telegram request failed")
	require.NotContains(t, err.Error(), "redirect-secret")
}

func TestDispatchNotificationDeliveryTransitionsClaimedToSuccess(t *testing.T) {
	setupNotificationServiceTestDB(t)
	useTelegramTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	work := createNotificationServiceWork(t, "invoice:dispatch-success")
	require.Equal(t, model.NotificationDeliveryClaimed, work.Delivery.Status)

	dispatchNotificationDelivery(context.Background(), work)

	var delivery model.NotificationDelivery
	require.NoError(t, model.DB.First(&delivery, work.Delivery.Id).Error)
	require.Equal(t, model.NotificationDeliverySuccess, delivery.Status)
	require.Equal(t, 1, delivery.AttemptCount)
}

func TestDispatchChannelNotificationMigratesLegacyTemplateAndSendsPayload(t *testing.T) {
	setupNotificationServiceTestDB(t)
	var request telegramMessageRequest
	useTelegramTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, common.Unmarshal(data, &request))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))

	bot := &model.NotificationBot{Name: "channel bot", Token: "test-token", Enabled: true}
	require.NoError(t, model.CreateNotificationBot(bot))
	// 直接写入历史任务，模拟旧版本已保存的发票默认模板。
	task := &model.NotificationTask{
		Name: "legacy channel task", EventType: model.NotificationEventTypeChannelDisabled,
		BotId: bot.Id, Template: model.NotificationTaskDefaultTemplate, Enabled: true,
	}
	require.NoError(t, model.DB.Create(task).Error)
	target := &model.NotificationTarget{TaskId: task.Id, ChatId: "-10001", Enabled: true}
	require.NoError(t, model.CreateNotificationTarget(target))
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		return model.EnqueueNotificationEventTx(tx, model.NotificationEventTypeChannelDisabled, "channel:42:disabled", map[string]any{
			"channel_id": 42, "channel_name": "Example channel", "status_code": 403,
			"error_code": "forbidden", "error_message": "upstream rejected", "reason": "status_code=403",
		})
	}))
	work, err := model.ClaimNotificationDeliveries(1, time.Now().Unix(), 120)
	require.NoError(t, err)
	require.Len(t, work, 1)

	dispatchNotificationDelivery(context.Background(), work[0])
	require.Equal(t, model.NotificationChannelDisabledTemplate,
		model.NormalizeNotificationTaskTemplate(work[0].Event.EventType, work[0].Task.Template))
	require.Contains(t, request.Text, "Example channel")
	require.Contains(t, request.Text, "status_code=403")
	require.NotContains(t, request.Text, "{{")
	require.NotContains(t, request.Text, "total_amount")
	require.Equal(t, "-10001", request.ChatID)
	require.Equal(t, "HTML", request.ParseMode)
	require.True(t, request.DisableWebPagePreview)

	var delivery model.NotificationDelivery
	require.NoError(t, model.DB.First(&delivery, work[0].Delivery.Id).Error)
	require.Equal(t, model.NotificationDeliverySuccess, delivery.Status)
}

func TestDispatchNotificationDeliveryPersistsRetryableFailure(t *testing.T) {
	setupNotificationServiceTestDB(t)
	useTelegramTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":7}}`))
	}))
	work := createNotificationServiceWork(t, "invoice:dispatch-retry")

	dispatchNotificationDelivery(context.Background(), work)

	var delivery model.NotificationDelivery
	require.NoError(t, model.DB.First(&delivery, work.Delivery.Id).Error)
	require.Equal(t, model.NotificationDeliveryRetrying, delivery.Status)
	require.Equal(t, 1, delivery.AttemptCount)
	require.Greater(t, delivery.NextAttemptAt, time.Now().Unix())
}
func TestDispatchNotificationDeliveryDoesNotRetryAmbiguousNetworkOutcome(t *testing.T) {
	setupNotificationServiceTestDB(t)
	server := useTelegramTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	server.Close()
	work := createNotificationServiceWork(t, "invoice:dispatch-ambiguous-network")

	// A transport failure after the request starts does not prove Telegram
	// missed the message. Marking it terminal avoids an automatic duplicate;
	// definite HTTP 429 responses remain retryable in the regression above.
	dispatchNotificationDelivery(context.Background(), work)

	var delivery model.NotificationDelivery
	require.NoError(t, model.DB.First(&delivery, work.Delivery.Id).Error)
	require.Equal(t, model.NotificationDeliveryDead, delivery.Status)
	require.Equal(t, 1, delivery.AttemptCount)
	require.Equal(t, work.Delivery.NextAttemptAt, delivery.NextAttemptAt, "ambiguous outcomes must not schedule another attempt")
}

func TestNotificationDispatcherSystemTaskRunsOnlyWhenWorkExists(t *testing.T) {
	setupNotificationServiceTestDB(t)
	handler := NotificationDispatcherSystemTaskHandler{}
	require.False(t, handler.Enabled())
	bot := &model.NotificationBot{Name: "bot", Token: "secret", Enabled: true}
	require.NoError(t, model.CreateNotificationBot(bot))
	task := &model.NotificationTask{Name: "task", EventType: "event", BotId: bot.Id, Enabled: true}
	require.NoError(t, model.CreateNotificationTask(task))
	target := &model.NotificationTarget{TaskId: task.Id, ChatId: "chat", Enabled: true}
	require.NoError(t, model.CreateNotificationTarget(target))
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		return model.EnqueueNotificationEventTx(tx, "event", "work:1", map[string]any{"id": 1})
	}))
	require.True(t, handler.Enabled())
	require.Equal(t, model.SystemTaskTypeNotificationDispatch, handler.Type())
	require.Equal(t, notificationDispatchInterval, handler.Interval())
}
