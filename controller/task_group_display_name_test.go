package controller

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTaskGroupDisplayNameTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := model.DB
	dsn := "file:" + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		_ = sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&model.Group{}, &model.GroupAlias{}, &model.Option{}))
	return db
}

func TestTasksToDtoUsesCurrentGroupDisplayName(t *testing.T) {
	db := setupTaskGroupDisplayNameTestDB(t)
	group := &model.Group{Code: "fixed-code", Name: "当前显示名称", Status: model.GroupStatusActive}
	require.NoError(t, db.Create(group).Error)
	require.NoError(t, db.Create(&model.GroupAlias{Alias: "legacy-code", GroupId: group.Id}).Error)

	items := tasksToDto([]*model.Task{
		{TaskID: "task-current", Group: "fixed-code"},
		{TaskID: "task-legacy", Group: "legacy-code"},
		{TaskID: "task-unknown", Group: "unknown-code"},
	}, false)
	require.Len(t, items, 3)
	require.Equal(t, "fixed-code", items[0].Group)
	require.Equal(t, "当前显示名称", items[0].GroupName)
	require.Equal(t, "legacy-code", items[1].Group)
	require.Equal(t, "当前显示名称", items[1].GroupName)
	require.Equal(t, "unknown-code", items[2].GroupName)
}

func TestTasksToDtoUsesLegacyUserUsableGroupDisplayName(t *testing.T) {
	db := setupTaskGroupDisplayNameTestDB(t)
	group := &model.Group{Code: "2", Name: "codex-basic", Status: model.GroupStatusActive}
	require.NoError(t, db.Create(group).Error)
	require.NoError(t, db.Create(&model.Option{
		Key:   "UserUsableGroups",
		Value: `{"Codex-Plus.group_2":"codex-basic"}`,
	}).Error)

	items := tasksToDto([]*model.Task{{
		TaskID: "task-legacy-option",
		Group:  "Codex-Plus.group_2",
	}}, false)

	require.Len(t, items, 1)
	require.Equal(t, "Codex-Plus.group_2", items[0].Group)
	require.Equal(t, "codex-basic", items[0].GroupName)
}
