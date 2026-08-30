package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestResolveRemixOriginTaskInstallsCompleteAttemptZeroContext(t *testing.T) {
	oldDB := model.DB
	oldMemoryCache := common.MemoryCacheEnabled
	oldRedisEnabled := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Group{}, &model.GroupAlias{}, &model.Channel{}, &model.ChannelGroupBinding{},
		&model.Ability{}, &model.Task{},
	))
	model.DB = db
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = oldDB
		common.MemoryCacheEnabled = oldMemoryCache
		common.RedisEnabled = oldRedisEnabled
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	group := &model.Group{Code: "default", Name: "Default", Ratio: 1, UserSelectable: true, Status: model.GroupStatusActive}
	require.NoError(t, db.Create(group).Error)
	baseURL := "https://origin.example.test"
	organization := "origin-org"
	concurrencyLimit := 1
	settingJSON := `{"proxy":"http://127.0.0.1:8080","force_format":true}`
	paramJSON := `{"temperature":0.25}`
	headerJSON := `{"X-Origin":"locked"}`
	channel := &model.Channel{
		Name: "locked-origin", Key: "origin-key", Models: "sora-remix", Group: group.Code,
		Status: common.ChannelStatusEnabled, BaseURL: &baseURL, OpenAIOrganization: &organization,
		ConcurrencyLimit: &concurrencyLimit,
		Setting:          &settingJSON, ParamOverride: &paramJSON, HeaderOverride: &headerJSON,
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.ChannelGroupBinding{ChannelId: channel.Id, GroupId: group.Id, Position: 0}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: group.Code, GroupId: group.Id, Model: "sora-remix", ChannelId: channel.Id, Enabled: true}).Error)
	require.NoError(t, db.Create(&model.Task{
		TaskID: "task_attempt_zero", UserId: 77, Group: group.Code, ChannelId: channel.Id,
		Properties: model.Properties{OriginModelName: "sora-remix"},
	}).Error)

	var observed bool
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/v1/videos/:video_id/remix",
		func(c *gin.Context) {
			c.Set("id", 77)
			common.SetContextKey(c, constant.ContextKeyUserGroup, group.Code)
			common.SetContextKey(c, constant.ContextKeyUserGroupId, group.Id)
			common.SetContextKey(c, constant.ContextKeyTokenGroupMode, model.TokenGroupModeInherit)
			common.SetContextKey(c, constant.ContextKeyUsingGroup, group.Code)
			c.Next()
		},
		ResolveRemixOriginTask(),
		func(c *gin.Context) {
			observed = true
			require.Equal(t, channel.Id, common.GetContextKeyInt(c, constant.ContextKeyChannelId))
			require.Equal(t, "origin-key", common.GetContextKeyString(c, constant.ContextKeyChannelKey))
			require.Equal(t, baseURL, common.GetContextKeyString(c, constant.ContextKeyChannelBaseUrl))
			require.Equal(t, organization, common.GetContextKeyString(c, constant.ContextKeyChannelOrganization))
			require.Equal(t, group.Code, common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
			require.Equal(t, group.Code, common.GetContextKeyString(c, constant.ContextKeySelectedChannelGroup))
			selected, ok := common.GetContextKeyType[*model.Channel](c, constant.ContextKeySelectedChannel)
			require.True(t, ok)
			require.Equal(t, channel.Id, selected.Id)
			require.Equal(t, "locked", common.GetContextKeyStringMap(c, constant.ContextKeyChannelHeaderOverride)["X-Origin"])
			require.EqualValues(t, 0.25, common.GetContextKeyStringMap(c, constant.ContextKeyChannelParamOverride)["temperature"])
			settings, ok := common.GetContextKeyType[relaydto.ChannelSettings](c, constant.ContextKeyChannelSetting)
			require.True(t, ok)
			require.Equal(t, "http://127.0.0.1:8080", settings.Proxy)
			require.True(t, settings.ForceFormat)
			auditRequest := service.PromptAuditRequest{}
			service.PopulatePromptAuditRequestRoutingMetadata(c, &auditRequest)
			require.Equal(t, channel.Id, auditRequest.ChannelId)
			require.Equal(t, group.Code, auditRequest.GroupCode)
			c.Status(http.StatusNoContent)
		},
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/videos/task_attempt_zero/remix", bytes.NewBufferString(`{"prompt":"remix this"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.True(t, observed)
	require.True(t, model.IsChannelConcurrencyAvailable(channel), "remix request must release its channel slot")
}
