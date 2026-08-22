package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMigrateWalletQuotaColumnsSQLitePreservesLargeValue(t *testing.T) {
	previousDB := DB
	previousDatabaseType := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousDatabaseType)
	})

	// 模拟升级前的整数列；SQLite 的 INTEGER 已支持有符号 64 位值。
	require.NoError(t, db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY,
		quota INTEGER NOT NULL DEFAULT 0,
		used_quota INTEGER NOT NULL DEFAULT 0,
		aff_quota INTEGER NOT NULL DEFAULT 0,
		aff_history INTEGER NOT NULL DEFAULT 0
	)`).Error)
	require.NoError(t, migrateWalletQuotaColumns())

	const largeQuota int64 = 5_000_000_000
	require.NoError(t, db.Exec("INSERT INTO users (id, quota) VALUES (?, ?)", 1, largeQuota).Error)
	var storedQuota int64
	require.NoError(t, db.Table("users").Select("quota").Where("id = ?", 1).Scan(&storedQuota).Error)
	assert.Equal(t, largeQuota, storedQuota)

	// 迁移入口可重复执行，不应重写或截断已有余额。
	require.NoError(t, migrateWalletQuotaColumns())
	storedQuota = 0
	require.NoError(t, db.Table("users").Select("quota").Where("id = ?", 1).Scan(&storedQuota).Error)
	assert.Equal(t, largeQuota, storedQuota)
}
