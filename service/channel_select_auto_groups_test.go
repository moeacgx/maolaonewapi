package service

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelSelectAutoGroupsTest(t *testing.T) *gorm.DB {
	t.Helper()

	originalDB := model.DB
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalRetryTimes := common.RetryTimes
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	originalMaxTokenAutoGroups := setting.GetMaxTokenAutoGroups()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	model.DB = db
	common.MemoryCacheEnabled = true
	common.RetryTimes = 0

	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`[]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2}`))
	require.NoError(t, setting.UpdateMaxTokenAutoGroups("2"))

	t.Cleanup(func() {
		model.DB = originalDB
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		common.RetryTimes = originalRetryTimes
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
		require.NoError(t, setting.UpdateMaxTokenAutoGroups(fmt.Sprintf("%d", originalMaxTokenAutoGroups)))

		if originalMemoryCacheEnabled && originalDB != nil &&
			originalDB.Migrator().HasTable(&model.Channel{}) && originalDB.Migrator().HasTable(&model.Ability{}) {
			model.InitChannelCache()
		}
		sqlDB, err := db.DB()
		if err == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	return db
}

func createChannelSelectAutoGroupsChannel(t *testing.T, db *gorm.DB, id int, group, modelName string, priority int64) {
	t.Helper()
	weight := uint(100)
	require.NoError(t, db.Create(&model.Channel{
		Id:       id,
		Type:     constant.ChannelTypeOpenAI,
		Key:      fmt.Sprintf("key-%d", id),
		Status:   common.ChannelStatusEnabled,
		Name:     fmt.Sprintf("channel-%d", id),
		Weight:   &weight,
		Models:   modelName,
		Group:    group,
		Priority: &priority,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     group,
		Model:     modelName,
		ChannelId: id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
}

func TestCacheGetRandomSatisfiedChannelUsesTokenAutoGroupsWhenGlobalAutoIsEmpty(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-groups-runtime-model"
	createChannelSelectAutoGroupsChannel(t, db, 2101, "vip", modelName, 0)
	createChannelSelectAutoGroupsChannel(t, db, 2102, "default", modelName, 0)
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"vip", "default"})
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)

	retry := 0
	param := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "auto",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       &retry,
	}

	first, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 2101, first.Id)
	assert.Equal(t, "vip", selectedGroup)
	assert.Equal(t, "vip", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
	assert.Empty(t, setting.GetAutoGroups(), "the selection must not depend on the global Auto list")

	param.ExcludeChannelID(first.Id, true)
	param.IncreaseRetry()
	second, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, 2102, second.Id)
	assert.Equal(t, "default", selectedGroup)
	assert.Equal(t, "default", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
}

func TestCacheGetRandomSatisfiedChannelExhaustsPrioritiesBeforeOrderedGroups(t *testing.T) {
	for _, tc := range []struct {
		name       string
		tokenGroup string
		configure  func(*gin.Context)
	}{
		{
			name:       "auto snapshot",
			tokenGroup: "auto",
			configure: func(ctx *gin.Context) {
				common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
				common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"vip", "default"})
				common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)
			},
		},
		{
			name:       "explicit authorized groups",
			tokenGroup: "vip,default",
			configure: func(ctx *gin.Context) {
				common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "vip,default")
				common.SetContextKey(ctx, constant.ContextKeyTokenGroups, []string{"vip", "default"})
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupChannelSelectAutoGroupsTest(t)
			common.RetryTimes = 3
			modelName := "ordered-priority-" + strings.ReplaceAll(tc.name, " ", "-")
			createChannelSelectAutoGroupsChannel(t, db, 2201, "vip", modelName, 100)
			createChannelSelectAutoGroupsChannel(t, db, 2202, "vip", modelName, 50)
			createChannelSelectAutoGroupsChannel(t, db, 2203, "default", modelName, 100)
			createChannelSelectAutoGroupsChannel(t, db, 2204, "default", modelName, 50)
			model.InitChannelCache()

			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			tc.configure(ctx)
			param := &RetryParam{
				Ctx: ctx, TokenGroup: tc.tokenGroup, ModelName: modelName,
				RequestPath: "/v1/chat/completions", Retry: common.GetPointer(0),
			}

			maxRetries := RelayMaxRetries(param)
			require.Equal(t, 3, maxRetries)
			channelIDs := make([]int, 0, maxRetries+1)
			groups := make([]string, 0, maxRetries+1)
			for attempt := 0; attempt <= maxRetries; attempt++ {
				channel, group, err := CacheGetRandomSatisfiedChannel(param)
				require.NoError(t, err)
				require.NotNil(t, channel, "attempt %d", attempt)
				channelIDs = append(channelIDs, channel.Id)
				groups = append(groups, group)
				if attempt < maxRetries {
					param.ExcludeChannelID(channel.Id, true)
					param.IncreaseRetry()
				}
			}

			assert.Equal(t, []int{2201, 2202, 2203, 2204}, channelIDs)
			assert.Equal(t, []string{"vip", "vip", "default", "default"}, groups)
			assert.Equal(t, 1, retryGroupIndex(ctx))
			assert.Equal(t, 2, retryGroupStartIndex(ctx))
		})
	}
}

func TestRelayMaxRetriesKeepsSingleAutoGroupBudget(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)
	common.RetryTimes = 5
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"default"})
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)

	param := &RetryParam{Ctx: ctx, TokenGroup: "auto", Retry: common.GetPointer(0)}
	require.Equal(t, 5, RelayMaxRetries(param))
}

func TestCacheGetRandomSatisfiedChannelContinuesAffinitySelectedGroupPriorities(t *testing.T) {
	for _, tc := range []struct {
		name       string
		tokenGroup string
		configure  func(*gin.Context)
	}{
		{
			name:       "auto snapshot",
			tokenGroup: "auto",
			configure: func(ctx *gin.Context) {
				common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
				common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"vip", "default"})
				common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)
			},
		},
		{
			name:       "explicit authorized groups",
			tokenGroup: "vip,default",
			configure: func(ctx *gin.Context) {
				common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "vip,default")
				common.SetContextKey(ctx, constant.ContextKeyTokenGroups, []string{"vip", "default"})
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupChannelSelectAutoGroupsTest(t)
			common.RetryTimes = 3
			modelName := "affinity-ordered-priority-" + strings.ReplaceAll(tc.name, " ", "-")
			createChannelSelectAutoGroupsChannel(t, db, 2301, "vip", modelName, 100)
			createChannelSelectAutoGroupsChannel(t, db, 2302, "vip", modelName, 50)
			createChannelSelectAutoGroupsChannel(t, db, 2303, "default", modelName, 100)
			createChannelSelectAutoGroupsChannel(t, db, 2304, "default", modelName, 50)
			model.InitChannelCache()

			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			tc.configure(ctx)
			// Distributor selected the lower-priority affinity channel from the
			// first group at attempt zero. Because its actual tier is unknown, the
			// first relay retry starts at local tier zero in that same group.
			common.SetContextKey(ctx, constant.ContextKeyAutoGroupIndex, 0)
			common.SetContextKey(ctx, constant.ContextKeyAutoGroupRetryIndex, 1)
			param := &RetryParam{
				Ctx: ctx, TokenGroup: tc.tokenGroup, ModelName: modelName,
				RequestPath: "/v1/chat/completions", Retry: common.GetPointer(1),
			}
			param.ExcludeChannelID(2302, true)

			channelIDs := make([]int, 0, 3)
			groups := make([]string, 0, 3)
			for attempt := range 3 {
				channel, group, err := CacheGetRandomSatisfiedChannel(param)
				require.NoError(t, err)
				require.NotNil(t, channel, "attempt %d", attempt+1)
				channelIDs = append(channelIDs, channel.Id)
				groups = append(groups, group)
				if attempt < 2 {
					param.ExcludeChannelID(channel.Id, true)
					param.IncreaseRetry()
				}
			}

			assert.Equal(t, []int{2301, 2303, 2304}, channelIDs)
			assert.Equal(t, []string{"vip", "default", "default"}, groups)
			assert.Equal(t, 1, retryGroupIndex(ctx))
			assert.Equal(t, 2, retryGroupStartIndex(ctx))
		})
	}
}
