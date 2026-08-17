package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupOriginTaskRouteTest(t *testing.T) (*gin.Context, *relaycommon.RelayInfo, *model.Group, *model.Channel, *model.Task) {
	t.Helper()
	oldDB := model.DB
	oldMemoryCache := common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Group{}, &model.GroupAlias{}, &model.Channel{}, &model.ChannelGroupBinding{},
		&model.Ability{}, &model.Task{},
	))
	model.DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = oldDB
		common.MemoryCacheEnabled = oldMemoryCache
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	group := &model.Group{Code: "default", Name: "Default", Ratio: 1, UserSelectable: true, Status: model.GroupStatusActive}
	require.NoError(t, db.Create(group).Error)
	channel := &model.Channel{
		Name: "origin-route", Key: "origin-key", Models: "sora-remix", Group: group.Code,
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.ChannelGroupBinding{ChannelId: channel.Id, GroupId: group.Id, Position: 0}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: group.Code, GroupId: group.Id, Model: "sora-remix", ChannelId: channel.Id, Enabled: true}).Error)
	task := &model.Task{
		TaskID: "task_origin", UserId: 41, Group: group.Code, ChannelId: channel.Id,
		Properties: model.Properties{OriginModelName: "sora-remix"},
	}
	require.NoError(t, db.Create(task).Error)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/task_origin/remix", nil)
	c.Params = gin.Params{{Key: "video_id", Value: task.TaskID}}
	common.SetContextKey(c, constant.ContextKeyUserGroup, group.Code)
	common.SetContextKey(c, constant.ContextKeyUserGroupId, group.Id)
	common.SetContextKey(c, constant.ContextKeyTokenGroupMode, model.TokenGroupModeInherit)
	info := &relaycommon.RelayInfo{UserId: task.UserId, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	return c, info, group, channel, task
}

func TestResolveOriginTaskInstallsAuthorizedPersistedGroupAndFullLockedChannel(t *testing.T) {
	c, info, group, channel, _ := setupOriginTaskRouteTest(t)
	require.Nil(t, ResolveOriginTask(c, info))
	require.Equal(t, constant.TaskActionRemix, info.Action)
	require.Equal(t, group.Code, info.UsingGroup)
	require.Equal(t, group.Code, common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
	require.Equal(t, group.Code, common.GetContextKeyString(c, constant.ContextKeySelectedChannelGroup))
	locked, ok := info.LockedChannel.(*model.Channel)
	require.True(t, ok)
	require.Equal(t, channel.Id, locked.Id)
	require.NotEmpty(t, locked.GroupDetails)
}

func TestResolveOriginTaskAcceptsCurrentExplicitTokenGroup(t *testing.T) {
	c, info, group, _, _ := setupOriginTaskRouteTest(t)
	common.SetContextKey(c, constant.ContextKeyTokenGroupMode, model.TokenGroupModeExplicit)
	common.SetContextKey(c, constant.ContextKeyTokenGroup, group.Code)
	common.SetContextKey(c, constant.ContextKeyTokenGroups, []string{group.Code})
	common.SetContextKey(c, constant.ContextKeyTokenGroupIds, []int{group.Id})
	require.Nil(t, ResolveOriginTask(c, info))
	require.Equal(t, group.Code, info.UsingGroup)
}

func TestResolveOriginTaskAcceptsCurrentAutoTokenGroup(t *testing.T) {
	c, info, group, _, _ := setupOriginTaskRouteTest(t)
	common.SetContextKey(c, constant.ContextKeyTokenGroupMode, model.TokenGroupModeAuto)
	common.SetContextKey(c, constant.ContextKeyTokenGroup, model.TokenGroupModeAuto)
	common.SetContextKey(c, constant.ContextKeyTokenAutoGroups, []string{group.Code})
	require.Nil(t, ResolveOriginTask(c, info))
	require.Equal(t, group.Code, info.UsingGroup)
}

func TestResolveOriginTaskRejectsOtherOwner(t *testing.T) {
	c, info, _, _, _ := setupOriginTaskRouteTest(t)
	info.UserId++
	taskErr := ResolveOriginTask(c, info)
	require.NotNil(t, taskErr)
	require.Equal(t, "task_not_exist", taskErr.Code)
}

func TestResolveOriginTaskRejectsRevokedPersistedGroup(t *testing.T) {
	c, info, group, _, _ := setupOriginTaskRouteTest(t)
	require.NoError(t, model.DB.Model(group).Update("status", model.GroupStatusDisabled).Error)
	taskErr := ResolveOriginTask(c, info)
	require.NotNil(t, taskErr)
	require.Equal(t, "task_group_forbidden", taskErr.Code)
}

func TestResolveOriginTaskRejectsGroupNoLongerAuthorizedByToken(t *testing.T) {
	c, info, _, _, _ := setupOriginTaskRouteTest(t)
	other := &model.Group{Code: "other", Name: "Other", Ratio: 1, UserSelectable: true, Status: model.GroupStatusActive}
	require.NoError(t, model.DB.Create(other).Error)
	common.SetContextKey(c, constant.ContextKeyUserGroup, other.Code)
	common.SetContextKey(c, constant.ContextKeyUserGroupId, other.Id)
	taskErr := ResolveOriginTask(c, info)
	require.NotNil(t, taskErr)
	require.Equal(t, "task_group_forbidden", taskErr.Code)
}

func TestResolveOriginTaskRejectsChannelWithoutCurrentGroupModelAbility(t *testing.T) {
	c, info, group, channel, _ := setupOriginTaskRouteTest(t)
	require.NoError(t, model.DB.Model(&model.Ability{}).
		Where("group_id = ? AND model = ? AND channel_id = ?", group.Id, "sora-remix", channel.Id).
		Update("enabled", false).Error)
	taskErr := ResolveOriginTask(c, info)
	require.NotNil(t, taskErr)
	require.Equal(t, "task_channel_ineligible", taskErr.Code)
}
