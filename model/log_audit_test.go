package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupOperationAuditLogTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	originalDB, originalLogDB := DB, LOG_DB
	originalMain, originalLog := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB, LOG_DB = db, db
	require.NoError(t, db.AutoMigrate(&User{}, &Log{}))
	require.NoError(t, db.Create(&User{Id: 1, Username: "audit-admin"}).Error)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		DB, LOG_DB = originalDB, originalLogDB
		common.SetDatabaseTypes(originalMain, originalLog)
	})
	return db
}

func TestRecordOperationAuditLogSuppressesChannelUpdates(t *testing.T) {
	db := setupOperationAuditLogTestDB(t)

	RecordOperationAuditLog(1, "updated channel", "127.0.0.1", "channel.update", map[string]interface{}{
		"id": 573,
	}, nil, nil)
	var count int64
	require.NoError(t, db.Model(&Log{}).Where("type = ?", LogTypeManage).Count(&count).Error)
	assert.Zero(t, count)

	RecordOperationAuditLog(1, "created channel", "127.0.0.1", "channel.create", map[string]interface{}{
		"name": "new-channel",
	}, nil, nil)
	require.NoError(t, db.Model(&Log{}).Where("type = ?", LogTypeManage).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}
