package controller

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/extension"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestValidateExtensionNotificationPayload(t *testing.T) {
	registered := extension.RegisteredNotificationEvent{
		ModuleID:  "orders",
		EventType: "extension.orders.created",
		Event: extension.NotificationEventContribution{
			ID:    "created",
			Label: "Order created",
			Variables: []extension.NotificationVariable{
				{Name: "order_id", Type: "string", Required: true},
				{Name: "amount", Type: "number"},
				{Name: "urgent", Type: "boolean"},
			},
		},
	}

	payload, err := validateExtensionNotificationPayload(registered, map[string]any{
		"order_id": "order-1",
		"amount":   float64(12.5),
	})
	require.NoError(t, err)
	require.Equal(t, "order-1", payload["order_id"])
	require.Equal(t, false, payload["urgent"])

	_, err = validateExtensionNotificationPayload(registered, map[string]any{"amount": float64(1)})
	require.ErrorContains(t, err, "order_id")
	_, err = validateExtensionNotificationPayload(registered, map[string]any{
		"order_id": "order-1",
		"unknown":  "value",
	})
	require.ErrorContains(t, err, "未在 manifest 中声明")
	_, err = validateExtensionNotificationPayload(registered, map[string]any{
		"order_id": "order-1",
		"amount":   "12.5",
	})
	require.ErrorContains(t, err, "必须是数字")
	_, err = validateExtensionNotificationPayload(registered, map[string]any{
		"order_id": "bad\u0000value",
	})
	require.ErrorContains(t, err, "控制字符")
}

func TestValidateExtensionNotificationEventKey(t *testing.T) {
	require.NoError(t, validateExtensionNotificationEventKey("order:123"))
	require.Error(t, validateExtensionNotificationEventKey(""))
	require.Error(t, validateExtensionNotificationEventKey(strings.Repeat("x", maxExtensionNotificationEventKeyRunes+1)))
	require.Error(t, validateExtensionNotificationEventKey("order\u0000id"))
}

func TestPublishExtensionNotificationEventDeniesNonRootBeforeEnqueue(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/extensions/orders/notification-events", strings.NewReader(`{}`))
	ctx.Set("role", common.RoleAdminUser)

	called := false
	publishExtensionNotificationEvent(ctx, func(*gorm.DB, string, string, map[string]any) (model.NotificationEnqueueResult, error) {
		called = true
		return model.NotificationEnqueueResult{}, nil
	})

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.False(t, called)
	require.Contains(t, recorder.Body.String(), "Root")
}

func TestPublishExtensionNotificationEventValidatesBodyLimitBeforeResolution(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/extensions/orders/notification-events",
		strings.NewReader(`{"event_type":"created","event_key":"key","payload":{"value":"`+strings.Repeat("x", maxExtensionNotificationEventBodyBytes)+`"}}`),
	)
	ctx.Set("role", common.RoleRootUser)

	publishExtensionNotificationEvent(ctx, func(*gorm.DB, string, string, map[string]any) (model.NotificationEnqueueResult, error) {
		t.Fatal("oversized request reached enqueue")
		return model.NotificationEnqueueResult{}, nil
	})

	require.Contains(t, recorder.Body.String(), "16 KiB")
}

func TestPublishExtensionNotificationEventEnqueuesValidatedManifestEvent(t *testing.T) {
	manager := installControllerNotificationTestModule(t)
	originalManager := extension.DefaultManager
	extension.DefaultManager = manager
	t.Cleanup(func() { extension.DefaultManager = originalManager })

	originalDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:extension-notification-publish?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		model.DB = originalDB
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	body, err := common.Marshal(map[string]any{
		"event_type": "created",
		"event_key":  "order:1",
		"payload":    map[string]any{"order_id": "1"},
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/extensions/orders/notification-events", bytes.NewReader(body))
	ctx.Params = gin.Params{{Key: "id", Value: "orders"}}
	ctx.Set("role", common.RoleRootUser)

	publishExtensionNotificationEvent(ctx, func(_ *gorm.DB, eventType, eventKey string, payload map[string]any) (model.NotificationEnqueueResult, error) {
		require.Equal(t, "extension.orders.created", eventType)
		require.Equal(t, "order:1", eventKey)
		require.Equal(t, "orders", payload["module_id"])
		require.Equal(t, "extension.orders.created", payload["event_type"])
		require.Equal(t, "1", payload["order_id"])
		return model.NotificationEnqueueResult{Status: "queued", DeliveryCount: 2}, nil
	})

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Status        string `json:"status"`
			EventType     string `json:"event_type"`
			EventKey      string `json:"event_key"`
			DeliveryCount int    `json:"delivery_count"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Equal(t, "queued", response.Data.Status)
	require.Equal(t, "extension.orders.created", response.Data.EventType)
	require.Equal(t, "order:1", response.Data.EventKey)
	require.Equal(t, 2, response.Data.DeliveryCount)
}

func TestPublishExtensionNotificationEventRedactsEnqueueErrors(t *testing.T) {
	manager := installControllerNotificationTestModule(t)
	originalManager := extension.DefaultManager
	extension.DefaultManager = manager
	t.Cleanup(func() { extension.DefaultManager = originalManager })

	originalDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:extension-notification-redaction?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		model.DB = originalDB
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	body := `{"event_type":"created","event_key":"order:1","payload":{"order_id":"1"}}`
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/extensions/orders/notification-events", strings.NewReader(body))
	ctx.Params = gin.Params{{Key: "id", Value: "orders"}}
	ctx.Set("role", common.RoleRootUser)

	publishExtensionNotificationEvent(ctx, func(*gorm.DB, string, string, map[string]any) (model.NotificationEnqueueResult, error) {
		return model.NotificationEnqueueResult{}, errors.New(`C:\\secret\\notification.db: permission denied`)
	})

	require.NotContains(t, recorder.Body.String(), "secret")
	require.Contains(t, recorder.Body.String(), "请稍后重试")
}

func installControllerNotificationTestModule(t *testing.T) *extension.Manager {
	t.Helper()
	manager := extension.NewManager(t.TempDir())
	moduleDir := filepath.Join(manager.RootDir(), "orders")
	require.NoError(t, os.MkdirAll(filepath.Join(moduleDir, "public"), 0o755))
	manifest := extension.Manifest{
		ID:      "orders",
		Name:    "Orders",
		Version: "1.0.0",
		Runtime: extension.Runtime{Type: extension.RuntimeTypeStatic, StaticDir: "public"},
		Permissions: extension.PermissionConfig{
			Roles:        []string{"root"},
			Capabilities: []string{extension.CapabilityNotificationEventsPublish},
		},
		Notifications: extension.NotificationContribution{Events: []extension.NotificationEventContribution{{
			ID:              "created",
			Label:           "Order created",
			DefaultTemplate: "{{mention}} {{order_id}}",
			Variables: []extension.NotificationVariable{{
				Name:     "order_id",
				Type:     "string",
				Required: true,
			}},
		}}},
	}
	manifestData, err := common.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "manifest.json"), manifestData, 0o644))
	require.NoError(t, manager.Scan())
	_, err = manager.SetEnabled("orders", true)
	require.NoError(t, err)
	return manager
}
