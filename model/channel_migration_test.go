package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelAutoMigrateAddsConcurrencyColumnWithoutChangingExistingValue(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:channel-concurrency-migration?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}))
	limit := 9
	require.NoError(t, db.Create(&Channel{Id: 1, Name: "legacy", Key: "key", ConcurrencyLimit: &limit}).Error)
	require.NoError(t, db.AutoMigrate(&Channel{}))
	var channel Channel
	require.NoError(t, db.First(&channel, 1).Error)
	require.NotNil(t, channel.ConcurrencyLimit)
	assert.Equal(t, 9, *channel.ConcurrencyLimit)
}

func TestChannelAutoMigrateAddsMissingConcurrencyColumnWithZeroDefault(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:channel-concurrency-migration-missing?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}))
	require.NoError(t, db.Create(&Channel{Id: 1, Name: "legacy", Key: "key"}).Error)
	require.NoError(t, db.Migrator().DropColumn(&Channel{}, "concurrency_limit"))
	require.NoError(t, db.AutoMigrate(&Channel{}))
	var channel Channel
	require.NoError(t, db.First(&channel, 1).Error)
	require.NotNil(t, channel.ConcurrencyLimit)
	assert.Equal(t, 0, *channel.ConcurrencyLimit)
}

func TestChannelAutoMigrateAddsVendorColumnWithoutChangingExistingValue(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:channel-vendor-migration?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}))
	vendorID := 42
	require.NoError(t, db.Create(&Channel{Id: 1, Name: "legacy", Key: "key", VendorID: &vendorID}).Error)
	require.NoError(t, db.AutoMigrate(&Channel{}))
	var channel Channel
	require.NoError(t, db.First(&channel, 1).Error)
	require.NotNil(t, channel.VendorID)
	assert.Equal(t, vendorID, *channel.VendorID)
}

func TestChannelAutoMigrateAddsMissingVendorColumnWithNullDefault(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:channel-vendor-migration-missing?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}))
	require.NoError(t, db.Create(&Channel{Id: 1, Name: "legacy", Key: "key"}).Error)
	require.NoError(t, db.Migrator().DropColumn(&Channel{}, "vendor_id"))
	require.NoError(t, db.AutoMigrate(&Channel{}))
	var channel Channel
	require.NoError(t, db.First(&channel, 1).Error)
	assert.Nil(t, channel.VendorID)
}
