package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type legacyLogForIdempotencyMigration struct {
	Id        int   `gorm:"column:id;primaryKey"`
	CreatedAt int64 `gorm:"column:created_at"`
	Type      int   `gorm:"column:type"`
}

func (legacyLogForIdempotencyMigration) TableName() string {
	return "logs"
}

func TestEnsureSQLiteLogIdempotencyKeyAllowsExistingLogsMigration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	require.NoError(t, db.AutoMigrate(&legacyLogForIdempotencyMigration{}))
	require.NoError(t, db.Create(&legacyLogForIdempotencyMigration{Id: 1, CreatedAt: 100, Type: LogTypeConsume}).Error)

	require.NoError(t, ensureSQLiteLogIdempotencyKey(db))
	require.True(t, db.Migrator().HasColumn(&Log{}, "idempotency_key"))
	require.NoError(t, db.AutoMigrate(&Log{}))
	require.True(t, db.Migrator().HasIndex(&Log{}, "uidx_logs_idempotency_key"))

	var count int64
	require.NoError(t, db.Model(&Log{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
