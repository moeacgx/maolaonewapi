package model

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMigrateAffiliateRecordSourceIndexReplacesLegacySQLiteIndex(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "affiliate-index.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&AffiliateRecord{}))
	require.NoError(t, db.Migrator().DropIndex(&AffiliateRecord{}, affiliateRecordSourceIndex))
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX idx_affiliate_record_source ON affiliate_records (source_type, source_id, level)").Error)

	require.NoError(t, migrateAffiliateRecordSourceIndex(db))
	require.NoError(t, migrateAffiliateRecordSourceIndex(db))

	type sqliteIndex struct {
		Name   string `gorm:"column:name"`
		Unique int    `gorm:"column:unique"`
	}
	var indexes []sqliteIndex
	require.NoError(t, db.Raw("PRAGMA index_list('affiliate_records')").Scan(&indexes).Error)
	var sourceIndex *sqliteIndex
	for i := range indexes {
		if indexes[i].Name == affiliateRecordSourceIndex {
			sourceIndex = &indexes[i]
			break
		}
	}
	require.NotNil(t, sourceIndex)
	assert.Equal(t, 1, sourceIndex.Unique)

	type sqliteIndexColumn struct {
		Seq  int    `gorm:"column:seqno"`
		Name string `gorm:"column:name"`
	}
	var indexColumns []sqliteIndexColumn
	require.NoError(t, db.Raw("PRAGMA index_info('idx_affiliate_record_source')").Scan(&indexColumns).Error)
	columns := make([]string, len(indexColumns))
	for i, column := range indexColumns {
		columns[i] = column.Name
	}
	assert.Equal(t, affiliateRecordSourceIndexColumns, columns)

	records := []AffiliateRecord{
		{UserId: 9001, InviteeId: 9101, Level: 1, SourceType: AffiliateSourceTopUp, SourceId: "same-source"},
		{UserId: 9002, InviteeId: 9102, Level: 1, SourceType: AffiliateSourceTopUp, SourceId: "same-source"},
	}
	require.NoError(t, db.Create(&records).Error)
	var count int64
	require.NoError(t, db.Model(&AffiliateRecord{}).Where("source_type = ? AND source_id = ? AND level = ?", AffiliateSourceTopUp, "same-source", 1).Count(&count).Error)
	assert.EqualValues(t, 2, count)
}
