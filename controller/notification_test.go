package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupNotificationControllerTestDB(t *testing.T) {
	t.Helper()
	originalDB := model.DB
	originalDBType := common.MainDatabaseType()
	dsn := fmt.Sprintf("file:notification-controller-%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	require.NoError(t, db.AutoMigrate(
		&model.NotificationBot{}, &model.NotificationTask{}, &model.NotificationTarget{},
		&model.NotificationEventReceipt{}, &model.NotificationEvent{}, &model.NotificationDelivery{},
	))
	t.Cleanup(func() {
		_ = sqlDB.Close()
		model.DB = originalDB
		common.SetMainDatabaseType(originalDBType)
	})
}

func invokeNotificationHandler(handler gin.HandlerFunc, role int, method, path, body string, id int) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("role", role)
	context.Request = httptest.NewRequest(method, path, bytes.NewBufferString(body))
	context.Request.Header.Set("Content-Type", "application/json")
	if id > 0 {
		context.Params = gin.Params{{Key: "id", Value: fmt.Sprint(id)}}
	}
	handler(context)
	return recorder
}

func TestNotificationControllersFailClosedWithoutRootRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handlers := []gin.HandlerFunc{
		ListNotificationEventTypes, ListNotificationBots, CreateNotificationBot, UpdateNotificationBot,
		DisableNotificationBot, TestNotificationBot, ListNotificationTasks, CreateNotificationTask,
		UpdateNotificationTask, DisableNotificationTask, ListNotificationDeliveries,
	}
	for index, handler := range handlers {
		recorder := invokeNotificationHandler(handler, common.RoleAdminUser, http.MethodPost, "/api/notification/blocked", `{}`, 0)
		require.Contains(t, recorder.Body.String(), `"success":false`, "handler %d", index)
		require.Contains(t, recorder.Body.String(), `"message":"no permission"`, "handler %d", index)
	}
}

func TestNotificationBotControllerSchemaAndEncryptedStorage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupNotificationControllerTestDB(t)
	originalSecret := common.CryptoSecret
	common.CryptoSecret = "controller-notification-secret"
	t.Cleanup(func() { common.CryptoSecret = originalSecret })

	create := invokeNotificationHandler(CreateNotificationBot, common.RoleRootUser, http.MethodPost, "/api/notification/bots", `{"name":"ops bot","token":"telegram-secret","enabled":true}`, 0)
	require.Contains(t, create.Body.String(), `"success":true`)
	require.Contains(t, create.Body.String(), `"token_configured":true`)
	require.NotContains(t, create.Body.String(), "telegram-secret")
	require.NotContains(t, create.Body.String(), `"token"`)

	var stored model.NotificationBot
	require.NoError(t, model.DB.First(&stored).Error)
	require.True(t, strings.HasPrefix(stored.Token, "enc:v1:"))
	require.NotContains(t, stored.Token, "telegram-secret")

	list := invokeNotificationHandler(ListNotificationBots, common.RoleRootUser, http.MethodGet, "/api/notification/bots", "", 0)
	require.Contains(t, list.Body.String(), `"data":[{`)
	require.Contains(t, list.Body.String(), `"token_configured":true`)
	require.NotContains(t, list.Body.String(), "telegram-secret")
	require.NotContains(t, list.Body.String(), `"token"`)

	update := invokeNotificationHandler(UpdateNotificationBot, common.RoleRootUser, http.MethodPut, "/api/notification/bots/1", `{"name":"renamed bot","token":"","enabled":true}`, stored.Id)
	require.Contains(t, update.Body.String(), `"success":true`)
	token, err := model.NotificationBotToken(stored.Id)
	require.NoError(t, err)
	require.Equal(t, "telegram-secret", token, "blank update token must retain the configured secret")
}

func TestNotificationTaskControllerRequestAndResponseSchema(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupNotificationControllerTestDB(t)
	bot := &model.NotificationBot{Name: "bot", Token: "secret", Enabled: true}
	require.NoError(t, model.CreateNotificationBot(bot))
	body := fmt.Sprintf(`{"name":"invoice task","event_type":"%s","bot_id":%d,"template":"{{mention}} {{invoice_id}}","enabled":true,"targets":[{"chat_id":"-10001","mention_user_id":"42","mention_name":"Ops","enabled":true}]}`, model.NotificationEventTypeInvoicePending, bot.Id)
	create := invokeNotificationHandler(CreateNotificationTask, common.RoleRootUser, http.MethodPost, "/api/notification/tasks", body, 0)
	require.Contains(t, create.Body.String(), `"success":true`)
	require.Contains(t, create.Body.String(), `"data":1`)

	list := invokeNotificationHandler(ListNotificationTasks, common.RoleRootUser, http.MethodGet, "/api/notification/tasks", "", 0)
	response := list.Body.String()
	require.Contains(t, response, `"success":true`)
	require.Contains(t, response, `"name":"invoice task"`)
	require.Contains(t, response, `"event_type":"invoice_pending"`)
	require.Contains(t, response, `"event_name":"新待开票订单"`)
	require.Contains(t, response, `"bot_name":"bot"`)
	require.Contains(t, response, `"targets":[{"id":1,"task_id":1,"chat_id":"-10001","mention_user_id":"42","mention_name":"Ops","enabled":true`)
}

func TestNotificationEventDefinitionsIncludeChannelStatusEvents(t *testing.T) {
	disabled, ok := findNotificationEventDefinition(model.NotificationEventTypeChannelDisabled)
	require.True(t, ok)
	require.Equal(t, "Channel disabled", disabled.Label)
	require.Contains(t, disabled.Variables, "channel_name")
	require.Contains(t, disabled.Variables, "status_code")
	require.Contains(t, disabled.Variables, "error_code")
	require.Contains(t, disabled.Variables, "error_message")
	require.Contains(t, disabled.Variables, "reason")

	enabled, ok := findNotificationEventDefinition(model.NotificationEventTypeChannelEnabled)
	require.True(t, ok)
	require.Equal(t, "Channel enabled", enabled.Label)
	require.Contains(t, enabled.Variables, "channel_name")
	require.NotContains(t, enabled.Variables, "reason")
	require.Contains(t, enabled.DefaultTemplate, "{{channel_id}}")
}

func TestNotificationTaskControllerAcceptsChannelFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupNotificationControllerTestDB(t)
	bot := &model.NotificationBot{Name: "channel bot", Token: "secret", Enabled: true}
	require.NoError(t, model.CreateNotificationBot(bot))
	body := fmt.Sprintf(`{"name":"channel task","event_type":"%s","bot_id":%d,"template":"{{channel_name}} {{status_code}} {{error_message}}","filter_config":{"status_codes":"403,500-599","error_keywords":["balance","quota"]},"enabled":true,"targets":[{"chat_id":"-10001","enabled":true}]}`, model.NotificationEventTypeChannelDisabled, bot.Id)
	create := invokeNotificationHandler(CreateNotificationTask, common.RoleRootUser, http.MethodPost, "/api/notification/tasks", body, 0)
	require.Contains(t, create.Body.String(), `"success":true`)

	list := invokeNotificationHandler(ListNotificationTasks, common.RoleRootUser, http.MethodGet, "/api/notification/tasks", "", 0)
	var response struct {
		Data []notificationTaskResponse `json:"data"`
	}
	require.NoError(t, common.Unmarshal(list.Body.Bytes(), &response))
	require.Len(t, response.Data, 1)
	require.NotNil(t, response.Data[0].FilterConfig)
	require.Equal(t, "403,500-599", response.Data[0].FilterConfig.StatusCodes)
	require.Equal(t, []string{"balance", "quota"}, response.Data[0].FilterConfig.ErrorKeywords)
}
func TestNotificationMutationBodiesAreCappedBeforeHandlerWork(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupNotificationControllerTestDB(t)
	oversizedBody := `{"padding":"` + strings.Repeat("x", int(maxNotificationRequestBodyBytes)) + `"}`
	cases := []struct {
		name    string
		handler gin.HandlerFunc
		method  string
		path    string
		id      int
	}{
		{name: "create bot", handler: CreateNotificationBot, method: http.MethodPost, path: "/api/notification/bots"},
		{name: "update bot", handler: UpdateNotificationBot, method: http.MethodPut, path: "/api/notification/bots/1", id: 1},
		{name: "test bot", handler: TestNotificationBot, method: http.MethodPost, path: "/api/notification/bots/1/test", id: 1},
		{name: "create task", handler: CreateNotificationTask, method: http.MethodPost, path: "/api/notification/tasks"},
		{name: "update task", handler: UpdateNotificationTask, method: http.MethodPut, path: "/api/notification/tasks/1", id: 1},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := invokeNotificationHandler(testCase.handler, common.RoleRootUser, testCase.method, testCase.path, oversizedBody, testCase.id)
			require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
			require.JSONEq(t, `{"success":false,"message":"通知请求体不能超过 64 KiB"}`, recorder.Body.String())
		})
	}
	var bots, tasks, targets, deliveries int64
	require.NoError(t, model.DB.Model(&model.NotificationBot{}).Count(&bots).Error)
	require.NoError(t, model.DB.Model(&model.NotificationTask{}).Count(&tasks).Error)
	require.NoError(t, model.DB.Model(&model.NotificationTarget{}).Count(&targets).Error)
	require.NoError(t, model.DB.Model(&model.NotificationDelivery{}).Count(&deliveries).Error)
	require.Zero(t, bots)
	require.Zero(t, tasks)
	require.Zero(t, targets)
	require.Zero(t, deliveries)
}

func TestNotificationControllerMasksStorageErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupNotificationControllerTestDB(t)
	require.NoError(t, model.DB.Migrator().DropTable(&model.NotificationBot{}))
	recorder := invokeNotificationHandler(ListNotificationBots, common.RoleRootUser, http.MethodGet, "/api/notification/bots", "", 0)
	require.Contains(t, recorder.Body.String(), `"message":"通知服务暂时不可用"`)
	require.NotContains(t, recorder.Body.String(), "no such table")
	require.NotContains(t, recorder.Body.String(), "notification_bots")
}

func TestReplaceNotificationTargetsCannotTakeTargetFromAnotherTask(t *testing.T) {
	setupNotificationControllerTestDB(t)
	bot := &model.NotificationBot{Name: "bot", Token: "secret", Enabled: true}
	require.NoError(t, model.CreateNotificationBot(bot))
	first := &model.NotificationTask{Name: "first", EventType: "event", BotId: bot.Id, Enabled: true}
	second := &model.NotificationTask{Name: "second", EventType: "event", BotId: bot.Id, Enabled: true}
	require.NoError(t, model.CreateNotificationTask(first))
	require.NoError(t, model.CreateNotificationTask(second))
	target := &model.NotificationTarget{TaskId: first.Id, ChatId: "first-chat", Enabled: true}
	require.NoError(t, model.CreateNotificationTarget(target))

	err := model.DB.Transaction(func(tx *gorm.DB) error {
		return replaceNotificationTargetsTx(tx, second.Id, []notificationTargetRequest{{Id: target.Id, ChatId: "stolen-chat"}})
	})
	require.ErrorContains(t, err, "通知接收目标不存在")
	require.NoError(t, model.DB.First(target, target.Id).Error)
	require.Equal(t, first.Id, target.TaskId)
	require.Equal(t, "first-chat", target.ChatId)
}

func TestValidateNotificationStringUsesCharacterLength(t *testing.T) {
	require.NoError(t, validateNotificationString(strings.Repeat("中", 128), "名称", 128))
	require.ErrorContains(t, validateNotificationString(strings.Repeat("中", 129), "名称", 128), "最多允许 128 个字符")
}
