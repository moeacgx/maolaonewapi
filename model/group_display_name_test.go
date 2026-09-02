package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetGroupDisplayNameForErrorUsesCurrentName(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})
	require.NoError(t, db.AutoMigrate(&Group{}, &GroupAlias{}, &Option{}))

	group := &Group{Code: "Codex-Pro", Name: "Codex 专业组", Status: GroupStatusActive}
	require.NoError(t, db.Create(group).Error)
	require.NoError(t, db.Create(&GroupAlias{Alias: "legacy-codex-pro", GroupId: group.Id}).Error)

	require.Equal(t, "Codex 专业组", GetGroupDisplayNameForError("Codex-Pro"))
	require.Equal(t, "Codex 专业组", GetGroupDisplayNameForError("legacy-codex-pro"))
	require.Equal(t, "Codex 专业组, Codex 专业组", GetGroupDisplayNameForError("Codex-Pro,legacy-codex-pro"))
}

func TestGetGroupDisplayNameForErrorFallsBackWithoutDatabase(t *testing.T) {
	previousDB := DB
	DB = nil
	t.Cleanup(func() {
		DB = previousDB
	})

	require.Equal(t, "unknown-code", GetGroupDisplayNameForError(" unknown-code "))
	require.Equal(t, "", GetGroupDisplayNameForError("  "))
}
