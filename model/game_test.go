package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/utils/tests"
)

func TestLockGameRowsUsesPortableLockingPolicy(t *testing.T) {
	db, err := gorm.Open(tests.DummyDialector{}, &gorm.Config{DryRun: true})
	require.NoError(t, err)
	buildSQL := func() string {
		var wallet GameWallet
		return LockGameRows(db).Where("user_id = ?", 7).First(&wallet).Statement.SQL.String()
	}

	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	t.Cleanup(func() { common.SetDatabaseTypes(oldMain, oldLog) })

	common.SetDatabaseTypes(common.DatabaseTypeMySQL, oldLog)
	assert.Contains(t, buildSQL(), "FOR UPDATE")
	common.SetDatabaseTypes(common.DatabaseTypePostgreSQL, oldLog)
	assert.Contains(t, buildSQL(), "FOR UPDATE")
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, oldLog)
	assert.NotContains(t, buildSQL(), "FOR UPDATE")
}
