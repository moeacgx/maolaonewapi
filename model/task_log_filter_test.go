package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTaskLogFilterTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := DB
	dsn := "file:" + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	DB = db
	t.Cleanup(func() {
		DB = oldDB
		_ = sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&User{}, &Task{}))
	return db
}

func TestTaskLogFiltersUsernameAndModelName(t *testing.T) {
	db := setupTaskLogFilterTestDB(t)
	require.NoError(t, db.Create(&[]User{
		{Id: 1, Username: "alice", AffCode: "alice-aff"},
		{Id: 2, Username: "bob", AffCode: "bob-aff"},
	}).Error)
	require.NoError(t, db.Create(&[]Task{
		{
			TaskID:     "visible-match",
			UserId:     1,
			ChannelId:  7,
			Platform:   constant.TaskPlatformImage,
			SubmitTime: 100,
			Properties: Properties{OriginModelName: "gpt-visible", UpstreamModelName: "hidden-upstream"},
		},
		{
			TaskID:     "other-user",
			UserId:     2,
			ChannelId:  7,
			Platform:   constant.TaskPlatformImage,
			SubmitTime: 101,
			Properties: Properties{OriginModelName: "gpt-visible"},
		},
		{
			TaskID:     "fallback-upstream",
			UserId:     1,
			ChannelId:  8,
			Platform:   constant.TaskPlatformImage,
			SubmitTime: 102,
			Properties: Properties{UpstreamModelName: "fallback-model"},
		},
	}).Error)

	adminQuery := SyncTaskQueryParams{Username: "alice", ModelName: "gpt-visible", ChannelID: "7"}
	adminItems := TaskGetAllTasksForLog(0, 10, adminQuery)
	require.Len(t, adminItems, 1)
	assert.Equal(t, "visible-match", adminItems[0].TaskID)
	assert.EqualValues(t, 1, TaskCountAllTasks(adminQuery))

	adminActualModelItems := TaskGetAllTasksForLog(0, 10, SyncTaskQueryParams{ModelName: "hidden-upstream"})
	require.Len(t, adminActualModelItems, 1)
	assert.Equal(t, "visible-match", adminActualModelItems[0].TaskID)

	selfHiddenItems := TaskGetAllUserTaskForLog(1, 0, 10, SyncTaskQueryParams{ModelName: "hidden-upstream"})
	assert.Empty(t, selfHiddenItems)
	assert.EqualValues(t, 0, TaskCountAllUserTask(1, SyncTaskQueryParams{ModelName: "hidden-upstream"}))

	selfFallbackItems := TaskGetAllUserTaskForLog(1, 0, 10, SyncTaskQueryParams{ModelName: "fallback-model"})
	require.Len(t, selfFallbackItems, 1)
	assert.Equal(t, "fallback-upstream", selfFallbackItems[0].TaskID)
	assert.EqualValues(t, 1, TaskCountAllUserTask(1, SyncTaskQueryParams{ModelName: "fallback-model"}))
}
